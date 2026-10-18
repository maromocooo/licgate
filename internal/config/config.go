// Package config reads and writes .licgate.toml.
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/maromocooo/licgate/internal/policy"
)

// FileName is the default config file name.
const FileName = ".licgate.toml"

// Config mirrors .licgate.toml.
type Config struct {
	License      string            `toml:"license"`
	Distribution string            `toml:"distribution"`
	Linking      string            `toml:"linking"`
	IncludeDev   bool              `toml:"include_dev"`
	FailOn       string            `toml:"fail_on"`
	Online       bool              `toml:"online"`
	Allow        []string          `toml:"allow"`
	Deny         []string          `toml:"deny"`
	Override     []policy.Override `toml:"override"`
	Ignore       []policy.Ignore   `toml:"ignore"`
	Jev          struct {
		Enabled   bool    `toml:"enabled"`
		Model     string  `toml:"model"`
		Provider  string  `toml:"provider"`
		Threshold float64 `toml:"threshold"`
	} `toml:"jev"`
}

// Load reads path; a missing file is not an error (returns nil, nil).
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	md, err := toml.Decode(string(b), &c)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return nil, fmt.Errorf("%s: unknown keys: %v", path, und)
	}
	return &c, nil
}

// Find looks for the config in dir.
func Find(dir string) string { return filepath.Join(dir, FileName) }

// Template renders a commented config file.
func Template(c Config) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, `# licgate configuration — https://github.com/maromocooo/licgate
#
# The license you intend to publish under: an SPDX expression
# (e.g. "MIT", "Apache-2.0", "GPL-3.0-or-later", "MIT OR Apache-2.0")
# or "proprietary" for closed source.
license = %q

# How the project reaches other people:
#   binary   – you ship an artifact containing the dependencies
#              (app, CLI binary, container image, bundled front-end)
#   library  – you publish source to a registry; users install deps themselves
#   saas     – you run it as a network service and hand nothing out
#   internal – used only inside your organisation
distribution = %q

# auto | static | dynamic  (auto: Go/Rust/bundled JS = static, Python/Java/Ruby/PHP = dynamic)
linking = %q

# Evaluate dev/test-only dependencies too (set true if your build bundles devDependencies).
include_dev = %v

# Exit non-zero when a finding is at least this level: block | review | never
fail_on = %q

# Look up missing licenses on npm / PyPI / crates.io / deps.dev (sends package names + versions).
online = %v

# Licenses to always reject / always accept (SPDX ids).
deny = []
allow = []

# Correct a package's license after checking it yourself:
# [[override]]
# package = "some-package"          # name, name@version, eco/name or purl
# license = "MIT"
# reason  = "LICENSE file checked manually on 2026-10-18"

# Exclude a package (e.g. your own internal library):
# [[ignore]]
# package = "@mycompany/*"
# reason  = "internal package"

# Optional: classify unrecognised license texts with Jev (TypeSafe AI).
# Needs TYPESAFE_API_KEY (or OPENROUTER_API_KEY with provider = "openrouter").
# Only license file texts are sent — never your source code.
[jev]
enabled = false
model = "jev-latest"
provider = "typesafe"
threshold = 0.85
`, c.License, c.Distribution, c.Linking, c.IncludeDev, c.FailOn, c.Online)
	return b.Bytes()
}
