# mise run backend:mocks が設定ファイル不在で必ず失敗する

Created: 2026-09-26
Model: Claude Opus 5
Completed: 2026-09-26

## 症状

`mise run backend:mocks` を実行すると、mockery が設定ファイルを読めないまま対象を決められず、終了コード 1 で失敗する。

```
[backend:mocks] $ "$(mise which mockery)"
INF couldn't read any config file version=v2.53.6
INF Starting mockery dry-run=false version=v2.53.6
INF Using config:  dry-run=false version=v2.53.6
FTL Use --name to specify the name of the interface or --all for all interfaces found dry-run=false version=v2.53.6
[backend:mocks] ERROR task failed
```

## 再現手順

1. リポジトリルートで `mise run backend:mocks` を実行する。
2. 上記の出力が出て終了コード 1 になる。
3. 期待する結果: モックが生成されて終了コード 0 で終わる。「## 未確定論点」の 2 案目 (タスクの削除) を採る場合は、タスクと `AGENTS.md` の該当行が削除されて `mise run` の一覧に `backend:mocks` が現れないことが期待する結果になる。どちらかは「## 未確定論点」の決着による。

docs/issues/closed/0198 の修正 (実行バイナリを `"$(mise which mockery)"` で解決する) の前後どちらの形でも同じ結果になる。修正前の形 (`run = "mockery"`) を `mise exec -- sh -c 'mockery'` で再現しても同じ FTL で終了コード 1 だった (2026-09-26 に実測)。mockery は `~/go/bin` に同名バイナリが無く、PATH 解決と `mise which` が同じ実体を返すため、docs/issues/closed/0198 の隠蔽とは独立した問題である。

## 原因

- `backend/` 配下に mockery の設定ファイル (`.mockery.yaml`、`.mockery.yml`、`.mockery.json`) が無い。
- `[tasks."backend:mocks"]` は引数を渡さず mockery を起動するため、`--name` も `--all` も指定されず生成対象が決まらない。
- リポジトリに mockery が生成したモックが無く、`backend/` の Go ファイルと `go.mod` に mockery / `go.uber.org/mock` / gomock への参照も無い (2026-09-26 時点で grep 0 件)。タスクは定義されているだけで、一度も成功していない可能性がある。
- `AGENTS.md` の backend テストの規約はモック生成に `go.uber.org/mock` または手書きモックを指定しており、mockery は規約上の選択肢に入っていない。一方で同ファイルのタスク表には `mise run backend:mocks | mockery でモック生成` の行が残っている。

## 未確定論点

どちらの方向を採るかの判断が必要。

- タスクを削除する。`[tasks."backend:mocks"]` と `AGENTS.md` のタスク表の該当行を消し、`[tools]` の mockery も併せて外すかを決める。モックを使っていない現状には合うが、将来モックを導入する際にタスクを作り直すことになる。
- mockery の設定ファイルを追加して生成対象を定義する。ただし backend の規約が mockery を選択肢としていないため、規約の改定とセットになる。

2026-09-26 にユーザーの決定で解消した。1 案目 (タスクの削除) を採り、`[tools]` の mockery も併せて外す。内容は「## 修正方針」に書く。

## 修正方針

`mise.toml` からタスクとツールの両方を削除し、`AGENTS.md` のタスク表を実態に合わせる。

- `mise.toml` の `[tools]` から `"go:github.com/vektra/mockery/v2" = "latest"` を削除する。
- `mise.toml` から `[tasks."backend:mocks"]` のブロックを削除する。
- `AGENTS.md` の backend タスク表から `mise run backend:mocks` の行を削除する。

採らなかった案と却下の理由。

- mockery の設定ファイル (`backend/.mockery.yaml`) を追加して生成対象を定義する案を却下した。`AGENTS.md` の backend テストの規約はモック生成に `go.uber.org/mock` または手書きモックを指定しており、mockery を選択肢に入れていない。この案は規約の改定とセットになる。`backend/` には mockery が生成したモックを使うテストが 1 件も無く (mockery / `go.uber.org/mock` / gomock への参照は 0 件)、規約に沿った手書きモック (`backend/internal/aws/iam_test.go` の `mockIAMUserDetailClient` など 22 箇所) で足りているため、mockery の生成対象を新たに定義する理由が無い。
- タスクだけを削除して `[tools]` の mockery を残す案を却下した。生成タスクが無ければツールを導入する理由が残らず、`mise install` が mockery を取得する分だけセットアップが遅くなる。

