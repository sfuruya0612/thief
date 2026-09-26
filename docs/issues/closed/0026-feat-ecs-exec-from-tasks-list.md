# 0026 ECS Exec の対象選択を Tasks 一覧からのコンテナ指定に変更する

Created: 2026-07-17
Completed: 2026-07-18
Model: Claude Fable 5 claude-fable-5

## 解決方法

backend は `ECSTaskContainerDetail` に `RuntimeID` / `ExecEnabled` を追加し、`ecsTaskFromSDK` で `ListECSContainers` と同じ判定式 (`EnableExecuteCommand && runtimeId != ""`) を埋めた (AWS API の追加呼び出しなし)。
frontend は Drawer に `pendingExecTarget` state を追加してタブ間の受け渡し役にし、Tasks タブの Containers テーブルに Exec ボタン列 (execEnabled で活性/非活性、無効理由をツールチップ表示) を追加、クリックで Terminal タブへ遷移する。
`DrawerTerminal` の `ECSExecTerminal` は事前選択 target がある場合は `ECSExecTerminalDropdown` (旧来のドロップダウンフロー) を経由せず直接 `Terminal` を描画するよう分割し、ドロップダウンのラベルは `{group} / {arnSuffix} ({startedAt})` 形式にした。
`backend/internal/aws/ecs_exec_test.go`、`DrawerECSTasks.test.tsx`、`DrawerTerminal.test.tsx` (新規)、`normalize.test.ts` を追加/更新した。
`mise run backend:build` / `go test -race ./internal/aws/...`、`mise run frontend:lint` (0 errors)、`mise run frontend:test` (282 件全通過) で確認済み。

## 背景 / 根拠

ECS Exec ターミナルの対象選択は、Drawer の Terminal タブ内のドロップダウンで行う設計になっている (`frontend/src/components/Drawer/DrawerTerminal.tsx` の `ECSExecTerminal`)。
このドロップダウンの選択肢ラベルは `arnSuffix(t.arn)` (`frontend/src/lib/format.ts:30`)、つまりタスク ARN 末尾のランダム文字列であり、どのサービスのどのタスクなのかをラベルから判別できない。

一方、Drawer の Tasks タブ (`frontend/src/components/Drawer/DrawerECSTasks.tsx`) には group (サービス名)、コンテナ名、ステータス、起動タイプなどの文脈情報を持つタスク一覧と、行選択時のコンテナ詳細ペインが既にある。
しかしこの一覧は読み取り専用で、そこから exec を起動する手段がない。
利用者は「Tasks タブで対象を特定し、Terminal タブに移動してランダム文字列のドロップダウンから同じタスクを探し直す」という二度手間を強いられている。

Tasks 一覧のコンテナ行から直接 exec を開始できるようにすれば、ランダム文字列の照合作業がなくなる。

## 対応内容

- Tasks タブのタスク詳細ペイン内 Containers テーブルに「Exec」起動アクションを追加し、クリックで該当タスク + コンテナのターミナルを開けるようにする
- Containers テーブルにコンテナ単位の exec 可否を表示し、不可のコンテナではアクションを無効化する
- Terminal タブを直接開いた場合の既存フロー (ドロップダウン選択) は残しつつ、選択肢ラベルに group と起動時刻を併記して判別可能にする

## 実装方針

### backend

exec の実行経路は変更しない。
`GET /api/aws/profiles/{profile}/ecs/{cluster}/tasks/{task}/exec` (`backend/internal/api/routes.go:19`) と `handleECSExec` (`backend/internal/api/handlers_session.go:41`) は task と container をパラメータで受け取る設計になっており、UI 側で対象を確定しさえすれば任意のタスク / コンテナに対して exec を開始できる。

変更するのはタスク一覧のコンテナ詳細のみ。
現状、コンテナ単位の exec 可否 (RuntimeID の有無) はタスク一覧に含まれず、別 API `ListECSContainers` (`backend/internal/aws/ecs_exec.go:196`) を呼ばないと分からない。

- `backend/internal/aws/ecs_exec.go` の `ECSTaskContainerDetail` (`:45`) に `RuntimeID string` と `ExecEnabled bool` を追加する
- `ecsTaskFromSDK` (`:156`) で `ExecEnabled = タスクの EnableExecuteCommand && runtimeId != ""` を埋める (`ListECSContainers` の `:221` と同じ判定)。元データは `DescribeTasks` のレスポンスに含まれているため、AWS API の追加呼び出しは発生しない
- `backend/internal/aws/ecs_exec_test.go` に新フィールドのテーブル駆動テストを追加する

この方式なら、Tasks 一覧の取得だけでコンテナ単位の exec 可否が判定でき、行ごとの追加フェッチ (`useECSContainers` の呼び出し) が不要になる。

### frontend

- `frontend/src/types/aws.ts` の `ECSTaskContainerDetailRaw` / `ECSTaskContainerDetailRow` (`:249` 付近) に `runtime_id` / `runtimeId` と `exec_enabled` / `execEnabled` を追加する
- `frontend/src/lib/normalize.ts` の `ecsTaskFromRaw` (`:255` 付近) の containers マッピングに新フィールドを追随させる
- `frontend/src/components/Drawer/Drawer.tsx` に exec 対象の共有 state (例: `pendingExecTarget: { taskArn: string; container: string } | null`) を追加する。Drawer は現状 `tab` state のみを持ちタブ間の連携手段がないため、この state が Tasks タブから Terminal タブへの受け渡し役になる
- `frontend/src/components/Drawer/DrawerECSTasks.tsx` の `ECSTaskDetail` 内 Containers テーブルに Exec ボタン列を追加する。`execEnabled` が false の行は無効化し、ツールチップ等で理由 (enableExecuteCommand 無効、または RuntimeID 未割り当て) を示す。クリックで `pendingExecTarget` を設定し `setTab('Terminal')` を呼ぶ
- `frontend/src/components/Drawer/DrawerTerminal.tsx` の `ECSExecTerminal` を、事前選択された target を props で受け取った場合はドロップダウンを経由せず直接 `<Terminal wsUrl={ecsExecUrl(...)} />` を描画するように拡張する。target 未指定時は従来のドロップダウンフローにフォールバックする
- 従来ドロップダウンの選択肢ラベルを `arnSuffix(t.arn)` 単独から `{group} / {arnSuffix} ({startedAt})` 形式に変更する
- `frontend/src/api/terminal.ts` の `ecsExecUrl` は既に task / container を明示引数で受けるため変更不要

### テスト

- `DrawerECSTasks` のコンポーネントテスト: exec 可否による Exec ボタンの活性 / 非活性、クリック時に target 設定とタブ遷移が行われること
- `ECSExecTerminal` のコンポーネントテスト: 事前選択 target がある場合にドロップダウンを出さず Terminal を描画すること、無い場合の従来フロー維持
- `normalize.ts` のユニットテスト: 新フィールドの変換

## スコープ外

- 複数ターミナルの同時表示、ターミナルセッションのタブ化 (ターミナルは現状どおり Drawer 内に 1 つ。セッション管理層の新設は別 issue とする)
- EC2 Start Session 側の UI 変更
- Services タブからの exec 起動 (タスク単位の選択が結局必要になるため、Tasks タブに集約する)

## 検証

- `mise run check` (fmt + lint + backend / frontend テスト) を通す
- 実環境で ECS クラスタの Drawer を開き、Tasks タブのコンテナ行から exec を起動してターミナルが接続されること、exec 不可コンテナでボタンが無効化されることを確認する
