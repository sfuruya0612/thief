# ゴールデン JSON で backend の JSON タグと frontend の Raw 型の契約テストを実装する

Created: 2026-08-05
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

docs/issues/0099 の調査で決定した設計 (対応方針の案 2、ゴールデン JSON) を実装する。
4 案の比較と却下理由、契約対象の対応表 (62 行) は docs/issues/0099 の「調査結果 (2026-08-05)」と「決定した設計」を正とする。
本 issue は単体で実装に着手できるよう要点を再掲する。

backend の API レスポンス形状は `backend/internal/aws/` の構造体の JSON タグが定め、frontend は `frontend/src/types/aws.ts` と `frontend/src/types/query.ts` の Raw インターフェースで受ける。
契約の対象は 60 対で、backend 側は internal/aws の契約対象 60 型のうち Raw の対応を持つ 59 型と `backend/internal/api/models.go` の `ValueResponse`、frontend 側は `aws.ts` の Raw 52 個と `query.ts` の Athena Raw 8 個である。
`SSOAccountResource` と `SSMValueResponse` は frontend に消費者が無いため対象外、`common.ts` の Raw 4 個と `nonaws.ts` の Raw も対象外である。
現状は両者の対応を検証する仕組みが皆無で、片側だけの変更に実行時まで気付けない。

docs/issues/0109 が契約対象の 4 構造体 (`WAFResource` 等) にフィールドを追加するため、本 issue は issue 0109 の完了後に行う。
先に実装した場合は 0109 の実装時にゴールデンの再生成が必要になる。

実装の前提となる既存の事実は次のとおり。

- API レスポンスにエンベロープは無い。`serveCached` (`server.go:125-145`) から `writeJSON` (`handlers_aws.go:406-409`) が構造体をそのままエンコードする。
- `encoding/json` は nil のスライスと map を null に、空を `[]` と `{}` にエンコードし、`omitempty` はゼロ値のキーを落とす。キー集合を固定するには全フィールドに非ゼロ値を入れる必要がある。
- frontend の `tsconfig.json` は `resolveJsonModule` が有効 (10 行) で、include は `src` 全体 (20 行)。`npm run lint` の `tsc --noEmit` がテストファイルも型検査する。
- TypeScript の構造的部分型では、単純な代入可能性や `satisfies` の検査は余分なキーを許すため、backend 側にだけあるフィールド (Raw の追加漏れ) を検出できない。キー集合の双方向一致は型レベルの検査 (両方向の `keyof` の包含を検査する utility type) で行う必要がある。

## 対応方針