新しい API と権限は不要である。削除だけで、CLI と API サーバの動作には影響しない。

検証は `mise tasks ls` の出力に `backend:mocks` が現れないことと、`mise run check` の通過で行う。タスク定義はテスト対象のコードではなく、検証が `mise` コマンドの実行そのものになるため、自動テストは追加しない。

## 完了条件

- `mise run backend:mocks` が終了コード 0 で終わる。またはタスクと `AGENTS.md` の該当行が削除され、`mise run` のタスク一覧に `backend:mocks` が現れない。
- `AGENTS.md` のタスク表の記載と `mise.toml` の実態が一致する。
- `mise run check` が通過する。

## 解決方法

`mise.toml` から mockery のタスクとツールを削除し、`AGENTS.md` のタスク表を実態に合わせた。

- `mise.toml` の `[tools]` から `"go:github.com/vektra/mockery/v2" = "latest"` を削除した。
- `mise.toml` から `[tasks."backend:mocks"]` のブロック (description、dir、run の 3 行) を削除した。
- `AGENTS.md` の backend タスク表から `mise run backend:mocks` の行を削除した。

完了条件の検証は次のとおり。

- 「`mise run backend:mocks` が終了コード 0 で終わる。またはタスクと `AGENTS.md` の該当行が削除され、`mise run` のタスク一覧に `backend:mocks` が現れない」: 後者で満たした。`mise tasks ls` の出力に `backend:mocks` は現れず (grep の一致 0 件)、`mise run backend:mocks` は `no task backend:mocks found` で終了コード 1 になる。
- 「`AGENTS.md` のタスク表の記載と `mise.toml` の実態が一致する」: `AGENTS.md` の表から `mise run <task>` を抽出し、`mise tasks ls --all` の一覧と突き合わせて、`mise.toml` に存在しないタスクが 0 件であることを確認した。表は主要タスクの抜粋であり、検査は記載側から実態側への片方向で行った (`mise.toml` にあって表に無いタスクは変更前から存在し、本 issue の対象ではない)。
- 「`mise run check` が通過する」: 終了コード 0。frontend は Test Files 113 passed / Tests 1234 passed、backend は 18 パッケージすべて ok。ベースライン (失敗 0 件) から新たな失敗は無い。

再現確認。修正前は `mise run backend:mocks` が `FTL Use --name to specify the name of the interface or --all for all interfaces found` を出して終了コード 1 になった。修正後はタスク自体が存在しないため、この失敗経路が消滅した。

自動テストは追加していない。タスク定義はテスト対象のコードではなく、検証が `mise` コマンドの実行そのものになるためである。

`CHANGES.md` の既存エントリには mockery と `backend:mocks` への言及が残るが、過去の変更履歴の記録であるため書き換えていない。

レビューで判明した記述の誤りを 2 件訂正した。1 件目は「## 修正方針」の却下理由にあった「`backend/` にモックを使うテストが 1 件も無い」で、`backend/` には規約に沿った手書きモックが 22 箇所ある。正しくは「mockery が生成したモックを使うテストが無い」であり、却下の根拠は規約が mockery を選択肢にしていないことと、手書きモックで足りていることに限られる。2 件目は自動テストを追加しない理由の「Go と TypeScript のテストコードから参照できない」で、テストコードから `mise.toml` を読むことも `mise` を実行することも技術的には可能であるため、上記の表現に改めた。

レビューで、「## 再現手順」の 3 が「## 未確定論点」の案を「2 案目 (タスクの削除)」と書いている誤りの指摘を受けた。同節の並びでは 1 案目がタスクの削除である。この行は起票時からある記述で本スキルの書き換え対象ではないため、訂正せずここに記録する。
