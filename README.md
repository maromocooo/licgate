# licgate

**Check your dependencies' licenses before you publish.**

licgate looks at every package your project pulls in — across npm, Python, Go, Rust, PHP, and anything
syft or an SBOM can describe — and tells you whether they can ship under the license you intend to
release with. It runs as an interactive TUI in your terminal and as a plain CLI in CI, and it writes
reports, a `THIRD_PARTY_NOTICES` file and an enriched CycloneDX SBOM.

```
$ licgate -l MIT
BLOCK  ffmpeg-static@5.3.0 (npm)  GPL-3.0-or-later
       Path: myapp → ffmpeg-static@5.3.0
       - GPL-3.0-or-later is strong copyleft: a distributed program that includes it must be
         released under the GPL as a whole, which conflicts with MIT.
REVIEW mapbox-gl@2.15.0 (npm)  (unknown)
       - LICENSE.txt might be "Proprietary / all rights reserved", but the match is weak — read it.
…
FAIL  Known incompatibilities with MIT were found. Resolve the BLOCK items before publishing.
```

> licgate reports *known* incompatibilities based on declared metadata and license files.
> It is not legal advice.

## Why

Most license tools list licenses. licgate answers the question you actually have right before
`git push --tags`: *"Can I publish this under X?"* That depends on more than the dependency's
license:

| Question licgate asks | Why it matters |
| --- | --- |
| Which license will **you** publish under? | GPL deps are fine in a GPL project, fatal in an MIT one. `GPL-2.0-only` can't take Apache-2.0. |
| How does it reach people? (`binary`, `library`, `saas`, `internal`) | GPL obligations trigger on distribution; AGPL also on network use; a source-only library doesn't redistribute its deps. |
| Static or dynamic linking? | LGPL is easy when dynamically linked, hard when statically linked (Go, Rust, bundled JS). |
| Does the dependency actually ship? | Test-only Go modules, `devDependencies`, proc-macro crates and Python tooling are skipped by default. |

Every verdict comes with the reasons and the dependency path that pulled the package in.

| Level | Meaning |
| --- | --- |
| `BLOCK` | Known incompatibility with your intended license. |
| `REVIEW` | A human should look (unknown/custom license, LGPL static linking, metadata vs. LICENSE mismatch …). |
| `NOTICE` | Fine, with obligations (attribution, NOTICE files, source for MPL files …). |
| `OK` | Nothing to do. |
| `SKIP` | Not shipped (dev/test-only) or ignored by config. |

## Install

