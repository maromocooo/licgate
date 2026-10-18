# licgate

**公開前に、依存パッケージのライセンスをまとめてチェックするツールです。**

licgate はプロジェクトが取り込むすべてのパッケージを調べ、あなたが公開しようとしているライセンスで配布できるかを判定します。対象は npm、Python、Go、Rust、PHP、さらに syft や SBOM で記述できるものすべてです。

- ターミナルでは対話型の TUI として、CI では通常の CLI として動きます。
- レポート、`THIRD_PARTY_NOTICES`、ライセンス情報を補った CycloneDX SBOM を書き出せます。

> licgate は宣言されたメタデータとライセンスファイルをもとに「既知の非互換」を報告するツールで、法的助言ではありません。

## 何を判定するか

ライセンス一覧を出すツールは多くありますが、licgate が答えるのは公開直前に知りたい「これを X ライセンスで出して大丈夫か」という問いです。答えは依存先のライセンスだけでは決まらず、次の条件によって変わります。

| licgate が考慮すること | 理由 |
| --- | --- |
| **あなたの**公開ライセンス | GPL の依存は、GPL のプロジェクトなら問題ありませんが、MIT のプロジェクトでは致命的です。`GPL-2.0-only` は Apache-2.0 を取り込めません。 |
| 提供形態（`binary` / `library` / `saas` / `internal`） | GPL の義務は配布で発生し、AGPL はネットワーク利用でも発生します。ソースだけを公開するライブラリは、依存を再配布しません。 |
| 静的リンクか動的リンクか | LGPL は動的リンクなら容易ですが、静的リンク（Go、Rust、バンドルされた JS）では難しくなります。 |
| その依存が本当に配布物に入るか | テスト専用の Go モジュール、`devDependencies`、proc-macro クレート、Python の開発ツールは、既定でスキップします。 |

各判定には、理由と「どの依存経由で入ってきたか」の経路が付きます。

| レベル | 意味 |
| --- | --- |
| `BLOCK` | 公開予定のライセンスと既知の非互換がある |
| `REVIEW` | 人が確認する必要がある（不明・独自ライセンス、LGPL の静的リンク、メタデータと LICENSE の不一致など） |
| `NOTICE` | 問題はないが、義務がある（著作権表示、NOTICE ファイル、MPL ファイルのソース提供など） |
| `OK` | 対応不要 |
| `SKIP` | 配布物に含まれない（開発・テスト専用）か、設定で除外したもの |

## インストール

