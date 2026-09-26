# 4xx / 5xx 応答のエラー内容をコンソールのログに出力する

Created: 2026-08-24
Model: Claude Fable 5
Completed: 2026-08-25

## 背景

docs/issues/TODO.md の次の項目に由来する。

> backend 起動時の標準出力がアクセスログだけなので、標準エラー出力など 4xx, 5xx 系のエラーが発生した場合にコンソール出力するようにして欲しい

現状のコンソール出力は次のとおり。

- リポジトリに `slog.New` / `slog.SetDefault` の呼び出しは無く、`log/slog` は既定ハンドラ (テキスト形式、標準エラー出力、Info レベル) のまま使われている。TODO は「標準出力」と書いているが、正確には既定の出力先は標準エラー出力である。「コンソールに出るのがアクセスログだけ」という趣旨は正しい。
- アクセスログは `loggingMiddleware` (backend/internal/api/middleware.go:10-22) が全リクエストについて `slog.Info("http", "method", ..., "path", ..., "status", ..., "duration_ms", ...)` を 1 行出す。4xx / 5xx でも status の数値が出るだけで、レベルは Info のままである。
- エラー応答の中身 (エラーコードとメッセージ) は `writeError` (backend/internal/api/errors.go:15-22) が JSON でクライアントへ返すのみで、ログには出ない。`writeBadRequest` / `writeAWSError` / `writePricingError` / `writeGCPError` / `writeSnippetError` などの JSON エラー応答はすべて `writeError` に集約されている。
- `http.ServeMux` 自体が返す 404 (未登録パス) と 405 は `writeError` を通らないが、`loggingMiddleware` は通る。
- WebSocket 系 (backend/internal/api/logtail.go、handlers_session.go) は接続確立後の失敗を既に個別の `slog.Warn` / `slog.Error` で出している。

このため、frontend でエラーが起きたとき、backend のコンソールには `status=400` のような数値しか出ず、原因 (どのエラーコードでどんなメッセージだったか) はブラウザの開発者ツールか curl でレスポンスボディを見ないと分からない。

## 目的

4xx / 5xx の応答が発生したとき、そのエラーコードとメッセージを backend のコンソール (標準エラー出力) のログに出力し、クライアント側を確認しなくても原因を特定できる状態にする。

## 設計判断

`loggingMiddleware` の `responseWriter` (backend/internal/api/middleware.go:42-57) を拡張し、ステータスが 400 以上の場合にレスポンスボディを上限付きで捕捉して、リクエスト完了時のログを 4xx は `Warn`、5xx は `Error` で出す。ログの属性は既存の `method` / `path` / `status` / `duration_ms` に、捕捉したボディ (エラーコードとメッセージを含む JSON。上限で切り詰めたもの) を加える。4xx / 5xx では Info のアクセスログは出さず、Warn / Error の 1 行に統合する。

この方式を採る理由は次のとおり。

- `writeError` を通らない経路 (`http.ServeMux` 自体の 404 / 405) も捕捉できる。ログの出所がミドルウェア 1 箇所に集約され、ハンドラ側の変更が不要である。
- `responseWriter` は既に `WriteHeader` をフックして status を記録しており (middleware.go:47-50)、拡張が既存の構造に沿う。

実装上の境界条件。

