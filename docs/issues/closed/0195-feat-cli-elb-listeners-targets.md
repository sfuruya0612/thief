# CLI に ELB のリスナー・ターゲットグループ・ターゲットヘルス取得を追加する

Created: 2026-09-18
Model: deepseek-v4p1-flash
Completed: 2026-09-25

## 背景

Web の ELB は Drawer に Listeners / Rules / Target groups / Targets の 4 タブを持ち、ロードバランサの構成とターゲットのヘルスを確認できる。backend の取得層には対応する関数 (`backend/internal/aws/elb.go` の `ListELBListeners` / `ListELBRules` / `ListELBTargetGroups` / `ListELBTargetHealth`) と API ルート (`/elb/listeners` /`rules` / `target-groups` / `target-health`) がある。

一方 CLI は `thief elb` (一覧のみ) しかなく、これらの詳細を取得できない。

## 目的

CLI から ELB のリスナー・ルール・ターゲットグループ・ターゲットヘルスを取得できるようにする。

## 設計判断

- `thief elb listeners` / `rules` / `target-groups` / `target-health` をサブコマンドとして追加し、`--lb` / `--target-group` などのフラグで対象を指定する。
- 既存の取得関数をそのまま使い、`printRowsOrGroupBy` で出力する。
- 追加の API・権限は不要。

## 完了条件

- `thief elb listeners --lb <name>` / `rules` / `target-groups` / `target-health --target-group <arn>` がそれぞれ一覧を tab / CSV で出力する。
- `mise run check` が通過する。

## 調査結果

implement-issues の Step 3 (2026-09-24) で背景の引用先を実コードと突き合わせた。

- 背景と設計判断が挙げる `ListELBTargetHealth` は `backend/internal/aws/elb.go` に存在しない。ターゲットヘルスを取得する関数は同ファイルの `DescribeELBTargetHealth(ctx, profile, region, tgArn string) ([]ELBTargetHealthResource, error)` である。API ルート `/elb/target-health` (`backend/internal/api/routes.go:63`) もこの関数を呼ぶ。本 issue の「既存の取得関数」はこの名前で読み替える。
- `ListELBListeners(ctx, profile, region, lbArn)` と `ListELBTargetGroups(ctx, profile, region, lbArn)` はロードバランサの ARN を受け取り、`ListELBRules(ctx, profile, region, listenerArn)` はリスナーの ARN を受け取る。完了条件の `--lb <name>` はロードバランサ名なので、`ListELBResources` の結果 (`ELBResource.Name` と `ELBResource.ID` (ARN)) で名前から ARN へ解決する。ARN がそのまま渡された場合はそのまま使う。
- `thief elb` は現在サブコマンドを持たない単一コマンド (`backend/internal/cli/elb.go` の `newELBCmd` が `RunE` で一覧を出す)。サブコマンドを追加しても `thief elb` 単体の一覧は既存どおり動くようにする (`RunE` を残す)。

## 解決方法

implement-issues (2026-09-25) で実装した。実装は `issue-implementer` エージェントが worktree で行い、親が作業ツリーへ統合した。

### 変更したファイルとシンボル

