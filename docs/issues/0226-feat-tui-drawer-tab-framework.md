# TUI の Drawer にタブの共通基盤と単純なサービス固有タブを追加する

Created: 2026-10-26
Model: DeepSeek V4.1 Flash 1017

## 背景

`docs/issues/TODO.md` の次の項目に由来する。

> TUI の Drawer にサービス固有タブを置くための共通のタブモデルを追加し、単純な一覧タブ (ECR Images、ECS Services、CloudFront Behaviors、CloudFormation Events / Resources、RDS Cluster Parameters、ElastiCache Parameters) を表示できるようにしたい

現在の実装は次のとおり。

- `backend/internal/tui/model.go` の `drawerTab` は `tabOverview` と `tabTags` の 2 値だけで、`drawerTabTitles` も `Overview` と `Tags` の固定配列である。`refreshDrawer` は選択中の行とこの 2 値から Drawer の文字列を組み立てる。
- `backend/internal/tui/service.go` の `Service` は `ID`、`Title`、`Columns`、`Load`、`SupportsSession` だけを持ち、`Row` は `ID`、`Name`、`State`、`Cells`、`Overview`、`Tags` を持つ。サービス固有タブの取得関数を差し込む口が無い。
- Web UI の `frontend/src/components/Drawer/Drawer.tsx` の `DRAWER_TABS` はサービスごとに `Overview`、サービス固有タブ、`Tags` を並べている。ECR だけは `Overview` と `Images` で、`Tags` を持たない。
- TUI の Drawer は `bubbles/viewport` に文字列を入れるだけなので、タブごとに非同期取得を行い、読み込み中、失敗、空の状態を区別し、キー操作を扱う仕組みが無い。

サービス固有タブを `model.go` の文字列分岐で追加していくと、タブごとの状態とキー操作が Model に集中し、ECS Tasks の詳細ペインや S3 の階層移動を足しにくくなる。まずタブをサブモデルとして扱う共通基盤を作り、単純な一覧タブから移行する。

## 目的

TUI の Drawer が Web UI と同じ順序でサービス固有タブを表示できる共通基盤を作る。この issue ではタブごとの非同期取得、読み込み中、失敗、空の状態の区別、キー操作を共通の仕組みで扱い、単純な一覧タブとして ECR Images、ECS Services、CloudFront Behaviors、CloudFormation Events / Resources、RDS Cluster Parameters、ElastiCache Parameters を追加する。残りのサービス固有タブは後続の issue で同じ基盤に追加する。

## 設計判断

