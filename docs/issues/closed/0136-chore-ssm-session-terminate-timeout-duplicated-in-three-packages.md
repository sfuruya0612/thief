# SSM セッションの切断タイムアウト 5 秒が 3 パッケージに別名で重複している

Created: 2026-08-09
Model: Claude Opus 5
Completed: 2026-08-11

## 背景

issue 0131 (CLI コマンドがシグナル連動 context を持たない) の作業中に、`TerminateSSMSession` の呼び出しに与える猶予が同じ値・同じ目的で 3 箇所に別名で定義されていることが判明した。

| 定義 | 値 | 用途 |
| --- | --- | --- |
| `backend/internal/session/bridge.go:17` の `terminateTimeout` | `5 * time.Second` | WebSocket ブリッジのセッション終了時の切断 |
| `backend/internal/api/handlers_session.go:20` の `sessionTerminateTimeout` | `5 * time.Second` | セッション確立に失敗した際の後始末。`api/logtail.go:97` からも使う |
| `backend/internal/cli/ec2.go:18` の `ec2TerminateTimeout` | `5 * time.Second` | CLI の SSM セッション終了時の切断 |

いずれも `context.WithTimeout(context.Background(), <定数>)` を作って `awsinternal.TerminateSSMSession` を呼ぶという、同じ形で使われている。

## 問題

同じ判断 (「中断後の後始末に何秒与えるか」) が 3 箇所に散っている。片方だけを変えても、残りは変わらないままコンパイルも lint も通る。値が揃っていることを保証するものが無い。

現状 3 つの値は一致しているため、挙動上の不具合は生じていない。値が食い違ったときに初めて、経路によって切断の猶予が違うという分かりにくい状態になる。

issue 0135 (`ecs execute-command` が切断を行っていない) を修正すると 4 箇所目が生まれる。増える前に定義場所を決めておきたい。

## なぜ対応が必要か

グローバル `~/.codex/AGENTS.md` は「マジックナンバーは定数化する」と定めており、3 箇所はいずれも定数化されている。問題は定数化の有無ではなく、同じ意味の定数が 3 つあることである。

`TerminateSSMSession` は `internal/aws` にあり、3 箇所すべてがそれを呼ぶ。猶予は呼び出し側の都合ではなく「この API に与える猶予」であり、呼び出し側ごとに異なる値を持つ理由が無い。

## 修正方針

案を検討したうえで決める。以下は候補である。

案 A: `internal/aws` に公開定数を置き、3 箇所がそれを参照する。

- 猶予が `TerminateSSMSession` の性質に属することを表現できる。
- `internal/aws` は AWS SDK の薄いラッパであり、呼び出し側のタイムアウト方針を持つ層かどうかは判断が必要。

案 B: `internal/aws` に `TerminateSSMSessionWithGrace(profile, region, sessionID string) error` のような、短命 context の生成を内側に閉じ込めた関数を追加し、3 箇所をそれに置き換える。

- 「キャンセル済みの context を使ってはいけない」という注意が 1 箇所に集約される。3 箇所に散っている同趣旨のコメントも 1 つになる。
- `internal/cli` と `internal/api` が `context.Background()` を呼ぶ理由が消えるため、`TestNoRootContextOutsideDesignatedFunctions` の一覧を 2 件減らせる。
- 呼び出し側が context を渡さない関数を `internal/aws` に置くことになる。第一引数に context を取るという AGENTS.md の方針から外れるため、その正当化 (中断後の後始末専用であること) をコメントで残す必要がある。

案 C: 現状のまま 3 箇所に残し、値が一致していることを検証するテストを置く。

- 実装は変わらない。
- 3 つの定数はいずれも非公開であり、パッケージを跨いで参照できない。テストを書く場所が無い。案として成立しない。

案 A と案 B のどちらを採るかは、`internal/aws` にタイムアウト方針を持たせるかの判断であり、実装前に決める必要がある。

### 決定 (ユーザー確認済み)

案 A を採用する。`internal/aws` (`ssm_session.go`) に公開定数 `TerminateSessionGracePeriod` を置き、4 箇所 (issue 0135 対応済みの `ecs.go` を含む) がそれを直接参照する。呼び出し側は従来どおり自分で `context.WithTimeout(context.Background(), awsinternal.TerminateSessionGracePeriod)` を組み立てる。案 B (`TerminateSSMSessionWithGrace` の新設) は採らない。

## 完了条件

- 切断の猶予の定義が 1 箇所になっている。
- 3 箇所 (issue 0135 を先に対応した場合は 4 箇所) すべてがその 1 箇所を参照している。
- 切断が中断後も通ることを検証している既存のテストが通る。
- `mise run check` が通る。

## 関連

- docs/issues/0131: 起票元。CLI コマンドへのシグナル連動 context の導入。
- docs/issues/0135: `ecs execute-command` に 4 箇所目の切断を追加する issue。本 issue を先に対応するか、あわせて対応するかを判断する。
- docs/issues/0142: `startEC2SessionWith` の欠陥修正待ちの issue。本文中のコード引用が旧定数名 `ec2TerminateTimeout` のままのため、本 issue の対応時点で読み替えの補足を追記した。

## 解決方法

