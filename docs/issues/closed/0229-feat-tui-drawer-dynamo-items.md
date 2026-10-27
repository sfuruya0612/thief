# TUI の DynamoDB Items タブに検索フォームと項目詳細を追加する

Created: 2026-10-26
Model: DeepSeek V4.1 Flash 1017
Completed: 2026-10-27

## 背景

`docs/issues/TODO.md` の次の項目に由来する。

> TUI の Drawer の DynamoDB Items タブで、既定のスキャン結果、PK / SK / 属性の検索フォーム、取得件数の切替、選択した項目の JSON 詳細を表示できるようにしたい

Web UI の `frontend/src/components/Drawer/DrawerDynamoItems.tsx` は、`backend/internal/aws/dynamo.go` の `DescribeDynamoTable` と `QueryDynamoItems` を使って次の操作を提供している。

- テーブルのキースキーマ（パーティションキー、ソートキー、GSI）を取得し、検索フォームのラベルに使う。
- PK 未指定のときは `Scan` を既定 10 件で 1 回実行し、PK を指定したときは `Query` を使う。属性名と属性値のフィルタと、取得件数 10 / 50 / 100 の切り替えを持つ。
- 取得した項目の属性名を列にし、値を表で表示する。
- 列ごとのフィルタを持つ。

issue 0226 は Drawer のタブ基盤を追加するが、DynamoDB の Items タブは追加しない。TUI からはテーブルの項目を確認できない。

## 目的

TUI の DynamoDB のテーブル行から Items タブを開き、既定のスキャン結果を表で確認できるようにする。PK / SK / 属性の検索と取得件数の切り替えができ、選択した項目の JSON を読めるようにする。

## 設計判断

- この issue は issue 0226 の完了後に実装する。DynamoDB Items タブは issue 0226 の `Tab`、`TabSpec`、`TabProvider`、`TabIntent` と、`Drawer` のキー委譲を使う。
- DynamoDB Items タブは `dynamoItemsTab` として実装する。キースキーマ、現在の検索条件、一覧、選択中の項目、フォームの開閉とフォーカスをタブ内で持つ。
  - 共通の `contentTab` を使う案は採らない。検索フォームの入力、フォーカス移動、動的な列、項目の JSON 詳細を持てない。
  - 検索フォームを常に表示する案は採らない。フォームの入力欄がキーを奪い、一覧の `j` / `k` やタブ切替の操作を妨げる。`f` で開閉する。
  - 項目の詳細を表の行内にすべて出す案は採らない。属性が多い項目で横方向に広がり、Drawer の幅に収まらない。`enter` で JSON の詳細ペインを開く。
- タブを開いたときに `aws.DescribeDynamoTable(ctx, profile, region, tableName)` と `aws.QueryDynamoItems(ctx, profile, region, tableName, DynamoItemQuery{Limit: 10})` を呼ぶ。PK 未指定の `QueryDynamoItems` は `Scan` を 1 回実行するため、既定表示は Web UI のプレビューと同じ 10 件になる。
- 一覧の列は取得した項目の属性名の和集合とし、列の順序はキーを辞書順にソートした順にする。`QueryDynamoItems` は `[]map[string]any` を返すため、Go の map の反復順をそのまま使うと列の順序が実行ごとに変わる。Web UI は行ごとの `Object.keys` の初出順で列を作るため、TUI の辞書順とは一致しない。TUI は実行ごとに列が入れ替わらないことを優先し、Web UI との一致は列の順序ではなくセル単位の値で確認する。
- 値の表示は Web UI の `formatItemValue` に合わせる。セルごとに行のキーの有無を確認し、キーが無いセルは空文字、存在する `nil` は `null`、map と slice は `json.Encoder` の `SetEscapeHTML(false)` でコンパクトにした JSON（末尾の改行を除く）、数値と真偽値は `fmt.Sprint`、文字列はそのまま表示する。`[]byte` は slice より先に判定し、`base64.StdEncoding.EncodeToString` で引用符を付けずに表示する。`encoding/json` で `[]byte` を Marshal すると引用符付きの base64 文字列になり、Web UI の表示と一致しないためである。列が多くて Drawer の幅に収まらないときは、共通の列幅の縮小規則で広い列から縮める。
- 一覧の行を選んで `enter` を押すと、その項目全体をインデントした JSON として表示する詳細ペインが開く。`esc` で一覧に戻る。項目の編集、削除、新規作成は扱わない。
- `f` で検索フォームを開く。フォームはパーティションキーの値、ソートキーの値（スキーマにある場合だけ）、属性名、属性値、取得件数の 5 項目を持つ。
  - フォームが開いている間は、`tab` と `shift+tab` で項目を移動し、`enter` で検索を実行し、`esc` でフォームを閉じる。`ctrl+c` は従来どおり TUI の終了にする。テキスト入力にフォーカスがある間は、`enter`、`esc`、`tab`、`shift+tab`、`ctrl+c` 以外の印刷可能なキーをすべて入力として扱う。`l`、`c`、`f`、`r`、`q`、`j`、`k`、数字も文字として入力する。
  - 取得件数の項目にフォーカスがあるときは、`l` と `←` / `→` で 10、50、100 の順に切り替える。先頭で `←`、末尾で `→` を押したときは止まる。
  - フォームを閉じているときの `l` は、取得件数を 10、50、100 の順に切り替えて現在の検索条件で再取得する。`f` はフォームを開く。`r` は従来どおり一覧の再取得にする。
  - フォームを閉じても確定済みの検索条件と一覧は残す。
