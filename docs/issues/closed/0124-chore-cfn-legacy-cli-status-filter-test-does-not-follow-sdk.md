# CloudFormation のレガシー CLI 経路のステータスフィルタのテストが SDK のステータス追加に追随しない

Created: 2026-08-08
Completed: 2026-08-08
Model: Claude Opus 5

## 背景

issue 0122 (CloudFormation の Web API 一覧が削除されていないスタックを取りこぼす) のレビューで見つかった。

0122 では、Web API 経路 (`listCFNStacks`) のテスト期待値を、手書きの文字列リストから SDK の列挙 (`cfntypes.StackStatus("").Values()` から `DELETE_COMPLETE` を除く) の導出に変更した。0122 のバグは「実装の列挙を写した手書きの期待値が、実装と同じ書き漏らしを共有していたため検出できなかった」という形の欠陥であり、期待値を実装と独立な情報源から組み立てることでこれを塞いだ。

一方、同じ `TestListStacksSendsStackStatusFilter` のレガシー CLI 経路 (`listCfnStackSummaries`) 側のケースは、22 種の文字列を手書きで並べたままである (`backend/internal/aws/cfn_test.go`)。

その結果、2 つの経路でテストの性質が非対称になった。

| 経路 | 期待値の作り方 | AWS がステータスを追加したとき |
| --- | --- | --- |
| Web API (`listCFNStacks`) | SDK の `Values()` から導出 | 実装が追随していなければテストが落ちる |
| レガシー CLI (`listCfnStackSummaries`) | 22 種を手書き | 実装も期待値も古いまま、テストは通り続ける |

両経路とも仕様は「`DELETE_COMPLETE` を除く全ステータス」で同一である。AWS が `StackStatus` に新しい値を追加した場合、レガシー CLI 経路だけが古い集合を送り続け、誰も気づかない。0122 と同種の「一覧からスタックが静かに落ちる」不具合が、CLI 側で再発しうる構造が残っている。

## 制約

レガシー CLI 経路の期待値も `Values()` 由来にする、という単純な対応は採れない。

issue 0122 の修正方針は、2 経路の列挙を共有の定数へ括り出す案を明示的に退けている。`listCfnStackSummaries` はレガシー CLI 互換の集合として独立に維持されており、片方の要件が変わったときにもう片方を巻き込まないためである。テストの期待値も同じ理由で独立に固定する必要がある。両方を `Values()` 由来にすると、2 つのケースが同一の期待値を見ることになり、「片方だけがずれれば検知できる」という 0112 以来の設計が失われる。

つまり、次の 2 つを同時に満たす必要がある。

- レガシー CLI 経路の期待値は、Web API 経路とは独立に固定されている。
- SDK に新しい `StackStatus` が増えたことに気づける。

## 対応方針

期待値そのものは手書きのまま維持し、SDK の `StackStatus` の総数が変わったことを検知するガードを別に置く案を軸に検討する。

- `cfntypes.StackStatus("").Values()` の要素数が既知の 23 であることを検証するアサーションを追加する。AWS がステータスを追加して SDK が更新されると、このアサーションが落ち、2 経路の集合を見直す契機になる。
- 落ちたときに何をすべきかが読み手に伝わるよう、失敗メッセージに「両経路の `statusFilter` を見直すこと」を明示する。
- ガードの置き場所 (既存の `cfnLiveStackStatuses` の中か、独立したテストか) は実装時に決める。`cfnLiveStackStatuses` には既に `len(live) != len(all)-1` の防御的チェックがあるため、そこへ寄せるか分離するかを判断する。

上記以外の案 (レガシー側も `Values()` 由来にする、共有定数へ括り出す) は、前節の制約により採らない。

## 完了条件

- SDK の `StackStatus` にステータスが追加されたときに `backend/internal/aws/cfn_test.go` のいずれかのテストが落ちる。
- レガシー CLI 経路の期待値が、Web API 経路の期待値とは独立に固定されたままである (共有の定数や共通のヘルパーへ寄せていない)。
- テストが落ちたときに、両経路の `statusFilter` を見直すべきことが失敗メッセージから読み取れる。
- `mise run check` が通る。

## 解決方法

`backend/internal/aws/cfn_test.go` にテストのみを追加した (53 行)。本番コードは変更していない。

### 要素数のガードではなく集合の固定にした

対応方針は「`Values()` の要素数が既知の 23 であることを検証する」案を軸に挙げていたが、実装では `cmp.Diff` による集合比較にした。要素数のガードでは、AWS が既存のステータスを 1 つ廃止して別の 1 つを新設した場合 (総数が 23 のまま中身だけ変わる) を検知できないためである。集合比較なら差分行に「どの値が増えて、どの値が消えたか」が具体的に現れる。

追加したのは次の 2 つである。

- `cfnPinnedStackStatuses()`: SDK が知る `StackStatus` 全 23 種を AWS API のステータス文字列で書き下した固定値。
- `TestCfnStackStatusSetIsPinned`: `cfntypes.StackStatus("").Values()` と上記を照合する。

