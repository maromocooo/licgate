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
			"Installed in the environment but not traced from the project's declared dependencies; evaluated anyway. If it is only a tool, add an [[ignore]].",
			"環境にはインストールされていますが、プロジェクトの依存宣言からはたどれませんでした。念のため評価対象にしています。ツール類であれば [[ignore]] に追加してください。"},
		"c.readme_says": {
			"No license file is shipped, but the README says \"%s\". A README mention is not a full license grant — confirm, then add an [[override]].",
			"ライセンスファイルは同梱されていませんが、README には「%s」とあります。README の記載だけでは正式な許諾として不十分なので、確認のうえ [[override]] を追加してください。"},
		"c.file_suggests": {
			"The bundled license file looks like %s; if the declaration is just badly worded, add an [[override]].",
			"同梱のライセンスファイルは %s に見えます。宣言の書き方が不正確なだけなら [[override]] を追加してください。"},
		"c.proc_macro": {
			"Build-time only (proc-macro / build script); not linked into your binary.",
			"ビルド時のみ使用（proc-macro / ビルドスクリプト）で、バイナリにはリンクされません。"},
		"c.go_stdlib": {
			"The Go standard library and runtime are compiled into every Go binary.",
			"Go の標準ライブラリとランタイムは、すべての Go バイナリに組み込まれます。"},
	})
}

func noteGPLVersion() string { return i18n.T("c.gpl_version") }
