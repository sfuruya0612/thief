# Datadog の親組織タブが認証済みでも常に未ログイン表示になる

Created: 2026-09-12
Model: Claude Sonnet 5
Completed: 2026-09-14

## 症状

issue 0168 で追加した Sub Organization (以下 Sub Org) タブのうち、親組織自身に対応するタブが、実際には親組織として OAuth ログイン済み (または `DATADOG_API_KEY`/`DATADOG_APP_KEY` の静的キー設定済み) であっても、常に「未ログイン」バッジ付きで表示される。このタブをクリックすると、既にある親組織の認証情報を使わず、その Sub Org 向けの新しい OAuth ログイン (DCR によるクライアント登録込み) が開始されてしまう。

## 再現手順

1. `thief datadog auth login` (または `DATADOG_API_KEY`/`DATADOG_APP_KEY` の環境変数設定) で親組織として認証済みの状態にする。
2. 親組織の Datadog アカウントの `GET /api/v1/org` (`ListOrgs`) のレスポンスに、親組織自身のエントリ (自身の `public_id` を持つ組織) が含まれる。
3. frontend の Datadog ビューで Sub Org タブ一覧を開くと、この親組織自身のエントリに対応するタブが未ログインバッジ付きで表示される。
4. そのタブをクリックすると、`org=<親組織の public_id>` を付けた新しい OAuth ログインフローが開始される。

## 原因

`backend/internal/datadogauth/storage.go` の設計 (issue 0167) では、親組織のトークン・クライアント登録は `org == ""` のファイル (`token_<site>.json`/`client_<site>.json`) に保存される。空文字列は「Sub Org を指定しない = 親組織」を表す予約値であり、親組織自身の `public_id` そのものとは対応付けられていない。

一方 issue 0168 の `ListOrgs`/`handleDatadogOrgs` は、Datadog の `GET /api/v1/org` が返す各組織のログイン状態を、その組織の `public_id` をそのまま `org` として `datadogauth.LoadToken(dir, site, org)` に渡して判定する。`GET /api/v1/org` は Sub Org だけでなく親組織自身のエントリ (親組織自身の `public_id` を持つ) も含めて返すため、親組織のエントリについても `org == <親組織の public_id>` でトークンの有無を調べることになる。しかし親組織のトークンは `org == ""` のファイルにしか保存されていないため、`org == <親組織の public_id>` のトークンファイルは常に存在せず、親組織のエントリは常に「未ログイン」と判定される。

この結果、`GET /api/v1/org` のレスポンスに含まれる「自分自身 (親組織) のエントリ」と、issue 0167 が定めた「親組織を表す予約値 `org == ""`」の間に対応付けが無いことが根本原因である。issue 0168 の実装自体は、team-lead が指示した「その org のトークンファイルの有無で判定する」という基準どおりに実装されており、実装上の逸脱ではない (issue 0168 の実装エージェントが調査時に発見・報告した)。

## 完了条件

- `GET /api/v1/org` のレスポンスに親組織自身のエントリが含まれる場合に、そのエントリのログイン状態が `org == ""` (親組織) のトークン・静的キーの有無を正しく反映することがテストで確認できる。
- 親組織自身のタブをクリックしたときに、新しい Sub Org 向け OAuth ログイン (別の DCR クライアント登録) ではなく、親組織として現在使われている認証経路 (既存の OAuth トークンの利用、または静的キーへのフォールバック) が使われることがテストで確認できる。
- 上記の判定に伴う設計判断 (親組織の `public_id` をどう特定し `org == ""` へ写像するか。例: `GET /api/v1/org` のレスポンス自体に親組織かどうかを示すフィールドが無いか確認し、無ければ別の判定手段を検討する) を実装時に行う。
- 既存の Sub Org (親組織以外) のログイン状態判定・ログインフローの挙動は変えない。
- `mise run check` (`fmt` + `lint` + `test`) が通過する。
- (2026-09-14 追記、issue 0171 拡張) `GET /api/v2/org` (SDK v2.65.0 の `ListOrgs`) を用いて Sub Organization の一覧が実環境で正しく取得できる (`GET /api/v1/org` は自組織 1 件しか返さないため、これまで一度も機能していなかったことが実機検証で判明した)。
- (2026-09-14 追記、issue 0171 拡張) 親組織自身のエントリが一覧の `IsSelf` で識別でき、親組織が一度も OAuth ログインしていない状態でそのタブを開いた場合も、正しく `org == ""` としてログインが開始されることがテストで確認できる。
- (2026-09-14 追記、多観点レビュー (追加レビュー 1 回目) での指摘を受けた拡張) 親組織自身のタブで Cost/Dashboards/Metrics のデータ取得と、認証エラー時の再ログインバナーが、`org == ""` (親組織) を対象にして正しく動作することがテストで確認できる。Dashboards/Metrics は元々 Sub Organization 専用として org の省略を 400 で弾く設計だったため (issue 0169/0170)、親組織にも開放する設計変更をユーザーに確認のうえ行う。

## 関連

- issue 0167 (Datadog OAuth 認証の Sub Org 対応、`org == ""` を親組織の予約値とする設計)。
- issue 0168 (Sub Org タブ UI、本バグの発見元)。

## 調査結果 (2026-09-14 追記)

本 issue の完了条件 (3 行目) が要求する設計判断について、実機検証の結果を記録する。

