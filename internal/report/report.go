// Package report renders a policy.Result as terminal text, Markdown, HTML,
// JSON, a third-party notices file, or an enriched CycloneDX SBOM.
package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/licgate/licgate/internal/i18n"
	"github.com/licgate/licgate/internal/license"
	"github.com/licgate/licgate/internal/model"
	"github.com/licgate/licgate/internal/policy"
)

// Meta is information about the run.
type Meta struct {
	Tool        string    `json:"tool"`
	Version     string    `json:"version"`
	Project     string    `json:"project"`
	GeneratedAt time.Time `json:"generated_at"`
	FailOn      string    `json:"fail_on"`
	Failed      bool      `json:"failed"`
}

func init() {
	i18n.Add(map[string][2]string{
		"rep.title":              {"License check", "ライセンスチェック"},
		"rep.intended":           {"Intended license", "公開予定のライセンス"},
		"rep.distribution":       {"Distribution", "配布形態"},
		"rep.linking":            {"Linking", "リンク方式"},
		"rep.include_dev":        {"Dev dependencies", "開発用依存"},
		"rep.included":           {"evaluated", "評価する"},
		"rep.excluded":           {"not shipped (skipped)", "配布物に含まれない（スキップ）"},
		"rep.sources":            {"Data sources", "データソース"},
		"rep.generated":          {"Generated", "生成日時"},
		"rep.summary":            {"Summary", "サマリー"},
		"rep.verdict_block":      {"Known incompatibilities with %s were found. Resolve the BLOCK items before publishing.", "%s との既知の非互換が見つかりました。公開前に BLOCK の項目を解消してください。"},
		"rep.verdict_review":     {"No known incompatibility with %s, but some dependencies need a manual check.", "%s との既知の非互換はありませんが、手動確認が必要な依存があります。"},
		"rep.verdict_incomplete": {"Part of the project could not be read (see ERROR above), so this result is incomplete — fix the errors and run again.", "プロジェクトの一部を読み取れなかったため（上記の ERROR を参照）、結果は不完全です。エラーを解消して再実行してください。"},
		"rep.verdict_ok":         {"No known incompatibility with %s was found.", "%s との既知の非互換は見つかりませんでした。"},
		"rep.disclaimer": {
			"licgate reports known license incompatibilities based on declared metadata and license files. It is not legal advice; when in doubt, ask a lawyer.",
			"licgate は宣言されたメタデータとライセンスファイルに基づいて既知の非互換を報告するもので、法的助言ではありません。判断に迷う場合は専門家に相談してください。"},
		"rep.path":        {"Path", "依存経路"},
		"rep.reasons":     {"Why", "理由"},
		"rep.obligations": {"Obligations", "義務"},
		"rep.declared":    {"Declared", "宣言"},
		"rep.source":      {"Source", "取得元"},
		"rep.chosen":      {"Using", "選択"},
		"rep.all_deps":    {"All dependencies", "すべての依存"},
		"rep.col_package": {"Package", "パッケージ"},
		"rep.col_version": {"Version", "バージョン"},
		"rep.col_eco":     {"Ecosystem", "エコシステム"},
		"rep.col_license": {"License", "ライセンス"},
		"rep.col_level":   {"Result", "判定"},
		"rep.col_scope":   {"Scope", "スコープ"},
		"rep.warnings":    {"Warnings", "警告"},
		"rep.ob_summary":  {"What you need to do when distributing", "配布時に必要な対応"},
		"rep.ob_count":    {"%d package(s)", "%d 件"},
		"rep.notice_hint": {"Generate THIRD_PARTY_NOTICES with `licgate notice`.", "`licgate notice` で THIRD_PARTY_NOTICES を生成できます。"},
		"rep.unknown":     {"(unknown)", "（不明）"},
		"rep.files":       {"License files", "ライセンスファイル"},
		"rep.notes":       {"Notes", "補足"},
		"rep.direct":      {"direct", "直接"},
		"rep.transitive":  {"transitive", "間接"},
		"rep.fail":        {"FAIL", "失敗"},
		"rep.pass":        {"PASS", "合格"},
		"rep.more_hidden": {"%d NOTICE/OK/SKIP entries hidden (use --all).", "NOTICE/OK/SKIP の %d 件は省略しました（--all で表示）。"},
	})
}

// Levels in display order.
var Levels = []policy.Level{policy.LevelBlock, policy.LevelReview, policy.LevelNotice, policy.LevelOK, policy.LevelSkip}

// LicenseLabel is what we print for a component's license.
func LicenseLabel(c *model.Component) string {
	if c.License == "" {
		return i18n.T("rep.unknown")
	}
	return c.License
}

// Verdict returns the one-line conclusion.
func Verdict(r *policy.Result) string {
	out := r.Outbound.Expr
	if r.Outbound.Proprietary {
		out = i18n.T("w.proprietary")
	}
	switch w := r.Worst(); {
	case r.Incomplete():
		return i18n.T("rep.verdict_incomplete")
	case w >= policy.LevelBlock:
		return i18n.T("rep.verdict_block", out)
	case w >= policy.LevelReview:
		return i18n.T("rep.verdict_review", out)
	}
	return i18n.T("rep.verdict_ok", out)
}

