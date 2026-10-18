package report

import (
	"html/template"
	"io"
	"strings"
	"time"

	"github.com/licgate/licgate/internal/i18n"
	"github.com/licgate/licgate/internal/policy"
)

type htmlRow struct {
	Level       string
	Name        string
	Version     string
	Eco         string
	License     string
	Scope       string
	Direct      bool
	Path        string
	Reasons     []string
	Obligations []string
	Declared    string
	Source      string
	Files       []string
	Homepage    string
	Chosen      string
}

type htmlData struct {
	Lang      string
	T         map[string]string
	Meta      Meta
	Generated string
	Verdict   string
	Failed    bool
	Worst     string
	Settings  [][2]string
	Counts    []struct {
		Level string
		N     int
	}
	Warnings    []string
	Obligations []struct {
		Text string
		N    string
	}
	Rows []htmlRow
}

// WriteHTML writes a self-contained HTML report.
func WriteHTML(w io.Writer, r *policy.Result, m Meta) error {
	d := htmlData{Lang: i18n.Lang, Meta: m, Generated: m.GeneratedAt.Format(time.RFC3339), Verdict: Verdict(r), Failed: m.Failed, Worst: r.Worst().String()}
	d.T = map[string]string{}
	for _, k := range []string{"rep.title", "rep.summary", "rep.warnings", "rep.ob_summary", "rep.all_deps", "rep.path", "rep.reasons",
		"rep.obligations", "rep.declared", "rep.source", "rep.files", "rep.disclaimer", "rep.notice_hint", "rep.generated",
		"rep.direct", "rep.transitive", "rep.chosen", "rep.fail", "rep.pass"} {
		d.T[strings.TrimPrefix(k, "rep.")] = i18n.T(k)
	}
	if i18n.Lang == "ja" {
		d.T["search"], d.T["all"], d.T["nomatch"] = "パッケージ名・ライセンスで絞り込み", "すべて", "該当する依存はありません"
	} else {
		d.T["search"], d.T["all"], d.T["nomatch"] = "Filter by package or license", "All", "No matching dependencies"
	}
	d.Settings = settingsLines(r)
	for _, l := range Levels {
		d.Counts = append(d.Counts, struct {
			Level string
			N     int
		}{l.String(), r.Counts[l.String()]})
	}
	d.Warnings = r.AllWarnings()
	for _, o := range ObligationSummary(r) {
		d.Obligations = append(d.Obligations, struct {
			Text string
			N    string
		}{policy.ObligationText(o.Key), i18n.T("rep.ob_count", o.Count)})
	}
	for _, f := range sortedFindings(r) {
		c := f.Component
		row := htmlRow{Level: f.Level.String(), Name: c.Name, Version: c.Version, Eco: c.Ecosystem, License: LicenseLabel(c),
			Scope: string(c.Scope), Direct: c.Direct, Path: strings.Join(f.Path, " → "), Reasons: f.Reasons,
			Declared: strings.Join(c.Declared, " | "), Source: c.LicenseSource, Homepage: c.Homepage, Chosen: f.Chosen}
		for _, o := range f.Obligations {
			row.Obligations = append(row.Obligations, policy.ObligationText(o))
		}
		for _, lf := range c.LicenseFiles {
			n := lf.Path
			if lf.DetectedID != "" {
				n += " → " + lf.DetectedID
			}
			row.Files = append(row.Files, n)
		}
		d.Rows = append(d.Rows, row)
	}
	return htmlTmpl.Execute(w, d)
}

