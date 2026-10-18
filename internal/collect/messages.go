package collect

import "github.com/licgate/licgate/internal/i18n"

func init() {
	i18n.Add(map[string][2]string{
		"c.gpl_version": {
			"Only the GPL license text was found, which does not say whether \"or any later version\" applies; assumed \"-only\" (the stricter reading).",
			"GPL の本文しか見つからず「またはそれ以降のバージョン」が適用されるか不明なため、厳しい解釈（-only）として扱いました。"},
		"c.low_confidence": {
			"%s might be \"%s\", but the match is weak (e.g. custom terms with bundled third-party notices) — read it.",
			"%s は「%s」の可能性がありますが確度が低いです（独自規約の後に第三者ライセンスが同梱されている等）。内容を確認してください。"},
		"c.unreachable": {
			"Installed in the environment but not reachable from the project's declared dependencies (likely a tool).",
			"環境にはインストールされていますが、プロジェクトの依存宣言からはたどれません（ツール類の可能性）。"},
		"c.proc_macro": {
			"Build-time only (proc-macro / build script); not linked into your binary.",
			"ビルド時のみ使用（proc-macro / ビルドスクリプト）で、バイナリにはリンクされません。"},
		"c.go_stdlib": {
			"The Go standard library and runtime are compiled into every Go binary.",
			"Go の標準ライブラリとランタイムは、すべての Go バイナリに組み込まれます。"},
	})
}

func noteGPLVersion() string { return i18n.T("c.gpl_version") }
