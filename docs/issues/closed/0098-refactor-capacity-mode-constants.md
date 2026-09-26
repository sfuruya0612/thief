# キャパシティモードの文字列リテラルを共通定数化し未知の enum 値のフォールバックを統一する

Created: 2026-08-05
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

TODO.md の「"on-demand" / "provisioned" のリテラルが dynamo.go と kinesis.go に重複しているので共通定数化したい (issue 0092 のスコープ外として送り)」に対応する。
子行の「未知の enum 値に対するフォールバックの実装が両者で非対称になっていないかも併せて確認する」も本 issue の範囲に含める。

キャパシティモードを表す文字列は次の 2 箇所に重複している。

- `backend/internal/aws/dynamo.go` の `dynamoFromDescription` (123-138 行): `BillingModeSummary.BillingMode` が `PAY_PER_REQUEST` なら `"on-demand"`、`PROVISIONED` なら `"provisioned"` を設定する。
- `backend/internal/aws/kinesis.go` の `kinesisFromSummary` (99-110 行): `StreamModeDetails.StreamMode` が `ON_DEMAND` なら `"on-demand"`、それ以外なら `"provisioned"` を設定する。

フォールバックの実装は実際に非対称である。

- kinesis.go は `ON_DEMAND` 以外の値 (未知の enum 値を含む) を全て `"provisioned"` に落とす。この挙動はコメントで意図として明記されている。
- dynamo.go は `BillingModeSummary` が nil のとき `"provisioned"` にするが、`BillingModeSummary` が非 nil で `BillingMode` が既知の 2 値のどちらでもないとき、`Mode` フィールドが空文字列のまま残る。

将来 AWS が新しいキャパシティモードを追加した場合、DynamoDB だけ一覧と Drawer の Mode が空表示になり、Kinesis と挙動が食い違う。
なお frontend の `columns.tsx` の mode セル (1184-1189 行と 1280-1285 行) は `r.mode` をそのまま表示し、空文字列を dash に変換しない。
フォールバックの統一により、仮に未知の enum 値が現れた場合の DynamoDB の表示は空欄から `provisioned` に変わる。
現行 SDK の enum 値ではこのパスに到達しない。

## 目的

キャパシティモードの文字列リテラルを 1 箇所の定数定義に集約し、未知の enum 値に対するフォールバックを DynamoDB と Kinesis で同じ挙動 (`"provisioned"` に縮退) に揃える。

## 設計判断

- 定数は `backend/internal/aws/resource.go` にパッケージ非公開の `capacityModeOnDemand` / `capacityModeProvisioned` として定義する。resource.go は `DisplayState` 等のサービス横断ヘルパーを既に置いているファイルであり、サービス固有ファイル (dynamo.go / kinesis.go) のどちらかに置くともう一方からの参照が不自然になる。定数専用の新規ファイルを作る案は、定数 2 つのためにファイルを増やす価値が無いため採らない。
- 文字列の値 (`"on-demand"` / `"provisioned"`) は変更しない。`"on-demand"` は frontend の `lib/normalize.test.ts` (838 行と 852 行) がフィクスチャとして参照しており、値を変えると frontend 側の変更も必要になって refactor の範囲を超える。`"provisioned"` を参照する frontend コードは無いが、`mode` の値は API レスポンスにそのまま載って表示されるため、片方の値だけ変える理由も無い。
- dynamo.go の未知の enum 値のフォールバックは kinesis.go に合わせて `"provisioned"` に統一する。空文字列のまま残す現状を維持する案は、一覧と Drawer で Mode が空表示になる挙動に利点が無く、kinesis.go がコメントで明記している「未知の値は provisioned に縮退」という方針と食い違うため採らない。
- 引数自体が nil のパス (`dynamoFromDescription` の `t == nil`、`kinesisFromSummary` の `s == nil`) は両者ともゼロ値の構造体を返し、`Mode` は空文字列になる。このパスは API から取得できなかったデータの表現であり、定数化とフォールバック統一の対象外としてそのまま残す。空文字列用の定数は設けない。
- この issue は性質の異なる 2 つの変更 (リテラルの定数化とフォールバックの統一) を含むが、分割しない。TODO の子行が両者を併せて確認することを求めており、どちらも同じ変換関数の switch を触るため、分割すると同一箇所への連続変更になる。フォールバック統一を bug として別に起票する案も採らない。未知の enum 値は現行 SDK (`BillingMode` は `PROVISIONED` / `PAY_PER_REQUEST` の 2 値) に存在せず、観測可能な症状も再現手順も書けないためである。実装時の CHANGES.md では、定数化を `### misc` に、フォールバック統一を `[UPDATE]` として記載する。現実の入力に対する出力が変わらない防御的変更のため `[FIX]` にはしない。
- 追加の API 呼び出しや権限は不要。

## 完了条件

- `"on-demand"` / `"provisioned"` のリテラル定義が `backend/internal/aws` パッケージ内で定数定義の 1 箇所だけになり、dynamo.go と kinesis.go は定数を参照する。
- `dynamoFromDescription` のテーブル駆動テストに「`BillingModeSummary` が非 nil で `BillingMode` が既知の 2 値以外」のケースを追加し、`Mode` が `"provisioned"` になることを検証する。
- `kinesis_test.go` の既存ケース `unknown stream mode falls back to provisioned` が定数化後も通る。
- 引数自体が nil のパスはゼロ値 (`Mode` は空文字列) のまま変更しない。この不変は、`dynamo_test.go` と `kinesis_test.go` の既存の nil 入力ケースが変更なしで通ることをもって検証する。
- JSON レスポンスの `mode` フィールドの値は既存の 2 値のままで、frontend の変更を伴わない。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0092: Kinesis の Mode 列の追加。本 issue はそのスコープ外として送られたリテラル重複の解消。

## 解決方法

- `backend/internal/aws/resource.go` にパッケージ非公開の定数 `capacityModeOnDemand` (`"on-demand"`) / `capacityModeProvisioned` (`"provisioned"`) を定義し、`dynamo.go` の `dynamoFromDescription` と `kinesis.go` の `kinesisFromSummary` を定数参照に置き換えた。文字列の値は変更していない。
- `dynamoFromDescription` のモード決定を kinesis と同じ形 (`capacityModeProvisioned` を初期値とし、`PAY_PER_REQUEST` のときだけ `capacityModeOnDemand`) に書き換え、`BillingModeSummary` が非 nil で `BillingMode` が既知の 2 値以外の場合も `provisioned` へ縮退するよう統一した。
- `dynamo_test.go` に `unknown billing mode falls back to provisioned` のケースを追加した。`kinesis_test.go` の既存ケース `unknown stream mode falls back to provisioned` と、両ファイルの nil 入力ケースは変更なしで通過する。
- リテラル定義は `resource.go` の 1 箇所のみになった (`pricing.go` の英語エラーメッセージ内の `on-demand/reserved pricing` はキャパシティモードの値ではないため対象外)。
- frontend の変更は無く、`mise run check` の通過を確認した。
