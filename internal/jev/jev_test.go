package jev

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/licgate/licgate/internal/model"
)

func fakeServer(t *testing.T, choice string, prob, orLater float64, status *int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Method != "POST" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth header %q", r.Header.Get("Authorization"))
		}
		var req struct {
			Model     string                     `json:"model"`
			State     map[string]string          `json:"state"`
			Questions map[string]json.RawMessage `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Model != "jev-latest" || req.State["license_text"] == "" || len(req.Questions) != 2 {
			t.Errorf("bad request: %+v", req)
		}
		var q struct {
			Type     string            `json:"type"`
			Criteria map[string]string `json:"criteria"`
		}
		json.Unmarshal(req.Questions["license"], &q)
		if q.Type != "choice" || q.Criteria["MIT"] == "" {
			t.Errorf("bad choice question: %+v", q)
		}
		if status != nil && *status != 200 {
			code := *status
			*status = 200 // succeed on retry
			w.WriteHeader(code)
			w.Write([]byte(`{"detail":"busy"}`))
			return
		}
		probs := map[string]float64{"not_a_license": 1 - prob}
		probs[choice] = prob
		json.NewEncoder(w).Encode(map[string]any{
			"model": "jev-1.13.0",
			"answers": map[string]any{
				"license":  map[string]any{"type": "choice", "choice": choice, "confidence": prob - 0.05, "probabilities": probs},
				"or_later": map[string]any{"type": "noul", "noul": orLater},
			},
			"usage": map[string]int{"input_tokens": 123, "output_tokens": 4},
		})
	}))
}

func client(url string) *Client {
	return &Client{BaseURL: url, APIKey: "test-key", Model: "jev-latest", HTTP: http.DefaultClient}
}

func TestClassifyGPLOrLater(t *testing.T) {
	srv := fakeServer(t, "GPL-2.0", 0.97, 0.93, nil)
	defer srv.Close()
	c := client(srv.URL)
	got, err := c.Classify(context.Background(), "some license text")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "GPL-2.0-or-later" || got.Probability != 0.97 {
		t.Errorf("got %+v", got)
	}
	if c.InputTokens != 123 || c.Calls != 1 {
		t.Errorf("usage not tracked: %d %d", c.InputTokens, c.Calls)
	}
}

func TestRetryOn529(t *testing.T) {
	st := 529
	srv := fakeServer(t, "MIT", 0.99, 0, &st)
	defer srv.Close()
	got, err := client(srv.URL).Classify(context.Background(), "text")
	if err != nil || got.ID != "MIT" {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestApplyThreshold(t *testing.T) {
	mk := func() *model.Graph {
		g := model.NewGraph()
		c := &model.Component{ID: "x", Name: "x", License: "", LicenseFiles: []model.LicenseFile{{Path: "LICENSE", Text: "Custom permissive terms: do anything."}}}
		g.Components["x"] = c
		k := &model.Component{ID: "k", Name: "k", License: "MIT"}
		g.Components["k"] = k
		return g
	}
	srv := fakeServer(t, "other_permissive", 0.9, 0, nil)
	defer srv.Close()
	g := mk()
	n, err := Apply(context.Background(), g, client(srv.URL), 0.85, func(string, ...any) {})
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v (known licenses must not be sent)", n, err)
	}
	if g.Components["x"].License != "LicenseRef-licgate-other-permissive" || g.Components["x"].LicenseSource != "jev" {
		t.Errorf("got %+v", g.Components["x"])
	}
	srv2 := fakeServer(t, "MIT", 0.6, 0, nil)
	defer srv2.Close()
	g2 := mk()
	Apply(context.Background(), g2, client(srv2.URL), 0.85, func(string, ...any) {})
	if g2.Components["x"].License != "" || !strings.Contains(strings.Join(g2.Components["x"].LicenseNotes, " "), "0.60") {
		t.Errorf("low confidence must not set a license: %+v", g2.Components["x"])
	}
}

func TestSubstituteUnknown(t *testing.T) {
	if got := substituteUnknown("MIT AND LicenseRef-foo", "BSD-3-Clause"); got != "MIT AND BSD-3-Clause" {
		t.Errorf("got %q", got)
	}
}
