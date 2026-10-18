package policy

import "github.com/licgate/licgate/internal/i18n"

func init() {
	i18n.Add(map[string][2]string{
		"w.proprietary": {"a proprietary license", "プロプライエタリ（非公開）ライセンス"},

		"warn.outbound_unknown": {
			"The intended license %q is not one licgate knows; compatibility checks for it are limited.",
			"公開予定のライセンス %q は licgate が把握していないため、互換性チェックが限定的になります。"},

		"r.gpl_version_conflict": {
			"%s only allows GPL-2.0, but %s (and %d package(s) in total) only allow GPL-3.0 — the combined work cannot satisfy both.",
			"%s は GPL-2.0 のみを許しますが、%s など %d 件は GPL-3.0 のみを許すため、結合物は両方を満たせません。"},
		"warn.gpl_version_conflict": {
			"GPL version conflict: %d dependency(ies) are GPL-2.0-only while %d require GPL-3.0.",
			"GPL バージョンの衝突: GPL-2.0 のみの依存が %d 件、GPL-3.0 が必要な依存が %d 件あります。"},
		"r.ignored":  {"Ignored by configuration. %s", "設定で除外されています。%s"},
		"r.dev_only": {"Development-only dependency; not part of what you ship (use --include-dev if your build bundles it).", "開発用の依存で、配布物には含まれません（ビルドで同梱される場合は --include-dev を指定）。"},
		"r.no_license_info": {
			"No license information found. Code without a license is \"all rights reserved\" by default — check the project's repository.",
			"ライセンス情報が見つかりません。ライセンスのないコードは原則「無断利用不可」です。配布元リポジトリを確認してください。"},
		"r.license_unrecognised": {"License declaration could not be recognised: %s", "ライセンス表記を判別できませんでした: %s"},
		"r.license_file_unrecognised": {
			"Found %s but could not recognise the license text — read it, then add an [[override]] (or try --jev).",
			"%s は見つかりましたが、ライセンス本文を判別できませんでした。内容を確認して [[override]] を追加するか、--jev を試してください。"},
		"r.bad_expr": {"Invalid license expression: %s", "ライセンス式を解釈できません: %s"},
		"r.file_mismatch": {
			"Metadata says %s but the bundled license file looks like %s — check which applies.",
			"メタデータでは %s ですが、同梱のライセンスファイルは %s に見えます。どちらが正しいか確認してください。"},
		"r.or_choice":         {"Dual/multi-licensed: you can use it under %s.", "複数ライセンスのうち %s を選択して利用できます。"},
		"r.denied":            {"%s is on the deny list.", "%s は拒否リストに入っています。"},
		"r.allowed":           {"%s is on the allow list.", "%s は許可リストに入っています。"},
		"r.unknown_license":   {"Unknown license %q — needs a manual check.", "不明なライセンス %q です。手動で確認が必要です。"},
		"r.unknown_exception": {"Unknown license exception %q.", "不明な例外条項 %q です。"},
		"r.ambiguous":         {"Declared as \"%s\" without an exact version/variant; assumed the common case.", "「%s」とだけ表記されていて版や種類が特定できないため、一般的なケースとして扱いました。"},
		"r.font":              {"Font license: fine to bundle fonts; do not sell the fonts on their own.", "フォント用ライセンスです。同梱は可能ですが、フォント単体での販売はできません。"},

		"r.dep_proprietary_block": {
			"Declared proprietary / UNLICENSED: no permission to redistribute it.",
			"プロプライエタリ（UNLICENSED）と宣言されていて、再配布の許可がありません。"},
		"r.dep_proprietary_review": {
			"Declared proprietary / UNLICENSED: make sure you have a separate agreement (e.g. it is your own company's package).",
			"プロプライエタリ（UNLICENSED）と宣言されています。別途契約があるか（自社パッケージ等）確認してください。"},
		"r.source_available_oss": {
			"%s is source-available, not open source (use restrictions such as non-commercial). Bundling it means your release cannot be offered under an open source license without those restrictions.",
			"%s はオープンソースではないソース公開ライセンス（非商用などの利用制限あり）です。同梱すると、制限なしのオープンソースとして配布できません。"},
		"r.source_available": {
			"%s is source-available with use restrictions (non-commercial, no competing service, etc.). Check that your use is allowed.",
			"%s は利用制限（非商用、競合サービス禁止など）付きのソース公開ライセンスです。用途が許されているか確認してください。"},

		"r.internal_network": {
			"%s: internal use is fine, but if users outside your organisation interact with it over a network you must offer them the source.",
			"%s: 社内利用なら問題ありませんが、社外ユーザーがネットワーク経由で使う場合はソース提供義務が生じます。"},
		"r.saas_agpl_ok": {
			"%s in a network service: users of the service must be offered the complete source.",
			"%s をネットワークサービスで使う場合、利用者に完全なソースを提供する必要があります。"},
		"r.saas_agpl_block": {
			"%s: running it in a network service triggers the obligation to publish your service's complete source under the AGPL — incompatible with keeping it closed.",
			"%s をネットワークサービスで動かすと、サービス全体のソースを AGPL で公開する義務が生じます。非公開のままにはできません。"},
		"r.saas_agpl_review": {
			"%s: as a network service, the combined program must be available as source under the AGPL to its users. Your own code can keep its license, but the running service is effectively AGPL.",
			"%s: ネットワークサービスとして提供すると、結合したプログラム全体を AGPL で利用者に公開する必要があります。自分のコードのライセンスは維持できますが、サービスとしては実質 AGPL になります。"},
		"r.saas_gpl": {
			"%s: providing a network service is not distribution under the GPL, so no source obligation today — this changes if you ever ship binaries.",
			"%s: GPL ではネットワーク提供は「配布」に当たらないため、現時点でソース公開義務はありません。将来バイナリを配布する場合は問題になります。"},

		"r.link_exception": {
			"Copyleft with a linking exception (%s): using it from your code is fine; changes to the library itself stay under its license.",
			"リンク例外（%s）付きのコピーレフトです。自分のコードから使うのは問題ありませんが、ライブラリ自体の改変部分は元のライセンスのままです。"},
		"r.gpl_incompatible": {
			"%s cannot be combined with %s (no common GPL version / incompatible terms).",
			"%s は %s と組み合わせられません（共通の GPL バージョンがない、または条件が衝突します）。"},
		"r.gpl_becomes_v3": {
			"%s is only compatible with GPL-3.0: the combined work can only be distributed under GPL-3.0.",
			"%s は GPL-3.0 とのみ互換です。結合物は GPL-3.0 でしか配布できなくなります。"},
		"r.agpl_in_gpl3": {
			"%s is AGPL: GPL-3.0 allows combining with it, but the AGPL part keeps its network-use source requirement.",
			"%s は AGPL です。GPL-3.0 との結合は認められていますが、AGPL 部分にはネットワーク利用時のソース提供義務が残ります。"},
		"r.epl2_secondary": {
			"EPL-2.0 is only GPL-compatible if the project designated GPL as a \"Secondary License\" — check its notices before combining with %s.",
			"EPL-2.0 は、配布元が GPL を「Secondary License」として指定している場合のみ GPL と互換です。%s と組み合わせる前に告知を確認してください。"},
		"r.gpl_in_gpl": {
			"%s is compatible with your GPL license; distribute the whole work with complete source.",
			"%s はあなたの GPL ライセンスと互換です。全体を完全なソースとともに配布してください。"},
		"r.outbound_rules_missing": {
			"licgate has no built-in compatibility rules for %s with copyleft dependencies — check manually.",
			"licgate には %s とコピーレフト依存の互換ルールが組み込まれていません。手動で確認してください。"},
		"r.lib_gpl": {
			"%s: your package can be published under %s, but anyone who ships a program containing it must follow the GPL for the whole program — in practice the combination is GPL. Consider replacing it, making it optional, or choosing a GPL license.",
			"%s: あなたのパッケージ自体は %s で公開できますが、これを含むプログラムを配布する人は全体を GPL に従わせる必要があり、実質 GPL になります。代替ライブラリへの置き換え、オプション化、または GPL での公開を検討してください。"},
		"r.lib_agpl": {
			"%s: your package can be published under %s, but programs using it (including network services) must follow the AGPL. Most users will treat that as a blocker.",
			"%s: あなたのパッケージ自体は %s で公開できますが、これを使うプログラム（ネットワークサービスを含む）は AGPL に従う必要があります。多くの利用者にとって採用の障害になります。"},
		"r.lib_weak": {
			"%s is weak copyleft: fine as a dependency; changes to it must stay under its license.",
			"%s は弱いコピーレフトです。依存として使うのは問題ありませんが、それ自体を改変した場合は同じライセンスで公開が必要です。"},
		"r.lgpl_static": {
			"%s linked statically: you must let users relink with a modified version of the library (e.g. ship object files), which is hard for Go/Rust/bundled JS. Prefer dynamic linking or a replacement.",
			"%s を静的リンクしています。利用者が改変版ライブラリと再リンクできるようにする必要があり（オブジェクトファイルの提供など）、Go/Rust/バンドルされた JS では困難です。動的リンクか代替ライブラリを検討してください。"},
		"r.lgpl_dynamic": {
			"%s linked dynamically: OK, keep it replaceable and provide its source (or a written offer).",
			"%s を動的リンクしているため利用可能です。差し替え可能な形を保ち、ライブラリのソース（または提供の申し出）を添えてください。"},
		"r.file_copyleft": {
			"%s is file-level copyleft: fine to combine; publish the source of those files (including your changes to them).",
			"%s はファイル単位のコピーレフトです。組み合わせは可能ですが、そのファイル（自分の改変を含む）のソースは公開が必要です。"},
		"r.bin_gpl": {
			"%s is strong copyleft: a distributed program that includes it must be released under the GPL as a whole, which conflicts with %s.",
			"%s は強いコピーレフトです。これを含むプログラムを配布するには全体を GPL にする必要があり、%s とは両立しません。"},
		"r.bin_agpl": {
			"%s is network copyleft: distributing (or serving) a program that includes it requires releasing the whole under the AGPL, which conflicts with %s.",
			"%s はネットワークコピーレフトです。これを含むプログラムの配布やサービス提供には全体を AGPL にする必要があり、%s とは両立しません。"},

		"ob." + ObAttribution: {"Include its license text and copyright notice in your distribution (e.g. THIRD_PARTY_NOTICES).", "ライセンス全文と著作権表示を配布物に含める（THIRD_PARTY_NOTICES など）。"},
		"ob." + ObNoticeFile:  {"Pass on the contents of its NOTICE file.", "NOTICE ファイルの内容を引き継いで表示する。"},
		"ob." + ObLGPLRelink:  {"Let users replace/relink the LGPL library and provide its source (or a written offer).", "LGPL ライブラリを利用者が差し替え・再リンクできるようにし、そのソース（または提供の申し出）を添える。"},
		"ob." + ObFileSource:  {"Make the source of files under this license (and your changes to them) available.", "このライセンスのファイル（と自分の改変）のソースを入手可能にする。"},
		"ob." + ObGPLSource:   {"Distribute the complete corresponding source of the whole work under the GPL.", "作品全体の完全な対応ソースを GPL で提供する。"},
		"ob." + ObAGPLNetwork: {"Offer the complete source to users who interact with it over a network.", "ネットワーク経由の利用者にも完全なソースを提供する。"},
		"ob." + ObUseTerms:    {"Confirm your use complies with its use restrictions.", "利用制限の条件を満たしているか確認する。"},
	})
}