- `GET /api/v1/org` (`datadogV1.OrganizationsApi.ListOrgs`) は、実際の親組織の認証情報で呼び出すと自組織 1 件だけを返し、Sub Organization を一切含まないことを実機の curl で確認した。issue 0168 の `internal/datadog/organizations.go` の godoc コメント「For a parent organization the response contains the parent itself and its sub organizations」は誤りであり、`DataDog/documentation#9830` (レスポンスがドキュメントと一致しないという報告) と符合する。レスポンスに含まれる `Organization` (`model_organization.go`) にも自組織かどうかを示すフィールドは無い。
- 一方 `GET /api/v2/org` を実機の curl で呼び出すと、JSON:API 形式で `data.relationships.current_org.data.id` (呼び出し元自身の組織 ID) と `data.relationships.managed_orgs.data` (親組織配下の全組織、自身を含む) が返り、`included[]` に各組織の `name` が入ることを確認した。この ID は v1 の `public_id` (例: `zzdme80rmkjupq94`) とは異なる UUID 形式 (例: `4c4f231c-00cc-11ea-a77b-17122b83a2a2`) である。
- 現在 `backend/go.mod` が固定している `github.com/DataDog/datadog-api-client-go/v2` の `v2.55.0` には `GET /api/v2/org` (managed orgs) のラッパーが無い (`datadogV2.OrganizationsApi` は `GetOrgConfig`/`ListOrgConfigs`/`UpdateOrgConfig`/`UploadIdPMetadata` のみ)。`go list -m -versions` で確認できる最新 `v2.65.0` では `datadogV2.OrganizationsApi.ListOrgs` (`ManagedOrgsResponse` を返す) が追加されており、`GET /api/v2/org` に対応する。
- `backend/internal/datadogauth/login.go` の既存コメントのとおり、`org` はトークン・クライアント登録のファイル名にのみ使う thief 内部のローカルな識別子であり、Datadog の OAuth プロトコル自体には一切渡らない。そのため `org` の値の形式を v1 の `public_id` から v2 の UUID へ切り替えても OAuth の挙動には影響しない (`datadogauth.ValidateOrg` の正規表現 `^[a-z0-9_-]+$` は UUID の形式 (小文字・数字・ハイフン、36 文字) もそのまま許容する)。

以上により、issue 0168 の Sub Organization 一覧取得は実環境では一度も機能していなかったことが判明した (`GET /api/v1/org` が自組織 1 件しか返さないため)。この不具合は本 issue の親組織タブの表示バグと根を同じくするため、issue 0168 を reopen して別 issue に分割せず、本 issue 0171 の完了条件を拡張して一体で修正する (ユーザー承認済み)。

**多観点レビューでの指摘と却下理由 (2026-09-14 追記)**: `GET /api/v2/org` を OAuth トークン経由で呼んだときに、既存の `DefaultScopes()` (`usage_read` のみ、`backend/internal/datadogauth/authorize.go`) で十分かという指摘を受けた。Datadog Go SDK (v2.65.0) の生成コードにはスコープの注記が一切無く、必要スコープは Datadog サーバ側の認可ポリシーであり SDK からは判別できない。ただし `backend/internal/api/datadog_auth_context.go` の `datadogCall` は、OAuth 認証での呼び出しが 403 を返した場合に静的キーへ 1 回だけフォールバックする汎用機構を既に備えており (同ファイルの doc コメントが「新しい Datadog API を呼び足す場合は、その API が OAuth に対応しているかを確認すること」と明記し、エンドポイントごとの個別対応を求めていない)、`handleDatadogOrgs` も `datadogCall` 経由で呼ぶためこの機構の対象になる。この経路自体は `TestDatadogOrgsErrors/the_oauth_token_is_rejected_with_403` で検証済み (静的キーが無い場合は 500 INTERNAL_ERROR に落ちることを確認)。スコープが実際に `/api/v2/org` を覆っているかは実機の OAuth トークンでしか確認できないが、不足時の 403 は個別対応が要らない汎用機構で既に吸収されるため、本 issue の完了条件としては扱わず、`DefaultScopes()` の拡張は不要と判断する。

## 修正方針

### 1. backend: Datadog SDK のバージョン確認

調査時点の `backend/go.mod` は `github.com/DataDog/datadog-api-client-go/v2 v2.65.0` を既に指定しており (`go.sum` にも記録済み)、`datadogV2.OrganizationsApi.ListOrgs` (`GET /api/v2/org`) を使うためのバージョン更新は不要と実装着手時に確認した。他の Datadog API (Dashboards/Metrics/UsageMetering) の呼び出しへの影響が無いことは `mise run check` で確認する。

### 2. `backend/internal/datadog/client.go`: v2 Organizations API のラッパーを追加

`OrganizationsV2API` (`api *datadogV2.OrganizationsApi`) と `NewOrganizationsV2API(cfg)` を、既存の `OrganizationsV1API` と同じパターンで追加する。`OrganizationsV1API` は `organizations.go` の `ListOrgs` からしか使われておらず (実装前に grep で確認済み)、v2 移行後は不要になるため削除する。「organizations エンドポイントは v1 にしか無い」という誤った godoc コメントも併せて削除する。

### 3. `backend/internal/datadog/organizations.go`: v2 の managed orgs API で書き換え

`ListOrgs(ctx, api *OrganizationsV2API) ([]OrgInfo, error)` を `datadogV2.OrganizationsApi.ListOrgs` (`ManagedOrgsResponse`) ベースに書き換える。