- `QueryDynamoItems` には、空でない値だけを `PKValue`、`SKValue`、`AttrName`、`AttrValue`、`Limit` として渡す。SK の値は PK が空でなく、スキーマにソートキーがあるときだけ送る。PK が空のときは Scan 経路になり `SKValue` が使われないため、SK の入力があっても送らない。AttrName が空でなければ AttrValue が空でも `AttrName` と `AttrValue` を送る。Web UI は PK が空でも AttrName だけで検索を実行でき、backend の `QueryDynamoItems` は Scan + FilterExpression を扱う。フォームがすべて空のときは既定の 10 件スキャンを実行する。
- 検索の実行中は一覧の上に読み込み中を表示し、前回の一覧は消さない。検索が失敗したときはフォームと前回の一覧を残し、理由を表示する。
- スキーマの取得に失敗したときは Items タブに理由を表示し、検索フォームのラベルは実際のキー名ではなく `PK` にする。ソートキーの有無が分からないため、SK の入力欄は無効にし、PK だけで検索する。`QueryDynamoItems` には `SKValue` を送らない。項目の取得だけが成功した場合は一覧を表示する。
- バイナリキー（DynamoDB の B 型）は、既存の `backend/internal/aws/dynamo.go` が S 型と同じ文字列として扱う。TUI もその挙動に従い、専用の入力規則を設けない。
- 一覧の取得はタブの context と `loadSeq` で世代管理し、検索条件の変更、タブ切替、行変更、`r` の再取得の後に届いた古い結果を捨てる。
- 列ごとのフィルタ、GSI の選択、項目の編集、追加、削除、ページングは扱わない。列ごとのフィルタは、TUI のキー操作と画面の高さの制約に対して効果が薄く、この issue の完了条件を超える。フォームのクリア操作は Web UI の `handleClear` に倣って設けない。検索条件はタブを閉じるまで保持し、`r` は現在の条件で再取得する。
- 追加の API 呼び出しは `DescribeTable`、`Scan`、`Query` で、Web UI が既に使う権限の範囲に収まる。新しい IAM 権限は要らない。
- 一覧の取得件数は最大 100 件とし、`QueryDynamoItems` の `resolveDynamoItemLimit` の上限に従う。ページングは行わない。

## 完了条件

- TUI の DynamoDB のテーブル行の Drawer に `Items` タブがあり、タブを開くと `DescribeDynamoTable` と `QueryDynamoItems` を呼び、既定 10 件のスキャン結果を表示する。
- 一覧の列は取得した項目の属性名の和集合を辞書順にソートしたもので、実行ごとに順序が変わらない。値の表示は Web UI の `formatItemValue` と同じ文字列になる。キーが無いセルは空文字、存在する `nil` は `null`、map と slice は HTML エスケープを無効にしたコンパクトな JSON、数値と真偽値は文字列、`[]byte` は引用符なしの base64 文字列になる。列が多い場合は Drawer の幅に収まるよう広い列から縮む。
- 一覧の行を選んで `enter` を押すと項目全体のインデントされた JSON が表示され、`esc` で一覧に戻る。
- `f` で検索フォームが開き、`tab` / `shift+tab` で項目を移動し、`enter` で検索を実行し、`esc` で閉じる。フォームを閉じても確定済みの検索条件と一覧が残る。
- 検索フォームで PK の値を指定すると `QueryDynamoItems` の `PKValue` に、PK と SK の値を指定すると `SKValue` に、属性名と属性値を指定すると `AttrName` / `AttrValue` に渡る。PK が空のときは SK の入力があっても `SKValue` を送らない。空の項目は送らない。
- 属性名だけが入力された状態で `enter` を押すと、PK が空でも AttrName と空の AttrValue を持つ `QueryDynamoItems` を呼び、Scan + FilterExpression で検索する。フォームがすべて空のときは既定の 10 件スキャンを実行する。
- 取得件数の項目にフォーカスがあるとき、`l` と `←` / `→` で 10、50、100 の順に切り替わり、先頭と末尾では止まる。フォームを閉じているときの `l` は取得件数を切り替えて現在の検索条件で再取得する。
- フォーム入力中の `l`、`c`、`f`、`r`、`q`、`j`、`k`、数字が文字として入力され、TUI の終了、タブ切替、件数切替、再取得を起こさない。`ctrl+c` は TUI を終了する。
- スキーマ取得の失敗、項目取得の失敗、空の結果、読み込み中が区別して表示される。項目取得の失敗時は前回の一覧とフォームが残る。
- スキーマ取得に失敗したときは PK の入力欄だけが有効になり、SK の入力欄は無効になる。`SKValue` は送らない。
- 検索条件の変更、タブ切替、行変更、`r` の再取得の後に届いた古い結果が表示にも保持にも使われない。
- `backend/internal/tui` の単体テストに、既定スキャン、PK / SK / 属性の検索、AttrName だけの Scan + FilterExpression、PK が空のときに SK を送らないこと、スキーマ失敗時に SK を送らないこと、件数の切替と端での停止、JSON 詳細、取得失敗、空、古い結果の破棄、フォーム入力中の `l` / `c` / `f` / `r` の扱い、キーが無いセルと `nil` の区別、`[]byte` の引用符なし base64、`<` を含む map の HTML エスケープ無効の各ケースがある。
- `backend/internal/cli/tui.go` の `newTUIDeps` が DynamoDB のタブに `aws.DescribeDynamoTable` と `aws.QueryDynamoItems` を渡していることを `backend/internal/cli/tui_test.go` で確認している。
- `example/README.md` に、floci の `thief-example-users` テーブルで既定の取得件数 10 件の設定で 3 件が表示され、`f` の検索、`l` の件数切替、項目の JSON 詳細を確認する手順が追加されている。
- `CHANGES.md` の `## develop` に、TUI の DynamoDB Items タブの [ADD] エントリが 1 つある。担当者行は `- @sfuruya0612` とする。
- `mise run check` が通る。
- 扱わない範囲: 列ごとのフィルタ、GSI の選択、項目の編集、追加、削除、ページング、検索条件の永続化、フォームのクリア操作。

