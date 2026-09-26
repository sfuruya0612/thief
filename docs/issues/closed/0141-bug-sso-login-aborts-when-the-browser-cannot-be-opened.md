# ブラウザを開けないと sso login がその場で中断し、テキスト URI による承認経路が使えない

Created: 2026-08-09
Model: Claude Opus 5
Completed: 2026-08-11

## 概要

`thief sso login` は、ブラウザの自動起動に失敗するとその時点でエラーを返して終了する。RFC 8628 §3.3.1 はブラウザ等による非テキストでの提示を MAY と定めており、必須ではない。必須なのは §3.3 のテキストでの `verification_uri` と `user_code` の提示である。現在の実装は表示に到達する前に中断するため、利用者が別の端末で承認する経路も、ヘッドレス環境での利用も塞がれている。

## 根拠 (RFC 8628 原文)

§3.3.1 (Non-textual Verification URI)。

> clients MAY present this URI in a non-textual manner

> it is RECOMMENDED for clients to still display the textual verification URI.

§3.3 (User Interaction)。

> the client displays or otherwise communicates the "user_code" and the "verification_uri" to the end user.

ブラウザで開くことは MAY であり、提示は RECOMMENDED / 必須の関係になっている。ブラウザが開けなかったことは、フロー全体を中断する理由にならない。

## 該当箇所

`backend/internal/cli/sso.go` の `getSSOTokenWith`。

```go
	if err := deps.openBrowser(deviceAuth.VerificationURIComplete); err != nil {
		return nil, fmt.Errorf("open browser: %w", err)
	}

	// 提示する URI はサーバが返した verification_uri である (RFC 8628 §3.2 / §3.3)。
	deps.display(deviceAuth.VerificationURI, deviceAuth.UserCode)
```

`openBrowser` が失敗すると `display` に到達しない。`openBrowser` は OS ごとに `open` / `xdg-open` / `cmd /c start` を実行するだけであり、以下の場合に失敗する。

- ヘッドレス環境で `xdg-open` が入っていない、または開く先が無い。
- `VerificationURIComplete` が空文字列である。RFC 8628 §3.2 の `verification_uri_complete` は OPTIONAL であり、サーバが返さないことは仕様違反ではない。

後者は 2 つの OPTIONAL / REQUIRED の扱いが逆転している状態である。OPTIONAL な `verification_uri_complete` の欠落でフローが止まり、REQUIRED な `verification_uri` の提示に到達しない。

## 再現手順

1. `xdg-open` が存在しない Linux 環境 (コンテナ等) を用意する。
2. `thief sso login --url <portal>` を実行する。

期待: 承認用の URI と user code が表示され、承認を待つ。

実際: `open browser: exec: "xdg-open": executable file not found in $PATH` を表示して終了コード 1 で終わる。デバイス認可自体は成功しており、承認すれば通るはずの device code が捨てられる。

`VerificationURIComplete` が空の場合は、`open ""` / `xdg-open ""` が失敗する環境で同じ結果になる。

## なぜ対応が必要か

- デバイス認可フローは、そもそも「ブラウザを開けない環境」のために存在する仕組みである。ブラウザが開けないことを致命的な失敗として扱うのは、この仕組みを使う意味を打ち消している。
- 直前で `StartDeviceAuthorization` が成功しており、device code は有効期限まで使える。中断すると次回の実行で再取得することになり、サーバへの無駄な往復が増える。
- issue 0132 で `verification_uri` をサーバの値に直したが、ブラウザが開けない場合はその表示に到達しない。0132 の修正が最も効くはずの経路が塞がったままである。

## 修正方針

`openBrowser` の失敗を警告に落とし、`display` とポーリングへ進む。

- 表示の順序を「テキストの提示 → ブラウザの起動」に変える。`writeSSOLoginPrompt` の 1 行目は "Attempting to automatically open the SSO authorization page in your default browser." であり、起動を試みる前に出るのが文言と合う。ブラウザの起動が失敗した場合も、利用者は既に URI と user code を見ている状態になる。
- 警告の出力先を決める。`getSSOTokenWith` は cobra のコマンドを受け取らないため `cmd.ErrOrStderr()` が使えない。`ssoTokenDeps` に警告用の関数を足すか、`display` の役割を広げるかを決める必要がある。
- `VerificationURIComplete` が空の場合はブラウザの起動を試みない。空文字列を `open` に渡す意味が無く、失敗の文言も分かりにくい。

## 完了条件

- `openBrowser` が失敗しても `display` が呼ばれ、`waitForToken` へ進むことをテストで検証する。
- `openBrowser` の失敗が利用者へ伝わることをテストで検証する。
- `VerificationURIComplete` が空の場合にブラウザの起動を試みないことをテストで検証する。
- `mise run check` が通る。

## 関連