- Drawer のタブを `Tab` インターフェースを満たすサブモデルにする。
  - `Tab` は `Init() tea.Cmd`、`Update(msg tea.Msg) (Tab, tea.Cmd, *TabIntent, bool)`、`View(width, height int) string`、`SetSize(width, height int)` を持つ。
  - `TabSpec` は `ID`、`Title`、`New(ctx, profile, region string, row Row) Tab` を持ち、`Service` が任意で実装する `TabProvider.Tabs() []TabSpec` から返す。
  - `TabProvider.Tabs()` はそのサービスで現在実装済みのタブの並びを返す。後続の issue がタブを追加するときは、このメソッドに `TabSpec` を追記する。`Overview`、`Tags`、`Terminal` は `New` が nil の組み込みタブで、Model が `ID` を見て既存の Drawer 描画または接続案内を出す。サービス固有タブだけが `New` を持つ。ECR のように `Tags` を持たないサービスは、その並びから `Tags` を外す。
  - `TabSpec.New` は `Row` を受け取る。各サービスの `Tabs()` は同じパッケージの具象行型（`rdsRow`、`s3Row` など）へ型アサーションし、private フィールドから `ClusterID`、`ParameterGroup`、`Behaviors`、`Scope`、`bucketRegion()` を読む。`Row` インターフェースにタブ専用のメソッドを増やす案は、全サービスの行型に無関係なメソッドを足すため採らない。
  - `TabIntent` はタブから Model への要求（通知行の表示、ECS Exec の開始）を表す。タブが `Deps` を直接呼ぶ案は、タブのテストが AWS のモックに依存するため採らない。
  - タブの `tea.Cmd` は Model が `tabInstance` で包み、タブ切替、行変更、再取得の後に届いた古い結果を捨てる。`wrapTabCmd` は常にトップレベルの `tea.Cmd` を包む。`tea.Batch` が 2 件以上のコマンドを返す場合は `tea.BatchMsg` になるため、`wrapTabCmd` が `tea.BatchMsg` を型スイッチし、内側の各 `tea.Cmd` を同じ `tabInstance` で再帰的に包んで `tea.Batch` を返す。`tea.Batch` が 1 件のときはそのコマンドを、0 件のときは nil を返すため、トップレベルの包みで世代が付く。タブは `tea.Sequence` を返さず、複数のコマンドは `tea.Batch` で返す。`tea.Sequence` の `sequenceMsg` は非公開で再帰的に包めないためである。
  - フロートセッションが開いている間は、issue 0225 のセッションのキー処理をタブと Model のグローバル操作より先に行う。セッション中は `Ctrl-]` だけをウィンドウ操作に使い、それ以外のすべてのキーをセッションへ送る。`Ctrl-C`、`q`、通常文字、数字、矢印、`tab`、`s`、`r` もタブやグローバル操作へ流さない。フロートセッションが開いていないときの `ctrl+c` は Model がタブへ渡す前に処理し、常に TUI の終了にする。それ以外のキーはタブが先に処理し、処理しなかったキーだけを Model のグローバル操作へ流す。フォーム入力中の `q` や数字を TUI の終了やタブ切替に奪われないようにするためである。
  - タブを `model.go` の文字列分岐で追加する案は採らない。タブごとの読み込み状態、スクロール、将来の行選択を Model に集中させると、`model.go` がタブの数だけ分岐を持つ。`Tab` サブモデルに閉じ込める。
  - `TabProvider` がサービス固有タブだけを返し、Model が `Overview` と `Tags` を自動で前後に挿入する案は採らない。ECR のように `Tags` を持たないサービスと、EC2 / ECS の `Terminal` の位置を Model の条件分岐で表すことになり、Web UI の `DRAWER_TABS` との対応が読みにくくなる。`TabProvider` は現在実装済みのタブの並びを返す。
- タブ本文の表示は `TabContent` のブロックで表す。
  - `TabContent` は `Title`、`KV`、`Table`、`Text` を持つ `TabBlock` の列とする。`Table` の型は `TabTable` とする。
  - `Table` は `Columns []Column` と `Rows [][]string` を持ち、列幅は見出しと値から決め、Drawer の内側の幅に収まるよう広い列から縮める。既存の `tableColumns` と `shrinkToFit` の計算を共通関数に切り出す。
  - `Column` に任意の `Align` を足し、タブの表の描画器だけが数値列を右寄せする。既存の一覧を描く `bubbles/table` は `Align` を解釈しないため、既存の一覧の寄せは変えない。
- 単純な一覧タブは `contentTab` という 1 つの `Tab` 実装で扱う。`contentTab` は `viewport` に `TabContent` を描画し、上下キー、`j` / `k`、PageUp / PageDown、Home / End でスクロールする。
- タブバーは Model が描画する。`1` から `9` で直接選択し、`[` と `]` で前後に移動する。タブ見出しには番号を付け、現在のタブを強調する。`tab` は従来どおり一覧と Drawer のフォーカス切替に使う。
  - サービスを切り替えたら `Overview` に戻す。行を切り替えたときは、同じサービスに現在のタブがあればそのタブを保ち、無ければ `Overview` に戻す。
  - タブ数より大きい数字は無視する。`[` は先頭、`]` は末尾で止める。ラップしない。
  - フッタの `1/2 tabs` という固定の案内を、`1-<n> tabs` と `[ ]` の案内に置き換える。
  - タブバーが見出しの合計幅で Drawer の内側に収まらないときは、先頭から `ansi.Truncate` で詰め、現在のタブが見出しに含まれない場合は `… | <active>` の形で現在のタブを末尾に出す。フッタの `1-<n> tabs` と `[ ]` は `MaxWidth` で切らず、狭い端末では短い `1-<n>` と `[ ]` に縮める。