「### 決定 (ユーザー確認済み)」のとおり案 A で実装した。

- `backend/internal/aws/ssm_session.go` に公開定数 `TerminateSessionGracePeriod = 5 * time.Second` を新設した。godoc は同ファイル内の既存の `StartSSMSession`/`TerminateSSMSession` の慣習に合わせて英語で書き、`TerminateSSMSession` 専用の猶予であることと、呼び出し元の (キャンセル済みかもしれない) context から切り離す必要がある理由を明記した。
- 重複していた 3 つの定義と、そこから派生する 5 箇所の参照をすべて新定数に置き換えた。
  - `backend/internal/session/bridge.go` の `terminateTimeout` (`cleanup` が参照)
  - `backend/internal/api/handlers_session.go` の `sessionTerminateTimeout` (同ファイルと `internal/api/logtail.go` が参照)
  - `backend/internal/cli/ec2.go` の `ec2TerminateTimeout` (同ファイルと issue 0135 で追加された `internal/cli/ecs.go` が参照)
  - 5 箇所とも `context.WithTimeout(context.Background(), awsinternal.TerminateSessionGracePeriod)` を自分で組み立てる形は変えていない (案 A どおり、案 B の `TerminateSSMSessionWithGrace` のような一括関数は新設していない)。
  - 旧定数の削除に伴い、`internal/cli/ec2.go` と `internal/api/handlers_session.go` で不要になった `"time"` import を削除し、新たに `awsinternal` を参照するようになった `internal/api/logtail.go` と `internal/session/bridge.go` に import を追加した。循環 import が生じないことを `go build ./...` と依存関係の実測で確認した。
- 完了条件の「切断の猶予の定義が 1 箇所」を実効あるものにするため、`backend/internal/aws/ssm_session_test.go` に `TerminateSessionGracePeriod` の値 (5 秒) を固定するテストを追加した (観点2 レビューで、統合前の 3 定数もその値を検証するテストが 1 つも無かったことが判明したための対応。ミューテーションテストで `1 * time.Millisecond` に変えると `internal/cli`/`internal/api`/`internal/session` の既存テストは 1 つも落ちないことを確認し、値そのものを検証する意味があると判断した)。

### 多観点レビューの結果

- 観点1 (完了条件充足): 指摘なし。定義の一元化、5 箇所すべての参照、既存の中断後切断テストの通過、`mise run check` の通過をいずれも実物確認で満たしていることを確認した。
- 観点2 (テスト品質): 中優先度の指摘「`TerminateSessionGracePeriod` の値そのものを検証するテストが無い」を反映し、`ssm_session_test.go` を追加した (上述)。反映分に限定した追加レビューを 1 回行い、指摘なしを確認した。低優先度の指摘 (`bridge.go`/`handlers_session.go`/`logtail.go` の切断動作を検証するテストが元から無い) は、本 issue のリファクタリングで悪化させたものではなく、issue 0136 のスコープ外として却下した (将来別 issue で検討する余地はあるが、バグではないため今回は登録しない)。
- 観点3 (堅牢性): 中優先度の指摘「`internal/session/bridge.go` が定数を得るためだけに `internal/aws` を新規 import するようになり、`TerminateFunc` による AWS 非依存の抽象化方針が部分的に崩れる」は却下した。理由は次のとおり。(1) `bridge.go` の `Terminate` フィールドの既存コメントが「SSM セッションのクリーンアップに使う」と明記しており、パッケージは元々概念的に SSM を意識した設計である。(2) 実装上の代替案 (`Bridge` に `TerminateTimeout time.Duration` フィールドを追加し呼び出し側から注入する) は、フィールドを設定し忘れると `context.WithTimeout` に 0 が渡って毎回即座に失敗するという新しい不具合の余地を生むうえ、本番の構築箇所は `internal/api/handlers_session.go` の 1 箇所のみで得られる decoupling の効果が小さい。YAGNI の原則から、公開定数への直接参照という単純な形を維持するほうが堅牢性上優れると判断した。低優先度の指摘 (godoc が `TerminateSSMSession` 専用であることを明示しきれておらず将来の誤用リスクがわずかにある) は反映し、コメントを「`TerminateSSMSession` specifically, not for cleanup in general」と明示する文言に修正した。
- 観点4 (規約準拠): 指摘なし。
- 観点5 (回帰と整合): 中優先度の指摘「issue 0142 のコード引用が旧定数名 `ec2TerminateTimeout` のままで、今後の実装者が誤認するリスクがある」を反映し、issue 0142 の「## 関連」に読み替えを促す補足を追記した (0142 の既存記述は書き換えず追記のみ)。低優先度の指摘「CHANGES.md の『4 箇所に... 別名定義』という表現が、実際には定義 3 箇所・参照 2 箇所の計 5 箇所である実態とややずれている」を反映し、CHANGES.md の文言を「3 箇所に...別名定義され、5 箇所 (定義 3 箇所 + 参照 2 箇所) で使われていた」に修正した。

### 補足

`go build ./...`、`go vet ./...`、`gofmt -l .`、`go test -race ./...` (backend 全パッケージ)、および `mise run check` (backend + frontend) がいずれも新たな失敗なく通過することを確認した。frontend への変更は無い。
