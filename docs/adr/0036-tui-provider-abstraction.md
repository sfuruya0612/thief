# 0036. TUI の一覧をプロバイダー抽象で切り替え、枠線にプロバイダーのイメージカラーを使う

Created: 2026-10-27
Model: DeepSeek V4.1 Flash 1017
Status: Accepted
Decided: 2026-10-27

## 状況

TUI (`thief tui`) は、`backend/internal/tui/service.go` の `Service` と `Row` を一覧の単位とし、`backend/internal/cli/tui.go` の `Deps.Services` に AWS の 19 サービスを並べていた。
`Model` は AWS のプロファイルとリージョンを直接持ち、`p` と `R` で切り替えていた。
この構成では、Google Cloud のプロジェクト、Datadog の組織、TiDB Cloud のプロジェクトのように、AWS のプロファイルともリージョンとも違う一覧の取得範囲を持つプロバイダーを扱えない。
また、枠線、タイトル、選択行、キー操作のヒントは 1 つのアクセント色 (紫) で固定されており、プロバイダーを切り替えても見た目が変わらなかった。

PRD の FR-29 と FR-30 は AWS を対象にした第 1 段階であり、Google Cloud、Datadog、TiDB Cloud のビューは第 2 段階以降の未確定論点だった。
2026-10-27 に、第 2 段階の最初の範囲として、プロバイダーの切り替えと、一覧を持つサービスのビューを実装することが決まった。

## 決定

- TUI の一覧の取得元を `Provider` で表す (`backend/internal/tui/provider.go`)。
  `Provider` は ID、表示名、イメージカラー、主スコープの表示名、副スコープの表示名、スコープの必須可否、既定値、サービス、スコープの一覧取得関数を持つ。
- スコープは `Scope` (表示名、補足、`Service.Load` に渡す値) で表す。
  AWS のプロファイル、Google Cloud のプロジェクト、Datadog の組織は、すべて同じ `Scope` として `p` のピッカーに並べる。
  AWS のリージョンは副スコープとして `R` のピッカーに並べる。Google Cloud、Datadog、TiDB Cloud は副スコープを持たない。
- `Service.Load(ctx, scope, region)` の形は変えない。
  Google Cloud は `scope` にプロジェクト ID、Datadog は `scope` に組織の ID (親組織は空文字)、TiDB Cloud はどちらも空文字を渡す。
  これにより、AWS の 19 サービスの実装とテストを大きく変えずに、プロバイダーごとの取得層を `Service` の実装として足せる。
- プロバイダーは `P` で切り替える。
  プロバイダーごとに最後に選んだスコープ、リージョン、サービスを `providerStates` に保持し、戻ったときに復元する。永続化はしない。
  主スコープが必須のプロバイダーでスコープが未選択なら、プロバイダーを切り替えたときに主スコープの一覧を取り、届いたらピッカーを開く。
- プロバイダーのイメージカラーを、フォーカス中の枠線、タイトル、選択中のタブ、一覧の選択行、キー操作のヒントに使う。
  フォーカスしていない側の枠線は、同じ色を暗くした色にする。色は ANSI 256 色の番号で持ち、端末のパレットに依存しない。
  - AWS: オレンジ (208、暗い色は 94)
  - Google Cloud: ブルー (69、暗い色は 25)
  - Datadog: パープル (97、暗い色は 55)
  - TiDB Cloud: レッド (160、暗い色は 88)
- 第 2 段階の最初に載せるサービスは、一覧を持つものに限る。
  - Google Cloud: Cloud Run、Cloud Storage (Objects タブを含む)、IAM、Service Accounts
  - Datadog: Dashboards (Widgets タブを含む)、Cost (Historical)、Cost (Estimated)
  - TiDB Cloud: Projects (Clusters タブを含む)、Cost
  BigQuery、Cloud Logging、Datadog Metrics はクエリエディタ / ログビューア / メトリクスクエリのため、AWS の Athena と CloudWatch Logs と同じく TUI の対象に含めない。
  リソースの変更操作は含めない。
