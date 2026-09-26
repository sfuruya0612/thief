# startEC2SessionWith が StartSSMSession 成功後の marshal / plugin 探索失敗でセッションを切断せず残す

Created: 2026-08-11
Model: Claude Sonnet 5
Completed: 2026-08-11

## 背景

issue 0135 (ecs execute-command が SSM セッションを切断せず残す) の多観点レビュー (堅牢性観点) で、参考実装として比較した `backend/internal/cli/ec2.go` の `startEC2SessionWith` に同型の欠陥があることが判明した。

`startEC2SessionWith` は `deps.startSession` (StartSSMSession) が成功した後、session-manager-plugin を実行する前に次の失敗経路を持つ。

```go
session, err := deps.startSession(ctx, cfg.Profile, cfg.Region, target)
if err != nil {
	return fmt.Errorf("start session: %w", err)
}

sessJSON, err := sessionManagerSessionJSON(session.SessionID, session.StreamURL, session.TokenValue)
if err != nil {
	return fmt.Errorf("marshal session: %w", err) // session を切断せず return
}

paramsJSON, err := util.Parser(...)
if err != nil {
	return fmt.Errorf("marshal params: %w", err) // 同上
}

plug, err := deps.lookupPlugin()
if err != nil {
	return err // 同上
}
```

`termCtx, cancelTerm := context.WithTimeout(context.Background(), ec2TerminateTimeout)` は、この後の `deps.execPlugin` 呼び出しの直前で初めて作られる。そのため `sessJSON` / `paramsJSON` の marshal 失敗、または `deps.lookupPlugin()` (`lookupSessionManagerPlugin`) の失敗で早期 return すると、`deps.terminateSession` が一度も呼ばれない。

## 問題

`deps.startSession` (StartSSMSession) は AWS 側にセッションを作る。それ以降のどの段階で失敗しても、`TerminateSSMSession` を呼ばない限りそのセッションは AWS 側の管理下に残り続ける。

`lookupSessionManagerPlugin` の失敗 (session-manager-plugin が PATH に無い) は、plugin 未インストールの環境で現実的に起こり得る。marshal の失敗は固定の文字列フィールドを持つ構造体の JSON 化であり実質到達不能だが、plugin 探索の失敗は違う。

これは issue 0135 が `ecs.go` の `ecsExecuteCommand` について問題にしたのと同じ性質の症状 (AWS 側にセッションが残る) が、`ec2.go` の `startEC2SessionWith` にも別のトリガ (plugin 未インストール) で存在するということである。

## 再現手順

1. `session-manager-plugin` を PATH から外す (または存在しない環境で実行する)。
2. `thief ec2 session --instance-id <id>` を実行する。
3. `lookupSessionManagerPlugin` が失敗し、`startEC2SessionWith` はセッションを切断せずに return する。
4. `aws ssm describe-sessions --state Active` を実行する。

手順 4 のセッション一覧に、手順 2 で `StartSSMSession` が作ったセッションが残る。

## なぜ対応が必要か

- 残ったセッションは AWS 側のアイドルタイムアウト (既定 20 分) まで Active のまま数えられ、同時セッション数の上限を圧迫する。
- issue 0135 で `ecs.go` 側は `deps.executeSession` 成功後の全失敗経路 (marshal、plugin 探索、plugin 実行) で切断を試みるように修正済みである。`ec2.go` 側だけがこの欠陥を残すと、2 つの経路の後始末の徹底度が食い違う。

## 修正方針

issue 0135 で `ecs.go` の `ecsExecuteCommandWith` に適用した形をそのまま `ec2.go` の `startEC2SessionWith` へ適用する。`termCtx` / `cancelTerm` の生成を `deps.startSession` 成功直後まで前倒しし、`sessJSON` / `paramsJSON` の marshal 失敗、`deps.lookupPlugin()` の失敗、`deps.execPlugin` の失敗のいずれの経路でも `deps.terminateSession` を試みてから return する。

`lookupSessionManagerPlugin` の失敗は現状ラップせずそのまま返している (呼び出し先が PATH に無いことを述べるため)。切断も失敗した場合の結合方法は `ecsExecuteCommandWith` の `terminateThen` と同じ形 (`"%w; terminate session: %w"`) に揃える。

## 完了条件

- `startEC2SessionWith` が `deps.startSession` 成功後のどの失敗経路 (marshal、plugin 探索、plugin 実行) でも `deps.terminateSession` を試みる。
- 切断の失敗が元の失敗の情報を上書きしない (`errors.Is` / `errors.As` で両方に到達できる)。
- 上記をテストで検証する (`ec2_test.go` に、plugin 探索失敗時に切断が呼ばれることを検証するテストケースを追加する。marshal 失敗は実質到達不能なため `ecsExecuteCommandWith` 側と同様にテスト対象外でよい)。
- 既存の `startEC2SessionWith` のテストが全て通過する。
- `mise run check` が通る。

## 関連

- docs/issues/0135: この欠陥が同型の症状として見つかった issue。修正済みの `ecsExecuteCommandWith` が本 issue の修正方針の参照実装になる。
- `backend/internal/cli/ec2.go` の `startEC2SessionWith`。

### 補足 (issue 0136 対応時点)