SDK の注記 (`enums.go:1793`) が「`Values()` の並び順は SDK 更新をまたいで安定するとは限らない」と述べているため、`cmpopts.SortSlices` で順序非依存にした。集合としての等価性だけを見るので、ステータスの増減が無いのに SDK のベンダー更新だけで落ちる偽陽性を避けられる。

### 3 つの手書き集合を共通化しなかった

`cfnPinnedStackStatuses` (23 種) は、`TestListStacksSendsStackStatusFilter` のレガシー CLI 側の期待値 (22 種) と `DELETE_COMPLETE` 1 件を除いて内容が重なる。それでも共有しない。

| 集合 | 意味 |
| --- | --- |
| `cfnLiveStackStatuses` (`Values()` から導出) | Web API 経路の仕様。実装が SDK に追随しているかを独立な情報源で検証する |
| レガシー CLI 側の手書き 22 種 | レガシー CLI が送るべき集合という、このリポジトリの仕様 |
| `cfnPinnedStackStatuses` (手書き 23 種) | SDK が知る全種という、リポジトリの外にある事実 |

3 つは意味が異なる。共通化すると、片方だけがずれたときに検知できなくなる (issue 0112 以来の設計) か、issue 0122 で塞いだ「実装と期待値が同じ書き漏らしを共有する」欠陥が再発する。

`cfnPinnedStackStatuses` を手書きすることによる転記ミスは、`Values()` との照合で毎回検証されるため、書き間違えれば即座にテストが落ちる。手書きであっても安全側に倒れる。

### 失敗メッセージに更新箇所を 3 つとも列挙した

レビューで「実装 2 つの見直しには言及しているが、`TestListStacksSendsStackStatusFilter` の手書き期待値の更新に触れていない」との指摘を受けて修正した。実装だけを直して期待値の配列を放置すると別のテストが落ちるが、最初の失敗メッセージだけを見た開発者はその連鎖を一手で理解できない。

実際の失敗出力は次のようになる (`IMPORT_ROLLBACK_COMPLETE` を期待値から 1 件落として確認した)。

```
--- FAIL: TestCfnStackStatusSetIsPinned (0.00s)
    cfn_test.go:214: StackStatus set changed (-want +got):
          []string(Inverse(cmpopts.SortSlices, []string{
          	... // 6 identical elements
          	"IMPORT_COMPLETE",
          	"IMPORT_IN_PROGRESS",
        + 	"IMPORT_ROLLBACK_COMPLETE",
          	"IMPORT_ROLLBACK_FAILED",
          	"IMPORT_ROLLBACK_IN_PROGRESS",
          	... // 12 identical elements
          }))

        update cfnPinnedStackStatuses, revisit statusFilter in both listCFNStacks and listCfnStackSummaries, and update the hand written expected set for listCfnStackSummaries in TestListStacksSendsStackStatusFilter
```

### 測定した検出力

`cfnPinnedStackStatuses` のブロック内だけを書き換えて測定した。

| 壊した状況 | 結果 |
| --- | --- |
| SDK に 1 種増えたのと等価 (期待値から 1 種落とす) | FAIL (検出) |
| SDK から 1 種消えたのと等価 (期待値に架空の 1 種を足す) | FAIL (検出) |
| 追加と削除の同時発生と等価 (1 種を別名に置換) | FAIL (検出) |
| SDK の列挙順だけ変わった場合 (落ちてはいけない) | PASS (検出せず。意図どおり) |

### このテストの限界

このテストは `listCFNStacks` も `listCfnStackSummaries` も呼ばない。検知するのは「SDK の集合が変わった」という事実だけであり、「レガシー CLI の実装が新しいステータスに追随したか」までは検証しない。SDK 更新を受けて `cfnPinnedStackStatuses` だけを更新し、実装と手書き期待値の見直しを忘れれば、テストは再び通る。

これは本 issue の「制約」節が課した 2 条件 (期待値を独立に維持する / SDK の追加に気づける) を同時に満たすために選んだ妥協である。本 issue の目的は見直しの契機を作ることであり、追随そのものの保証ではない。失敗メッセージで見直し箇所を 3 つとも名指しするのは、この限界を人手で埋めるための措置である。

### 採用しなかったレビュー指摘

- 「`cfnPinnedStackStatuses` の doc コメントと `TestCfnStackStatusSetIsPinned` の doc コメントで、共有しない理由の説明が重複している」(低)。前者は「外部の事実か、このリポジトリの仕様か」という意味の違いを述べ、後者は「共通化すると何を検知できなくなるか」という検出力を述べており、論点が異なる。指摘した本人も必須の修正ではないとしている。
- 「テスト名が受け身の表現で、他のテストの『関数名 + 動作』型と異なる」(低)。このテストの対象は関数ではなく SDK の enum であり、`関数名 + 動作` 型にできない。`Pinned` は固定値テストの一般的な語彙である。

## 関連

- docs/issues/closed/0122: 起票元。Web API 経路の期待値を `Values()` 由来に変更した issue。
- docs/issues/closed/0112: 2 経路のステータス集合をそれぞれ独立に固定するテストを追加した issue。
