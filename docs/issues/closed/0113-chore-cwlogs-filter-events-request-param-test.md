# CloudWatch Logs のログ検索とライブテイルに渡すリクエストパラメータを検証するテストを整備する

Created: 2026-08-07
Model: Claude Fable 5
Completed: 2026-08-07

## 背景

issue 0102 (ElastiCache のリクエストパラメータ検証) の棚卸しで見つかった。

`backend/internal/aws/cloudwatchlogs.go` の `FilterLogEvents` は、条件に応じて `FilterLogEventsInput` に次のフィールドを設定する (128-137 行)。

- `FilterPattern`: 検索パターン。削除すると指定パターンが無視され全イベントが返る。
- `StartTime` / `EndTime`: 期間の絞り込み。削除すると期間指定が静かに効かなくなる。
- `StartFromHead: aws.Bool(false)`: 取得方向。削除すると API デフォルト (古い順) に戻り、件数上限に達したときに返るイベントの母集合が「最新側」から「最古側」へ静かに変わる。

また `StartLiveTail` (186 行) の `LogEventFilterPattern` を削除しても Live Tail は起動に成功するが、フィルタが無視されて全イベントが流れる。

`cloudwatchlogs_test.go` は変換関数 (`logGroupFromSDK` 等) のテストのみで、リクエスト構築は検証していない。
cloudwatchlogs.go にはテスト用インターフェースが無く、`*cloudwatchlogs.Client` を直接使っているため、現状の構造ではリクエスト構築を単体テストで検証できない。

## 対応方針

- `FilterLogEvents` については docs/issues/closed/0102 と同じ方式を採る。`FilterLogEvents` を持つ狭いインターフェースを定義し、呼び出しロジックをインターフェースを受け取る内部関数に抽出し、受け取った Input を記録する手書きモックで各フィールドの設定を検証する。
- `StartLiveTail` は API 呼び出し層のモックが SDK の構造上できない (docs/issues/0107 の背景に記録があるとおり、`StartLiveTailOutput` はイベントストリームを非公開フィールドに持ち外部から構築できない)。そのため Input を記録するモックでも Output を返せない。代わりに `StartLiveTailInput` の構築 (LogGroupIdentifiers と LogEventFilterPattern の設定) を純関数として切り出し、その関数の戻り値を直接検証する。
- docs/issues/0107 (StartLiveTail のストリーム読み取りの単体テスト) とは対象の層が異なる。0107 は読み取りループの検証でリクエスト構築をスコープ外としており、本 issue はリクエスト構築だけを対象とする。0107 と本 issue の両方が `StartLiveTail` 関数を触るため、両方を実装する場合は先に実装した側の抽出結果に他方が合わせる。

## 完了条件

- `FilterLogEvents` の呼び出しが狭いインターフェースを受け取る関数に抽出され、`FilterPattern` / `StartTime` / `EndTime` / `StartFromHead` の設定 (指定時に設定され、未指定時に nil であること) を検証するテストが存在する。
- `StartLiveTailInput` の構築が検証可能な形に切り出され、`LogEventFilterPattern` の設定を検証するテストが存在する。
- 上記フィールドの設定行を削除するとテストが失敗する (実装時に一時的に削除して確認する)。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0102: 起票元の棚卸し。検証手法の先例。
- docs/issues/0107: 同じ関数のストリーム読み取り層のテスト整備。対象の層が異なる。

## 解決方法

`FilterLogEvents` の呼び出しを狭いインターフェースを受け取る内部関数に抽出し、`StartLiveTail` のリクエスト構築を純関数に切り出して、それぞれのリクエストパラメータを検証するテストを追加した。挙動は変えていない。

### 実装 (backend/internal/aws/cloudwatchlogs.go)

`FilterLogEvents` 1 メソッドだけを持つ `cwLogsFilterEventsClient` を定義し、呼び出しロジックを `filterLogEvents(ctx, client, groupIdentifiers, pattern, start, end, pageToken, perGroupLimit)` に抽出した。

