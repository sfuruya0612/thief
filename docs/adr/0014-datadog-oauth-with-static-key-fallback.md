# 0014. Datadog は OAuth でログインし、親組織だけ静的キーに切り替える

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-09-11

## 状況

Datadog の API は、API キーと Application キーの静的な組で呼べる。
静的キーは組織ごとに発行と保管が要り、Sub Organization ごとに持つのは手間が大きい。
Datadog 公式の CLI (pup) は、OAuth による組織ごとのログインを提供している。
Sub Organization はデータが完全に分かれているため、組織ごとのログインが要る。

## 決定

- OAuth 2.0 の Authorization Code (RFC 6749)、PKCE (RFC 7636)、Dynamic Client Registration (RFC 7591) を `backend/internal/datadogauth/` に実装し、CLI と API で共有する (issue 0164、0165)。
- トークンは OS のキーチェーンではなく、`~/.config/thief/datadog/token_<site>[_<org>].json` に権限 0600 で保存する。
- API はリクエストごとにトークンのファイルを読み直し、ファイルを唯一の正とする。
  CLI でログインした結果がサーバにもそのまま反映される。
- トークンの更新の競合は、`singleflight` と、ディスクの読み直しによる楽観的な再試行で緩和する。
- 親組織 (`org == ""`) は、トークンが使えないとき (無い、ファイルが壊れている、期限切れで更新に失敗した)、静的キー (`DATADOG_API_KEY`、`DATADOG_APP_KEY`) に切り替える。
  壊れたファイルと更新の失敗では `slog.Warn` を出す (`backend/internal/api/datadog_auth_context.go`)。
  トークンでの呼び出しが 403 のときも、静的キーで 1 回だけやり直す。
  Sub Organization は切り替えない (issue 0167)。

## 検討した代替案

- issue 0167: pup を外部依存かサブプロセスとして取り込む。
  依存を最小にする方針 (ADR 0004) に反するため採らなかった。
- issue 0165: `Server` のトークンを mutex で書き換える。
  プロセス間の同期を解決しないため採らなかった。
  `flock` で排他する。
  プラットフォームごとの差が大きいため採らなかった。
  AWS SSO と同じくブロッキングの `complete` にする。
  Authorization Code では不要なため採らなかった。
  コールバックの URL を `ListenAddr` から導く。
  リバースプロキシに対応できないため採らなかった。

## 結果

- コールバックの基点 (`THIEF_DATADOG_OAUTH_REDIRECT_BASE`) を変えると、クライアントの再登録が要り、既存のトークンは実質的に使えなくなる。
- CLI のログインは `127.0.0.1:8400` を使い、ポートが使用中ならエラーにする。
- 静的キーの環境変数名は `DATADOG_*` である。
  `CHANGES.md` の一部は `DD_*` と書いており、食い違っている (`docs/prd/thief.md` の「未確定論点」)。

## 根拠資料

- `docs/issues/closed/0164`、`0165`、`0166`、`0167`
- `backend/internal/datadogauth/`、`backend/internal/api/datadog_auth_context.go`
- RFC 6749、RFC 7636、RFC 7591
