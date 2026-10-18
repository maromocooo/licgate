// Command licgate checks, before you publish, whether your dependencies'
// licenses are compatible with the license you intend to release under.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"

	"github.com/licgate/licgate/internal/collect"
	"github.com/licgate/licgate/internal/config"
	"github.com/licgate/licgate/internal/i18n"
	"github.com/licgate/licgate/internal/jev"
	"github.com/licgate/licgate/internal/policy"
	"github.com/licgate/licgate/internal/report"
	"github.com/licgate/licgate/internal/tui"
)

var version = "0.1.0-dev"

const usage = `licgate — check your dependencies' licenses before you publish

Usage:
  licgate [check] [flags] [path]     scan and judge (interactive TUI in a terminal)
  licgate report  [flags] [path]     write a report (--format md|html|json, default md)
  licgate notice  [flags] [path]     write THIRD_PARTY_NOTICES.md for what you ship
  licgate sbom    [flags] [path]     write a CycloneDX SBOM with resolved licenses
  licgate init    [flags] [path]     create .licgate.toml
  licgate version

Flags:
  -l, --license EXPR     license you intend to publish under: SPDX (MIT, Apache-2.0,
                         GPL-3.0-or-later, "MIT OR Apache-2.0" …) or "proprietary"
  --dist MODE            binary | library | saas | internal       (default binary)
  --linking MODE         auto | static | dynamic                  (default auto)
  --include-dev          also evaluate dev/test-only dependencies
  --sbom FILE            use a CycloneDX/SPDX JSON SBOM instead of scanning (repeatable)
  --generator NAME       auto | native | syft                     (default auto)
  --online               look up missing licenses on npm/PyPI/crates.io/deps.dev
  --jev                  classify unrecognised license texts with Jev (needs TYPESAFE_API_KEY)
  --jev-provider NAME    typesafe | openrouter
  --jev-model NAME       e.g. jev-latest
  --jev-threshold P      minimum probability to accept Jev's answer (default 0.85)
  --fail-on LEVEL        block | review | never                   (default block)
  --format FMT           text | json | md | html | cyclonedx | notice
  -o, --output FILE      write to FILE instead of stdout
  --all                  text output: also list NOTICE/OK/SKIP entries
  --no-tui               never start the interactive UI
  --lang LANG            en | ja (default: from $LANG)
  --config FILE          config file (default <path>/.licgate.toml)
  -q, --quiet            no progress messages
  --force                init: overwrite an existing config

Exit status: 0 = pass, 1 = findings at or above --fail-on, 2 = error.
`

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

type opts struct {
	license, dist, linking, generator, failOn, format, output, lang, configPath string
	jevProvider, jevModel                                                       string
	jevThreshold                                                                float64
	includeDev, online, useJev, all, noTUI, quiet, force                        bool
	sboms                                                                       multiFlag
	set                                                                         map[string]bool
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cmd := "check"
	if len(args) > 0 {
		switch args[0] {
		case "check", "report", "notice", "sbom", "init":
			cmd, args = args[0], args[1:]
		case "version", "--version", "-v":
			fmt.Println("licgate", version)
			return 0
		case "help", "-h", "--help":
			fmt.Print(usage)
			return 0
		}
	}
	o := opts{set: map[string]bool{}}
	fs := flag.NewFlagSet("licgate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.license, "license", "", "")
	fs.StringVar(&o.license, "l", "", "")
	fs.StringVar(&o.dist, "dist", "", "")
	fs.StringVar(&o.dist, "distribution", "", "")
	fs.StringVar(&o.linking, "linking", "", "")
	fs.BoolVar(&o.includeDev, "include-dev", false, "")
	fs.Var(&o.sboms, "sbom", "")
	fs.StringVar(&o.generator, "generator", "auto", "")
	fs.BoolVar(&o.online, "online", false, "")
	fs.BoolVar(&o.useJev, "jev", false, "")
	fs.StringVar(&o.jevProvider, "jev-provider", "", "")
	fs.StringVar(&o.jevModel, "jev-model", "", "")
	fs.Float64Var(&o.jevThreshold, "jev-threshold", 0, "")
	fs.StringVar(&o.failOn, "fail-on", "", "")
	fs.StringVar(&o.format, "format", "", "")
	fs.StringVar(&o.output, "output", "", "")
	fs.StringVar(&o.output, "o", "", "")
	fs.BoolVar(&o.all, "all", false, "")
	fs.BoolVar(&o.noTUI, "no-tui", false, "")
	fs.StringVar(&o.lang, "lang", "", "")
	fs.StringVar(&o.configPath, "config", "", "")
	fs.BoolVar(&o.quiet, "quiet", false, "")
	fs.BoolVar(&o.quiet, "q", false, "")
	fs.BoolVar(&o.force, "force", false, "")

	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				fmt.Print(usage)
				return 0
			}
			fmt.Fprintln(os.Stderr, "licgate:", err)
			fmt.Fprintln(os.Stderr, "run `licgate --help` for usage")
			return 2
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
	fs.Visit(func(f *flag.Flag) { o.set[f.Name] = true })
	if o.lang != "" {
		i18n.Set(o.lang)
	}
	path := "."
	if len(pos) > 0 {
		path = pos[0]
	}
	if len(pos) > 1 {
		fmt.Fprintln(os.Stderr, "licgate: only one path may be given")
		return 2
	}
	if cmd == "init" {
		return runInit(path, o)
	}
	code, err := runCheck(cmd, path, o)
	if err != nil {
		fmt.Fprintln(os.Stderr, "licgate:", err)
		return 2
	}
	return code
}

