// Package jev is a minimal client for TypeSafe's System One API (model
// "Jev"), used to classify license texts that the built-in detector could
// not recognise. Jev returns calibrated probabilities instead of prose, so
// low-confidence answers are routed to manual review instead of guessed.
//
// Wire format: POST {base}/v1/systemone with
//
//	{"model": "...", "state": ..., "questions": {"id": {"type": "choice", "instructions": "...", "criteria": {...}}}}
//
// answered by {"model": "...", "answers": {"id": {"type": "choice", "choice": "...", "confidence": 0.9, "probabilities": {...}}}, "usage": {...}}.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maromocooo/licgate/internal/i18n"
	"github.com/maromocooo/licgate/internal/license"
	"github.com/maromocooo/licgate/internal/model"
)

// Client talks to the System One API directly or through OpenRouter.
type Client struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
	// Usage accumulates token counts across calls.
	mu          sync.Mutex
	InputTokens int
	Calls       int
}

// New builds a client for provider "typesafe" (default) or "openrouter",
// reading the API key from TYPESAFE_API_KEY / OPENROUTER_API_KEY.
func New(provider, modelName string) (*Client, error) {
	c := &Client{HTTP: &http.Client{Timeout: 60 * time.Second}, Model: modelName}
	switch strings.ToLower(provider) {
	case "", "typesafe":
		c.BaseURL = "https://api.typesafe.ai"
		c.APIKey = os.Getenv("TYPESAFE_API_KEY")
		if c.APIKey == "" {
			c.APIKey = os.Getenv("JEV_API_KEY")
		}
		if c.Model == "" {
			c.Model = "jev-latest"
		}
		if c.APIKey == "" {
			return nil, fmt.Errorf("TYPESAFE_API_KEY is not set")
		}
	case "openrouter":
		c.BaseURL = "https://openrouter.ai/api"
		c.APIKey = os.Getenv("OPENROUTER_API_KEY")
		if c.Model == "" || c.Model == "jev-latest" {
			c.Model = "typesafe/jev-1.13"
		}
		if c.APIKey == "" {
			return nil, fmt.Errorf("OPENROUTER_API_KEY is not set")
		}
	default:
		return nil, fmt.Errorf("unknown Jev provider %q (typesafe | openrouter)", provider)
	}
	if v := os.Getenv("LICGATE_JEV_BASE_URL"); v != "" {
		c.BaseURL = strings.TrimRight(v, "/")
	}
	return c, nil
}

// Question is a System One question.
type Question struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

// Answer is a System One answer (fields depend on Type).
type Answer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
}

type request struct {
	Model     string              `json:"model"`
	State     any                 `json:"state"`
	Questions map[string]Question `json:"questions"`
}

type response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   struct {
		InputTokens int `json:"input_tokens"`
	} `json:"usage"`
}

// Ask sends one request, retrying on 429/529/5xx.
func (c *Client) Ask(ctx context.Context, state any, qs map[string]Question) (map[string]Answer, error) {
	body, err := json.Marshal(request{Model: c.Model, State: state, Questions: qs})
	if err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/systemone", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			sleep(ctx, backoff(attempt, ""))
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if resp.StatusCode == 200 {
			var r response
			if err := json.Unmarshal(b, &r); err != nil {
				return nil, fmt.Errorf("jev: bad response: %w", err)
			}
			c.mu.Lock()
			c.InputTokens += r.Usage.InputTokens
			c.Calls++
			c.mu.Unlock()
			return r.Answers, nil
		}
		msg := strings.TrimSpace(string(b))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		lastErr = fmt.Errorf("jev: HTTP %d: %s", resp.StatusCode, msg)
		if resp.StatusCode == 429 || resp.StatusCode == 529 || resp.StatusCode >= 500 {
			sleep(ctx, backoff(attempt, resp.Header.Get("Retry-After")))
			continue
		}
		return nil, lastErr
	}
	return nil, lastErr
}