- `resp.Data.Relationships.CurrentOrg.Data.Id` を自組織 (self) の UUID として保持する。
- `resp.Data.Relationships.ManagedOrgs.Data` (各要素は UUID) を、`resp.Included` (各要素の `Attributes.Name`) と ID で突き合わせて名前を解決する。
- `OrgInfo` に `IsSelf bool` を追加する。各組織の UUID が自組織の UUID と一致する場合に true にする。
- ID は SDK 側で `uuid.UUID` になるため `.String()` で文字列化する。v1 の `public_id` と異なり UUID は小文字 16 進数とハイフンのみだが、既存の `strings.ToLower` による正規化は防御的にそのまま残す (実害は無い)。
- `Included` に対応する要素が無い、または `Name` が `nil` の組織は、issue 0168 時点の「`public_id` が空なら警告してスキップ」と異なり、警告ログを出したうえで ID をそのまま表示名にフォールバックする (スキップはしない。一覧から消えることの実害の方が大きいため)。

### 4. `backend/internal/api/server.go`: `ddOrgV1` を `ddOrgV2` に置き換え

`s.ddOrgV1 *ddclient.OrganizationsV1API` を `s.ddOrgV2 *ddclient.OrganizationsV2API` に置き換え、`ddclient.NewOrganizationsV2API(ddCfg)` で構築する。

### 5. `backend/internal/api/handlers_datadog_orgs.go`: self 判定に基づくログイン状態の修正と IsSelf の公開

- `handleDatadogOrgs` の呼び出しを `ddclient.ListOrgs(ctx, s.ddOrgV2)` に変更する。
- `datadogOrgResponse` に `IsSelf bool` (JSON タグ `is_self`) を追加する。
- `datadogOrgsWithLoginState` で、`org.IsSelf` が true の場合は `s.ddAuth.loadToken(site, datadogParentOrg)` (空文字、親組織) を見る。false の場合は現行どおり `s.ddAuth.loadToken(site, org.ID)` を見る。

### 6. frontend: `IsSelf` の反映とログイン開始対象の修正

- `frontend/src/types/nonaws.ts`: `DatadogOrgRaw` に `is_self: boolean`、`DatadogOrgRow` に `isSelf: boolean` を追加する。
- `frontend/src/lib/normalizeNonAws.ts`: `datadogOrgFromRaw` で `isSelf: raw.is_self` を渡す。
- `frontend/src/components/session/DatadogOrgSessionTabs.tsx`: `loginIfNeeded(id)` 内で対象組織の `isSelf` を見て、true なら `flow.begin('')` (親組織としてログイン)、false なら現行どおり `flow.begin(id)` を呼ぶ。親組織自身が一度も OAuth ログインしていない状態でそのタブを開いた場合に、`org == <self の UUID>` としてトークンが保存され続け「ログインしても未ログイン表示のまま」になるバグを避けるための変更 (ユーザー承認済み)。
- 同ファイルの `headerNote="GET /api/v1/org"` を `headerNote="GET /api/v2/org"` に更新する (実際に呼び出す API を反映させるだけの表示文言であり、挙動には影響しない)。

### 7. テストの更新

- `backend/internal/datadog/organizations_test.go`: `newTestOrgsAPI` を含め、v2 の `ManagedOrgsResponse` 形状 (JSON:API) を返す `httptest` サーバに書き換え、`IsSelf` の判定 (自組織/Sub Org/名前解決フォールバック) をテーブル駆動で検証する。
- `backend/internal/api/handlers_datadog_orgs_test.go` 等の関連テスト: `IsSelf` に応じたログイン状態判定 (`org == ""` を見る分岐) をケース追加する。
- `frontend/src/components/session/DatadogOrgSessionTabs.test.tsx`: `isSelf: true` の未ログイン組織のタブを開いたときに `flow.begin('')` が呼ばれることを検証するケースを追加する。
- `frontend/src/hooks/useDatadogOrgs.test.tsx` / `frontend/src/lib/normalizeNonAws.test.ts` 等、`DatadogOrgRow`/`datadogOrgFromRaw` を直接使う既存テストに `isSelf` フィールドの受け渡しを反映する。
- 既存テストが使う `'suborg1'`/`'suborg2'` 等のテスト用 ID 文字列は、テスト内で完結する不透明な識別子であり実際の Datadog ID 形式を検証していないため、UUID 形式への置き換えは不要と判断する (実装時に他に UUID 形式を前提にした検証がテストに紛れ込んでいないか確認する)。

### 8. 検討したが採らなかった案

- **Sub Org を手動登録させる案 (当初検討)**: `GET /api/v1/org` が Sub Org を返さないと判明した時点で一度検討したが、`GET /api/v2/org` (SDK v2.65.0) で自動列挙・自己判定の両方が可能と確認できたため不採用。
- **v1 API を残したまま `GetOrg(publicId)` で個別に自組織を特定する案**: 呼び出し元が自身の `public_id` を得る手段が v1 に無い (`ListOrgs` が自組織のみ返す以外に自己を示す手掛かりが無い) ため成立しない。

### 9. (2026-09-14 追記、多観点レビュー (追加レビュー 1 回目) での指摘を受けた拡張) 親組織タブの Cost/Dashboards/Metrics と再ログインバナーの修正方針

