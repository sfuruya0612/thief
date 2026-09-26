# Datadog 組織一覧のログイン状態表示に静的キーの有無も反映する

Created: 2026-09-14
Completed: 2026-09-14
Model: Claude Sonnet 5

## 背景

ユーザーから次の要望があった。

> Datadog のタグに表示するのは Id ではなくて、attributes.name にして、null の場合は id にして
> また、OAuth だけでなく、APP_KEY, API_KEY がある場合はそっちもみるようにして

要望は 2 点で構成される。

1. Sub Organization タブのラベル表示を `attributes.name` にし、null の場合は id にフォールバックする。
2. ログイン状態の判定を OAuth トークンだけでなく、静的キー (`DATADOG_API_KEY`/`DATADOG_APP_KEY`) の有無でも行う。

要望 1 は調査の結果、`backend/internal/datadog/organizations.go` の `ListOrgs` (issue 0171 で実装済み) がすでに `attributes.name` を id 名の名前解決マップとして読み取り、一致が無い場合は id にフォールバックしたうえで `OrgInfo.Name` に詰めている。`frontend/src/components/session/DatadogOrgSessionTabs.tsx` の `label: names.get(id) ?? id` もこの `Name` をそのままラベルに使っている。よって要望 1 に対する実装済みの動作は要望と一致しており、追加の実装は不要である。

要望 2 について、`handlers_datadog_orgs.go` の `datadogOrgsWithLoginState` は issue 0171 の時点では親組織自身 (`org.IsSelf`) だけを静的キーの有無で判定しており、Sub Organization のエントリは OAuth トークンの有無だけで判定していた。ユーザーに AskUserQuestion で確認したところ、静的キーの判定は「ログイン状態表示」だけを全 org 対象に広げる (「タグ」は Sub Org タブのラベルを指す) 。実際のデータ取得 (`datadogAuthContext`/`datadogFallbackContext`) の静的キーフォールバックは issue 0171 の設計 (親組織のみ、Sub Organization は対象外) のまま変更しないことが確認された。これは、静的キーが org 非依存のグローバル環境変数であり、Sub Organization のデータ取得にそのまま使うと要求した Sub Organization ではなく静的キーが属する組織のデータを黙って返しかねないためである (issue 0171 参照)。

## 修正方針

`handlers_datadog_orgs.go` の `datadogOrgsWithLoginState` から `org.IsSelf` による静的キー判定の制限を外し、`hasStaticKeys` の算出をループの外へ出して全エントリに適用する。

```go
loggedIn := ok || hasStaticKeys
```

この結果、静的キーを設定していても OAuth 未ログインの Sub Organization タブは、一覧では「ログイン済み」と表示されつつ、実際にタブを開くとデータ取得はエラーになりうる。この非対称性は意図した仕様であることをコード上のコメントに明記する。

要望 1 (タブラベルの `attributes.name` 表示) は前述のとおりすでに実装済みのため、本 issue のスコープからは除く (コードの変更を行わない)。

## 完了条件

- `datadogOrgsWithLoginState` が、静的キー (`DATADOG_API_KEY` と `DATADOG_APP_KEY` の両方) が設定されている場合、Sub Organization のエントリも `LoggedIn: true` を返すこと。
- 静的キーが設定されておらず、かつ対象 Sub Organization に OAuth トークンが無い場合は、引き続き `LoggedIn: false` を返すこと (回帰テストで確認する)。
- Sub Organization の実データ取得 (`datadogAuthContext`/`datadogFallbackContext`) の静的キーフォールバック挙動は変更しないこと。
- `mise run check` が新たな失敗なく通過すること。

## 解決方法

`backend/internal/api/handlers_datadog_orgs.go` の `datadogOrgsWithLoginState` を修正した。`hasStaticKeys := s.cfg.DatadogAPIKey() != "" && s.cfg.DatadogAppKey() != ""` をループの外で算出し、`loggedIn := ok || hasStaticKeys` として `org.IsSelf` の制限を外し、全エントリの判定に静的キーの有無を反映させた。あわせて `datadogOrgResponse` 構造体と `datadogOrgsWithLoginState` の doc コメントを更新し、静的キーによる判定が「一覧の表示」だけの話であり、Sub Organization の実データ取得は親組織のときしか静的キーへフォールバックしないという非対称性が意図した仕様であることを明記した。

要望 1 (タブラベルの `attributes.name` 表示、null 時は id にフォールバック) は、issue 0171 で実装済みの `ListOrgs`/`DatadogOrgSessionTabs.tsx` がすでにこの要望どおりに動作することを確認済みのため、コードの変更は行わなかった。

`backend/internal/api/handlers_datadog_orgs_test.go` の `TestDatadogOrgsSelfLoggedInViaStaticKey` を `TestDatadogOrgsLoggedInViaStaticKey` に改名し、Sub Organization のエントリも `LoggedIn: true` になることを検証する内容に更新した。あわせて、静的キーが無い状態で Sub Organization が OAuth トークン無しのままなら `LoggedIn: false` のままであることを確認する `TestDatadogOrgsSubOrgNotLoggedInWithoutStaticKeys` を新設した。

完了条件の検証は `mise run check` で行った (`gofmt -l .`・`goimports -w .`・`npm run fmt`・`go vet ./...`・`staticcheck ./...`・`govulncheck ./...`・`npm run lint`・`go test -race -cover ./...`・`npm run test -- --run`) 。