公開関数 `FilterLogEvents` に残したのは、ロググループ未選択時の早期 return とクライアント生成の 2 つだけである。早期 return を内部関数へ移さなかったのは、移すとロググループ未選択でもクライアント生成 (資格情報の解決) が走り、SSO 期限切れ時の挙動が変わるためである。この理由はコードにコメントとして残した。

Live Tail は API 呼び出し層をモックに差し替えられない (`StartLiveTailOutput` がイベントストリームを非公開フィールドに持ち外部から構築できない。docs/issues/closed/0107 の背景に記録がある)。そのため Input の構築だけを純関数 `newStartLiveTailInput(groupIdentifiers, pattern)` として切り出し、`StartLiveTail` はその戻り値を渡すだけにした。

issue の対応方針は「0107 と本 issue の両方が `StartLiveTail` 関数を触るため、両方を実装する場合は先に実装した側の抽出結果に他方が合わせる」としている。0107 は既に close 済み (`runLiveTailStream` の抽出) であるため、その抽出結果には手を触れず、Input 構築の切り出しだけを追加した。

### テスト (backend/internal/aws/cloudwatchlogs_test.go)

- `mockCWLogsFilterEventsClient` — 受け取った `*FilterLogEventsInput` を呼び出し順に記録し、イベントを含まない空のレスポンスを返す手書きモック。
- `TestFilterLogEventsSendsRequestParams` — 検索条件の組み合わせ (条件なし / パターンのみ / 開始のみ / 終了のみ / 全指定) をテーブル駆動で検証する。`FilterPattern` / `StartTime` / `EndTime` / `StartFromHead` について、指定時の値と未指定時に nil のままであることを固定する。`StartFromHead` が `StartTime` の指定時だけ設定される連動 (startFromHead=false は startTime が 2024-01-01 以降のときのみ許可される API 制約に由来する) も固定した。ロググループを 2 つ渡し、条件がグループごとの全呼び出しに等しく載ることと、`LogGroupIdentifier` がグループごとに正しいことも確認する。
- `TestNewStartLiveTailInput` — `LogGroupIdentifiers` の設定と、`LogEventFilterPattern` が pattern 指定時に設定され空のとき nil であることを検証する。

### 検出力の確認

完了条件が対象とする 5 つの設定を 1 つずつ削除し、そのたびにテストが失敗することを実測した (まとめて削除すると相互に隠れる可能性があるため個別に確認した)。

| 削除した設定 | 失敗したサブテスト数 |
| --- | --- |
| `in.FilterPattern = aws.String(pattern)` | 2 |
| `in.StartTime = aws.Int64(startMs)` | 2 |
| `in.StartFromHead = aws.Bool(false)` | 2 |
| `in.EndTime = aws.Int64(endMs)` | 2 |
| `in.LogEventFilterPattern = aws.String(pattern)` | 1 |

確認後、実装は元に戻した。

### レビューで削除したテスト

当初は `perGroupLimit` の既定値フォールバックを検証する `TestFilterLogEventsAppliesDefaultPerGroupLimit` と、`Limit` の値を検証するアサーションも入れていた。`Limit` は本 issue の背景にも完了条件にも挙がっておらず、起票元の docs/issues/closed/0102 の棚卸しでも対象に含まれていない。完了条件を超える変更にあたるため、テストと `Limit` のアサーション、および CHANGES.md エントリからの言及をいずれも削除した。

### 今回のテストで検証していない範囲

モックは常に空のレスポンスを返すため、`out.NextToken` を集約する処理、複数グループ横断の時刻降順ソート、composite token のエンコードは実質的に素通りする。`pageToken` 経由の `NextToken` 設定も、全ケースで `pageToken` が空のため検証していない。いずれも本 issue の完了条件が対象とするリクエストパラメータの検証ではないためスコープ外とした。

`mise run check` の通過を確認済み。CHANGES.md の `## develop` の `### misc` に記載済み。
