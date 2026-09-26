# 0013. AWS SSO のデバイス認可を internal/ssoauth に集め、ログアウトで AWS 側のセッションも失効させる

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-08-24

## 状況

AWS SSO のトークンが切れると、thief の一覧の取得が失敗する。
CLI には RFC 8628 のデバイス認可のログインがあり、RFC への準拠の修正 (issue 0129、0132、0133) を重ねていた。
Web UI からも同じログインをしたいという要望があった (issue 0147〜0149)。
また、ローカルのキャッシュを消すだけのログアウトでは、AWS 側のセッションが期限まで残ることが分かった (issue 0153)。

## 決定

- デバイス認可を `backend/internal/ssoauth/` に切り出し、CLI と API で共有する。
  `Start` が RegisterClient と StartDeviceAuthorization を、`Wait` が CreateToken のポーリングと AWS CLI 互換のキャッシュの保存 (`deps.SaveCache`) を行う (issue 0147、2026-08-24)。
  `thief sso generate-config` は保存を無効にした依存を渡すため、この経路ではキャッシュを保存しない (`backend/internal/cli/sso.go`)。
- Web UI のログインは、`POST .../sso/login/start` と `POST .../sso/login/complete` の 2 段で行う (issue 0148、0149)。
- ログアウトは、キャッシュの各アクセストークンを `sso:Logout` で失効させてから、キャッシュを消す。
  失効に失敗しても、ローカルの削除は止めず警告にとどめる (issue 0159)。
- トークンのキャッシュのファイル名と権限 (ディレクトリ 0700、ファイル 0600) は AWS CLI と互換にする。

## 検討した代替案

- issue 0147: `internal/cli` のシンボルを公開して API から使う。
  層の依存に反するため採らなかった。
  API に再実装する。
  RFC 準拠の修正を二重に保守することになるため採らなかった。
- issue 0159: 失効を `errgroup` で並列にする。
  件数が少なく、順序が非決定になるため採らなかった。
  全体に 1 つのタイムアウトを掛ける。
  原因を切り分けられないため採らなかった。

## 結果

- RFC 8628 への準拠の修正は、1 か所で CLI と Web の両方に効く。
- 旧来の `aws sso login` を呼び出す方式のエンドポイントは削除した (issue 0149)。
- 同じ start URL を共有するプロファイルは 1 つのキャッシュファイルを共有するため、1 つのプロファイルのログアウトが他のプロファイルにも効く。

## 根拠資料

- `docs/issues/closed/0129`、`0132`、`0133`、`0147`、`0148`、`0149`、`0153`、`0159`
- `backend/internal/ssoauth/ssoauth.go`、`backend/internal/api/handlers_sso.go`、`backend/internal/cli/sso.go`
- RFC 8628: https://www.rfc-editor.org/rfc/rfc8628