- `backend/internal/cli/elb.go`: `newELBCmd` の `elb` コマンドに `listeners` / `rules` / `target-groups` / `target-health` の 4 サブコマンドを追加した。親の `elb` は従来どおり `RunE` でロードバランサ一覧を出す。対象の指定は `listeners` と `target-groups` が `--lb` (ロードバランサの名前または ARN)、`rules` が `--listener` (リスナー ARN)、`target-health` が `--target-group` (ターゲットグループ ARN) で、いずれも `MarkFlagRequired` で必須にした。取得は既存の `ListELBListeners` / `ListELBRules` / `ListELBTargetGroups` / `DescribeELBTargetHealth` を `runList` の `Fetch` から呼ぶ。5 つの取得関数は `elbOps` に束ね (`defaultELBOps` が `internal/aws` の実装を返す)、`newELBCmd` は `newELBCmdWithOps(defaultELBOps())` の薄い包みにして、テストで取得関数を差し替えて結線を検証できるようにした (0193 の `athenaQueryOps` と同じ形)。親の `elb` には `Args: cobra.NoArgs` を付け、綴りを誤ったサブコマンド名 (`thief elb listener` など) が一覧の出力に化けずエラーになるようにした。`--lb` の名前は `resolveELBLoadBalancerArn` が `ListELBResources` の結果 (`ELBResource.Name` と `ID`) で ARN へ解決し、`arn:` で始まる値は一覧を引かずにそのまま使う。一覧取得は関数型 `elbLister` で受け取る (`elbOps.listLoadBalancers` と同じ型)。列定義は `elbListenerColumns` (Port / Protocol / DefaultAction / TargetGroup / ARN)、`elbRuleColumns` (Priority / Conditions / Action / TargetGroup / ARN)、`elbTargetGroupColumns` (Name / Protocol / Port / TargetType / VPC / HealthCheckPath / ARN)、`elbTargetHealthColumns` (Target / Port / AZ / State / Reason / Description)。
- `backend/internal/aws/torow.go`: `ELBListenerResource` / `ELBRuleResource` / `ELBTargetGroupResource` / `ELBTargetHealthResource` に `util.Row` を満たす `ToRow()` を追加した。`ELBRuleResource.Conditions` は 1 条件の中で `,` を使う (`field=value1,value2`) ため、複数条件は空白で連結する。
- `backend/internal/cli/elb_test.go` (新規): `TestNewELBCmd` (親に `RunE` が残ること、サブコマンド 4 つの名前)、`TestELBSubcommandsRequireTargetFlag` (対象未指定の実行が AWS へ接続する前に `required flag(s) "<flag>" not set` で止まること)、`TestResolveELBLoadBalancerArn` (ARN の素通し、名前の解決、未発見、空値、一覧エラーの伝播と profile / region の受け渡し、一覧の呼び出し回数)、`TestELBRejectsUnknownSubcommand` (`thief elb listener` が `unknown command` のエラーになること)、`TestDefaultELBOpsWired` (本番の取得関数の束 `defaultELBOps()` の 5 フィールドがすべて非 nil であること)、`TestELBSubcommandOutput` (実コマンドツリーの `elb` を `newELBCmdWithOps` の偽の取得関数に入れ替え、親の `elb` 単体の一覧が CSV で出力されることと、4 サブコマンドそれぞれについて `-p` / `-r` とフラグの値が取得関数の引数へ届くこと、`--lb` の名前が一覧を引いて ARN へ解決され ARN はそのまま渡ること、結果が列定義どおりに CSV (listeners / rules / target-groups) と tab (target-health) で出力されること。rules は 1 条件の中に `,` を含む値で CSV の引用と空白連結を固定する)。
- `backend/internal/cli/columns_torow_test.go`: `TestColumnsToRowOrder` に 4 型の列ヘッダと `ToRow()` の値の対応を追加した。

### 完了条件の検証

- 「`thief elb listeners --lb <name>` / `rules` / `target-groups` / `target-health --target-group <arn>` がそれぞれ一覧を tab / CSV で出力する」: 4 コマンドがフラグの値を取得関数へ渡し、その結果を列定義どおりに tab / CSV で出力することは `TestELBSubcommandOutput` が実コマンドツリーの実行 (`-o csv` / `-o tab` を含む) と標準出力の捕捉で検証した。コマンド木と親コマンドの維持は `TestNewELBCmd`、対象フラグの必須化は `TestELBSubcommandsRequireTargetFlag`、`--lb` の名前から ARN への解決の各経路は `TestResolveELBLoadBalancerArn`、4 コマンドの列と `ToRow()` の対応は `TestColumnsToRowOrder` で検証した。出力は既存の `runList` → `printRowsOrGroupBy` → `util.TableFormatter` の経路で、`-o csv` は root の永続フラグから `cfg.Output` に載る (整形自体の既存テストは `backend/internal/util/formatter_test.go` の `TestTableFormatter_PrintHeader_And_PrintRows_Table` / `TestTableFormatter_PrintHeader_CSV` / `TestTableFormatter_PrintRows_CSV`、`printRowsOrGroupBy` の tab / csv の分岐は `backend/internal/cli/athena_test.go` の `TestAthenaTableListOutput` が固定する)。実 AWS への接続を伴うため、実データでの出力確認は行っていない (`example/` の floci 環境に ELB のシードが無い)。
- 「`mise run check` が通過する」: 統合後の作業ツリーで実行し終了コード 0。ベースライン (失敗テスト無し) からの新たな失敗は無い。