func backoff(attempt int, retryAfter string) time.Duration {
	if s, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && s > 0 && s < 60 {
		return time.Duration(s) * time.Second
	}
	return time.Duration(1<<attempt) * 500 * time.Millisecond
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// Option → SPDX id. GPL-family entries get -only / -or-later from the
// second question.
var options = []struct{ key, desc, id string }{
	{"MIT", "MIT License", "MIT"},
	{"Apache-2.0", "Apache License 2.0", "Apache-2.0"},
	{"BSD-2-Clause", "BSD 2-Clause (Simplified) License", "BSD-2-Clause"},
	{"BSD-3-Clause", "BSD 3-Clause (New/Revised) License, with a non-endorsement clause", "BSD-3-Clause"},
	{"ISC", "ISC License", "ISC"},
	{"0BSD", "Zero-Clause BSD / public-domain-like permissive license without attribution", "0BSD"},
	{"Unlicense", "The Unlicense or another explicit public domain dedication", "Unlicense"},
	{"CC0-1.0", "Creative Commons Zero", "CC0-1.0"},
	{"Zlib", "zlib/libpng license", "Zlib"},
	{"BSL-1.0", "Boost Software License 1.0", "BSL-1.0"},
	{"MPL-2.0", "Mozilla Public License 2.0", "MPL-2.0"},
	{"LGPL-2.1", "GNU Lesser General Public License 2.1", "LGPL-2.1"},
	{"LGPL-3.0", "GNU Lesser General Public License 3.0", "LGPL-3.0"},
	{"GPL-2.0", "GNU General Public License 2.0", "GPL-2.0"},
	{"GPL-3.0", "GNU General Public License 3.0", "GPL-3.0"},
	{"AGPL-3.0", "GNU Affero General Public License 3.0", "AGPL-3.0"},
	{"EPL-2.0", "Eclipse Public License 2.0", "EPL-2.0"},
	{"SSPL-1.0", "Server Side Public License", "SSPL-1.0"},
	{"BUSL-1.1", "Business Source License", "BUSL-1.1"},
	{"Elastic-2.0", "Elastic License 2.0", "Elastic-2.0"},
	{"other_permissive", "Another permissive license: allows use, modification and redistribution (incl. commercial) with at most attribution requirements", license.RefPermissive},
	{"other_weak_copyleft", "Another weak/file-level copyleft license: modifications to the licensed files must be shared, but it can be combined with other code", license.RefWeakCopyleft},
	{"other_strong_copyleft", "Another strong copyleft license: derivative works as a whole must be released under the same license", license.RefCopyleft},
	{"source_available", "Source-available but restricted: non-commercial only, no competing services, field-of-use or ethical restrictions", license.RefNonFree},
	{"proprietary", "Proprietary / all rights reserved / no permission to redistribute", license.RefProprietary},
	{"not_a_license", "The text is not a license (e.g. a README, changelog or empty template)", ""},
}

// Classification is Jev's answer for one license text.
type Classification struct {
	ID          string
	Option      string
	Probability float64
	OrLater     float64
}

// Classify asks Jev which license a text is.
func (c *Client) Classify(ctx context.Context, text string) (Classification, error) {
	if len(text) > 24000 {
		text = text[:24000]
	}
	crit := map[string]string{}
	for _, o := range options {
		crit[o.key] = o.desc
	}
	qs := map[string]Question{
		"license": {
			Type:         "choice",
			Instructions: "This is the license file of a third-party software package. Which license is it? Choose the specific license if it is one of the named ones (allowing for small wording differences or added copyright lines); otherwise choose the category that best describes its terms.",
			Criteria:     crit,
		},
		"or_later": {
			Type:         "noul",
			Instructions: "Does the text grant permission to use the work under a later version of the license (for example \"either version 2 of the License, or (at your option) any later version\")?",
			Criteria:     map[string]string{"true": "A later version may be chosen", "false": "No later-version clause, or only the license body itself"},
		},
	}
	ans, err := c.Ask(ctx, map[string]string{"license_text": text}, qs)
	if err != nil {
		return Classification{}, err
	}
	a, ok := ans["license"]
	if !ok || a.Choice == "" {
		return Classification{}, fmt.Errorf("jev: no answer")
	}
	cl := Classification{Option: a.Choice, Probability: a.Confidence}
	if p, ok := a.Probabilities[a.Choice]; ok {
		cl.Probability = p
	}
	if ol, ok := ans["or_later"]; ok && ol.Noul != nil {
		cl.OrLater = *ol.Noul
	}
	for _, o := range options {
		if o.key == a.Choice {
			cl.ID = o.id
		}
	}
	switch cl.ID {
	case "LGPL-2.1", "LGPL-3.0", "GPL-2.0", "GPL-3.0", "AGPL-3.0":
		if cl.OrLater >= 0.8 {
			cl.ID += "-or-later"
		} else {
			cl.ID += "-only"
		}
	}
	return cl, nil
}

func init() {
	i18n.Add(map[string][2]string{
		"jev.classified":  {"License text classified by Jev as %s (p=%.2f).", "Jev がライセンス本文を %s と判定しました（確率 %.2f）。"},
		"jev.unsure":      {"Jev's best guess is %s (p=%.2f), below the %.2f threshold — check manually.", "Jev の推定は %s（確率 %.2f）で、しきい値 %.2f 未満のため手動確認が必要です。"},
		"jev.not_license": {"Jev: the file found does not look like a license.", "Jev: 見つかったファイルはライセンスではないようです。"},
	})
}

// candidates returns the text to classify for c, or "" if Jev is not
// needed (license known and recognised).
func candidates(c *model.Component) string {
	needs := c.License == ""
	if !needs {
		if e, err := license.Parse(c.License); err == nil {
			for _, l := range license.Leaves(e) {
				if !license.IsKnown(l.ID) {
					needs = true
				}
			}
		}
	}
	if !needs {
		return ""
	}
	var parts []string
	for _, f := range c.LicenseFiles {
		if f.IsNotice || (f.DetectedID != "" && f.Confidence >= 0.8) || strings.TrimSpace(f.Text) == "" {
			continue
		}
		parts = append(parts, f.Text)
		if len(parts) == 2 {
			break
		}
	}
	return strings.Join(parts, "\n\n----\n\n")
}

// Apply classifies unrecognised license texts in g. It returns how many
// components it looked at.
func Apply(ctx context.Context, g *model.Graph, cl *Client, threshold float64, logf func(string, ...any)) (int, error) {
	if threshold <= 0 {
		threshold = 0.85
	}
	type job struct {
		c    *model.Component
		text string
	}
	var jobs []job
	for _, c := range g.Components {
		if t := candidates(c); t != "" {
			jobs = append(jobs, job{c, t})
		}
	}
	if len(jobs) == 0 {
		return 0, nil
	}
	logf("jev: classifying %d license text(s)", len(jobs))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			res, err := cl.Classify(ctx, j.text)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			switch {
			case res.Option == "not_a_license":
				j.c.LicenseNotes = append(j.c.LicenseNotes, i18n.T("jev.not_license"))
			case res.Probability >= threshold && res.ID != "":
				j.c.License = substituteUnknown(j.c.License, res.ID)
				j.c.LicenseSource = "jev"
				j.c.LicenseNotes = append(j.c.LicenseNotes, i18n.T("jev.classified", res.ID, res.Probability))
			default:
				j.c.LicenseNotes = append(j.c.LicenseNotes, i18n.T("jev.unsure", res.Option, res.Probability, threshold))
			}
		}(j)
	}
	wg.Wait()
	return len(jobs), firstErr
}

// substituteUnknown puts id in place of the unrecognised leaves of expr (or
// returns id when expr is empty).
func substituteUnknown(expr, id string) string {
	if expr == "" {
		return id
	}
	e, err := license.Parse(expr)
	if err != nil {
		return id
	}
	out := expr
	for _, l := range license.Leaves(e) {
		if !license.IsKnown(l.ID) {
			out = strings.ReplaceAll(out, l.ID, id)
		}
	}
	if n, err := license.Parse(out); err == nil {
		return n.String()
	}
	return id
}
