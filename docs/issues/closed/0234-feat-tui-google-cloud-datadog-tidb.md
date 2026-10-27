# TUI に Google Cloud、Datadog、TiDB Cloud のビューを追加し、枠線をプロバイダーのイメージカラーに合わせる

Created: 2026-10-27
Completed: 2026-10-27
Model: DeepSeek V4.1 Flash 1017

## なぜこの対応が必要か

TUI (`thief tui`) は AWS の 19 サービスだけを表示していた。
Google Cloud、Datadog、TiDB Cloud のリソースを見るには Web UI と API サーバを起動する必要があり、端末の中だけで完結するという TUI の目的を満たしていなかった。
また、枠線、タイトル、選択行が 1 つのアクセント色で固定されており、表示中のプロバイダーを色で見分けられなかった。

PRD の FR-29 と FR-30 は AWS を対象にした第 1 段階であり、非 AWS プロバイダーのビューは第 2 段階以降の未確定論点だった。
2026-10-27 に、第 2 段階の最初の範囲として、プロバイダーの切り替えと、一覧を持つサービスのビューを実装することが決まった (FR-31、ADR 0036)。

## スコープ

- TUI の一覧の取得元を `Provider` で表し、`P` で AWS、Google Cloud、Datadog、TiDB Cloud を切り替える。
- プロバイダーごとに主スコープ (AWS のプロファイル、Google Cloud のプロジェクト、Datadog の組織) を `p` で選ぶ。AWS のリージョンは `R` で選ぶ。
- プロバイダーごとに最後に選んだスコープ、リージョン、サービスを保持し、戻ったときに復元する。
- Google Cloud: Cloud Run、Cloud Storage (Objects タブを含む)、IAM、Service Accounts。
- Datadog: Dashboards (Widgets タブを含む)、Cost (Historical)、Cost (Estimated)。認証は保存済み OAuth トークンと静的キーを使う。
- TiDB Cloud: Projects (Clusters タブを含む)、Cost。
- 枠線、タイトル、選択中のタブ、一覧の選択行、キー操作のヒントをプロバイダーのイメージカラーに合わせる。
- BigQuery、Cloud Logging、Datadog Metrics などの分析のビュー、リソースの変更操作、オブジェクトのプレビュー / アップロード / ダウンロード / SQL 検索、コストの期間指定は対象に含めない。

## 受け入れ基準

- `P` のピッカーで 4 プロバイダーを切り替えられ、ヘッダ、サービスの一覧、スコープの表示、枠線の色が切り替わる。
- Google Cloud に切り替えてプロジェクトを選ぶと Cloud Run が表示され、`S` で Cloud Storage、IAM、Service Accounts に切り替えられる。Cloud Storage の Objects タブでフォルダの階層移動、prefix 検索、階層 / フラット表示の切り替えができる。
- Datadog に切り替えると親組織の Dashboards が表示され、`p` で Sub Organization を選べる。Widgets タブにウィジェットの種別、タイトル、クエリが表示される。Cost (Historical) と Cost (Estimated) は当月の請求を表示する。
- TiDB Cloud に切り替えると Projects が表示され、Clusters タブでプロジェクトのクラスタが表示される。Cost は当月の請求を表示する。
- プロバイダーを切り替えて戻ると、最後に選んだスコープとサービスが復元される。
- 非 AWS プロバイダーの一覧の取得に失敗したときは、AWS の SSO の期限切れのバナーを出さず、失敗の内容を一覧の領域に表示する。
- `backend/internal/tui/` は `backend/internal/api` に依存しない。
- `mise run check` が成功する。

## 解決方法

- `backend/internal/tui/provider.go` に `Provider`、`Scope`、`Target` を追加した。
  `Provider` は ID、表示名、イメージカラー、主スコープと副スコープの表示名、スコープの必須可否、既定値、サービス、スコープの一覧取得関数を持つ。
- `backend/internal/tui/model.go` の `Model` をプロバイダー、スコープ、サービスを持つ形に変えた。
  プロバイダーごとの選択状態を `providerStates` に保持し、`P` で切り替えたときに復元する。
  主スコープが必須で未選択なら、スコープの一覧を取ってピッカーを開く。
- プロバイダーごとの実装を追加した。
  - `provider_aws.go`: プロファイルとリージョンを `Scope` に写す。
  - `provider_gcp.go`、`gcp_cloudrun.go`、`gcp_gcs.go`、`gcp_iam.go`、`gcp_serviceaccounts.go`: Google Cloud の 4 サービスと、プロジェクトの一覧。
  - `provider_datadog.go`、`datadog_dashboards.go`、`datadog_cost.go`、`datadog_auth.go`: Datadog の 3 サービス、組織の一覧、OAuth と静的キーの認証。
  - `provider_tidb.go`、`tidb_projects.go`、`tidb_cost.go`: TiDB Cloud の 2 サービス。
- `backend/internal/tui/styles.go` に `accentStyle`、`selectedAccentStyle`、`paneStyleFor` を追加し、枠線、タイトル、選択中のタブ、キー操作のヒントをプロバイダーの色にした。
  `applyProviderTheme` が一覧の選択行の背景とスピナーの色を切り替える。
  セッションウィンドウは AWS だけで使うため、AWS のオレンジにした。
- 既存の S3 Objects タブを GCS でも使えるように、列見出しを差し替えられるようにした (`tab_s3.go`、`gcp_gcs.go`)。
- `backend/internal/cli/tui.go` の `newTUIDeps` を `cfg` を受け取り、4 プロバイダーを組み立てる形に変えた。
  GCP のプロジェクト一覧はローカルキャッシュを優先し、無いときだけ Cloud Resource Manager から取得する。
- テストを追加した。
  - プロバイダーの色、AWS / Google Cloud / Datadog / TiDB Cloud のスコープ変換、プロバイダー切り替えと状態の復元 (`provider_test.go`)。
  - Google Cloud の 4 サービス、GCS Objects タブ、IAM のメンバー集約 (`gcp_*_test.go`)。
  - Datadog の Dashboards、Widgets、コスト、OAuth の更新とフォールバック (`datadog_*_test.go`)。
  - TiDB Cloud の Projects、Clusters、Cost (`tidb_*_test.go`)。
  - CLI のプロバイダーとサービスの並び、タブの並び (`backend/internal/cli/tui_test.go`)。
- PRD に FR-31 と 2026-10-27 の決定を追加し、ADR 0036 と `CHANGES.md` の `[ADD]` を更新した。
