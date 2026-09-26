# Cloud Logging の UI をログビューアデザインに刷新する

Created: 2026-07-18
Completed: 2026-07-18
Model: Claude Opus 4.8

## 背景

現行の Cloud Logging ビュー (CloudLoggingView.tsx) は Athena/BigQuery と同じクエリエディタ流用のレイアウトで、フィルタ入力と行リストのみを持つ。
デザイン検討 (Query Editor.dc.html, Turn 8b) では、左にリソースツリー、右上にフィルタ式とヒストグラム、右下に JSON 展開可能なログ一覧を持つ専用ログビューアが示された。
CloudWatch Logs 追加 (docs/issues/0035) で共通ログビューアコンポーネントを新設するため、Cloud Logging もこれに揃えて刷新する。

## 目的

Cloud Logging を docs/issues/0035 で新設する共通ログビューアコンポーネントの上に再実装し、デザイン Turn 8b に準拠させる。

## 要件

- 左ペインをリソースタイプ別ツリー (チェックボックスで `resource.type` フィルタを構築) にする。
- 右上をフィルタ式 (Logging query language 複数行) + 期間指定 + Live Tail トグル + ヒストグラム (severity 積み上げ) にする。
- 右下を JSON 展開可能なログ一覧にする。
  - 1 行 1 イベント、クリックで payload/labels を展開する。
  - 展開行から各フィールドをフィルタに追加できる。
  - フィールド値のコピー、trace がある場合は Trace リンクを表示する。
  - severity はバッジで色分けする (ERROR / WARNING / INFO)。
- ヘッダーに CSV/JSON コピー (表示中の行) を置く。
- 既存の検索 (`useGcpLogEntries`) と Live Tail (`gcpLoggingTailUrl`) の API はそのまま流用する。バックエンド変更は行わない。

## 検証

- mise run frontend:lint / frontend:test
- floci ではなく GCP 実プロジェクト、もしくはモック相当でブラウザ表示確認

## 解決方法

- `views/nonaws/CloudLoggingView.tsx` を、docs/issues/0035 で新設した共通ログビューアコンポーネント (`LogViewerShell` / `LogTree` / `LogHistogram` / `LogList` / `LogToolbarActions` / `useLiveTail`) の上に再実装した。
- 左ペインは GCP の代表的なリソースタイプ (GKE / Cloud Run / Compute Engine / Cloud SQL / Cloud Storage / BigQuery / Load Balancer) を静的にツリー表示し、チェックで `resource.type` フィルタを構築する。バックエンドにリソースタイプ列挙 API が無いため静的定義とした。
- フィルタは Logging query language の複数行入力とし、選択された resource.type とユーザー入力を改行 (AND) で結合する。複数タイプは OR でまとめる。
- ログ一覧は severity バッジ列と SUMMARY 列を持ち、行クリックで jsonPayload / labels / resource.type / trace を展開する。各フィールドはフィルタ追加とコピー、trace は Cloud Trace コンソールへのリンクを提供する。
- ヒストグラムは severity 積み上げ表示とした。エクスポートはヘッダーの JSON コピーとツールバーの CSV コピー。
- 既存の検索 API (`useGcpLogEntries`) と Live Tail (`gcpLoggingTailUrl`) をそのまま流用し、バックエンドは変更していない。

### 検証

- frontend lint / test 全通過。
- ブラウザで GCP プロジェクトセッションを開き、リソースタイプツリー、severity 列、クエリエディタ、ツールバーの描画をライトテーマで確認した (検索 / Live Tail の実行には実 GCP 認証が必要なため UI 描画とフィルタ構築までを確認)。共通コンポーネント側の検索 / ヒストグラム / 行展開は CloudWatch Logs で実データ確認済み。