func runInit(path string, o opts) int {
	p := o.configPath
	if p == "" {
		p = config.Find(path)
	}
	if _, err := os.Stat(p); err == nil && !o.force {
		fmt.Fprintf(os.Stderr, "licgate: %s already exists (use --force to overwrite)\n", p)
		return 2
	}
	c := config.Config{License: o.license, Distribution: o.dist, Linking: o.linking, IncludeDev: o.includeDev, FailOn: o.failOn, Online: o.online}
	if c.License == "" {
		c.License = "MIT"
	}
	if c.Distribution == "" {
		c.Distribution = "binary"
	}
	if c.Linking == "" {
		c.Linking = "auto"
	}
	if c.FailOn == "" {
		c.FailOn = "block"
	}
	if err := os.WriteFile(p, config.Template(c), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "licgate:", err)
		return 2
	}
	fmt.Println("wrote", p)
	return 0
}

// resolved holds the effective settings after config + flags.
type resolved struct {
	settings  policy.Settings
	failOn    policy.Level
	failName  string
	online    bool
	jevOn     bool
	jevProv   string
	jevModel  string
	jevThresh float64
	cfgPath   string
	cfgFound  bool
}

func resolve(path string, o opts) (resolved, error) {
	var r resolved
	r.cfgPath = o.configPath
	if r.cfgPath == "" {
		r.cfgPath = config.Find(path)
	}
	cfg, err := config.Load(r.cfgPath)
	if err != nil {
		return r, err
	}
	if cfg == nil {
		cfg = &config.Config{}
	} else {
		r.cfgFound = true
	}
	pick := func(flagName, flagVal, cfgVal, def string) string {
		if o.set[flagName] && flagVal != "" {
			return flagVal
		}
		if cfgVal != "" {
			return cfgVal
		}
		return def
	}
	lic := cfg.License
	if o.set["license"] || o.set["l"] {
		lic = o.license
	}
	distName := pick("dist", o.dist, cfg.Distribution, "")
	if o.set["distribution"] {
		distName = o.dist
	}
	if distName == "" {
		distName = "binary"
	}
	dist, ok := policy.ParseDistribution(distName)
	if !ok {
		return r, fmt.Errorf("invalid distribution %q (binary | library | saas | internal)", distName)
	}
	linkName := pick("linking", o.linking, cfg.Linking, "auto")
	link, ok := policy.ParseLinking(linkName)
	if !ok {
		return r, fmt.Errorf("invalid linking %q (auto | static | dynamic)", linkName)
	}
	r.failName = pick("fail-on", o.failOn, cfg.FailOn, "block")
	if r.failOn, ok = policy.ParseLevel(r.failName); !ok {
		return r, fmt.Errorf("invalid --fail-on %q (block | review | never)", r.failName)
	}
	r.settings = policy.Settings{
		License: lic, Distribution: dist, Linking: link,
		IncludeDev: cfg.IncludeDev || o.includeDev,
		Allow:      cfg.Allow, Deny: cfg.Deny, Overrides: cfg.Override, Ignores: cfg.Ignore,
	}
	r.online = cfg.Online || o.online
	r.jevOn = cfg.Jev.Enabled || o.useJev
	r.jevProv = pick("jev-provider", o.jevProvider, cfg.Jev.Provider, "typesafe")
	r.jevModel = pick("jev-model", o.jevModel, cfg.Jev.Model, "")
	r.jevThresh = cfg.Jev.Threshold
	if o.set["jev-threshold"] {
		r.jevThresh = o.jevThreshold
	}
	return r, nil
}