- この issue は issue 0225 の完了後に実装する。`Terminal` タブの `s` の開始処理と、フロートセッション中のキー優先順位は issue 0225 の実装に依存する。0225 が未完了の状態で Terminal タブだけを先に追加するフォールバックは採らない。
- タブの並びは Web UI の `DRAWER_TABS` と同じにする。
  - EC2: `Overview`、`Terminal`、`Tags`
  - ECR: `Overview`、`Images`（`Tags` は出さない）
  - RDS: `Overview`、`Instance Parameters`、`Cluster Parameters`、`Tags`
  - ElastiCache: `Overview`、`Parameters`、`Tags`
  - ECS: `Overview`、`Services`、`Tasks`、`Instances`、`Terminal`、`Tags`
  - S3: `Overview`、`Objects`、`Tags`
  - ELB: `Overview`、`Listeners`、`Targets`、`Tags`
  - CloudFront: `Overview`、`Behaviors`、`Tags`
  - WAF: `Overview`、`Rules`、`Tags`
  - DynamoDB: `Overview`、`Items`、`Tags`
  - Secrets Manager: `Overview`、`Value`、`Tags`
  - Parameter Store: `Overview`、`Value`、`Tags`
  - CloudFormation: `Overview`、`Events`、`Resources`、`Tags`
  - Lambda、IAM、API Gateway、NAT Gateway、SQS、Kinesis: `Overview`、`Tags`
  この並びは最終形である。この issue が追加するのは EC2 / ECS の `Terminal`、ECR の `Images`、ECS の `Services`、CloudFront の `Behaviors`、CloudFormation の `Events` と `Resources`、RDS の `Cluster Parameters`、ElastiCache の `Parameters` である。ECS の `Tasks` と `Instances`、S3 の `Objects`、ELB の `Listeners` と `Targets`、WAF の `Rules`、DynamoDB の `Items`、Secrets Manager と Parameter Store の `Value`、RDS の `Instance Parameters` は後続の issue で追加する。後続の issue が追加するまでのタブバーには、そのサービスの実装済みのタブだけを表示する。
- `Overview` と `Tags` は Model の既存の組み込みタブとして残し、`Terminal` はこの issue で組み込みタブとして追加する。`Terminal` は接続先の説明と `s` の案内を表示し、セッションの開始は Model が行う。`Terminal` タブは TODO の単純な一覧タブではないが、Web UI の `DRAWER_TABS` との一致と、issue 0225 のフロートセッションへの導線のため、この issue で追加する。この issue は issue 0225 の完了後に実装するため、`Terminal` は常に `s` の開始処理を持つ。
- この issue で追加する単純な一覧タブの取得元は次のとおり。
  - ECR Images: `aws.ListECRImages(ctx, profile, region, repositoryName)`。列は Web UI の `ecrImageColumns` と同じ Tag、Digest、Size、Pushed、Pulled とする。
  - ECS Services: `aws.ListECSServices(ctx, profile, region, clusterName)`。列は `ecsServiceColumns` と同じ Service、State、Desired、Running、Pending、Launch type、Task definition とする。
  - CloudFront Behaviors: `CloudFrontResource.Behaviors`。列は Web UI の `cloudfrontBehaviorColumns` と同じ Precedence、Path pattern、Target origin、Viewer protocol、Allowed methods、Compress とする。既定ビヘイビアは Path pattern に `Default (*)` を表示する。CloudFront は行のデータだけで描画し、追加の API 呼び出しをしない。
  - CloudFormation Events: `aws.ListCFNStackEvents(ctx, profile, region, stackName)`。列は `cfnEventColumns` と同じにする。
  - CloudFormation Resources: `aws.ListCFNStackResources(ctx, profile, region, stackName)`。列は `cfnResourceColumns` と同じにする。
  - RDS Cluster Parameters: `RDSResource.ClusterID` が空なら `Not part of a DB cluster.` を表示し、AWS を呼ばない。空でなければ `aws.ListRDSClusterParameters(ctx, profile, region, clusterID)` を呼ぶ。列は `rdsParameterColumns` と同じにする。
  - ElastiCache Parameters: `ElastiCacheResource.ParameterGroup` が空なら `No parameter group.` を表示し、AWS を呼ばない。空でなければ `aws.ListElastiCacheParameters(ctx, profile, region, parameterGroup)` を呼ぶ。列は `cacheParameterColumns` と同じにする。