var htmlTmpl = template.Must(template.New("r").Funcs(template.FuncMap{
	"lower": strings.ToLower,
	"safeURL": func(s string) template.URL {
		if strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") {
			return template.URL(s)
		}
		return ""
	},
}).Parse(`<!doctype html>
<html lang="{{.Lang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>licgate · {{.Meta.Project}}</title>
<style>
:root{
  --bg:#f7f6f3;--panel:#ffffff;--ink:#1d1d1b;--muted:#6b6a66;--line:#e4e2dc;--chip:#efede8;
  --block:#c62828;--block-bg:#fdecea;--review:#a35f00;--review-bg:#fff4e0;--notice:#1f5fbf;--notice-bg:#e9f1fd;
  --ok:#2e7d32;--ok-bg:#e9f5ea;--skip:#7a7a7a;--skip-bg:#f0f0f0;
  --mono:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;
  --sans:-apple-system,BlinkMacSystemFont,"Segoe UI","Hiragino Sans","Noto Sans JP",system-ui,sans-serif;
}
@media (prefers-color-scheme:dark){:root{
  --bg:#141413;--panel:#1c1c1a;--ink:#ecebe6;--muted:#9b9a94;--line:#2e2e2b;--chip:#262624;
  --block:#ff7b72;--block-bg:#3a1d1b;--review:#f2b84b;--review-bg:#352a14;--notice:#82b4ff;--notice-bg:#17263d;
  --ok:#7ad77f;--ok-bg:#16301a;--skip:#9a9a9a;--skip-bg:#262624;
}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--ink);font:15px/1.55 var(--sans)}
main{max-width:1040px;margin:0 auto;padding:32px 16px 64px}
.brand{font:600 12px var(--mono);letter-spacing:.08em;text-transform:uppercase;color:var(--muted)}
h1{font-size:26px;line-height:1.25;margin:6px 0 12px;word-break:break-all}
h2{font-size:15px;margin:32px 0 10px;letter-spacing:.02em}
.verdict{display:flex;gap:12px;align-items:flex-start;padding:14px 16px;border-radius:10px;background:var(--panel);border:1px solid var(--line)}
.pill{flex:none;font:700 12px var(--mono);padding:4px 10px;border-radius:999px;color:#fff}
.pill.fail{background:var(--block)}.pill.pass{background:var(--ok)}
.settings{display:grid;grid-template-columns:max-content 1fr;gap:4px 16px;margin:16px 0 0;font-size:13px}
.settings dt{color:var(--muted)}.settings dd{margin:0;font-family:var(--mono);word-break:break-word}
.counts{display:grid;grid-template-columns:repeat(5,1fr);gap:8px}
.count{border:1px solid var(--line);background:var(--panel);border-radius:10px;padding:10px 12px;cursor:pointer;text-align:left;font:inherit;color:inherit}
.count b{display:block;font:600 22px var(--mono)}
.count span{font:600 11px var(--mono);letter-spacing:.06em}
.count[aria-pressed=true]{outline:2px solid currentColor;outline-offset:-2px}
.BLOCK{color:var(--block)}.REVIEW{color:var(--review)}.NOTICE{color:var(--notice)}.OK{color:var(--ok)}.SKIP{color:var(--skip)}
.box{background:var(--panel);border:1px solid var(--line);border-radius:10px;padding:12px 16px}
.box ul{margin:0;padding-left:18px}
.warn{border-color:var(--review);background:var(--review-bg)}
.toolbar{display:flex;gap:8px;margin:0 0 10px}
.toolbar input{flex:1;min-width:0;padding:9px 12px;border-radius:8px;border:1px solid var(--line);background:var(--panel);color:var(--ink);font:inherit}
.toolbar button{padding:9px 12px;border-radius:8px;border:1px solid var(--line);background:var(--panel);color:var(--ink);font:inherit;cursor:pointer}
.list{background:var(--panel);border:1px solid var(--line);border-radius:10px;overflow:hidden}
details{border-top:1px solid var(--line)}details:first-child{border-top:0}
summary{display:grid;grid-template-columns:70px minmax(0,1.6fr) minmax(0,1fr) 80px;gap:12px;align-items:center;padding:10px 14px;cursor:pointer;list-style:none}
summary::-webkit-details-marker{display:none}
summary:hover{background:var(--chip)}
.lv{font:700 11px var(--mono);letter-spacing:.05em;padding:2px 6px;border-radius:5px;text-align:center}
.lv.BLOCK{background:var(--block-bg)}.lv.REVIEW{background:var(--review-bg)}.lv.NOTICE{background:var(--notice-bg)}.lv.OK{background:var(--ok-bg)}.lv.SKIP{background:var(--skip-bg)}
.pkg{font-family:var(--mono);font-size:13.5px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.pkg small{color:var(--muted)}
.lic{font-family:var(--mono);font-size:13px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.eco{font:12px var(--mono);color:var(--muted);text-align:right}
.detail{padding:4px 14px 14px 96px;font-size:14px}
.detail dl{display:grid;grid-template-columns:max-content 1fr;gap:4px 14px;margin:0}
.detail dt{color:var(--muted);font-size:12.5px;padding-top:2px}.detail dd{margin:0;word-break:break-word}
.detail ul{margin:0;padding-left:18px}
.mono{font-family:var(--mono);font-size:13px}
a{color:var(--notice)}
.empty{padding:24px;text-align:center;color:var(--muted)}
footer{margin-top:32px;color:var(--muted);font-size:12.5px}
@media (max-width:640px){
  .counts{grid-template-columns:repeat(3,1fr)}
  summary{grid-template-columns:62px minmax(0,1fr);row-gap:2px}
  .lic{grid-column:2}.eco{display:none}
  .detail{padding:4px 14px 14px}
  .detail dl{grid-template-columns:1fr}
}
</style>
</head>
<body>
<main>
<div class="brand">licgate · {{index .T "title"}}</div>
<h1>{{.Meta.Project}}</h1>
<div class="verdict"><span class="pill {{if .Failed}}fail{{else}}pass{{end}}">{{if .Failed}}{{index .T "fail"}}{{else}}{{index .T "pass"}}{{end}}</span><div>{{.Verdict}}</div></div>
<dl class="settings">
{{range .Settings}}<dt>{{index . 0}}</dt><dd>{{index . 1}}</dd>{{end}}
<dt>{{index .T "generated"}}</dt><dd>{{.Generated}}</dd>
</dl>

<h2>{{index .T "summary"}}</h2>
<div class="counts" role="group">
{{range .Counts}}<button class="count {{.Level}}" data-filter="{{.Level}}" aria-pressed="false"><b>{{.N}}</b><span>{{.Level}}</span></button>{{end}}
</div>

{{if .Warnings}}<h2>{{index .T "warnings"}}</h2><div class="box warn"><ul>{{range .Warnings}}<li>{{.}}</li>{{end}}</ul></div>{{end}}

{{if .Obligations}}<h2>{{index .T "ob_summary"}}</h2>
<div class="box"><ul>{{range .Obligations}}<li>{{.Text}} <span class="mono" style="color:var(--muted)">— {{.N}}</span></li>{{end}}</ul>
<p style="margin:8px 0 0;color:var(--muted);font-size:13px">{{index .T "notice_hint"}}</p></div>{{end}}

<h2>{{index .T "all_deps"}}</h2>
<div class="toolbar"><input id="q" type="search" placeholder="{{index .T "search"}}" autocomplete="off"><button id="reset">{{index .T "all"}}</button></div>
<div class="list" id="list">
{{range .Rows}}<details data-level="{{.Level}}" data-q="{{lower .Name}} {{lower .License}} {{.Eco}}"{{if eq .Level "BLOCK"}} open{{end}}>
<summary><span class="lv {{.Level}}">{{.Level}}</span><span class="pkg">{{.Name}} <small>{{.Version}}</small></span><span class="lic">{{.License}}</span><span class="eco">{{.Eco}}</span></summary>
<div class="detail"><dl>
{{if .Reasons}}<dt>{{index $.T "reasons"}}</dt><dd><ul>{{range .Reasons}}<li>{{.}}</li>{{end}}</ul></dd>{{end}}
{{if .Obligations}}<dt>{{index $.T "obligations"}}</dt><dd><ul>{{range .Obligations}}<li>{{.}}</li>{{end}}</ul></dd>{{end}}
{{if .Chosen}}<dt>{{index $.T "chosen"}}</dt><dd class="mono">{{.Chosen}}</dd>{{end}}
{{if .Path}}<dt>{{index $.T "path"}}</dt><dd class="mono">{{.Path}}</dd>{{end}}
{{if .Declared}}<dt>{{index $.T "declared"}}</dt><dd class="mono">{{.Declared}}{{if .Source}} <span style="color:var(--muted)">({{.Source}})</span>{{end}}</dd>{{end}}
{{if .Files}}<dt>{{index $.T "files"}}</dt><dd class="mono">{{range .Files}}{{.}}<br>{{end}}</dd>{{end}}
<dt>scope</dt><dd class="mono">{{.Scope}} · {{if .Direct}}{{index $.T "direct"}}{{else}}{{index $.T "transitive"}}{{end}}</dd>
{{with safeURL .Homepage}}<dt>URL</dt><dd><a href="{{.}}" rel="noopener noreferrer">{{.}}</a></dd>{{end}}
</dl></div>
</details>
{{end}}
<div class="empty" id="empty" hidden>{{index .T "nomatch"}}</div>
</div>
<footer>{{index .T "disclaimer"}} · licgate {{.Meta.Version}}</footer>
</main>
<script>
(function(){
  var level = "", q = document.getElementById("q"), rows = document.querySelectorAll("#list details"),
      btns = document.querySelectorAll(".count"), empty = document.getElementById("empty");
  function apply(){
    var s = q.value.trim().toLowerCase(), n = 0;
    rows.forEach(function(r){
      var ok = (!level || r.dataset.level === level) && (!s || r.dataset.q.indexOf(s) >= 0);
      r.hidden = !ok; if (ok) n++;
    });
    empty.hidden = n > 0;
    btns.forEach(function(b){ b.setAttribute("aria-pressed", b.dataset.filter === level ? "true" : "false"); });
  }
  btns.forEach(function(b){ b.addEventListener("click", function(){ level = level === b.dataset.filter ? "" : b.dataset.filter; apply(); }); });
  document.getElementById("reset").addEventListener("click", function(){ level = ""; q.value = ""; apply(); });
  q.addEventListener("input", apply);
})();
</script>
</body>
</html>
`))