- ボディの捕捉には上限を定数で設け、超過分は切り詰める。上限は数 KB (例: 2048 バイト。`writeError` の JSON はエラーコードとメッセージのみで通常は数百バイトに収まる) とし、値と根拠を定数のコメントに書く。バッファはリクエストごとに最大で上限分であり、401 が並行して多数発生する状況でもメモリ使用量は上限 × 同時エラー応答数に抑えられる。捕捉するのはクライアントへ返した内容の写しであり、応答に含めていない情報はログに載らない。
- 捕捉は複数回の `Write` 呼び出しをまたいで累積し、上限に達した以降は捕捉だけを打ち切る。クライアントへの書き込みは捕捉の有無や上限到達に関わらず全量そのまま透過する。
- 現状の 4xx / 5xx 応答はすべて JSON (`writeError` 経由) または text/plain (`http.Error` 経由) である。バイナリの 4xx / 5xx 応答は現状存在しないため Content-Type による捕捉の切り替えは行わず、将来発生した場合も上限までのバイト列がそのままログ属性の値になる (slog のテキストハンドラが属性値をエスケープして出力する)。
- `responseWriter` は `http.Flusher` を実装せず、`Unwrap` (middleware.go:55) 経由で到達させる現行構造を保つ。捕捉は `Write` に渡されたバイト列の写しであり、Flush の動作と干渉しない。
- WebSocket のアップグレードが確立した後の失敗はハイジャック済みで `WriteHeader` を通らないため捕捉対象にならず、既存の個別ログ (logtail.go、handlers_session.go) のまま変わらない。`responseWriter.Unwrap` (middleware.go:55) によるハイジャック互換も維持する。
- WebSocket のアップグレードが確立する前の失敗は捕捉対象になる。`websocket.Accept` はハイジャック前に失敗すると `http.Error` で 4xx / 5xx を返すため `WriteHeader` を通り、この経路では既存の個別ログ (logtail.go:40、handlers_session.go:83 の `slog.Warn`) とミドルウェアのログの両方が出る。個別ログは失敗の文脈 (どの接続の何の失敗か) を、ミドルウェアのログは HTTP 応答の事実を持つ別種のログであり、共存を許容する。本 issue でこれらの個別ログは変更しない。
- `WriteHeader` が呼ばれないままボディが書かれた応答は 200 として扱われる (現状の `responseWriter` と同じ) ため、捕捉対象にならない。
- 401 SSO_TOKEN_EXPIRED は SSO 期限切れ時に全 AWS リソースハンドラから定常的に発生するため、ログの行数は増えるが、これは「エラーの発生をコンソールで把握したい」という要望そのものであり抑制しない。

採らなかった案とその却下理由。

- `writeError` に slog 出力を足す案。`writeError` は `*http.Request` を受け取っておらず、`method` / `path` を出すには `writeBadRequest` 以下すべてのヘルパーと呼び出し箇所のシグネチャ変更が要る。また `http.ServeMux` 自体の 404 / 405 を捕捉できない。
- `loggingMiddleware` のログレベルだけを status で変える案 (ボディ捕捉なし)。status の数値以上の情報が増えず、原因をコンソールで把握したいという要望を満たさない。
- `slog.NewJSONHandler` の導入やログレベルの外部設定化などロギング基盤の整備。AGENTS.md は JSON ハンドラを推奨しているが、出力形式や設定の変更は本 TODO の要望 (エラーの可視化) を超える。必要になれば別 issue として起票する。

追加の API 呼び出しや権限は不要である。変更は `backend/internal/api/middleware.go` とそのテストに閉じる。

## 完了条件

- 未登録パスへのリクエスト (例: `GET /api/nonexistent`) で、`status=404` を含む Warn レベルのログが標準エラー出力に 1 行出る。
- 400 応答 (例: 名前が空のスニペット保存の `BAD_REQUEST`。スラッシュを含む名前の 400 は docs/issues/0150 の実装後に発生しなくなるため例に使わない) で、応答ボディのエラーコードとメッセージを含む Warn レベルのログが出る。
- 500 応答で、応答ボディのエラーコードとメッセージを含む Error レベルのログが出る。
- 401 SSO_TOKEN_EXPIRED の応答でも Warn レベルのログが出る (抑制や集約が入っていない)。
- 2xx / 3xx の応答では従来どおり Info のアクセスログが 1 行出て、属性の構成が変わらない。
- 4xx / 5xx の応答で、同一リクエストに対する `loggingMiddleware` 由来のログが 2 行出ない (Info と Warn / Error の重複が無い)。ハンドラや WebSocket Accept 失敗時の個別の slog 出力との共存は禁止しない (設計判断のとおり)。
- ボディの捕捉に上限があり、上限を超える応答ボディは切り詰めてログに出す。
- ログに出るボディが、クライアントへ返した応答ボディの先頭から上限までの写しと一致することをテストで検証する (応答に含めていない情報がログに載らないことはこの一致検証で担保する)。
- `middleware_test.go` に `net/http/httptest` によるテストが追加され、次の分岐を検証する: 404 (未登録パス)、4xx ボディ付き (`writeError` 経由の JSON)、`http.Error` 経由の 4xx (text/plain ボディ。WebSocket Accept のハイジャック前失敗と同じ経路)、5xx、2xx、`WriteHeader` を呼ばずにボディを書く 200 応答が捕捉対象にならないこと、上限超過の切り詰め、複数回の `Write` に分けて書かれたボディの累積捕捉。
- 扱わない範囲: slog ハンドラの JSON 化とログレベルの外部設定化、WebSocket 系ハンドラの既存ログの変更、frontend の変更。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0037-bug-gcp-error-mapping-500.md、docs/issues/closed/0105-bug-cost-handlers-sso-expired-mapping.md: エラー応答のステータスとコードのマッピングを整備してきた issue 群。本 issue はマッピングの結果をコンソールへ可視化する側を扱い、マッピング自体は変更しない。
- docs/issues/0150-bug-snippet-name-with-slash-rejected.md: 同時に起票した issue。実装順序の依存は無いが、0150 の実装後はスラッシュを含む名前のスニペット保存が 400 を返さなくなるため、本 issue の 400 の検証例には 0150 実装後も 400 のままの入力 (名前が空、など) を使う。