- 各サービスのコンストラクタにタブ用の取得関数を足す。タブ用の取得関数が 3 つ以上になるサービスは `xxxTabDeps` という小さな構造体にまとめる。`backend/internal/cli/tui.go` の `newTUIDeps` で `aws.ListXxx` を配線し、`backend/internal/cli/tui_test.go` の関数の同一性の確認を更新する。
- 追加の API 呼び出しは上記の一覧取得だけで、Web UI が既に使う権限の範囲に収まる。新しい IAM 権限は要らない。
- 既存の `Tags` タブの取得 (`TagFetcher`、`TagsFetchFailer`) は変えない。ECR のように `Tags` タブを持たないサービスではタグ取得を開始しない。
- この issue では対話型の一覧と詳細を持つタブ、S3 の階層移動、DynamoDB の検索フォーム、Value の表示は扱わない。それぞれ後続の issue で追加する。

## 完了条件

- `backend/internal/tui/service.go` に `Tab`、`TabSpec`、`TabIntent`、`TabProvider`、`TabContent`、`TabBlock`、`TabTable` の型があり、`Service` の任意実装として `TabProvider.Tabs() []TabSpec` を返せる。
- `backend/internal/tui/model.go` の `drawerTab` と `drawerTabTitles` が無くなり、選択中の `Service` の `TabProvider` からタブの並びを得る。`TabProvider` を実装しないサービスは `Overview` と `Tags` を既定にする。
- `TabProvider.Tabs()` が返す現在実装済みの並びのうち、`New` が nil の `Overview`、`Tags`、`Terminal` は Model が組み込みで描画し、`New` があるサービス固有タブだけを `Tab` サブモデルとして動かす。ECR の並びには `Tags` が無い。後続の issue がタブを追加するときは `Tabs()` に追記する。
- タブの `tea.Cmd` が `tabInstance` で包まれ、タブ切替、行変更、`r` の再取得の後に届いた古い結果が表示にも保持にも使われない。`tea.Batch` を返すコマンドは `tea.BatchMsg` を型スイッチして内側のコマンドも再帰的に包まれる。
- フロートセッションが開いていないときの `ctrl+c` はタブへ渡す前に Model が処理して TUI を終了する。フロートセッションが開いている間は issue 0225 のセッションのキー処理が先に行われ、`Ctrl-]` 以外のすべてのキーがセッションへ送られる。タブやグローバル操作へは流れない。それ以外のキーはタブが先に処理し、処理しなかったキーだけが `q`、`1` から `9`、`[`、`]`、`tab`、`s`、`r`、`p`、`R`、`S` のグローバル操作に届く。フォーム入力中の `q` と数字が TUI の終了やタブ切替を起こさない。
- この issue が追加するタブ（ECR Images、ECS Services、CloudFront Behaviors、CloudFormation Events / Resources、RDS Cluster Parameters、ElastiCache Parameters、EC2 と ECS の Terminal）が、この時点で実装済みのタブの中で Web UI の `DRAWER_TABS` と同じ相対順序に表示され、`1` から `9` と `[` / `]` で切り替えられる。ECR では `Tags` が表示されない。
- サービスを切り替えると `Overview` に戻り、行を切り替えたときは同じサービスに現在のタブがあればそのタブを保ち、無ければ `Overview` に戻る。タブ数より大きい数字は無視され、`[` は先頭、`]` は末尾で止まる。フッタに `1-<n> tabs` と `[ ]` の案内が表示される。
- RDS Cluster Parameters タブは、取得した `RDSClusterParameterGroup.GroupName` を見出しに表示する。
- 上記の単純な一覧タブのうち、CloudFront Behaviors 以外の 6 タブが選択中の行に対して取得関数を呼び、読み込み中、失敗、空、成功の各状態を区別して表示する。CloudFront Behaviors は行の `Behaviors` から描画し、空と成功を区別する。
- RDS Cluster Parameters は `ClusterID` が空のとき `Not part of a DB cluster.` を表示し、`ListRDSClusterParameters` を呼ばない。ElastiCache Parameters は `ParameterGroup` が空のとき `No parameter group.` を表示し、`ListElastiCacheParameters` を呼ばない。
- 取得成功時は `TabContent` の表の合計幅が Drawer の内側の幅以下になり、列の見出しと値が Web UI の同じ列定義の表示文字列になる。`minColumnWidth` まで縮めても収まらないときは右端の列を切り落とし、切り落とした列があることを末尾の `…` で示す。
- タブの取得関数は `backend/internal/aws` の上記の関数をプロセス内で直接呼び、`backend/internal/api` を import しない。
- `backend/internal/cli/tui.go` の `newTUIDeps` が各サービスのタブ用取得関数に `aws.ListECRImages`、`aws.ListECSServices`、`aws.ListCFNStackEvents`、`aws.ListCFNStackResources`、`aws.ListRDSClusterParameters`、`aws.ListElastiCacheParameters` を渡していることを `backend/internal/cli/tui_test.go` で確認している。
- `backend/internal/tui` の単体テストに、この issue が追加するタブの順序、番号と `[` / `]` の切替、サービス切替と行切替でのタブ選択、範囲外の番号の無視、`[` / `]` の端での停止、80 桁 x 24 行と 120 桁 x 30 行でのタブバーとフッタの表示、タブのキー委譲、フロートセッションが無いときの `ctrl+c` がタブより先に終了すること、フロートセッション中の `Ctrl-C` と `q` がリモートへ送られること、`tea.Batch` の 0 件、1 件、2 件以上と内側のコマンドにも `tabInstance` が付くこと、古い結果の破棄、再取得でのタブの作り直し、読み込み中、失敗、空、RDS と ElastiCache の取得対象なし、表の幅の切り詰めと右端の切り落としの各ケースがある。
- 6 つの API 取得タブそれぞれに、偽の取得関数を渡したときに正しい profile、region、リソース識別子で呼ばれ、返した行が表示されることを確かめるテストがある。CloudFront Behaviors には行の `Behaviors` が空のときと値があるときのテストがある。
- `docs/prd/thief.md` の FR-29 がサービス固有タブとタブ操作を必須とし、「TUI の Drawer のサービス固有のタブ」を未確定論点から決定済みへ移している。あわせて TUI の第 1 段階を「Drawer (Overview と Tags)」と書いている箇所（概要、背景と課題、目的、UC-14、スコープ、決定した論点）を、サービス固有タブを含む記述に更新する。「決定した論点」の「TUI の第 1 段階の範囲」の後続の作業列も、サービス固有タブの追加を反映する。ADR 0033 は履歴として残し、本文は書き換えない。
- `docs/prd/thief.md` の FR-29 の受け入れ基準に、その時点で実装済みのタブの並びが `DRAWER_TABS` の相対順序と一致し、未実装のタブは表示されないこと、`1` から `9` と `[` / `]` で切り替えられること、ECR では `Tags` を表示しないことを追加している。全タブの一致は、0227、0228、0229 の完了後に実装する issue 0230 の完了条件で確認する。
- `AGENTS.md` の「プロジェクト構造」の TUI の項目に、タブをサブモデルとして実装すること、タブの取得関数を各サービスのコンストラクタへ渡すこと、`drawerTab` を固定値で増やさないことが追記されている。
- `example/README.md` に、floci で確認できる範囲として CloudFormation Events / Resources、CloudFront Behaviors、ElastiCache Parameters の手順が追加されている。ECR Images と ECS Services は seed にイメージとサービスが無いため空状態を確認する。RDS Cluster Parameters は seed の DB インスタンスがクラスタに属さないため `Not part of a DB cluster.` を確認する。ElastiCache Parameters を floci が返さない場合は、その理由と実 AWS で確認することを明記する。
- `CHANGES.md` の `## develop` に、TUI のサービス固有タブとタブ操作の [ADD] エントリが 1 つある。担当者行は `- @sfuruya0612` とする。
- `mise run check` が通る。
- 扱わない範囲: 対話型の一覧と詳細を持つタブ、S3 の階層移動、DynamoDB の検索フォーム、Value の表示、タブ状態の永続化、マウス操作。

## 関連

- PRD `docs/prd/thief.md` の FR-29
- `docs/issues/0225`、`docs/issues/0227`、`docs/issues/0228`、`docs/issues/0229`、`docs/issues/0230`
- `frontend/src/components/Drawer/Drawer.tsx` の `DRAWER_TABS`
- `frontend/src/components/tables/columns.tsx` の `ecrImageColumns`、`ecsServiceColumns`、`cfnEventColumns`、`cfnResourceColumns`、`rdsParameterColumns`、`cacheParameterColumns`