**背景**: 追加レビュー (完了条件の充足の観点) で、本節 5〜6 の修正は「タブをクリックしたときのログイン開始」だけを `org == ''` に直し、他の経路 (`frontend/src/App.tsx` が `DatadogView` に渡す `orgId`、`DatadogAuthBanner` の再ログイン対象、Cost/Dashboards/Metrics のデータ取得) は Sub Org タブと同じく `datadogOrg` (タブの id、= 親組織自身の Datadog 側 UUID) をそのまま使い続けていることが指摘された。`backend/internal/api/datadog_auth_context.go` の `datadogAuthContext`/`datadogFallbackContext`/`datadogCall` は `org == ""` (空文字) だけを親組織として扱い、それ以外の値 (親組織自身の UUID を含む) は常に「別の (存在しない) Sub Organization」として扱うため、この経路が残る限り親組織タブの Cost/Dashboards/Metrics は永久に 401 になり、再ログインバナーをクリックすると本 issue の症状 (不要な Sub Org 向け OAuth ログインの開始) がデータ取得経路から再現する。修正には `frontend/src/App.tsx` が `orgId` を解決する時点で `isSelf` を見る必要があるが、Dashboards/Metrics は issue 0169/0170 で意図的に「Sub Organization 専用」として設計されており (`backend/internal/api/handlers_datadog_dashboards.go` の `datadogRequiredOrgFromQuery` が `org == ""` を 400 で拒否)、この制限を撤廃して親組織にも開放するかどうかは設計判断が必要なため、AskUserQuestion でユーザーに確認した。

**ユーザーの選択**: 「親組織にも Dashboards/Metrics を開放する (推奨)」。影響範囲は次のとおり。

- frontend: `App.tsx` が `isSelf` を見て `DatadogView` に渡す `orgId` を `''` に解決する (タブの再マウント用の `key` にはタブの id を引き続き使う)。
- backend: `handlers_datadog_dashboards.go`/`handlers_datadog_metrics.go` の `datadogRequiredOrgFromQuery` による `org == ""` の 400 判定を撤廃し、Cost と同じ `datadogOrgFromQuery` (org 省略時は親組織として扱う) に統一する。
- 結果として self タブで Cost/Dashboards/Metrics が全てそのまま使えるようになる。

**検討したが採らなかった案**: Dashboards/Metrics を Sub Organization 専用のまま維持し、親組織タブでは Dashboards/Metrics セクション自体を非表示にする案も提示したが、ユーザーは推奨案 (開放) を選んだ。非表示案は完了条件が要求する「親組織タブで Cost/Dashboards/Metrics が動作すること」を満たさず、機能の非対称性 (Sub Org は使えるが親組織は使えない) を恒久的に残すため、レビューでも次善の代替案という位置づけだった。

### 10. (2026-09-14 追記、多観点レビュー (追加レビュー 2 回目 = 最終ラウンド) での指摘を受けた修正方針)

追加レビュー 2 回目 (完了条件の充足の観点) で、本節 9 の実装だけでは親組織タブの Dashboards/Metrics が実際には動作しないという高優先度の不具合が見つかった。

`frontend/src/api/queries.ts` の `useDatadogDashboards`/`useDatadogDashboard`/`useDatadogMetricsQueries` は、Sub Organization 専用だった時代の実装のまま `enabled: !!org` (または `!!org && ...`) を維持していた。この判定は「org が非空文字列であること」を「組織が選択済みであること」の代用にしていたもので、Sub Organization しか対象にしなかった当時は org が常に非空だったため問題にならなかった。しかし本節 9 で `App.tsx` が親組織タブに `orgId=''` を渡すよう変更した結果、`!!org` が `false` になり、親組織タブでは Dashboards/Metrics の TanStack Query が **恒久的に発火しない** (`enabled: false` のまま) という回帰を生んでいた。`queries.ts` は本節 9 の差分に含まれておらず見落とされており、既存テストは `DatadogDashboardView`/`DatadogMetricsView` またはそのフック自体をモックしているため、この `enabled` ロジックを実際に通すテストが存在せず検出されなかった。

修正方針: `org` は空文字 (親組織自身) も含めて常に「対象組織が確定している」値であり、「未選択」を表すことはない (呼び出し元はすべて `orgId: string` を必須 prop として渡す)。したがって `enabled` の判定から `!!org` を外し、真に「未選択」を表す値 (ダッシュボード ID・クエリ文字列) の判定だけを残す。`api/queries.ts` に直接テストが存在しない現状を踏まえ、`useDatadogOrgs.test.tsx` と同じ `renderHook` + `api/endpoints` のモック化のパターンで `frontend/src/api/queries.test.tsx` を新設し、org が空文字でも実際に取得関数が呼ばれることを検証する。

追加レビュー 2 回目では次の 3 件も指摘されたが、いずれも 中 / 低優先度で、実装を変える指摘ではないため却下理由を記録する。

