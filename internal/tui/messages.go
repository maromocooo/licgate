package tui

import "github.com/maromocooo/licgate/internal/i18n"

func init() {
	i18n.Add(map[string][2]string{
		"w.setup":     {"setup", "セットアップ"},
		"w.q_license": {"Which license will you publish under?", "どのライセンスで公開しますか？"},
		"w.q_dist":    {"How will people get it?", "どのような形で提供しますか？"},
		"w.q_link":    {"How are dependencies linked?", "依存ライブラリのリンク方式は？"},
		"w.q_dev":     {"Does your build bundle dev/test dependencies?", "開発・テスト用の依存もビルド成果物に同梱されますか？"},

		"w.mit":               {"Permissive; just keep the copyright notice. The most common choice.", "寛容型。著作権表示を残せば自由に使える、最も一般的な選択肢。"},
		"w.apache":            {"Permissive with an explicit patent grant; needs NOTICE handling.", "寛容型＋明示的な特許許諾。NOTICE ファイルの扱いが必要。"},
		"w.dual":              {"Rust-style dual license; users pick either.", "Rust で一般的なデュアルライセンス。利用者がどちらかを選べる。"},
		"w.bsd":               {"Permissive, with a no-endorsement clause.", "寛容型。名前を宣伝に使わせない条項付き。"},
		"w.mpl":               {"File-level copyleft: changes to your files stay open, the rest is free.", "ファイル単位のコピーレフト。あなたのファイルの改変は公開が必要、組み合わせは自由。"},
		"w.lgpl":              {"Library copyleft: apps may link to it, changes to it stay open.", "ライブラリ向けコピーレフト。リンクは自由、ライブラリ自体の改変は公開が必要。"},
		"w.gpl":               {"Strong copyleft: anything distributed with it must be GPL too.", "強いコピーレフト。一緒に配布するものも GPL にする必要がある。"},
		"w.agpl":              {"GPL plus network use: running it as a service also requires sharing source.", "GPL＋ネットワーク条項。サービスとして提供する場合もソース公開が必要。"},
		"w.proprietary_label": {"proprietary (closed source)", "proprietary（非公開・商用）"},
		"w.proprietary_desc":  {"You won't publish the source.", "ソースコードを公開しない。"},
		"w.other":             {"Other…", "その他…"},
		"w.other_desc":        {"Type an SPDX expression.", "SPDX 式を入力する。"},
		"w.unknown_license":   {"Note: %q is not a license licgate knows; checks will be limited.", "注意: %q は licgate が把握していないライセンスのため、チェックが限定的になります。"},

		"w.d_binary":   {"You hand out an artifact containing the dependencies: app, CLI binary, container image, bundled front-end.", "依存を含む成果物を配布する（アプリ、CLI バイナリ、コンテナイメージ、バンドル済みフロントエンド）。"},
		"w.d_library":  {"You publish source to a registry (npm, PyPI, crates.io…); users install dependencies themselves.", "ソースをレジストリ（npm、PyPI、crates.io など）に公開し、依存は利用者がインストールする。"},
		"w.d_saas":     {"You run it as a network service; nothing is handed out.", "ネットワークサービスとして運用し、何も配布しない。"},
		"w.d_internal": {"Used only inside your organisation.", "組織内でのみ使用する。"},

		"w.l_auto":    {"Go/Rust/bundled JS: static. Python/Java/Ruby/PHP: dynamic.", "Go/Rust/バンドルされた JS は静的、Python/Java/Ruby/PHP は動的として扱う。"},
		"w.l_static":  {"Everything is compiled/bundled into one artifact.", "すべてを 1 つの成果物にコンパイル・バンドルする。"},
		"w.l_dynamic": {"Dependencies stay separate, replaceable files.", "依存は差し替え可能な別ファイルのまま。"},

		"w.dev_no":       {"No", "いいえ"},
		"w.dev_no_desc":  {"devDependencies / test tools are not shipped (skip them).", "devDependencies やテストツールは配布物に含まれない（スキップする）。"},
		"w.dev_yes":      {"Yes", "はい"},
		"w.dev_yes_desc": {"e.g. a front-end where the bundler pulls in devDependencies.", "例: バンドラーが devDependencies も取り込むフロントエンド。"},

		"w.k_choose":  {"choose", "選択"},
		"w.k_confirm": {"confirm", "決定"},
		"w.k_back":    {"back", "戻る"},
		"w.k_quit":    {"quit", "終了"},

		"t.all":               {"ALL", "すべて"},
		"t.search_ph":         {"package or license", "パッケージ名・ライセンス"},
		"t.empty":             {"Nothing here.", "該当なし"},
		"t.k_move":            {"move", "移動"},
		"t.k_filter":          {"filter", "絞り込み"},
		"t.k_search":          {"search", "検索"},
		"t.k_detail":          {"details", "詳細"},
		"t.k_scroll":          {"scroll details", "詳細をスクロール"},
		"t.k_settings":        {"change license/distribution", "ライセンス・配布形態を変更"},
		"t.k_export":          {"export", "書き出し"},
		"t.export_title":      {"Export", "書き出し"},
		"t.export_dir":        {"into %s", "保存先: %s"},
		"t.ex_html":           {"HTML report", "HTML レポート"},
		"t.ex_md":             {"Markdown report", "Markdown レポート"},
		"t.ex_json":           {"JSON report", "JSON レポート"},
		"t.ex_notice":         {"Third-party notices", "サードパーティ表記"},
		"t.ex_sbom":           {"CycloneDX SBOM", "CycloneDX SBOM"},
		"t.ex_config":         {"Save settings", "設定を保存"},
		"t.wrote":             {"Wrote %s", "%s を書き出しました"},
		"t.reevaluated":       {"Re-evaluated for %s / %s", "%s / %s で再評価しました"},
		"t.config_has_custom": {"%s has overrides/ignores/allow/deny entries; edit it by hand instead", "%s には override/ignore/allow/deny の設定があるため、手動で編集してください"},
		"t.levels_help": {
			"BLOCK: known incompatibility · REVIEW: check by hand · NOTICE: fine, with obligations · OK: nothing to do · SKIP: not shipped",
			"BLOCK: 既知の非互換 · REVIEW: 要手動確認 · NOTICE: 義務付きで問題なし · OK: 対応不要 · SKIP: 配布物に含まれない"},
	})
}