- docs/issues/0132: 起票元。`verification_uri` をサーバの値に直した際に、その表示へ到達しない経路があることに気付いた。
- docs/issues/0129: RFC 8628 §3.2 / §3.5 のポーリングの準拠。

## 解決方法

`backend/internal/cli/sso.go` の `getSSOTokenWith` で、表示の順序を「テキストの提示 (`display`) → ブラウザの起動 (`openBrowser`)」に入れ替え、`openBrowser` の失敗をエラーとして返さず警告として報告するだけに変えた。`VerificationURIComplete` が空の場合はブラウザの起動を試みない。

### 実装

- 修正方針で決める必要があった「警告の出力先」は、`ssoTokenDeps` に `reportBrowserFailure func(err error)` を新設する形で決めた。既存の `display` の役割を広げず、既存フィールドと同じ「外部処理 1 つに関数値 1 つ」のパターンを保つ方を選んだ。`display` は成功時の提示、`reportBrowserFailure` は失敗時の警告という役割の違いがあり、広げると 1 つの関数が 2 つの責務を持つことになるため。
- `getSSOTokenWith` は `deviceAuth.VerificationURIComplete != ""` を `attemptingBrowser` として 1 度だけ計算し、`display` への引数とブラウザ起動を試みるかどうかの分岐の両方で使う。
- `writeSSOLoginPrompt` に `attemptingBrowser bool` を追加し、この後ブラウザの起動を試みない場合は "Attempting to automatically open..." の行を出さないようにした (観点3 レビューで判明。「対応方針」節にはこの詳細は無く、実装中に見つけた整合性の問題)。
- `writeSSOBrowserFailureWarning` を新設し、`openBrowser` の失敗を標準エラー出力へ書く。別の端末から `display` で既に提示済みの URI を使って承認できることを伝える。

### 多観点レビューの結果

5 観点のレビューを 1 ラウンド実施した。

- 観点1 (完了条件充足): 指摘なし。完了条件 4 行、修正方針の 3 点いずれも満たされていることを確認した。
- 観点2 (テスト品質)
  - [優先度: 高、反映] `openBrowser` が成功したときに `reportBrowserFailure` が呼ばれないことを検証するテストが無かった。`TestGetSSOTokenPassesDeviceAuthorizationThrough` に検証を追加した。
  - [優先度: 高、反映] `display` と `openBrowser` の呼び出し順序 (テキスト提示が先) を検証するテストが無かった。`TestGetSSOTokenContinuesWhenBrowserFailsToOpen` に呼び出し順の記録と検証を追加した。1 回目の修正では `openBrowser` 自体を呼び出し順の記録対象に含め忘れており、ミューテーションテスト (順序を入れ替えても検出できるかの確認) で検出漏れに気付いて修正した。
  - [優先度: 中、反映] `reportBrowserFailure` へ渡るエラーの文言接頭辞 (`open browser: `) が未検証だった。同テストに追加した。
  - [優先度: 中、反映] `TestWriteSSOBrowserFailureWarning` が「別の端末からでも承認できる」という案内文言を検証していなかった。追加した。
  - [優先度: 低、反映] `TestGetSSOTokenContinuesWhenBrowserFailsToOpen` がキャッシュの内容 (AccessToken) を検証していなかった。追加した。
- 観点3 (堅牢性)
  - [優先度: 中、反映] `writeSSOLoginPrompt` が `VerificationURIComplete` が空でブラウザの起動を試みない場合でも "Attempting to automatically open..." を無条件に出していた。実際の動作と矛盾するため、`attemptingBrowser` 引数を追加して条件分岐にした。
  - [優先度: 低、対応不要] `VerificationURIComplete` が空白文字のみの場合はトリムしていない。RFC 8628 はこの値の空白除去の扱いを定めておらず、空白のみの値自体がサーバ側の仕様違反である。issue が求める「意味の無い URL を `open` に渡さない」は空文字列チェックで既に満たされており、トリム対応は完了条件を超える。
  - [優先度: 低、対応不要] `display` / `reportBrowserFailure` の呼び出し前に nil チェックを入れていない。既存の `display` フィールドも同じパターンであり、`TestDefaultSSOTokenDepsIsFullyWired` で配線漏れは検知できる。新設フィールドだけに nil チェックを入れると既存パターンと不整合になり、この issue の範囲を超える。
- 観点4 (規約準拠): 指摘なし。
- 観点5 (回帰と整合): 指摘なし。`getSSOToken` の公開シグネチャは変わっておらず、呼び出し元 (`ssoLoginDeps.getToken`、`ssoGenerateConfigDeps.getToken`) に追随は不要。既存テーブル駆動テストからの `openBrowser` ケース削除は新設テストへ検証意図が移っている。docs/issues/0132、docs/issues/0129 との矛盾も無い。

反映後、`mise run check` が通過することを確認した (backend 全パッケージ成功、frontend 732 件のテストすべて成功、脆弱性 0 件)。