// ObligationSummary counts packages per obligation (excluding skipped).
func ObligationSummary(r *policy.Result) []struct {
	Key   string
	Count int
} {
	counts := map[string]int{}
	for _, f := range r.Findings {
		if f.Level == policy.LevelSkip {
			continue
		}
		for _, o := range f.Obligations {
			counts[o]++
		}
	}
	order := []string{policy.ObGPLSource, policy.ObAGPLNetwork, policy.ObLGPLRelink, policy.ObFileSource, policy.ObUseTerms, policy.ObNoticeFile, policy.ObAttribution}
	var out []struct {
		Key   string
		Count int
	}
	for _, k := range order {
		if counts[k] > 0 {
			out = append(out, struct {
				Key   string
				Count int
			}{k, counts[k]})
		}
	}
	return out
}

// ---- JSON --------------------------------------------------------------------

// WriteJSON writes the machine-readable report.
func WriteJSON(w io.Writer, r *policy.Result, m Meta) error {
	type out struct {
		Meta     Meta              `json:"meta"`
		Verdict  string            `json:"verdict"`
		Settings policy.Settings   `json:"settings"`
		Outbound policy.Outbound   `json:"outbound"`
		Counts   map[string]int    `json:"counts"`
		Sources  []string          `json:"sources"`
		Warnings []string          `json:"warnings,omitempty"`
		Findings []*policy.Finding `json:"findings"`
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out{
		Meta: m, Verdict: Verdict(r), Settings: r.Settings, Outbound: r.Outbound, Counts: r.Counts,
		Sources: r.Graph.Sources, Warnings: r.AllWarnings(), Findings: r.Findings,
	})
}

// ---- Markdown ----------------------------------------------------------------

func mdEscape(s string) string {
	return strings.NewReplacer("|", `\|`, "\n", " ", "<", "&lt;", ">", "&gt;").Replace(s)
}

func settingsLines(r *policy.Result) [][2]string {
	dev := i18n.T("rep.excluded")
	if r.Settings.IncludeDev {
		dev = i18n.T("rep.included")
	}
	return [][2]string{
		{i18n.T("rep.intended"), r.Outbound.Expr},
		{i18n.T("rep.distribution"), string(r.Settings.Distribution)},
		{i18n.T("rep.linking"), string(r.Settings.Linking)},
		{i18n.T("rep.include_dev"), dev},
		{i18n.T("rep.sources"), strings.Join(r.Graph.Sources, ", ")},
	}
}

// WriteMarkdown writes a Markdown report.
func WriteMarkdown(w io.Writer, r *policy.Result, m Meta) error {
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }
	status := i18n.T("rep.pass")
	if m.Failed {
		status = i18n.T("rep.fail")
	}
	p("# %s: %s — %s\n\n", i18n.T("rep.title"), mdEscape(m.Project), status)
	p("> %s\n\n", Verdict(r))
	p("| | |\n|---|---|\n")
	for _, l := range settingsLines(r) {
		p("| %s | %s |\n", l[0], mdEscape(l[1]))
	}
	p("| %s | %s |\n\n", i18n.T("rep.generated"), m.GeneratedAt.Format(time.RFC3339))
	p("## %s\n\n", i18n.T("rep.summary"))
	p("| BLOCK | REVIEW | NOTICE | OK | SKIP |\n|---:|---:|---:|---:|---:|\n")
	p("| %d | %d | %d | %d | %d |\n\n", r.Counts["BLOCK"], r.Counts["REVIEW"], r.Counts["NOTICE"], r.Counts["OK"], r.Counts["SKIP"])
	warns := r.AllWarnings()
	if len(warns) > 0 {
		p("### %s\n\n", i18n.T("rep.warnings"))
		for _, wn := range warns {
			p("- %s\n", mdEscape(wn))
		}
		p("\n")
	}
	for _, lvl := range []policy.Level{policy.LevelBlock, policy.LevelReview} {
		var fs []*policy.Finding
		for _, f := range r.Findings {
			if f.Level == lvl {
				fs = append(fs, f)
			}
		}
		if len(fs) == 0 {
			continue
		}
		p("## %s (%d)\n\n", lvl, len(fs))
		for _, f := range fs {
			c := f.Component
			p("### `%s` (%s) — %s\n\n", c.Key(), c.Ecosystem, mdEscape(LicenseLabel(c)))
			if len(f.Path) > 0 {
				p("- **%s:** %s\n", i18n.T("rep.path"), mdEscape(strings.Join(f.Path, " → ")))
			}
			if len(c.Declared) > 0 {
				p("- **%s:** %s (%s: %s)\n", i18n.T("rep.declared"), mdEscape(strings.Join(c.Declared, " | ")), i18n.T("rep.source"), c.LicenseSource)
			}
			if len(c.LicenseFiles) > 0 {
				var names []string
				for _, lf := range c.LicenseFiles {
					n := lf.Path
					if lf.DetectedID != "" {
						n += " (" + lf.DetectedID + ")"
					}
					names = append(names, n)
				}
				p("- **%s:** %s\n", i18n.T("rep.files"), mdEscape(strings.Join(names, ", ")))
			}
			for _, rs := range f.Reasons {
				p("- %s\n", mdEscape(rs))
			}
			for _, o := range f.Obligations {
				p("- ☐ %s\n", policy.ObligationText(o))
			}
			p("\n")
		}
	}
	if obs := ObligationSummary(r); len(obs) > 0 {
		p("## %s\n\n", i18n.T("rep.ob_summary"))
		for _, o := range obs {
			p("- %s — %s\n", policy.ObligationText(o.Key), i18n.T("rep.ob_count", o.Count))
		}
		p("\n%s\n\n", i18n.T("rep.notice_hint"))
	}
	p("## %s\n\n", i18n.T("rep.all_deps"))
	p("| %s | %s | %s | %s | %s | %s |\n|---|---|---|---|---|---|\n", i18n.T("rep.col_level"), i18n.T("rep.col_package"), i18n.T("rep.col_version"), i18n.T("rep.col_eco"), i18n.T("rep.col_license"), i18n.T("rep.col_scope"))
	for _, f := range sortedFindings(r) {
		c := f.Component
		p("| %s | %s | %s | %s | %s | %s |\n", f.Level, mdEscape(c.Name), mdEscape(c.Version), c.Ecosystem, mdEscape(LicenseLabel(c)), c.Scope)
	}
	p("\n---\n\n_%s_\n", i18n.T("rep.disclaimer"))
	return nil
}