- 中: `DatadogOrgSessionTabs.tsx` の `loginIfNeeded` が `resolveDatadogRequestOrg` と等価なロジックをインラインで重複している (追加レビュー 1 回目の `review-0171-r2-kiyaku` 指摘と同内容)。→ 反映した。`resolveDatadogRequestOrg(orgs, id)` を呼ぶよう統一した (本節 11 参照)。
- 中: `TestDatadogDashboardsParentOrg`/`TestDatadogMetricsQueryParentOrg` が HTTP 200 のみを検証し、実際に上流へ 1 回だけリクエストが届いたことを検証していない。→ 反映した。他の同ファイル内のテスト (`TestDatadogDashboardsCaching` 等) と同じ `calls` カウンタの検証を追加した (本節 11 参照)。
- 中: `App.tsx` での `resolveDatadogRequestOrg(datadogOrgs, datadogOrg)` の呼び出しと `<DatadogView key={datadogOrg} orgId={datadogRequestOrg} />` への配線を直接検証するテストが無い。→ 却下。`resolveDatadogRequestOrg` 自体は純関数として `sessionMeta.test.ts` で isSelf true/false・一覧に無い id・空文字の 4 パターンを検証済みであり、`App.tsx` 側で行っているのはその戻り値をそのまま prop に渡すだけの単純な配線である。`App.tsx` にはこの種の配線を検証するテストファイル (`App.test.tsx`) がそもそも存在せず、同じ形の配線である GCP の `activeProject={gcpProject}` (`App.tsx` 160 行目付近) にも前例テストが無いことを確認した。`App.tsx` は `useHealthCheck`/`useProfiles`/`useActiveGcpProject`/`useActiveDatadogOrg`/`useTweaks` など多数のフックをモックする必要があるトップレベルコンポーネントであり、1 行の配線検証のために新規にこの規模のテスト基盤を持ち込むのは、既存コードベースの慣行 (トップレベルコンポーネントは配線を持たず個々のフック・純関数側で検証する) から外れ、この issue のスコープを超える。純関数側のテストで実質的なロジックは検証済みであるため、実装・テストとも変更しない。
- 低: ページリロード直後、Datadog 組織一覧の取得が完了するまでの短い間は `resolveDatadogRequestOrg` が一覧に無い id (取得中の自組織 UUID) をそのまま返すため、親組織タブでも一瞬生の UUID が `orgId` として渡り、本 issue の症状が一時的に再現しうる。→ 却下。この挙動は `sessionMeta.test.ts` の「一覧に無い id (未取得中など) はそのまま返す」で意図的にテストされている既存の設計であり (root cause は `useSessionTabs` の同期的な localStorage 復元と `useDatadogOrgsQuery` の非同期取得のタイミング差であって、本 issue が対象とする「ログイン済みなのに未ログイン表示になる恒久的な不具合」とは異なる)、影響は初回ロード時の一時的な表示ゆれに限られ、一覧取得完了後は自動的に訂正される。この issue の完了条件(「親組織タブで Cost/Dashboards/Metrics が動作すること」)は恒常状態について定義したものであり、この一過性の競合状態の解消は完了条件に含まれないと判断し、実装は変更しない。

### 11. (2026-09-14 追記、追加レビュー 2 回目の指摘を反映した修正方針)

- `frontend/src/api/queries.ts`: `useDatadogDashboards`/`useDatadogDashboard`/`useDatadogMetricsQueries` の `enabled` から `!!org` を外す (`useDatadogDashboard` は `!!id`、`useDatadogMetricsQueries` は `!!query` のみ残す)。
- `frontend/src/api/queries.test.tsx` (新設): `useDatadogOrgs.test.tsx` と同じパターンで `api/endpoints` をモックし、org が空文字でも各フックが実際に取得関数を呼ぶこと、id/query が空の間は呼ばないことを確認する。
- `frontend/src/components/session/DatadogOrgSessionTabs.tsx`: `loginIfNeeded` のインライン `org.isSelf ? '' : id` を `resolveDatadogRequestOrg(orgs, id)` の呼び出しに置き換える。
- `backend/internal/api/handlers_datadog_dashboards_test.go`/`handlers_datadog_metrics_test.go`: `TestDatadogDashboardsParentOrg`/`TestDatadogMetricsQueryParentOrg` に、上流ハンドラが実際に 1 回だけ呼ばれたことを検証する `calls` カウンタのアサーションを追加する。

## 解決方法

### 1. backend: Datadog SDK のバージョン確認

方針どおり、実装着手時点で `backend/go.mod` は既に `github.com/DataDog/datadog-api-client-go/v2 v2.65.0` を指定しており、バージョン更新は不要だった。他の Datadog API (Dashboards/Metrics/UsageMetering) 呼び出しへの影響が無いことは `mise run check` の通過で確認した。方針からの乖離なし。

### 2. `backend/internal/datadog/client.go`

方針どおり `OrganizationsV2API` (`api *datadogV2.OrganizationsApi`) と `NewOrganizationsV2API(cfg)` を `OrganizationsV1API` と同じパターンで追加し、`ListOrgs` からしか使われていなかった `OrganizationsV1API` と、実態と異なる godoc コメントを削除した。方針からの乖離なし。

### 3. `backend/internal/datadog/organizations.go`

方針どおり `ListOrgs(ctx, api *OrganizationsV2API) ([]OrgInfo, error)` を `datadogV2.OrganizationsApi.ListOrgs` (`ManagedOrgsResponse`) ベースに書き換えた。`resp.Data.Relationships.CurrentOrg.Data.Id` を self の UUID として保持し、`resp.Data.Relationships.ManagedOrgs.Data` の各 UUID を `resp.Included` の `Attributes.Name` と ID で突き合わせて名前解決する。`OrgInfo.IsSelf` を追加し、self の UUID と一致する組織を true にする。`Included` に対応する要素が無い組織は警告ログを出したうえで ID を表示名にフォールバックする (スキップしない)。方針からの乖離なし。

### 4. `backend/internal/api/server.go`

方針どおり `s.ddOrgV1 *ddclient.OrganizationsV1API` を `s.ddOrgV2 *ddclient.OrganizationsV2API` に置き換え、`ddclient.NewOrganizationsV2API(ddCfg)` で構築するようにした。方針からの乖離なし。

### 5. `backend/internal/api/handlers_datadog_orgs.go`

方針どおり `handleDatadogOrgs` の呼び出しを `ddclient.ListOrgs(ctx, s.ddOrgV2)` に変更し、`datadogOrgResponse` に `IsSelf bool` (JSON タグ `is_self`) を追加した。`datadogOrgsWithLoginState` は `org.IsSelf` が true のとき `s.ddAuth.loadToken(site, datadogParentOrg)` (空文字、親組織) を見て、false のときは現行どおり `s.ddAuth.loadToken(site, org.ID)` を見るようにした。これが本 issue の症状 (親組織自身のタブが常に未ログイン表示になる) の直接の修正箇所である。