**Ubuntu / Debian**（amd64・arm64）は、[最新リリース](https://github.com/licgate/licgate/releases/latest)の `.deb` を使います。

```sh
arch=$(dpkg --print-architecture)   # amd64 または arm64
ver=0.1.0
curl -fLO "https://github.com/licgate/licgate/releases/download/v${ver}/licgate_${ver}_${arch}.deb"
sudo apt install "./licgate_${ver}_${arch}.deb"
```

**その他の Linux / macOS** は、リリースの `.tar.gz` に入っている単体のバイナリを置くだけです。

```sh
tar xzf licgate_0.1.0_linux_amd64.tar.gz
sudo install licgate_0.1.0_linux_amd64/licgate /usr/local/bin/
```

**ソースから**（Go 1.24 以上）:

```sh
go install github.com/licgate/licgate/cmd/licgate@latest
```

licgate 自体は実行時の依存を持ちません。ただし `go`（Go プロジェクト）や `cargo`（Rust）、インストール済みの `node_modules` や Python の仮想環境があれば、それらを使ってより正確に判定します。

メンテナー向け: `scripts/package.sh 0.1.0` で全アーカイブと `.deb` を `dist/` に作成します。`v*` タグを push すると CI が同じ処理を行い、リリースを公開します。

## 使い方

```sh
licgate                         # 対話モード: ライセンスと提供形態を聞いてから TUI で結果を表示
licgate -l MIT                  # MIT として判定（既定はバイナリ配布）
licgate -l Apache-2.0 --dist library
licgate -l proprietary --dist saas
licgate -l "MIT OR Apache-2.0"  # デュアルライセンス: どちらの選択肢でも成立する必要あり

licgate report --format html -o licgate-report.html
licgate notice -o THIRD_PARTY_NOTICES.md
licgate sbom -o bom.cdx.json
licgate init -l MIT --dist binary   # .licgate.toml を作成
```

TUI のキー操作は次のとおりです。

| キー | 操作 |
| --- | --- |
| `↑↓` | 移動 |
| `tab` | レベルで絞り込み |
| `/` | 検索 |
| `enter` | 詳細表示（幅の狭い端末のみ） |
| `s` | ライセンスや提供形態を変えて、その場で再評価 |
| `e` | レポート、NOTICE、SBOM、設定を書き出し |
| `q` | 終了 |

### CI で使う

終了コードは、`--fail-on`（既定 `block`）以上の指摘があれば `1`、エラーなら `2` です。標準出力が端末でない場合や `CI` 環境変数がある場合、TUI は起動しません。GitHub Actions の例は英語版 README を参照してください。

## 読み取るデータ

| エコシステム | 情報源 | スコープ（配布されるもの） |
| --- | --- | --- |
| npm / yarn / pnpm | `package-lock.json`（v1〜v3）。ない場合はインストール済みの `node_modules`（pnpm の `.pnpm` を含む） | ロックファイルの `dev` フラグ、または `dependencies` からの到達性 |
| Python | 仮想環境（`.venv`、`venv`、`$VIRTUAL_ENV`）の `METADATA`（PEP 639 の `License-Expression`、classifier、ライセンスファイル）。ない場合は `uv.lock`、`poetry.lock`、バージョン固定の `requirements.txt`（ライセンスは `--online` で取得） | `pyproject.toml` / `requirements*.txt` からの到達性 |
| Go | `go list -deps ./...`（実際にリンクされるモジュール）、`go mod graph`、モジュールキャッシュ内の LICENSE。Go 標準ライブラリも列挙 | テスト専用モジュールはスキップ |
| Rust | `cargo metadata` | dev-dependencies、build-dependencies、proc-macro はスキップ |
| PHP | `composer.lock` | `packages-dev` はスキップ |
| その他 | インストール済みなら [syft](https://github.com/anchore/syft) を自動利用。または `--sbom` で CycloneDX / SPDX 2.x JSON を指定 | SBOM の情報に従う |

ライセンス表記の正規化と検出は次のように行います。

- 表記ゆれ（`"Apache 2.0"`、`"MIT/Apache-2.0"`、`GPLv3+`、PyPI の classifier、古い `licenses` 配列など）は SPDX に正規化します。
- LICENSE ファイルは本文から判別します。
- メタデータと LICENSE ファイルが食い違うパッケージは指摘します。
- 独自規約の後ろに OSS の表記を付け足しただけのファイルも見分けます。

## オプション

- `--online`: 不明なライセンスを npm、PyPI、crates.io、deps.dev で照会します。送るのはパッケージ名とバージョンだけです。
- `--jev`: 組み込みの判別で特定できないライセンス本文を [Jev](https://docs.typesafe.ai)（TypeSafe の System One モデル）で分類します。
  - Jev は確率付きで答えを返します。`--jev-threshold`（既定 0.85）未満の答えは採用せず、`REVIEW` のまま残します。
  - `TYPESAFE_API_KEY` が必要です。`--jev-provider openrouter` を指定する場合は `OPENROUTER_API_KEY` を使います。
  - 送信するのは判別できなかったパッケージのライセンス本文だけで、あなたのコードは送りません。

## 設定ファイル

`licgate init` を実行すると、コメント付きの `.licgate.toml` が作られます。手動で確認したパッケージは `[[override]]` に、社内パッケージなどは `[[ignore]]` に書けます。

## 限界

licgate に組み込んでいるのは、広く受け入れられている互換性ルールです（FSF の GPL 互換リスト、コピーレフトの範囲、リンク、ネットワーク条項）。すべてのライセンスや法的な細部までは扱いません。不明なライセンス、独自規約、曖昧なケースは推測せず `REVIEW` として表示します。出力言語は `$LANG` に従い、`--lang en|ja` で切り替えられます。
