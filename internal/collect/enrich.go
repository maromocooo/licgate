package collect

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/licgate/licgate/internal/model"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

const userAgent = "licgate (https://github.com/licgate/licgate)"

func getJSON(u string, v any) error {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 20<<20)).Decode(v)
}

// enrichOnline looks up licenses for components that have none, using the
// package registries (npm, PyPI, crates.io) and deps.dev for the rest.
// Only package names and versions are sent.
func enrichOnline(g *model.Graph, logf func(string, ...any)) {
	var todo []*model.Component
	for _, c := range g.Components {
		if c.License == "" && c.Version != "" && c.Ecosystem != "generic" {
			todo = append(todo, c)
		}
	}
	if len(todo) == 0 {
		return
	}
	logf("online lookup for %d packages", len(todo))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failures int
	for _, c := range todo {
		wg.Add(1)
		sem <- struct{}{}
		go func(c *model.Component) {
			defer wg.Done()
			defer func() { <-sem }()
			raws, src, err := lookupLicense(c)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures++
				return
			}
			declare(c, src, raws...)
		}(c)
	}
	wg.Wait()
	if failures > 0 {
		g.Warnings = append(g.Warnings, fmt.Sprintf("online lookup failed for %d package(s)", failures))
	}
}

func lookupLicense(c *model.Component) ([]string, string, error) {
	switch c.Ecosystem {
	case "npm":
		var v struct {
			License  json.RawMessage `json:"license"`
			Licenses json.RawMessage `json:"licenses"`
		}
		name := strings.Replace(url.PathEscape(c.Name), "%40", "@", 1)
		if err := getJSON("https://registry.npmjs.org/"+name+"/"+url.PathEscape(c.Version), &v); err != nil {
			return nil, "", err
		}
		return npmLicenses(v.License, v.Licenses), "registry:npm", nil
	case "pypi":
		var v struct {
			Info struct {
				License           string   `json:"license"`
				LicenseExpression string   `json:"license_expression"`
				Classifiers       []string `json:"classifiers"`
			} `json:"info"`
		}
		if err := getJSON("https://pypi.org/pypi/"+url.PathEscape(c.Name)+"/"+url.PathEscape(c.Version)+"/json", &v); err != nil {
			return nil, "", err
		}
		if v.Info.LicenseExpression != "" {
			return []string{v.Info.LicenseExpression}, "registry:pypi", nil
		}
		if e := fromClassifiers(v.Info.Classifiers); e != "" {
			return []string{e}, "registry:pypi", nil
		}
		return []string{v.Info.License}, "registry:pypi", nil
	case "cargo":
		var v struct {
			Version struct {
				License string `json:"license"`
			} `json:"version"`
		}
		if err := getJSON("https://crates.io/api/v1/crates/"+url.PathEscape(c.Name)+"/"+url.PathEscape(c.Version), &v); err != nil {
			return nil, "", err
		}
		return []string{v.Version.License}, "registry:crates.io", nil
	}
	return lookupDepsDev(c)
}

var depsDevSystem = map[string]string{
	"golang": "go", "maven": "maven", "nuget": "nuget", "npm": "npm", "pypi": "pypi", "cargo": "cargo",
}

// lookupDepsDev uses the deps.dev v3 API (GetVersion), whose response has
// a "licenses" array of SPDX expressions.
func lookupDepsDev(c *model.Component) ([]string, string, error) {
	sys, ok := depsDevSystem[c.Ecosystem]
	if !ok {
		return nil, "", fmt.Errorf("no online source for %s", c.Ecosystem)
	}
	var v struct {
		Licenses []string `json:"licenses"`
	}
	u := "https://api.deps.dev/v3/systems/" + sys + "/packages/" + url.PathEscape(c.Name) + "/versions/" + url.PathEscape(c.Version)
	if err := getJSON(u, &v); err != nil {
		return nil, "", err
	}
	var out []string
	for _, l := range v.Licenses {
		if !strings.EqualFold(l, "non-standard") {
			out = append(out, l)
		}
	}
	if len(out) > 1 {
		// Several entries: treat as all applying (conservative).
		for i, l := range out {
			if strings.Contains(l, " ") {
				out[i] = "(" + l + ")"
			}
		}
		out = []string{strings.Join(out, " AND ")}
	}
	return out, "deps.dev", nil
}