**乖離 (方式を保ったままの実装詳細、多観点レビューで発見)**: 上記の実装は OAuth トークンの有無しか見ておらず、完了条件 1 行目が明記する「静的キーの有無を正しく反映する」を満たしていなかった。`DATADOG_API_KEY`/`DATADOG_APP_KEY` の静的キーのみで運用している親組織 (OAuth ログインを一度もしていない) の self タブが `LoggedIn: false` のままになる、元のバグと同種の症状が静的キー経路で残っていた。`datadogOrgsWithLoginState` に、`org.IsSelf` かつ OAuth トークンが無い場合は `s.cfg.DatadogAPIKey() != "" && s.cfg.DatadogAppKey() != ""` (`datadogFallbackContext` が親組織のときに静的キーへフォールバックするのと同じ基準) を追加で見る分岐を実装した。静的キーは org 非依存のグローバル環境変数のため、この判定は `IsSelf` なエントリにだけ適用し、Sub Organization のエントリには適用しない。

### 6. frontend: `IsSelf` の反映とログイン開始対象の修正

方針どおり `DatadogOrgRaw.is_self`/`DatadogOrgRow.isSelf` を追加し、`datadogOrgFromRaw` で伝播させた。`DatadogOrgSessionTabs.tsx` の `loginIfNeeded(id)` は、対象組織が `isSelf` なら `flow.begin('')` (親組織としてログイン)、そうでなければ現行どおり `flow.begin(id)` を呼ぶようにした。`headerNote` の表示文言も `GET /api/v2/org` に更新した。方針からの乖離なし。

### 7. テストの更新 (方針からの乖離あり)

- 方針どおり `organizations_test.go` を v2 の `ManagedOrgsResponse` 形状 (JSON:API) を返す `httptest` サーバへ書き換え、`IsSelf` の判定 (自組織/Sub Org/名前解決フォールバック/空一覧/API エラー) をテーブル駆動で検証する `TestListOrgs` を書いた。
- **乖離 (方式を保ったままの実装詳細)**: テスト用の `included[]` JSON を当初 `name` と `public_id: null` のみを持つ最小形にしていたところ、SDK (`v2.65.0`) の `OrgAttributes.UnmarshalJSON` (`model_org_attributes.go`) が `created_at`/`description`/`disabled`/`modified_at`/`name`/`public_id`/`sharing`/`url` の 8 フィールドすべてを必須として検証しており、1 つでも欠ける (または null になる) と該当の `OrgData` 全体が `UnparsedObject` にフォールバックして `GetId()` がゼロ値の UUID を返すことが判明した。この結果、名前解決の ID 突き合わせが常に失敗していた。`organizations_test.go` の `managedOrgsBody` ヘルパーと `handlers_datadog_orgs_test.go` の `datadogOrgsBody` 定数の両方で、8 フィールドすべてを実 API 相当の値で埋める形に修正した。これはテスト用フィクスチャの不備であり、本番コードの不具合ではない (実際の Datadog API レスポンスは必須フィールドを常に含む)。
- 方針どおり `handlers_datadog_orgs_test.go` の `TestDatadogOrgsReturnsLowerCasedIdsAndLoginState` を、self エントリが `LoggedIn: true`/`IsSelf: true`、Sub Org エントリが `IsSelf: false` になることを検証する形に更新した。これが本 issue の回帰テストの本体である。
- **乖離 (方式を保ったままの実装詳細、多観点レビューで訂正)**: 方針は「親組織自身が一度も OAuth ログインしていない状態」を backend 側のテストケースとして追加する含みだった。「OAuth トークンも静的キーも一切無い」場合は、一覧取得自体が `DATADOG_NO_CREDENTIALS` で 401 になり self のログイン状態判定に到達しない (この場合に限りテストは成立しない)。しかし「静的キーはあるが OAuth トークンが無い」場合は一覧取得自体は静的キーへのフォールバックで成功し、self のログイン状態判定に到達する。多観点レビューの指摘まで、この経路を検証しないまま `TestDatadogOrgsSelfNotLoggedIn` を「一覧取得自体が失敗するので成立しない」という誤った理由で削除しており、本節 5 に記載した静的キー未対応のバグを見落としていた。レビュー指摘を受けて `datadogOrgsWithLoginState` を修正し、この経路を検証する `TestDatadogOrgsSelfLoggedInViaStaticKey` (OAuth トークン無し・静的キーのみ設定の状態で self エントリが `LoggedIn: true`、Sub Org エントリは静的キーの影響を受けず `LoggedIn: false` のままであることを確認) を追加した。完了条件 2 行目の「親組織が一度も OAuth ログインしていない状態でそのタブを開いた場合も、正しく `org == ""` としてログインが開始される」は、引き続き `DatadogOrgSessionTabs.test.tsx` の既存ケース「親組織自身が未ログインのタブをクリックすると org を空文字にしてログインを始める」で検証する (この検証対象は backend のログイン状態判定ではなく frontend のログイン開始対象の分岐であり、backend 側の資格情報の有無とは独立に成立する)。
- 方針どおり `TestDatadogOrgsCaching`/`TestDatadogOrgsCorruptTokenIsNotLoggedIn` の Sub Org 識別子を UUID 形式の `datadogTestSubID` を使う形に更新した。
- `DatadogOrgSessionTabs.test.tsx`/`useDatadogOrgs.test.tsx`/`normalizeNonAws.test.ts`/`sessionMeta.test.ts` は方針どおり `isSelf` フィールドの受け渡しを反映済み (`sessionMeta.test.ts` は `DatadogOrgRow` フィクスチャへの `isSelf` 追加のみの機械的な追随で、方針 6 の `DatadogOrgRow.isSelf` 追加に伴う型エラー修正である)。
- 既存の `'suborg1'`/`'suborg2'` 等の不透明なテスト用 ID を使う他のテスト (`datadogauth`、CLI、dashboards、metrics ハンドラのテスト) は、方針が示したとおり v2 SDK の `uuid.UUID` 型フィールドを経由しないため UUID 形式への置き換えは不要と確認した (全リポジトリを grep して照合済み)。

