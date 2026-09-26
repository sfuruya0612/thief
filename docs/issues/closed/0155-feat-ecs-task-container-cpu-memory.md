# ECS タスク詳細のコンテナ一覧にコンテナ単位の CPU と Memory を表示する

Created: 2026-08-26
Model: Claude Fable 5
Completed: 2026-08-27

## 背景

docs/issues/TODO.md の次の項目に由来する。

> Amazon ECS のタスクの詳細にコンテナの CPU, memory も載せるようにしてほしい

ECS のタスク一覧は `GET /api/aws/profiles/{profile}/ecs/{cluster}/tasks` (backend/internal/api/routes.go、backend/internal/api/handlers_aws.go の `handleECSTasks`) が返す。
実装は backend/internal/aws/ecs_exec.go の `ListECSTasks` から `listECSTasks` で、`ListTasks` の後に `DescribeTasks` を呼び、`ecsTaskFromSDK` で `ECSTaskResource` に変換する。

`ECSTaskResource` はタスクレベルの `CPU` / `Memory` (`Task.Cpu` / `Task.Memory`、文字列) を持ち、frontend/src/components/Drawer/DrawerECSTasks.tsx の `ECSTaskDetail` が kv 表示に出している。
一方、コンテナ単位の `ECSTaskContainerDetail` (同ファイル) は Name / Image / LastStatus / HealthStatus / ExitCode / Reason / RuntimeID / ExecEnabled のみで、`ecsTaskFromSDK` は SDK の `Container.Cpu` / `Container.Memory` / `Container.MemoryReservation` を参照していない。
frontend 側も `ECSTaskContainerDetailRaw` / `ECSTaskContainerDetailRow` (frontend/src/types/aws.ts) と `ecsTaskFromRaw` (frontend/src/lib/normalize.ts) に該当フィールドが無く、`ECSTaskDetail` の Containers テーブル (Name / Image / Last status / Health / Exit code / Reason / Exec の 7 列) にも CPU と Memory の列が無い。

AWS SDK for Go v2 の `ecs/types.Container` は `DescribeTasks` の応答に次のフィールドを含む (service/ecs/types/types.go)。

- `Cpu *string`: コンテナに設定された CPU ユニット。タスク定義のコンテナ定義で未指定なら `0`。
- `Memory *string`: メモリのハードリミット (MiB)。
- `MemoryReservation *string`: メモリのソフトリミット (MiB)。

既に呼んでいる `DescribeTasks` の応答にコンテナ単位の値が入っているのに、backend が構造体へコピーせず捨てていることが、要望が満たされていない理由である。

## 目的

タスク詳細の Containers テーブルで、コンテナごとの CPU ユニット、メモリのハードリミット、ソフトリミットを読める。
タスクレベルの値と並べて、どのコンテナがタスクの資源をどう分け合っているかを Drawer だけで把握できる。

## 設計判断

### 値の取得元は `DescribeTasks` の `Container` とする

- `ecsTaskFromSDK` で `Container.Cpu` / `Memory` / `MemoryReservation` を `ECSTaskContainerDetail` の新フィールド `CPU` / `Memory` / `MemoryReservation` (JSON は `cpu` / `memory` / `memory_reservation`、文字列) にコピーする。追加の API 呼び出しと IAM 権限は不要である。
- 採らなかった案: `DescribeTaskDefinition` を呼んで `ContainerDefinition.Cpu` / `Memory` / `MemoryReservation` を取る。タスク定義 ARN ごとに追加の API 呼び出しとキャッシュが必要になる。値は `DescribeTasks` の `Container` と同じ設定値 (どちらもタスク定義の指定値で、実測値ではない) であり、追加コストに見合う情報の差が無いため却下する。
- 採らなかった案: CloudWatch Container Insights の実測メトリクスを表示する。TODO の要望は「詳細に載せる」であり実測値の要求ではなく、Container Insights の有効化という前提を課すため却下する。

### 表示形式