**Ubuntu / Debian** (amd64 or arm64) — download the `.deb` from the
[latest release](https://github.com/maromocooo/licgate/releases/latest):

```sh
arch=$(dpkg --print-architecture)   # amd64 or arm64
ver=0.1.0
curl -fLO "https://github.com/maromocooo/licgate/releases/download/v${ver}/licgate_${ver}_${arch}.deb"
sudo apt install "./licgate_${ver}_${arch}.deb"
```

**Any Linux / macOS** — the release `.tar.gz` contains a single static binary:

```sh
tar xzf licgate_0.1.0_linux_amd64.tar.gz
sudo install licgate_0.1.0_linux_amd64/licgate /usr/local/bin/
```

**From source** (Go 1.24+):

```sh
go install github.com/maromocooo/licgate/cmd/licgate@latest
```

licgate itself has no runtime dependencies. For the most accurate results it
uses the toolchains you already have when present: `go` (Go projects), `cargo`
(Rust), and an installed `node_modules` / Python virtualenv.

Maintainers: `scripts/package.sh 0.1.0` builds all archives and `.deb` files
into `dist/`; pushing a `v*` tag does the same in CI and publishes a release.

## Use

```sh
licgate                         # interactive: asks your license & distribution, then a browsable TUI
licgate -l MIT                  # judge against MIT (binary distribution by default)
licgate -l Apache-2.0 --dist library
licgate -l proprietary --dist saas
licgate -l "MIT OR Apache-2.0"  # dual-licensed: every alternative must work

licgate report --format html -o licgate-report.html
licgate notice -o THIRD_PARTY_NOTICES.md
licgate sbom -o bom.cdx.json
licgate init -l MIT --dist binary   # write .licgate.toml
```

In the TUI: `↑↓` move · `tab` filter by level · `/` search · `enter` details (narrow terminals) ·
`s` change license/distribution and re-evaluate instantly · `e` export reports/notices/SBOM/config ·
`q` quit.

### In CI

`licgate` exits `1` when a finding reaches `--fail-on` (default `block`), `2` on errors, and never
starts the TUI when stdout isn't a terminal or `CI` is set.

```yaml
# .github/workflows/license.yml
name: license
on: [pull_request, push]
jobs:
  licgate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.24" }
      - run: npm ci            # or: uv sync / cargo fetch / go mod download …
      - run: go install github.com/maromocooo/licgate/cmd/licgate@latest
      - run: licgate report --format md -o licgate.md --fail-on block
      - if: always()
        run: cat licgate.md >> "$GITHUB_STEP_SUMMARY"
```

## What it reads

| Ecosystem | Source | Scope (what ships) |
| --- | --- | --- |
| npm / yarn / pnpm | `package-lock.json` (v1–v3), otherwise the installed `node_modules` (incl. pnpm's `.pnpm`) | lockfile `dev` flags, or reachability from `dependencies` |
| Python | the project's virtualenv (`.venv`, `venv`, `$VIRTUAL_ENV`): `METADATA` incl. PEP 639 `License-Expression`, classifiers and license files; else `uv.lock` / `poetry.lock` / pinned `requirements.txt` (+ `--online`) | reachability from `pyproject.toml` / `requirements*.txt` |
| Go | `go list -deps ./...` (modules actually linked), `go mod graph`, LICENSE files in the module cache; the Go stdlib is listed too | test-only modules are skipped |
| Rust | `cargo metadata` | dev-dependencies, build-dependencies and proc-macro crates are skipped |
| PHP | `composer.lock` | `packages-dev` are skipped |
| everything else | [syft](https://github.com/anchore/syft) if installed (auto), or `--sbom` with CycloneDX/SPDX 2.x JSON | from the SBOM |

Declared licenses are normalised to SPDX (`"Apache 2.0"`, `"MIT/Apache-2.0"`, `GPLv3+`, PyPI
classifiers, legacy `licenses` arrays …). License files are recognised by their text, and licgate
flags packages whose metadata disagrees with their LICENSE file, as well as custom terms that merely
*append* open source notices.

## Optional extras

* `--online` — look up missing licenses on npm, PyPI, crates.io and deps.dev (only package names and
  versions are sent).
* `--jev` — classify license texts the built-in detector can't place with
  [Jev](https://docs.typesafe.ai) (TypeSafe's System One model), which returns calibrated
  probabilities. Answers below `--jev-threshold` (default 0.85) stay `REVIEW`. Needs
  `TYPESAFE_API_KEY` (or `OPENROUTER_API_KEY` with `--jev-provider openrouter`). Only the license
  file texts of unrecognised packages are sent — never your code.

## Configuration

`licgate init` writes a commented `.licgate.toml`:

```toml
license = "MIT"
distribution = "binary"   # binary | library | saas | internal
linking = "auto"          # auto | static | dynamic
include_dev = false
fail_on = "block"         # block | review | never
deny = ["SSPL-1.0"]

[[override]]              # after checking a package by hand
package = "some-package"
license = "BSD-3-Clause"
reason = "LICENSE file checked on 2026-10-18"

[[ignore]]
package = "@mycompany/*"
reason = "internal packages"
```

## Limits

licgate encodes widely accepted compatibility rules (FSF's GPL compatibility list, copyleft scope,
linking, network clauses) — not every license and not every legal nuance. Unknown licenses, custom
terms and ambiguous cases are surfaced as `REVIEW` rather than guessed. Output language follows
`$LANG` (`--lang en|ja`).

## License

Apache License 2.0 — see [LICENSE](LICENSE).