### 8. 検討したが採らなかった案

方針から変更なし。

### 9. 親組織タブの Cost/Dashboards/Metrics と再ログインバナーの修正

方針どおり、ユーザーが選んだ「親組織にも Dashboards/Metrics を開放する」案で実装した。

- `frontend/src/lib/sessionMeta.ts`: `resolveDatadogRequestOrg(orgs, activeOrg)` を新設した。対象組織が `isSelf` なら空文字を、そうでなければ `activeOrg` (タブの id) をそのまま返す純関数で、`datadogOrgPickerItems` と同じく `DatadogOrgRow[]` を扱う既存ファイルに置いた。
- `frontend/src/App.tsx`: `datadog.orgs`/`datadog.activeOrg` から `resolveDatadogRequestOrg` で `datadogRequestOrg` を求め、`<DatadogView orgId={datadogRequestOrg}>` に渡すよう変更した。`key={datadogOrg}` はタブの id のまま維持し、タブ切替時の再マウント (前の組織の期間・絞り込みのリセット) は従来どおり保つ。`DatadogView` 自体 (Cost/Dashboards/Metrics の取得と `DatadogAuthBanner` の対象) はこの `orgId` を一律に使う既存実装のままで変更不要だった (受け取る値が正しくなれば全経路が連動して直るため)。
- `backend/internal/api/handlers_datadog_dashboards.go`: `datadogRequiredOrgFromQuery` (org 省略/空文字を 400 で拒否するラッパー) を削除し、2 箇所の呼び出し元 (`handleDatadogDashboards`/`handleDatadogDashboard`) を Cost と同じ `datadogOrgFromQuery` (省略時は親組織として扱う) に統一した。
- `backend/internal/api/handlers_datadog_metrics.go`: 同じく `handleDatadogMetricsQuery` の呼び出しを `datadogOrgFromQuery` に統一した。
- 方針からの乖離なし。

### 10. 追加レビュー 2 回目 (最終ラウンド) で発見された `api/queries.ts` の `enabled: !!org` 回帰の修正

方針 (本節「修正方針」10) どおりに実装した。

- `frontend/src/api/queries.ts`: `useDatadogDashboards` の `enabled: !!org` を削除 (常に有効)。`useDatadogDashboard` の `enabled` を `!!org && !!id` から `!!id` に変更。`useDatadogMetricsQueries` の各クエリの `enabled` を `!!org && !!query` から `!!query` に変更。
- `frontend/src/api/queries.test.tsx` (新設): `useDatadogDashboards`/`useDatadogDashboard`/`useDatadogMetricsQueries` を対象に、`renderHook` + `api/endpoints` のモック化で org が空文字でも取得関数が呼ばれること (2 件)、id/query が空の間は呼ばれないこと (2 件) を検証する計 5 ケースを追加した。
- `frontend/src/components/session/DatadogOrgSessionTabs.tsx`: `loginIfNeeded` のインライン `org.isSelf ? '' : id` を `resolveDatadogRequestOrg(orgs, id)` の呼び出しに置き換えた (指摘反映)。
- `backend/internal/api/handlers_datadog_dashboards_test.go`: `TestDatadogDashboardsParentOrg` の各ケースに、上流ハンドラの呼び出し回数 (`calls`) が 1 であることのアサーションを追加した (指摘反映)。
- `backend/internal/api/handlers_datadog_metrics_test.go`: `TestDatadogMetricsQueryParentOrg` に同様のアサーションを追加した (指摘反映)。
- `App.tsx` の配線を直接検証するテストが無い (中優先度の指摘) と、ページリロード直後の一時的な生 UUID 露出 (低優先度の指摘) は、方針で記録したとおりいずれも却下し、実装は変更していない。
- 方針からの乖離なし。

## 完了条件の充足

