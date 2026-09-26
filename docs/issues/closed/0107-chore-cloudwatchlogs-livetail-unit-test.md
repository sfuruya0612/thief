# CloudWatch Logs の StartLiveTail のストリーム読み取りに単体テストを整備する

Created: 2026-08-05
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

TODO.md の「internal/aws/cloudwatchlogs.go の StartLiveTail に単体テストを整備したい (issue 0095 のスコープ外として送り)」に対応する。

`backend/internal/aws/cloudwatchlogs.go` の `StartLiveTail` (175-211 行) は、API 呼び出しからイベントストリームの読み取りループまでを 1 つの関数に持つ。
テストがあるのはイベント変換の `liveTailEventFromSDK` だけで、ループ本体 (SessionUpdate 以外のイベントの読み飛ばし、送信コールバックのエラーでの中断、ストリームエラーの伝播) は検証されていない。
GCP 側の同等機能 `backend/internal/gcp/logging_tail.go` は `tailStream` インターフェース (13-18 行) と `runTailLogEntries` (50 行) の分離により `logging_tail_test.go` の `fakeTailStream` で間接テストを持っており、AWS 側だけテストが無い非対称がある。

テストが書けない原因は API 呼び出し層の SDK の構造にある。
`StartLiveTailOutput` (aws-sdk-go-v2 cloudwatchlogs v1.79.1) はイベントストリームを非公開フィールドに持ち、外部パッケージから値を構築できないため、`StartLiveTail` の API 呼び出し自体をインターフェースで差し替えるモックは作れない。
一方、`GetStream()` が返すストリームの層には SDK 公式のモック手段がある。
`NewStartLiveTailEventStream` はテストとモック用と godoc に明記されたコンストラクタで、`StartLiveTailResponseStreamReader` インターフェース (`Events()` / `Close()` / `Err()`) の実装を差し込める。
つまりモックできないのは API 呼び出しと `StartLiveTailOutput` の構築までで、ストリームの読み取り層はテスト可能である。

## 対応方針

- `StartLiveTail` から、イベントチャネル (`<-chan types.StartLiveTailResponseStream`)、ストリームエラー取得関数、送信コールバックを引数に取る内部関数へ読み取りループを抽出する。GCP 側が `runTailLogEntries` の分離でテストを可能にしたのと同じ発想だが、ループの構造は揃えない。GCP 側は同期ポーリングの各周回の頭で `ctx.Err()` を確認する構造であるのに対し、AWS 側はチャネル受信のループであり、揃える対象にならない。
- ストリーム層のモックに SDK の `NewStartLiveTailEventStream` と `StartLiveTailResponseStreamReader` を使う案は採らない。読み取りループが消費するのは `Events()` と `Err()` だけであり、チャネルとエラー取得関数を直接渡す抽出で同じ検証ができる。モック案はテストが SDK のモック用コンストラクタと Reader インターフェースの実装に依存する分だけ配線が増え、検証できる範囲は変わらない。
- 現状の読み取りループは `ctx` を参照しない。`ctx` キャンセル時の明示的な中断とエラー返却を加えることは外部から見た挙動の追加であり、テスト整備という本 issue の範囲を超えるため行わない。必要になった場合は別 issue として起票する。
- クライアント呼び出し層 (API 呼び出しと `GetStream()` の取得) はテスト対象外とする。前述のとおり `StartLiveTailOutput` を外部から構築できず、この層をモックする手段が無い。SDK の実クライアントを叩く結合テストを書く案は、実 AWS 環境と有効なロググループを要するため単体テストの範囲を超え、採らない。
- 抽出した関数にテーブル駆動テストを書く。ケースは次の 5 つ。
  - SessionUpdate イベントのログが送信コールバックへ変換されて渡ること
  - SessionUpdate 以外のイベント (SessionStart 等) が読み飛ばされること
  - 送信コールバックがエラーを返したらループが中断してそのエラーが返ること
  - チャネルクローズ後にストリームエラーがあればそれが伝播すること
  - チャネルクローズ後にストリームエラーが無ければ nil を返して正常終了すること (`cloudwatchlogs.go` 207-210 行の正常終了分岐)

## 完了条件

- `StartLiveTail` の読み取りループが、チャネルと関数を引数に取る内部関数に抽出されている。
- 上記 5 ケースのテーブル駆動テストが存在し、SDK のクライアントもネットワークも使わずに動く。
- `StartLiveTail` の外部から見た挙動が変わらない。シグネチャの不変は、呼び出し元の `backend/internal/api/handlers_cwlogs.go` を変更せずに `mise run backend:build` が通ることで判定する。イベントの変換と配信のロジックは抽出した内部関数に移した上で、上記 5 ケースで検証する。
- クライアント呼び出し層のテストはスコープ外とし、テスト対象にしない。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0095: 本 issue の起票元。0095 の本文はテスト整備に言及しておらず、スコープ外送りの出典は TODO.md の記録のみである。

## 解決方法

- `backend/internal/aws/cloudwatchlogs.go` の `StartLiveTail` から、イベントストリームの読み取りループを内部関数 `runLiveTailStream` に抽出した。イベントチャネル、ストリームエラー取得関数、送信コールバックを引数に取り、SDK のストリームリーダー型 (`StartLiveTailResponseStreamReader`) やモック用コンストラクタには依存しない。抽出理由と引数の役割を説明する godoc コメントを添えた。`StartLiveTail` は `stream.Events()` と `stream.Err` を渡して呼ぶだけになり、外部から見た挙動とシグネチャは変わらない。
- `backend/internal/aws/cloudwatchlogs_test.go` に `TestRunLiveTailStream` としてテーブル駆動テストを追加した。ケースは、SessionUpdate イベントのログが送信コールバックへ変換されて渡ること、SessionUpdate 以外のイベントが読み飛ばされること、送信コールバックのエラーでループが中断してそのエラーがラップされずに返ること、チャネルクローズ後のストリームエラーの伝播、ストリームエラーが無い場合の正常終了の 5 つで、SDK のクライアントもネットワークも使わない。
- 呼び出し元の `backend/internal/api/handlers_cwlogs.go` を変更せずに `mise run backend:build` が通ることを確認した。
- クライアント呼び出し層 (API 呼び出しと `GetStream()` の取得) は方針どおりテスト対象にしていない。
- `mise run check` が通ることを確認した。