### 方針からの乖離 (方式は保ったままの実装詳細)

- issue の背景と設計判断が挙げる `ListELBTargetHealth` は存在せず、調査結果 (Step 3 で追記) のとおり `DescribeELBTargetHealth` を使った。
- `rules` の対象指定は `--listener <listener ARN>` とした。設計判断の「`--lb` / `--target-group` などのフラグ」の範囲内で、`ListELBRules` がリスナー ARN を受け取るため。
- `--lb` は名前と ARN の両方を受け付ける (調査結果どおり)。名前の解決には `DescribeLoadBalancers` が 1 回増え、ARN 指定では増えない。
- 親の `elb` に `Args: cobra.NoArgs` を付けた。従来は位置引数を無視して一覧を出していたが、サブコマンドを持つようになったため、綴りを誤ったサブコマンド名が一覧の出力に化けないようエラーにする。リポジトリ内に `thief elb <引数>` の使い方は無い。
- 表示列は Web の Drawer の Listeners タブ (リスナーとルール) と Targets タブ (ターゲットグループとターゲットヘルス) の表を基に CLI 向けに絞った。listeners と rules と target-groups には ARN 列を含め、`rules --listener` と `target-health --target-group` に渡す値を一覧から取れるようにした。

### 多観点レビューの反映

ラウンド 1 (3 観点) で高 1 件、中 0 件、低 2 件が出た。

- 高 (テストと堅牢性): 4 サブコマンドの `RunE` がテストで一度も本体に入らず、どのフラグをどの取得関数へ渡しどの列定義で出力するかの結線が検証されていなかった (`TestELBSubcommandsRequireTargetFlag` はフラグ検証で止まる)。取得関数を `elbOps` に束ねて `newELBCmdWithOps` で差し替えられるようにし、`TestELBSubcommandOutput` で実コマンドツリーの実行から標準出力まで固定した。指摘にあった `,` を含む条件の CSV 引用もこのテストで固定した。
- 低 (テストと堅牢性): 親の `elb` が位置引数を受け付けるため `thief elb listener` のような綴りの誤りが一覧の出力に化けていた。`Args: cobra.NoArgs` を付け、`TestELBRejectsUnknownSubcommand` で固定した。
- 低 (規約と整合): CHANGES.md エントリの「Web の ELB Drawer の 4 タブに当たる情報」が実際の Drawer (Overview / Listeners / Targets / Tags) と合っていなかった。Listeners タブと Targets タブの 4 種類の情報という表現に直し、乖離記録の列の説明も同じく直した。issue 本文の同じ言い回しは既存の記述なので書き換えない。
- 却下: 無し。

追加レビュー 1 (3 観点) で高 0 件、中 0 件、低 1 件が出た。

- 低 (テストと堅牢性): 5 つの取得関数を `elbOps` へ寄せた際に親の `elb` の `RunE` も `ops.listLoadBalancers` へ変わったが、親の `RunE` を実行するテストが無く、`defaultELBOps()` の各フィールドが埋まっていることもどのテストも見ていなかった。`TestELBSubcommandOutput` に `thief elb -o csv` のケースを足して親の出力まで固定し、`TestDefaultELBOpsWired` で 5 フィールドが非 nil であることを固定した。
- 却下: 無し。

追加レビュー 2 (3 観点、追加レビューの上限) で高 0 件、中 0 件、低 0 件。反映すべき指摘は無く、実装とテストは追加レビュー 1 の反映後の状態 (`mise run check` 通過) のまま確定した。