- Containers テーブルに `CPU` と `Memory` の 2 列を追加する。列見出しは既存 7 列と同じく英語ハードコードとする (既存の `<th>` が i18n を経由していないため揃える)。`Memory` 列は `<hard> / <soft>` の形式で `Memory` と `MemoryReservation` を並べ、どちらか一方のみのときはその値だけを出す。
- SDK の仕様どおり `Cpu` が `0` のとき (コンテナ定義で未指定) は `Dash()` (frontend/src/components/tables/cells.tsx) と同じ扱いで `-` を表示する。EC2 起動タイプでコンテナ単位の値を指定していない構成では 3 つの値とも `-` になる。Fargate ではタスクレベルの値のみ必須でコンテナ単位は任意のため、同様に `-` になることがある。
- 空文字列 (SDK が nil を返した場合) も `-` にする。`Memory` / `MemoryReservation` が文字列 `0` のときも `CPU` と同じく未設定として `-` にする (MiB の上限 0 は指定できない値であり、表示しても意味を持たない)。
- `colgroup` の幅配分は既存 7 列を詰めて 9 列に収める。Drawer の横幅は広げない。

### 変更しない範囲

- タスク一覧 (`ecsTaskColumns`) の列は変えない。コンテナ単位の値はタスク詳細でのみ表示する。
- `ListECSContainers` が返す Exec 対象選択用の `ECSContainerResource` は変えない。
- レガシー CLI 互換の backend/internal/aws/ecs_cli.go の型は変えない。

## 完了条件

- `ECSTaskContainerDetail` に `CPU` / `Memory` / `MemoryReservation` (JSON `cpu` / `memory` / `memory_reservation`) が追加され、`ecsTaskFromSDK` が `Container.Cpu` / `Memory` / `MemoryReservation` をコピーする。
- backend/internal/aws/ecs_exec_test.go に、3 フィールドがコピーされること、SDK 側が nil のとき空文字列になることのテストが追加されている。
- `ECSTaskContainerDetailRaw` / `ECSTaskContainerDetailRow` と `ecsTaskFromRaw` に対応フィールドが追加され、frontend/src/lib/normalize.test.ts に、3 フィールドが camelCase へ写されること、空文字列がそのまま写されることのテストが追加されている。`ECSTaskContainerDetailRaw` の 3 フィールドは必須 (optional にしない) とする。backend 側に `omitempty` を付けないためキーは常に出力され、frontend/src/types/contract.check.ts の冒頭コメントが定める「optional は backend の `omitempty` と対応させる」規約に従う。このため normalize.test.ts と DrawerECSTasks.test.tsx の既存テストで `ECSTaskContainerDetailRaw` のリテラルを書いている箇所に 3 フィールドを足す (期待値は変えない)。
- `ECSTaskDetail` の Containers テーブルに `CPU` 列と `Memory` 列が追加され、`cpu` が `0` または空のとき `CPU` 列に `-` を表示する。`Memory` 列は両方あるとき `<hard> / <soft>`、一方だけのとき (もう一方が空文字列または `0`) その値だけを表示し、両方が空または `0` のとき `-` を表示する。
- Containers テーブルの `colgroup` が 9 列になり、`<col>` の `width` の合計が変更前の 7 列の合計と同じである (自動テストで検証する)。Drawer の横幅と `table.dt` の横スクロールの有無は jsdom で判定できないため自動テストにせず、ブラウザで Containers テーブルに横スクロールが発生しないことと Drawer の横幅が変わっていないことを目視確認し、その結果を PR 説明に書く。発生した場合は `colgroup` の `width` 配分を調整し、発生しなくなるまで再確認する。
- frontend/src/components/Drawer/DrawerECSTasks.test.tsx に、値あり、`cpu` が `0`、`cpu` が空文字列、ハードリミットのみ (ソフトが空文字列)、ハードリミットのみ (ソフトが `0`)、ソフトリミットのみ (ハードが空文字列)、ソフトリミットのみ (ハードが `0`)、Memory が両方未設定 (空と空、`0` と空の 2 通り。hard と soft は独立に「空文字列または `0`」で未設定と判定するため残りの組み合わせは同値)、の 9 通りの表示テストと、`colgroup` の `<col>` が 9 本で `width` の合計が変更前と同じであるテストが追加されている。
- `ECSTaskContainerDetail` は backend/internal/contract/contract.go の `Registry` に登録済みのため、`UPDATE_GOLDEN=1 go test ./internal/contract/` で frontend/src/types/__contract__/ECSTaskContainerDetail.json を再生成し、差分が `cpu` / `memory` / `memory_reservation` の追加のみであることを PR 説明に書く。再生成しないと `TestGolden` / `TestTagsGolden` が失敗し、`ECSTaskContainerDetailRaw` にフィールドを足さないと frontend/src/types/contract.check.ts の `Contract<typeof ecsTaskContainerDetail, ECSTaskContainerDetailRaw>` が `tsc --noEmit` で失敗する。
- `DescribeTaskDefinition` を呼ばない。
- タスク一覧の列定義 `ecsTaskColumns`、`ListECSContainers` と `ECSContainerResource`、backend/internal/aws/ecs_cli.go の型は変更しない。
- `mise run check` が通過する。

