# CloudFormation の Web API 一覧が削除されていないスタックを 4 状態分だけ取りこぼす

Created: 2026-08-07
Model: Claude Opus 5
Completed: 2026-08-08

## 背景

issue 0112 (CloudFormation の ListStacks に渡すステータスフィルタの検証テスト) の実装中に発見した別バグである。0112 は「テストの追加のみで挙動は変えない」issue であり完了条件に含まれないため、独立した issue として登録する。

`backend/internal/aws/cfn.go` の `listCFNStacks` (45-82 行) は Web API のスタック一覧 (`internal/api/handlers_aws.go:233` の `ListCFNStacks`) が使う経路である。実装のコメントは削除済みスタックの除外を宣言している。

```go
func listCFNStacks(ctx context.Context, client cfnListStacksClient) ([]CFNStackResource, error) {
	// Exclude deleted stacks.
	statusFilter := []cfntypes.StackStatus{
		// ... 18 種
	}
```

CloudFormation の `StackStatus` は 23 種あり、スタックが存在しなくなるのは `DELETE_COMPLETE` の 1 種だけである。コメントどおりなら残る 22 種を列挙すべきだが、実際の `statusFilter` は 18 種であり、次の 4 種が欠けている。

- `DELETE_FAILED` — 削除に失敗して残っているスタック
- `UPDATE_ROLLBACK_IN_PROGRESS` — 更新のロールバック実行中
- `UPDATE_COMPLETE_CLEANUP_IN_PROGRESS` — 更新完了後の旧リソース削除中
- `UPDATE_ROLLBACK_COMPLETE_CLEANUP_IN_PROGRESS` — ロールバック完了後の旧リソース削除中

いずれも実在するスタックの状態であり、削除済みではない。同じファイルの `listCfnStackSummaries` (393 行以降、CLI の `thief cfn` が使う経路) は `DELETE_COMPLETE` のみを除いた 22 種を渡しており、こちらはコメントの宣言と一致する。同一リポジトリの 2 経路が同じ意図 (削除済みの除外) に対して異なる集合を送っている。

18 種の側に 4 種を落とす意図を示す記述は、コード、コメント、`CHANGES.md`、`issues/closed/` のいずれにも無い。

## 再現手順

1. 対象アカウント/リージョンに、上記 4 状態のいずれかにあるスタックを用意する。最も再現させやすいのは `DELETE_FAILED` で、削除できないリソース (例: 空でない S3 バケット) を含むスタックを削除すると遷移する。
2. API サーバを起動する (`mise run backend:run`)。
3. Web UI の AWS ビューで CloudFormation を開く (または `GET /api/aws/cfn?profile=<profile>&region=<region>` を直接叩く)。
4. 当該スタックが一覧に現れない。
5. 同じプロファイル/リージョンに対して CLI (`thief cfn --profile <profile> --region <region>`) を実行すると、同じスタックが一覧に現れる。

`UPDATE_COMPLETE_CLEANUP_IN_PROGRESS` と `UPDATE_ROLLBACK_*_CLEANUP_IN_PROGRESS` は遷移時間が短いため、更新中のスタックを繰り返し取得すると、一覧から一時的に消えて再び現れる形で観測できる。

## 影響

- 削除に失敗したスタック (`DELETE_FAILED`) が Web UI に表示されない。手当てが必要な状態のスタックが一覧から見えなくなるため、影響が大きい。
- 更新のロールバック中 (`UPDATE_ROLLBACK_IN_PROGRESS`) のスタックが表示されない。デプロイ失敗の確認という一覧の主要な用途で、最も見たい状態が抜ける。
- クリーンアップ中の 2 状態は、更新完了直後にスタックが一覧から一時的に消える形で現れる。
- Web UI と CLI で同じアカウント/リージョンに対する結果が食い違う。
- 読み取り専用の一覧取得であり、データの破壊や外部への影響はない。

## 修正方針

- `listCFNStacks` の `statusFilter` に上記 4 種を追加し、`listCfnStackSummaries` と同じ「`DELETE_COMPLETE` のみを除く 22 種」に揃える。コメントの宣言 (削除済みの除外) と実装を一致させる。
- 2 つの列挙を共有の定数へ括り出す案は、`listCfnStackSummaries` がレガシー CLI 互換の集合として独立して維持されており、片方の要件が変わったときに他方を巻き込むため採らない。issue 0112 で追加した `TestListStacksSendsStackStatusFilter` が 2 経路の集合をそれぞれ独立に固定しているため、重複を残しても片方だけがずれれば検知できる。
- 逆に 18 種を正としてコメントの側を直す案は採らない。`DELETE_FAILED` と `UPDATE_ROLLBACK_IN_PROGRESS` の非表示を正当化する要件が存在せず、UI の用途 (デプロイ結果と要対応スタックの確認) に反するためである。
- 新しい API と権限は不要である。`cloudformation:ListStacks` の呼び出し内容が変わるだけである。

## 完了条件