func isTTY(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

func runCheck(cmd, path string, o opts) (int, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 2, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return 2, err
	}
	if !st.IsDir() {
		// A single file: treat it as an SBOM.
		o.sboms = append(o.sboms, abs)
		abs = filepath.Dir(abs)
	}
	r, err := resolve(abs, o)
	if err != nil {
		return 2, err
	}
	format := o.format
	if format == "" {
		switch cmd {
		case "report":
			format = "md"
		case "notice":
			format = "notice"
		case "sbom":
			format = "cyclonedx"
		default:
			format = "text"
		}
	}
	switch format {
	case "text", "json", "md", "markdown", "html", "cyclonedx", "cdx", "notice":
	default:
		return 2, fmt.Errorf("unknown --format %q", format)
	}
	interactive := cmd == "check" && format == "text" && o.output == "" && !o.noTUI && isTTY(os.Stdout) && isTTY(os.Stdin) && os.Getenv("CI") == ""

	if strings.TrimSpace(r.settings.License) == "" {
		if !interactive {
			return 2, fmt.Errorf("no intended license given: pass --license (e.g. --license MIT, --license proprietary) or create %s with `licgate init`", config.FileName)
		}
		s, ok, err := tui.AskSettings(r.settings)
		if err != nil {
			return 2, err
		}
		if !ok {
			return 2, nil
		}
		r.settings = s
	}

	logf := func(format string, a ...any) {
		if o.quiet {
			return
		}
		fmt.Fprintln(os.Stderr, lipgloss.NewStyle().Foreground(report.ColorDim).Render("· "+fmt.Sprintf(format, a...)))
	}
	g, err := collect.Run(collect.Options{Path: abs, SBOMs: o.sboms, Generator: o.generator, Online: r.online, Log: logf})
	if err != nil {
		return 2, err
	}
	if r.jevOn {
		cl, err := jev.New(r.jevProv, r.jevModel)
		if err != nil {
			g.Warnings = append(g.Warnings, "jev: "+err.Error())
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			n, err := jev.Apply(ctx, g, cl, r.jevThresh, logf)
			cancel()
			if err != nil {
				g.Warnings = append(g.Warnings, "jev: "+err.Error())
			}
			if n > 0 {
				logf("jev: %d call(s), %d input tokens", cl.Calls, cl.InputTokens)
			}
		}
	}
	res := policy.Evaluate(g, r.settings)
	meta := report.Meta{Tool: "licgate", Version: version, Project: projectName(abs, g.Roots, g.RootNames), GeneratedAt: time.Now(), FailOn: r.failName}
	meta.Failed = res.Worst() >= r.failOn

	if interactive {
		return tui.Run(tui.Options{
			Result: res, Meta: meta, FailOn: r.failOn, Dir: abs, ConfigPath: r.cfgPath, ConfigFound: r.cfgFound,
			Reevaluate: func(s policy.Settings) *policy.Result { return policy.Evaluate(g, s) },
			Write:      func(format, p string, rr *policy.Result, m report.Meta) error { return writeFile(format, p, rr, m) },
		})
	}

	var buf bytes.Buffer
	if err := render(&buf, format, res, meta, o.all); err != nil {
		return 2, err
	}
	if o.output != "" {
		if err := os.WriteFile(o.output, buf.Bytes(), 0o644); err != nil {
			return 2, err
		}
		if !o.quiet {
			fmt.Fprintf(os.Stderr, "wrote %s\n", o.output)
			if format != "text" {
				// Still show the one-line result in the terminal.
				status := "PASS"
				if meta.Failed {
					status = "FAIL"
				}
				fmt.Fprintf(os.Stderr, "%s  %s\n", status, report.Verdict(res))
			}
		}
	} else {
		os.Stdout.Write(buf.Bytes())
	}
	if cmd == "notice" || cmd == "sbom" {
		return 0, nil
	}
	if meta.Failed {
		return 1, nil
	}
	return 0, nil
}

func render(w io.Writer, format string, res *policy.Result, meta report.Meta, all bool) error {
	switch format {
	case "text":
		report.WriteText(w, res, meta, all)
		return nil
	case "json":
		return report.WriteJSON(w, res, meta)
	case "md", "markdown":
		return report.WriteMarkdown(w, res, meta)
	case "html":
		return report.WriteHTML(w, res, meta)
	case "cyclonedx", "cdx":
		return report.WriteCycloneDX(w, res, meta)
	case "notice":
		return report.WriteNotices(w, res, meta)
	}
	return fmt.Errorf("unknown format %q", format)
}

func writeFile(format, p string, res *policy.Result, meta report.Meta) error {
	var buf bytes.Buffer
	if err := render(&buf, format, res, meta, true); err != nil {
		return err
	}
	return os.WriteFile(p, buf.Bytes(), 0o644)
}

func projectName(abs string, roots []string, names map[string]string) string {
	if len(roots) == 1 {
		if n := names[roots[0]]; n != "" && n != "project" {
			return n
		}
	}
	return filepath.Base(abs)
}