## 関連

- docs/issues/0156-feat-ecs-container-instances-api.md: ECS コンテナインスタンス一覧 API とタスクへの `container_instance_arn` の追加。同じ `ecs_exec.go` の `ecsTaskFromSDK` に手を入れるが、本 issue は `ECSTaskContainerDetail` (コンテナのループ内)、0156 は `ECSTaskResource` (return 直下) で編集箇所が分かれており、どちらを先に実装しても他方は壊れない。
- docs/issues/0157-feat-ecs-container-instances-drawer-tab.md: コンテナインスタンスの Drawer タブ。本 issue が変更する `DrawerECSTasks.tsx` には触れない。frontend/src/lib/normalize.ts の `ecsTaskFromRaw` と normalize.test.ts は両 issue が編集するが、本 issue は `containers` の map 内 (コンテナ単位のフィールド)、0157 はトップレベルの `containerInstanceArn` で編集箇所が分かれており、どちらを先に実装しても他方は壊れない。

## 解決方法

backend/internal/aws/ecs_exec.go の `ECSTaskContainerDetail` に `CPU` / `Memory` / `MemoryReservation` (JSON `cpu` / `memory` / `memory_reservation`、文字列、`omitempty` 無し) を追加し、`ecsTaskFromSDK` のコンテナのループで `Container.Cpu` / `Memory` / `MemoryReservation` を `ptrStr` で写す。SDK が nil を返した場合は空文字列になる。backend では `0` を変換せず、未指定の判定は frontend の表示側で行う。追加の API 呼び出しと IAM 権限は無い。

backend/internal/aws/ecs_exec_test.go の `TestECSTaskFromSDK` に、値を持つコンテナで 3 フィールドが写されること、nil のコンテナで空文字列になること (`sidecar`)、`Cpu` が `0` で `Memory` のみのコンテナがそのまま写されることのケースを加えた。

`ECSTaskContainerDetail` は contract の `Registry` に登録済みのため `UPDATE_GOLDEN=1 go test ./internal/contract/` で golden を再生成した。差分は frontend/src/types/__contract__/ECSTaskContainerDetail.json と、`containers` に同型を含む ECSTaskResource.json の 2 ファイルに `cpu` / `memory` / `memory_reservation` が追加されたものと、backend/internal/contract/testdata/tags.golden への同 3 タグの追加のみである。

frontend/src/types/aws.ts の `ECSTaskContainerDetailRaw` に必須の `cpu` / `memory` / `memory_reservation`、`ECSTaskContainerDetailRow` に `cpu` / `memory` / `memoryReservation` を追加し、frontend/src/lib/normalize.ts の `ecsTaskFromRaw` の `containers` の map で写す。normalize.test.ts は既存のリテラルに空文字列の 3 フィールドを足して空文字列がそのまま写されることを検証し、値の camelCase 変換は新しいテストで検証する。DrawerECSTasks.test.tsx の既存リテラルにも空文字列の 3 フィールドを足した (期待値は変えていない)。