issue 0136 の対応で、本文中のコード引用にある `ec2TerminateTimeout` は削除され、`internal/aws/ssm_session.go` の公開定数 `awsinternal.TerminateSessionGracePeriod` に置き換わっている (値は 5 秒のまま変更なし)。本 issue の修正方針を実装する際は、引用中の `ec2TerminateTimeout` を `awsinternal.TerminateSessionGracePeriod` と読み替えること。

## 解決方法

`backend/internal/cli/ec2.go` の `startEC2SessionWith` に、issue 0135 で `ecs.go` の `ecsExecuteCommandWith` へ適用した `terminateThen` パターンをそのまま移植した。

- `termCtx, cancelTerm := context.WithTimeout(context.Background(), awsinternal.TerminateSessionGracePeriod)` の生成を `deps.startSession` 成功直後まで前倒しし、`defer cancelTerm()` を直後に置いた。
- `terminateThen := func(cause error) error { ... }` クロージャを新設し、`deps.terminateSession` を試みてから、失敗すれば `fmt.Errorf("%w; terminate session: %w", cause, termErr)` で両方の情報を保持したエラーを返し、成功すれば `cause` をそのまま返す。
- `sessJSON` の marshal 失敗、`paramsJSON` の marshal 失敗、`deps.lookupPlugin()` の失敗、`deps.execPlugin` の失敗の 4 経路すべてを `terminateThen` 経由に変更した。`lookupSessionManagerPlugin` の失敗は方針どおりラップせず `terminateThen(err)` に渡している。
- `ec2SessionDeps` 構造体へのフィールド追加は不要だった。

方針セクションからの乖離は無く、実装詳細もすべて `ecsExecuteCommandWith` の形と一致させた。

### テスト

`ec2_test.go` に以下を追加・変更した。

- `TestStartEC2SessionPropagatesSetupErrors` のテーブルに `wantTerminateIDs` フィールドを追加し、"plugin lookup fails" ケースが `deps.terminateSession` を `sess-1` で呼ぶことを検証するようにした。
- 新規テスト `TestStartEC2SessionTerminatesOnLookupPluginFailureAfterSessionEstablished` を追加し、plugin 探索の失敗と切断の失敗が同時に起きた場合、`errors.Is` で両方のエラーに到達できること、結合後のメッセージが `"...; terminate session: ..."` の形になることを検証した。

marshal 2 箇所 (`sessJSON`/`paramsJSON`) は、いずれも固定の文字列フィールドのみを持つ構造体の `json.Marshal` であり、`encoding/json` が失敗するのは非対応型・NaN/Inf・循環参照・カスタム `MarshalJSON` の失敗などに限られるため、方針どおりテスト対象外とした。

実装とテストは `ec2.go` を一時的に改変してテストが RED になることを確認したうえで復元するミューテーションテストで検証した (plugin 探索失敗時に `terminateThen` を経由しない改変で、`TestStartEC2SessionPropagatesSetupErrors/plugin_lookup_fails` と新規テストの両方が想定通り FAIL することを確認)。

### 多観点レビュー

5 観点をそれぞれ 1 ラウンド、`issue-reviewer` サブエージェントで順次実施した。

- **観点1 (完了条件の充足)**: 指摘なし。完了条件 5 行、修正方針 4 点いずれも実装・テスト・`mise run check` の実行結果と一致していることを確認。
- **観点2 (テストの品質)**: 中 1 件、低 1 件。
  - 中: `TestStartEC2SessionTerminatesOnLookupPluginFailureAfterSessionEstablished` が `rec` の内容 (切断先の profile / region / sessionID) を一度も検証していなかった。誤った宛先で切断を呼んでも検出できない状態だったため、`rec.terminateProfile`/`rec.terminateRegion`/`rec.terminateIDs` の検証を追加し、ミューテーションテスト (`deps.terminateSession` へ渡す `cfg.Profile`/`cfg.Region` を入れ替える改変) で実際に検出できることを確認した。反映済み。
  - 低: `TestStartEC2SessionTerminatesWithLiveContextAfterInterrupt` が `termCtx` の生存を検証しているのは `execPlugin` の成功/失敗経路のみで、`lookupPlugin` 失敗経路は直接検証していない、という指摘。`termCtx` は `startSession` 成功直後に単一変数として 1 回だけ生成される設計であり (観点3 で構造的に確認済み)、生成箇所自体を分岐させない限り経路ごとに独立して壊れることはない。追加のテストは同じ生成箇所を再度検証するだけで新たな回帰検出力を持たないため、却下した。
- **観点3 (堅牢性)**: 指摘なし。`cancelTerm` の `defer` による確実な解放、`terminateThen` の単発呼び出し、複数 `%w` の `errors.Is`/`errors.As` 両到達、`-race` 実行結果、`session.SessionID` の nil ポインタ非該当をそれぞれ確認済み。
- **観点4 (規約準拠)**: 指摘なし。`%w`/`errors.Is`/`errors.As`、コメント言語、全角半角スペース、絵文字なし、テーブル駆動テスト、`gofmt`/`goimports`/`go vet` を確認済み。
- **観点5 (回帰と整合)**: 指摘なし。呼び出し元 (`startEC2Session`) の互換性、`ec2SessionDeps` のフィールド数不変、既存テスト全 PASS、`mise run check` の 2 回実行での正常終了、issue 0135 の参照実装との整合を確認済み。