- `listCFNStacks` の `StackStatusFilter` が `DELETE_COMPLETE` を除く 22 種になっている。
- issue 0112 で追加した `TestListStacksSendsStackStatusFilter` の `listCFNStacks` 側の期待値が 22 種に更新され、そのコメントから本 issue への参照が取り除かれている。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0112: 起票元。2 経路のステータス集合を固定するテストを追加した issue。

## 解決方法

`backend/internal/aws/cfn.go` の `listCFNStacks` の `statusFilter` に欠けていた 4 種を追加し、`DELETE_COMPLETE` のみを除く 22 種にした。SDK (`aws-sdk-go-v2/service/cloudformation` v1.71.7) の `types/enums.go` を読み、`StackStatus` が 23 種であること、スタックが存在しなくなるのは `DELETE_COMPLETE` の 1 種だけであることを確認した上で、実装の 22 種が `StackStatus("").Values()` から `DELETE_COMPLETE` を除いた集合と過不足なく一致することを集合演算で突き合わせた。

コメントには、なぜ 22 種なのかと、`listCfnStackSummaries` と同じ集合でありながら共有の定数へ括り出さない理由 (レガシー CLI 互換の集合として独立に維持されているため) を残した。

`ListStacks` の `StackStatusFilter` は省略すると削除済みを含む全件が返る (SDK の `api_op_ListStacks.go` の doc comment に明記されている)。22 種を明示することで初めて削除済みの除外という意図した動作になるため、フィルタを空にする案は採っていない。

### テストの期待値を実装から独立させた

`cfn_test.go` の `TestListStacksSendsStackStatusFilter` の Web API 経路の期待値を、手書きの文字列リストから `cfntypes.StackStatus("").Values()` 由来の導出 (`cfnLiveStackStatuses`) に変更した。レビューで採用した改善である。

本 issue のバグは、実装が 18 種を列挙し、テストの期待値もその 18 種を手で写していたために検出できなかった。実装と期待値が同じ書き漏らしを共有すると、テストは通り続ける。実装はホワイトリスト方式 (22 種を個別に列挙)、期待値はブラックリスト方式 (全種から `DELETE_COMPLETE` を除く) と導出方法を変えたことで、同型の欠陥を再び作り込んでも検知できる。実装は `Values()` を参照していないため、循環論法にはなっていない。

AWS がステータスを追加して SDK が更新されたときも、実装が追随していなければこのテストが落ちる。

レガシー CLI 経路の期待値は手書きの 22 種のまま残した。両方を `Values()` 由来にすると 2 つのケースが同一の期待値を見ることになり、「片方だけがずれればもう片方で検知できる」という issue 0112 以来の設計が失われるためである。

### 測定した検出力

| 壊した箇所 | 結果 |
| --- | --- |
| `DELETE_FAILED` を再び落とす | 失敗する |
| `UPDATE_ROLLBACK_IN_PROGRESS` を再び落とす | 失敗する |
| `UPDATE_COMPLETE_CLEANUP_IN_PROGRESS` を再び落とす | 失敗する |
| `UPDATE_ROLLBACK_COMPLETE_CLEANUP_IN_PROGRESS` を再び落とす | 失敗する |
| `DELETE_COMPLETE` を誤って追加 | 失敗する |
| `IMPORT_COMPLETE` を重複させる | 失敗する |

修正後は `listCFNStacks` と `listCfnStackSummaries` に同一の行が並ぶため、置換対象を `statusFilter := []cfntypes.StackStatus{` 以降のブロックに限定してから測定した。確認後、実装は元に戻し `git diff` で復元を確認した。

### frontend の変更が不要であることの確認

新たに流れてくる 4 状態が表示経路のどこかで弾かれないかを確認した。`cfn.go` は `State: string(s.StackStatus)` で生の値を載せ、`normalize.ts` の `cfnFromRaw` は `state: raw.state` で素通しし、`columns.tsx` の state 列は `StatusBadge` に渡すだけである。`StatusBadge` は `MAP[state] ?? { cls: 'muted', label: state }` で未知の値をそのまま表示する。`MAP` のキーはすべて小文字で、CFN の大文字スネークケースのステータスは既存 18 種を含めて 1 つも登録されていない。既存の 18 種も新規の 4 種も同じ fallback を通るため、表示の扱いに差は生じない。`FacetBar` の集計も `Set<string>` に生文字列を積むだけで、ホワイトリストによる制限はない。

### レビューで見つかり別 issue にしたもの

Web API 経路の期待値だけが SDK 更新に追随するようになり、レガシー CLI 経路の手書き期待値との間に非対称が生まれた。AWS が `StackStatus` を追加した場合、CLI 側は実装も期待値も古いまま気づかれない。ただし両経路の期待値を共通化することは前述の独立性の要件と衝突するため、独立性を保ったまま SDK の更新に気づく方法の検討を issue 0124 として起票した。

`mise run check` の通過を確認済み。CHANGES.md の `## develop` に `[FIX]` として記載済み。