frontend/src/components/Drawer/DrawerECSTasks.tsx の Containers テーブルに `CPU` 列と `Memory` 列を Health と Exit code の間に追加した (Name / Image / Last status / Health / CPU / Memory / Exit code / Reason / Exec の 9 列)。`isUnsetResource` が空文字列と `0` を未設定と判定し、CPU 列は未設定なら `-`、Memory 列は `formatContainerMemory` で設定済みの値だけを ` / ` で結び (両方あれば `<hard> / <soft>`)、両方未設定なら `-` を出す。未設定の表示は既存の Containers テーブルが空値に使っているリテラル `-` に揃えた (`Dash()` は `—` を描く別部品で、この表では使われていないため採らなかった)。`colgroup` は 16 / 20 / 10 / 10 / 7 / 11 / 7 / 10 / 9 % の 9 列で合計は変更前の 7 列 (18 / 26 / 12 / 12 / 7 / 13 / 12 = 100) と同じ 100 % とし、No containers 行の `colSpan` を 9 にした。

DrawerECSTasks.test.tsx に `describe('コンテナ単位の CPU / Memory 列')` を追加し、`it.each` で完了条件の 9 通り (値あり / cpu が 0 / cpu が空 / ハードのみ (ソフト空) / ハードのみ (ソフト 0) / ソフトのみ (ハード空) / ソフトのみ (ハード 0) / 両方未設定 (空と空) / 両方未設定 (0 と空)) の CPU 列と Memory 列の表示を検証し、`colgroup` の `<col>` が 9 本で `thead th` も 9 個、`width` の合計が 100 であることを検証する。`isUnsetResource` から `0` の判定を外すと `0` を含む 4 ケースが落ちることを一時変更で確認した。

Drawer の横幅と横スクロールの有無の目視確認は完了条件のとおり自動テストにしておらず、この実行ではブラウザでの確認を行っていない。確認には SSO でログインした profile と実際の ECS タスクが必要で、対話的なブラウザ認証を自律実行では完了できないためである (0153 と同じ環境不在の扱い)。代わりに CSS から判定した。frontend/src/app.css の `table.dt` は `width: 100%` と `table-layout: fixed` を持ち、テーブル幅は常に親要素の幅に固定され、`<col>` の `%` 幅 (合計 100) でその幅を分け合うため、セルの内容が長くてもテーブルが親より広がることは無く、この表を原因とする横スクロールと Drawer の横幅の変化は CSS の仕様上起きない。PR 作成は本スキルの範囲外 (禁止事項) のため、PR 説明への目視確認結果と golden 差分の転記は PR 作成時に本節の記載から行う。

### 完了条件の確認

- `ECSTaskContainerDetail` の 3 フィールドと `ecsTaskFromSDK` のコピー: 差分で確認
- ecs_exec_test.go のコピーと nil のテスト: `TestECSTaskFromSDK` の 1 ケース目 (app / sidecar) と追加ケース
- Raw / Row / `ecsTaskFromRaw` の追加と normalize.test.ts の 2 テスト、Raw の 3 フィールドが必須: 差分で確認。既存リテラルへの 3 フィールド追加も実施
- `CPU` / `Memory` 列の表示規則: it.each の 9 ケースで検証
- `colgroup` 9 列と `width` 合計不変: テストで検証。ブラウザの目視確認は環境不在で未実施。`table.dt` の `table-layout: fixed` + `width: 100%` により表がはみ出さないことを CSS で確認 (PR 説明に書く)
- golden の再生成: 実施。差分は 3 キーの追加のみ (ECSTaskResource.json と tags.golden にも同 3 キーの追加が出る)
- `DescribeTaskDefinition` を呼ばない、`ecsTaskColumns` / `ListECSContainers` / `ECSContainerResource` / ecs_cli.go を変更しない: 差分に該当箇所が無い
- `mise run check`: 通過 (frontend 765 テスト PASS、backend 全パッケージ ok、lint は 0 エラー。警告 10 件はベースラインと同じ)