- backend に契約テスト用のパッケージを追加し、契約対象 60 型と出力ファイル名の対応をコード上のレジストリ (スライス) として定義する。対応表は docs/issues/0099 の 62 行の表 (対応なしの `SSOAccountResource` と `SSMValueResponse` を除く 60 行) を転記する。親構造体のゴールデンに埋め込みとしても現れる型 (例: `DynamoKeyAttribute`) も単独の行として残し、各 Raw 型にキー集合検査のゴールデンを 1 対 1 で対応させる。検査対象はキー集合と型の適合であり値の同一性ではないため、埋め込み側と単独側の生成値の一致は要求しない。
- reflect で構造体の全フィールドに決定的な非ゼロ値を埋めるフィラーを実装する。値はフィールドの型と順序から決定的に導出し、`time.Time` は固定値を使う。乱数と現在時刻は使わない。ネストした構造体は再帰し、スライスは 1 要素、map は 1 エントリ、ポインタは指す先を生成して埋める。`json:"-"` のフィールド (例: `WAFResource.ARN`、`waf.go:27`) はエンコードに現れないため埋めずに飛ばす。
- 各型のインスタンスを `json.MarshalIndent` でエンコードした結果をゴールデンファイルとして `frontend/src/types/__contract__/` にコミットする。backend の go test が、生成結果とコミット済みファイルを比較し、差分があれば失敗する。環境変数 (例: `UPDATE_GOLDEN=1`) を付けた実行でゴールデンを再生成する。
- frontend に契約検査ファイルを追加し、ゴールデン JSON を import して、各 Raw 型との間でキー集合の双方向一致と各フィールドの型の適合を型レベルで検査する。ネストした構造体はトップレベルのキー検査だけでは内部の不整合を素通しするため、各階層のキー集合の差分を再帰的に検査する utility type を書き、全階層で検査する。不一致は `tsc --noEmit` (`npm run lint` に含まれる) のエラーとして現れる。vitest の実行時アサーションではなく型検査で落とすのは、検出を lint に前倒しし、テスト実行を待たずに気付けるようにするためである。Raw 側が意図的に optional を宣言するフィールド (backend が `omitempty` を付けるフィールド。docs/issues/0109 が追加する 7 つの `*_fetch_failed` フラグが該当する) も検査に含める。フィラーが非ゼロ値を入れるためゴールデンにはキーとして必ず現れ、`keyof` は optional のキーも含むため、キー集合の双方向一致は optional の宣言に影響されずに成立する。型の適合はゴールデンの値の型が Raw のフィールド型から undefined を除いた型に適合するかで検査する。
- Raw 側がリテラルユニオンで backend 側が string のフィールドは、フィラーの生成値がユニオン外になるため型適合の検査を通らない。契約対象の範囲で該当するのは `PriceRateRaw.model` (`aws.ts:1078` の `PriceModel` ユニオン。backend 側は `PriceRate.Model` の string、`pricing.go:50`) のみである。該当フィールドは型適合の検査から除外し (キー集合の検査には含める)、除外一覧を理由付きの表として契約検査ファイルに残す。
- 検出できない不整合として null 許容 (`| null`) の過不足がある。ゴールデンは全フィールドに非 null の値を持つため、backend が null を返し得るかどうかは検査に現れない。backend が `omitempty` を付けるフィールドを Raw 側が必須 (optional でない) と宣言する食い違いも、ゴールデンにキーが常に現れるため検出できない。ゼロ値で生成した第 2 のゴールデンで optional の宣言を強制する案は、ゴールデンと検査の層が倍になる保守負担が見合わないため採らない。これらの限界は docs/issues/0099 の「決定した設計」のとおり受け入れ、`omitempty` と optional の対応付けはフィールドを追加する issue 側のレビューで担保する。
- backend のゴールデン生成と frontend の型検査を別 issue に分ける案は採らない。両者は同一のゴールデンファイルの形状とレジストリを共有し、片方だけ close してもゴールデンが検査されないまま残るか検査対象の無い検査ファイルになるかで、契約の検証が成立しないためである。
- ゴールデンの置き場所 `frontend/src/types/__contract__/` を prettier の整形対象から除外する。Go の `MarshalIndent` の整形と prettier の整形は一致するとは限らず、`mise run fmt` がゴールデンを書き換えると backend のテストと矛盾するためである (一致するかは未検証であり、完了条件の対照確認で確かめる)。prettier は frontend ディレクトリを起点に実行されるため、`frontend/.prettierignore` に追記するエントリは frontend からの相対の `src/types/__contract__/` とする。
- 新規依存は追加しない。go test、tsc、vitest の既存基盤だけで実装する。

## 完了条件