// sortedFindings orders by level (worst first) then name.
func sortedFindings(r *policy.Result) []*policy.Finding {
	fs := append([]*policy.Finding(nil), r.Findings...)
	sort.SliceStable(fs, func(i, j int) bool {
		if fs[i].Level != fs[j].Level {
			return fs[i].Level > fs[j].Level
		}
		return strings.ToLower(fs[i].Component.Name) < strings.ToLower(fs[j].Component.Name)
	})
	return fs
}

// ---- notices -----------------------------------------------------------------

// WriteNotices writes THIRD_PARTY_NOTICES (Markdown) for everything that
// ships: one entry per package and each distinct license text once.
func WriteNotices(w io.Writer, r *policy.Result, m Meta) error {
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }
	var fs []*policy.Finding
	for _, f := range r.Findings {
		if f.Level != policy.LevelSkip {
			fs = append(fs, f)
		}
	}
	sort.Slice(fs, func(i, j int) bool {
		return strings.ToLower(fs[i].Component.Name) < strings.ToLower(fs[j].Component.Name)
	})
	p("# Third-party notices\n\n")
	p("%s includes the following third-party software. Generated by licgate on %s.\n\n", m.Project, m.GeneratedAt.Format("2006-01-02"))
	type textGroup struct {
		title string
		text  string
		pkgs  []string
	}
	groups := map[string]*textGroup{}
	var order []string
	addText := func(title, text, pkg string) string {
		h := sha256.Sum256([]byte(strings.TrimSpace(text)))
		k := hex.EncodeToString(h[:8])
		if g, ok := groups[k]; ok {
			g.pkgs = appendUniq(g.pkgs, pkg)
			return k
		}
		groups[k] = &textGroup{title: title, text: text, pkgs: []string{pkg}}
		order = append(order, k)
		return k
	}
	for _, f := range fs {
		c := f.Component
		p("## %s %s\n\n", c.Name, c.Version)
		lic := LicenseLabel(c)
		if f.Chosen != "" {
			lic = fmt.Sprintf("%s (used under %s)", lic, f.Chosen)
		}
		p("- License: %s\n", lic)
		if c.Homepage != "" {
			p("- Source: %s\n", c.Homepage)
		}
		for _, cr := range c.Copyrights {
			p("- %s\n", cr)
		}
		var refs []string
		for _, lf := range c.LicenseFiles {
			if strings.TrimSpace(lf.Text) == "" {
				continue
			}
			title := lf.DetectedID
			if lf.IsNotice {
				title = "NOTICE"
			}
			if title == "" {
				title = lf.Path
			}
			k := addText(title, lf.Text, c.Key())
			refs = append(refs, fmt.Sprintf("[%s](#text-%s)", title, k))
		}
		if len(refs) > 0 {
			p("- Text: %s\n", strings.Join(refs, ", "))
		} else if c.License != "" {
			if e, err := license.Parse(c.License); err == nil {
				for _, l := range license.Leaves(e) {
					if u := license.SPDXURL(l.ID); u != "" {
						p("- Text: %s — %s\n", l.ID, u)
					}
				}
			}
		}
		p("\n")
	}
	if len(order) > 0 {
		p("# License texts\n\n")
		for _, k := range order {
			g := groups[k]
			p("<a id=\"text-%s\"></a>\n\n## %s\n\nUsed by: %s\n\n```text\n%s\n```\n\n", k, g.title, strings.Join(g.pkgs, ", "), strings.TrimRight(strings.ReplaceAll(g.text, "```", "ˋˋˋ"), "\n"))
		}
	}
	return nil
}

func appendUniq(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