- 組織一覧のログイン状態が `org == ""` (親組織) のトークン・静的キーの有無を正しく反映すること: `TestDatadogOrgsReturnsLowerCasedIdsAndLoginState` (OAuth トークンの有無) と `TestDatadogOrgsSelfLoggedInViaStaticKey` (静的キーの有無) の両方で確認した。
- 親組織自身のタブをクリックしたときに、新しい Sub Org 向け OAuth ログインではなく親組織の認証経路が使われること: `DatadogOrgSessionTabs.test.tsx` の `flow.begin('')` 呼び出しを検証するケースで確認した。
- 判定に伴う設計判断を実装時に行うこと: `OrgInfo.IsSelf`/`datadogOrgResponse.IsSelf` を導入し、これを判定基準とする設計を「調査結果」および本節 2〜5 で確定・実装した。
- 既存の Sub Org (親組織以外) のログイン状態判定・ログインフローの挙動を変えないこと: `TestDatadogOrgsCaching`/`TestDatadogOrgsCorruptTokenIsNotLoggedIn` で non-self エントリが従来どおり `org.ID` で判定されることを確認した。
- `mise run check` が通過すること: 「テスト結果」節のとおり通過を確認した。
- `GET /api/v2/org` で Sub Organization の一覧が実環境で正しく取得できること: `TestListOrgs` のテーブル駆動テストに加え、「調査結果」節で実機の curl による確認を行った。
- 親組織自身のエントリが `IsSelf` で識別でき、一度も OAuth ログインしていない状態でタブを開いた場合も `org == ""` としてログインが開始されること: `DatadogOrgSessionTabs.test.tsx` の該当ケースで確認した (backend 側のシナリオが成立しない理由は本節 7 の乖離に記載)。
- 親組織自身のタブで Cost/Dashboards/Metrics と再ログインバナーが `org == ""` を対象に動作すること: frontend は `sessionMeta.test.ts` の `resolveDatadogRequestOrg` のテーブル駆動テストで解決ロジックを検証した。backend は `TestDatadogDashboardsParentOrg`/`TestDatadogMetricsQueryParentOrg` で、org 省略・空文字のどちらでも静的キーへフォールバックして 200 が返ることを確認し、`TestDatadogDashboardsInvalidOrgIsRejected` でパストラバーサル値は引き続き 400 で拒否されることを確認した。Cost は本節 5〜7 の修正で既に `org == ""` を親組織として扱っており、今回の変更は不要だった。なお追加レビュー 2 回目で、frontend の `api/queries.ts` が `enabled: !!org` により self タブで Dashboards/Metrics のクエリを恒久的に発火させない回帰を起こしていたことが判明し、本節「解決方法」10 で修正した。修正後は `frontend/src/api/queries.test.tsx` (新設) で、org が空文字でも `useDatadogDashboards`/`useDatadogDashboard`/`useDatadogMetricsQueries` が実際に取得関数を呼ぶことを検証している。

## テスト結果

- backend: `go build ./...` 全パッケージ成功。`go test -race -cover ./...` 全 18 パッケージ `ok` (新規/更新テストを含め失敗なし)。`go vet ./...`・`staticcheck ./...`・`govulncheck ./...` はいずれも指摘なし (自コードに起因する脆弱性 0 件)。`gofmt -l .` は差分なし。
- frontend: `npm run test -- --run` は 87 テストファイル・917 テストすべて成功。`npm run lint` (`eslint . && tsc --noEmit`) はエラー 0、警告は本変更と無関係な既存 10 件のみで新規警告なし。
- ベースライン (Step 1 の 8) からの新たな失敗は無く、`mise run check` に相当する上記一式がすべて通過することを確認した。
- なお `mise run check` を一括実行した際に `backend:lint` (`go vet`/`staticcheck`/`govulncheck` の 3 コマンド) が frontend タスクとの並列実行によるリソース競合で `Killed` (OOM) となったため、`go vet`/`staticcheck`/`govulncheck`/`go test -race -cover`/`npm run test -- --run` をそれぞれ単体で再実行して通過を確認した。コンテナのメモリ制約による環境要因であり、コードの不具合ではない。
- 多観点レビューの指摘 (本節 7 の乖離、および「多観点レビューでの指摘と却下理由」節) を反映した後、`go build ./...`・`go vet ./...`・`staticcheck ./...`・`govulncheck ./...`・`gofmt -l .`・`go test -race -cover ./...` (全 18 パッケージ `ok`、新規追加した `TestDatadogOrgsSelfLoggedInViaStaticKey` を含め失敗なし)・`npm run test -- --run` (87 テストファイル・917 テストすべて成功、テスト数は変更前と同一)・`npm run lint` (エラー 0、警告は変更前と同じ既存 10 件のみ) を単体実行し直し、新たな失敗が無いことを確認した。
- 追加レビュー 1 回目で発見された親組織タブの Cost/Dashboards/Metrics/再ログインバナーの不具合 (本節 9) を修正した後、`go build ./...`・`gofmt -l .`・`go vet ./...` は差分・指摘なし、`go test -race -cover ./...` は全 18 パッケージ `ok` (`TestDatadogDashboardsInvalidOrgIsRejected`/`TestDatadogDashboardsParentOrg`/`TestDatadogMetricsQueryParentOrg` を含め失敗なし)、`npm run test -- --run` は 87 テストファイル・921 テスト (`resolveDatadogRequestOrg` の 4 ケース追加分を含む) すべて成功、`npm run lint` はエラー 0・警告は変更前と同じ既存 10 件のみ (`App.tsx` の既存警告は行番号のみ移動) を確認した。
- 追加レビュー 2 回目 (最終ラウンド) で発見された `api/queries.ts` の `enabled: !!org` 回帰 (本節「解決方法」10) を修正した後、`go build ./...`・`gofmt -l .`・`go vet ./...`・`staticcheck ./...`・`govulncheck ./...` はいずれも差分・指摘なし (自コードに起因する脆弱性 0 件)、`go test -race -cover ./...` は全 18 パッケージ `ok` (`handlers_datadog_dashboards_test.go`/`handlers_datadog_metrics_test.go` に追加した `calls` カウンタのアサーションを含め失敗なし)、`npm run test -- --run` は 88 テストファイル・926 テスト (新設した `frontend/src/api/queries.test.tsx` の 5 ケースを含む) すべて成功、`npm run lint` はエラー 0・警告は変更前と同じ既存 10 件のみを確認した。