## 解決方法

設計判断のとおり、変更を backend/internal/api/middleware.go とそのテストに閉じて実装した。ハンドラ層と WebSocket 系の個別ログは変更していない。

- `maxErrorBodyLogBytes` 定数 (2048 バイト) を新設し、値と根拠 (writeError の JSON は通常数百バイトに収まる、リクエストごとのバッファ上限を兼ねる) をコメントに書いた。
- `responseWriter` に `body` フィールドと `Write` メソッドを追加した。`Write` は status が 400 以上の場合だけ、呼び出しをまたいで残り容量を再計算しながら上限までボディの写しを蓄積し、クライアントへの書き込みは捕捉の有無に関わらず全量透過する。`WriteHeader` を経ない応答は status が既定の 200 のままなので捕捉対象にならない。`Unwrap` は変更せず、websocket.Accept のハイジャック互換を維持した (interface 埋め込みのため `io.ReaderFrom` の昇格は変更前から発生しておらず、`Write` の明示追加による最適化喪失の回帰も無い)。
- `loggingMiddleware` のリクエスト完了時のログを status で分岐させ、5xx は Error、4xx は Warn で既存 4 属性 (`method` / `path` / `status` / `duration_ms`) に `body` 属性を加えた 1 行を出し、2xx / 3xx は従来どおり同じ属性構成の Info を出す。4xx / 5xx で Info と Warn / Error が重複しないことはテストで固定した。

完了条件の検証結果:

- middleware_test.go に `recordingHandler` / `captureLogs` (slog の既定ロガーを記録ハンドラへ差し替えるヘルパー。t.Parallel と併用しない旨をコメントに明記) を新設し、`TestLoggingMiddlewareLevels` で 404 (未登録パス) / 405 (メソッド不一致。http.ServeMux 自体の応答) / writeError 経由の JSON 400 / http.Error 経由の text/plain 400 / 401 SSO_TOKEN_EXPIRED / 500 / 2xx / 3xx / WriteHeader を呼ばない暗黙 200 の各分岐を、`TestLoggingMiddlewareBodyCapture` で上限超過の切り詰め / 上限ちょうど (切り詰めなし) / 上限を跨ぐ複数 Write の累積 / 2xx でバッファされないこと (responseWriter 単体) / 複数 Write の累積を検証した。ログのボディが応答ボディの先頭からの写しと一致することも各エラーケースで検証した。
- 完了条件の「標準エラー出力に 1 行出る」は、記録ハンドラで Warn / Error レコードのレベル・属性・行数 (1 行) を検証する形で確認した。出力先そのもの (標準エラー出力) は slog の既定ハンドラの挙動であり、リポジトリに slog.SetDefault が無いことを確認済み。
- `mise run check` 通過 (frontend 73 ファイル 742 テスト、backend `go test -race` 全パッケージ ok、govulncheck 0 件)。

多観点レビュー: 5 観点 × 1 ラウンドで、実装が変わる指摘はゼロ (反映はテスト追加とドラフト修正のみ) のため追加ラウンドは実施していない。反映 8 件 (3xx の Info ケース、複数 Write 累積の応答一致検証、上限を跨ぐ累積、2xx 非バッファの単体検証、上限ちょうどの境界、405 ケース、CHANGES.md の種別を ADD から UPDATE へ変更、CHANGES.md の文言を「エラーコードとメッセージの JSON」から「応答ボディの写し (JSON または text/plain)」へ修正)。記録のみ 1 件 (標準エラー出力そのものではなくレコードレベルで検証するテスト戦略の妥当性)。却下 0 件。観点 3 (堅牢性) は指摘なし (ログインジェクションは slog TextHandler のエスケープで不可、エラー応答経由でシークレット値がログへ漏れる経路が現存しないことを横断確認)。