- Datadog の認証は、API サーバと同じ保存済み OAuth トークンと静的キーを使う (`backend/internal/tui/datadog_auth.go`)。
  期限切れのトークンはリフレッシュして保存する。Sub Organization では静的キーへ倒さない。
  OAuth の 403 で静的キーへ 1 回だけやり直す処理は API サーバだけが持ち、TUI は再ログインを案内する。

## 検討した代替案

- プロバイダーごとに別の TUI コマンド (`thief tui aws`、`thief tui gcp`) を作る。
  利用者はプロバイダーを切り替えるたびに TUI を終了して起動し直すことになり、同じキー操作で一覧と詳細を行き来するという TUI の目的に反する。
- API サーバを経由してプロバイダーを切り替える。
  TUI は API サーバを起動せずに動く決定 (ADR 0033) を崩すことになる。
  取得層は CLI と同じ形でプロセス内から呼べるため、HTTP を挟む利点が無い。
- すべてのプロバイダーのリソースを 1 つの汎用モデル (`kind`、`name`、`attributes`) に正規化する。
  Web UI の列、Overview、タブはプロバイダーとサービスごとに違い、正規化するとサービス固有の表示を組み立てるための情報が失われる。
  既存の `Service` と `Row` の抽象をプロバイダーで束ねる方が、変更範囲と表示の一致を保ちやすい。
- プロバイダーの色を設定ファイルで変更可能にする。
  今回はプロバイダーの識別と結び付いた固定の色で足りる。設定項目を増やすと、色の検証と永続化の面倒だけが増える。

## 結果

- 利用者は `P` で AWS、Google Cloud、Datadog、TiDB Cloud を切り替え、プロバイダーごとのスコープとサービスを選べる。
  `p` はプロバイダーに応じてプロファイル、プロジェクト、組織のピッカーになる。`R` は AWS だけで使う。
- TUI の枠線、タイトル、選択行、キー操作のヒントが、選択中のプロバイダーのイメージカラーになる。
- `Deps.Providers` を組み立てる `backend/internal/cli/tui.go` の `newTUIDeps` が、プロバイダーごとの取得層の関数を配線する。
  `Deps.Services` は既存のテストが AWS プロバイダーを組み立てるためだけに残す。
- プロバイダーを足すときは、`Provider` を実装し、`Service` と `Row` を `backend/internal/tui/<provider>_<service>.go` に置き、`newTUIDeps` の `Providers` に足す。
- TUI の状態 (最後に使ったプロバイダー、スコープ、サービス) は永続化しない。
  TUI を起動し直すと AWS のプロファイルとリージョン (コマンドの `-p` / `-r`) から始まる。
- Datadog の OAuth トークンのリフレッシュは TUI のプロセス内でも行うが、複数プロセス間の競合を singleflight で防ぐ仕組みは API サーバだけが持つ。
  TUI と API サーバが同時に更新した場合は、後から保存されたトークンが使われる。

## 根拠資料

- PRD `docs/prd/thief.md` の FR-29、FR-30、FR-31、「決定した論点」の 2026-10-27 の表
- ADR 0004、ADR 0033、ADR 0034
- `backend/internal/tui/provider.go`、`backend/internal/tui/model.go`、`backend/internal/tui/styles.go`
- `backend/internal/cli/tui.go` `newTUIDeps`
- `frontend/src/lib/serviceMeta.ts`、`frontend/src/components/tables/gcpColumns.tsx`、`frontend/src/components/tables/nonAwsColumns.tsx`、`frontend/src/components/Drawer/overviewRows.tsx`
- AWS 公式ブランドカラー: https://aws.amazon.com/architecture/icons/
- Google Cloud 公式ブランドカラー: https://cloud.google.com/icons
- Datadog ブランドカラー: https://www.datadoghq.com/about/resources/
- TiDB ブランドガイドライン (ロゴ SVG の `#dc150b`): https://www.pingcap.com/tidb-brand-guidelines/
