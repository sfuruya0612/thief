# 0024. 端末に残すデータの置き場所をデータの種類ごとに決める

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-07-08

## 状況

thief はデータベースを持たない (ADR 0005) が、再起動の後も残したいデータがある。
以前は保存クエリ (スニペット) を `/tmp/thief` に置いていたため、OS を再起動すると消えた (`docs/issues/TODO.md`、issue 0143)。

## 決定

| データ | 置き場所 | 変更の方法 |
| --- | --- | --- |
| 保存クエリ (スニペット) | backend を起動したディレクトリの `.thief/snippets` | `THIEF_SNIPPETS_DIR`、設定ファイル |
| 料金のキャッシュ | `/tmp/thief/price/v2/<service>/<region>.json` (EC2 Spot は保存しない) | `THIEF_PRICE_CACHE_DIR`、設定ファイル (`price-cache-dir`) |
| Google Cloud のプロジェクト一覧 | `~/.config/thief/gcp-projects.json` | `XDG_CONFIG_HOME` (`$XDG_CONFIG_HOME/thief/`) |
| Datadog の OAuth のトークン | `~/.config/thief/datadog/` | `XDG_CONFIG_HOME` (`$XDG_CONFIG_HOME/thief/datadog/`) |
| AWS SSO のトークン | `~/.aws/sso/cache` (AWS CLI と共有) | なし |
| UI の状態 | ブラウザの `localStorage` の `cloudlens:v1` (例外の 2 つのキーは ADR 0006) | なし |

- 上の表のファイルは、UI の状態とスニペットを除き、権限 0600 で作る。
  スニペットはファイル 0644、ディレクトリ 0755 で作る (`backend/internal/snippet/snippet.go`)。
- 料金のキャッシュ、Datadog のトークン、スニペットは、一時ファイルに書いてから rename する。
  読み手が書きかけの内容を見ないようにするためである。
  スニペットの rename は、同時の保存による競合に備えて最大 16 回試す (`backend/internal/snippet/snippet.go` `renameAttemptLimit`、issue 0161)。

## 検討した代替案

issue 0143 はスニペットの置き場所について次の案を採らなかった。

- `config.Dir()` (`~/.config/thief`) の下に置く。
  TODO が「サーバが起動しているディレクトリ配下」と指定しているため。
- 起動したディレクトリの直下の `snippets/`。
  利用者の既存のディレクトリと衝突しうるため。
- 起動時に `os.MkdirAll` で作る。
  スニペットを使わない起動でも `.thief/` ができるため。
- 旧い置き場所を探すフォールバック。
  同名のスニペットの所在によって挙動が変わるため。

## 結果

- スニペットは、backend を起動したディレクトリごとに別になる。
- 料金のキャッシュは既定で `/tmp` にあり、OS を再起動すると消える。
  料金のキャッシュには期限が無く、消えたら取り直す。

## 根拠資料

- `docs/issues/closed/0143`、`0161`
- `backend/internal/snippet/snippet.go`、`backend/internal/pricecache/pricecache.go`、`backend/internal/gcp/projectstore.go`、`backend/internal/datadogauth/storage.go`