- 契約対象 60 型のゴールデン JSON が `frontend/src/types/__contract__/` にコミットされている。
- backend の go test が、構造体の JSON タグの変更 (フィールドの追加、削除、改名、`omitempty` の増減) でゴールデンとの差分を検出して失敗する。
- フィラーの生成が決定的であること (同一コードで 2 回生成した結果が一致すること) を backend のテストが検証する。
- `UPDATE_GOLDEN=1` を付けた go test の手動実行でゴールデンが再生成されることを確認し、確認した事実を close 時に本 issue の「解決方法」に記録する。
- frontend の型検査が、Raw のフィールドの削除、改名、型変更で失敗する。backend 側だけのフィールド追加も、ゴールデン再生成後の型検査で失敗する。Raw 側だけのフィールド追加 (backend に対応が無いフィールド) も失敗する。ネストした構造体 (例: `DynamoTableSchemaRaw` の内部) のフィールドの不一致も失敗する。
- docs/issues/0109 が追加した 7 つの `*_fetch_failed` フラグが該当するゴールデンにキーとして現れ、Raw 側の optional のフラグフィールドがキー集合の検査に含まれている。
- 検査から除外したフィールドの一覧が理由付きで契約検査ファイルに存在する。
- `frontend/.prettierignore` に `src/types/__contract__/` のエントリが追加されており、`mise run fmt` がゴールデンを書き換えない。書き換えないだけでは両者の整形が偶然一致している場合と区別できないため、エントリを一時的に外して `mise run fmt` を実行し、ゴールデンに差分が出ることを対照として確認してから元に戻す。差分が出ない場合は、両者の整形が現時点で一致している事実を close 時に「解決方法」に記録し、エントリは将来の不一致に備えて残す。
- 新規依存が追加されていない。
- 検証として、backend の構造体と frontend の Raw のそれぞれに一時的な変更を入れ、対応する検査が失敗することを確認してから元に戻す。この手順が、go test がタグの変更で失敗すること、型検査が Raw の変更で失敗すること、backend 側だけのフィールド追加がゴールデン再生成後の型検査で失敗することの 3 つの完了条件の確認手段であり、失敗を起こす変更をテストコードとして常設することは求めない。backend 側はフィールドの追加、削除、改名、`omitempty` の増減の 4 パターン、frontend 側は Raw のフィールドの削除、改名 (docs/issues/0109 の 7 つの `*_fetch_failed` フラグのいずれか 1 つの改名を含む)、型変更、ネストした構造体のフィールドの改名、Raw 側だけのフィールドの追加の 5 パターンを、それぞれ 1 回以上試す。backend 側のフィールド追加のパターンでは、go test の失敗を確認した後に `UPDATE_GOLDEN=1` でゴールデンを再生成し、frontend の型検査が失敗することも確認する。加えた変更と観測した失敗の内容は、close 時に本 issue の「解決方法」に記録する。
- `SSOAccountResource`、`SSMValueResponse`、`common.ts` の Raw、`nonaws.ts` の Raw は対象に含めない。
- `mise run check` が通る。

## 実装詳細の乖離の記録 (2026-08-07)

完了条件は go test が `omitempty` の増減で失敗することを求めるが、対応方針のゴールデン JSON の比較だけではこれを検出できない。
フィラーが全フィールドに非ゼロ値を入れるため、`omitempty` の有無はエンコード結果を 1 バイトも変えないからである。
そこで方式 (ゴールデンの比較と `UPDATE_GOLDEN=1` による再生成) は保ったまま、レジストリ全型のフィールド名と json タグの一覧を第 2 のゴールデン (`backend/internal/contract/testdata/tags.golden`) として比較するテストを追加した。
対応方針が却下した「ゼロ値で生成した第 2 のゴールデン」案とは異なり、このゴールデンは backend 専用のテキストで frontend の検査の層を増やさない。

frontend 側では、`PriceRateRaw.model` の除外を型適合の検査のスキップではなく、`model` の型を string に広げた検査用の型 (`Omit` と交差の合成でキー集合は同一) で実現した。
除外キーを検査の utility type に引数として渡す案は、`PriceTableRaw` の `rates[].model` のようにネストの内側に現れる同じフィールドへ除外が届かないため採らなかった。

## 関連

- docs/issues/0099: 設計の決定元。4 案の比較と契約対象の対応表 (62 行) を持つ。
- docs/issues/0109: 契約対象の構造体にフィールドを追加する先行 issue。本 issue はその完了後に行う。
- docs/issues/closed/0092: 0099 の起票元。

## 解決方法