## 関連

- PRD `docs/prd/thief.md` の FR-29
- `docs/issues/0226`
- `frontend/src/components/Drawer/DrawerDynamoItems.tsx`
- `backend/internal/aws/dynamo.go` の `DescribeDynamoTable`、`QueryDynamoItems`、`DynamoItemQuery`

## 解決方法

- `backend/internal/tui/dynamo.go` の `dynamoService` に `DescribeDynamoTable` と `QueryDynamoItems` の取得関数を追加し、`Tabs()` に Items タブを追加した。
- `backend/internal/tui/tab_dynamo.go` に `dynamoItemsTab` を追加した。タブを開くと `DescribeDynamoTable` と既定 10 件の `QueryDynamoItems` を呼び、属性名の和集合を辞書順に並べた列で表示する。
- 値の表示は Web UI の `formatItemValue` に合わせ、キーが無いセルは空文字、存在する `nil` は `null`、map と slice は HTML エスケープを無効にしたコンパクトな JSON、数値と真偽値は `fmt.Sprint`、文字列はそのまま、`[]byte` は引用符なしの base64 にした。
- `f` で PK、SK、属性名、属性値、取得件数の検索フォームを開く。`tab` / `shift+tab` で移動し、`enter` で検索し、`esc` で閉じる。検索中もフォームと前回の一覧を残し、取得に失敗したときは理由を表示する。`r` は現在の検索条件のまま取り直す。
- `SKValue` は PK が空でなく、スキーマにソートキーがあるときだけ送る。スキーマにソートキーが無いときと、スキーマの取得に失敗したときは SK の入力欄を無効にし、フォーカスも飛ばす。スキーマ取得に失敗したときのラベルは `PK` に戻す。
- 取得件数の入力欄にフォーカスがあるときは `l` と `←` / `→` で 10、50、100 を切り替え、端で止まる。フォームを閉じているときの `l` は取得件数を切り替えて現在の条件で再取得する。
- 選択した項目の `enter` で項目全体のインデントした JSON を表示し、`esc` で一覧に戻る。取得中、取得失敗、空、成功を区別し、空のときは `No items` を表示する。
- 一覧の取得はタブの context と `loadSeq` で世代管理し、検索条件の変更、`r` の再取得の後に届いた古い結果を捨てる。表の高さは再取得のたびに縮まないよう、Drawer の高さから確保する。
- `backend/internal/tui` の単体テストに、既定スキャン、PK / SK / 属性の検索、AttrName だけの検索、PK が空のときに SK を送らないこと、スキーマ失敗時とソートキーが無いときに SK を無効にすること、取得件数の切替と端での停止、フォーム入力中のグローバルキー隔離、JSON 詳細、空、失敗時のフォームと前回の一覧の保持、古い結果の破棄、列の和集合と順序、キーが無いセルと `nil` の区別、`[]byte` の引用符なし base64、HTML エスケープ無効、`r` の条件保持を追加した。
- `example/README.md` に floci の `thief-example-users` テーブルで既定 10 件、`f` の検索、`l` の件数切替、項目の JSON 詳細を確認する手順を追加した。`CHANGES.md` に [ADD] エントリを追加した。
- `mise run check` が通る。