- backend に `internal/contract` パッケージを追加し、契約対象 60 型とゴールデンファイル名の対応をレジストリ (`Registry`) として定義した。docs/issues/closed/0099 の対応表 62 行のうち frontend に Raw の対応を持つ 60 行を転記した。
- reflect で構造体の全フィールドに決定的な非ゼロ値を埋めるフィラー (`Fill`) を実装した。値は走査順の連番から導出し、`time.Time` は固定値を使う。`json:"-"` のフィールドと未エクスポートのフィールドは飛ばし、未対応の型ではエラーを返す。
- 各型の生成結果を `json.MarshalIndent` でエンコードした 60 個のゴールデン JSON を `frontend/src/types/__contract__/` に置き、backend の `TestGolden` がコミット済みファイルとの一致を検証するようにした。`UPDATE_GOLDEN=1 go test ./internal/contract/` の手動実行で 60 ファイルが再生成されることを確認した。
- フィラーの決定性 (同一コードで 2 回生成した結果の一致) を `TestFillDeterministic` が、レジストリの型名の重複と件数を `TestRegistryNamesUnique` が、レジストリに対応しないゴールデンの取り残しを `TestGoldenDirHasNoStrayFiles` が検証する。`Fill` が対応しない型のフィールドでフィールド経路を含むエラーを返すことは `TestFillUnsupportedKind` が検証する。
- `omitempty` の増減はフィラーが全フィールドに非ゼロ値を入れるためゴールデン JSON の出力を変えない。完了条件が求めるタグの変更そのものの検出のため、レジストリ全型のフィールド名と json タグの一覧を第 2 のゴールデン `backend/internal/contract/testdata/tags.golden` として `TestTagsGolden` が比較するようにした (本文「実装詳細の乖離の記録」参照)。
- frontend に `src/types/contract.check.ts` を追加し、60 個のゴールデンを import して各 Raw 型との間でキー集合の双方向一致と各フィールドの型の適合を再帰的な utility type で検査するようにした。不一致は `tsc --noEmit` (`npm run lint` に含まれる) のエラーとして現れる。
- `PriceRateRaw.model` (と `PriceTableRaw` の `rates[].model`) は、`model` の型を `string` に広げた検査用の型で型適合の検査から除外した (キー集合の検査には含まれる)。除外の一覧は理由付きの表として `contract.check.ts` の冒頭コメントに置いた。
- docs/issues/0109 が追加した 7 つの `*_fetch_failed` フラグが `DynamoResource` (1 つ)、`IAMResource` (3 つ)、`SQSResource` (1 つ)、`WAFResource` (2 つ) のゴールデンにキーとして現れ、Raw 側の optional の宣言がキー集合の検査に含まれることを確認した。
- 検証として backend 側 4 パターンを一時的に加えて元に戻した。`SQSResource` へのフィールド追加 (`json:"temp_contract"`) で `TestGolden` と `TestTagsGolden` が失敗し、`UPDATE_GOLDEN=1` の再生成後に frontend の型検査が `keys only in golden` (`temp_contract`) で失敗した。`RetentionDays` の削除と `in_flight` の改名 (`in_flight_v2`) はいずれも `TestGolden` と `TestTagsGolden` が失敗した。`omitempty` の増減 (`cost_monthly` へ付与、`tags_fetch_failed` から除去) は `TestGolden` を通過し `TestTagsGolden` が失敗した。
- 検証として frontend 側 5 パターンを一時的に加えて元に戻した。`SQSRaw.retention_days` の削除は `keys only in golden` で、`tags_fetch_failed` の改名 (`tags_fetch_failed2`) は `keys only in golden` で、`available_messages` の型変更 (`number` から `string`) は `field type mismatch` で、`DynamoKeyAttributeRaw.name` の改名 (`name2`) は単独の検査に加え `DynamoIndexSchemaRaw` と `DynamoTableSchemaRaw` のネストの検査でも、`SQSRaw` だけへの `temp_only` の追加は `keys only in raw` で、それぞれ `tsc --noEmit` が失敗した。
- `frontend/.prettierignore` に `src/types/__contract__/` を追加した。対照確認としてエントリを一時的に外して `mise run fmt` を実行すると 10 ファイルに差分が出た (prettier は印字幅に収まるネストした配列を 1 行に畳むが、Go の `MarshalIndent` は常に展開する)。確認後にエントリを戻し、ゴールデンを再生成して元の内容に一致することを確認した。
- 新規依存は追加していない。
- `mise run check` が通ることを確認した。
