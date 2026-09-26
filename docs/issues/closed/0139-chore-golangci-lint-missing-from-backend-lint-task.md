# golangci-lint が backend:lint タスクに入っておらず AGENTS.md の記載と食い違う

Created: 2026-08-09
Model: Claude Opus 5
Completed: 2026-08-11

## 背景

`AGENTS.md` のタスク一覧は `mise run backend:lint` の内容を以下と記載している。

| タスク | 内容 |
| --- | --- |
| `mise run backend:lint` | `go vet` + `staticcheck` + `govulncheck` + `golangci-lint` |

実際の `mise.toml` は以下である。

```toml
[tasks."backend:lint"]
description = "backend 静的解析"
dir = "backend"
run = """
go vet ./...
staticcheck ./...
govulncheck ./...
"""
```

`golangci-lint` が入っていない。`backend/.golangci.yml` は存在し、内容は `version: "2"` の 1 行のみである (v2 の既定の linter 集合をそのまま使う設定であり、errcheck が有効になる)。

golangci-lint は導入されており、単体では動く (確認したバージョンは 2.10.1)。`mise run check` は `fmt` + `lint` + `test` を順に実行するため、golangci-lint はどの経路からも実行されない。

## 問題

設定ファイルとドキュメントは golangci-lint が検査に含まれることを示しているが、実際には実行されない。ドキュメントを信じて「lint は通っている」と判断すると、golangci-lint の検出は見ていないことになる。

現状の検出件数は以下である (`golangci-lint run ./...`)。

```
37 issues:
* errcheck: 36
* staticcheck: 1
```

ファイル別の内訳は以下である。

| 件数 | ファイル |
| --- | --- |
| 4 | internal/gcp/gcs.go |
| 4 | internal/cli/run.go |
| 4 | internal/api/handlers_gcp.go |
| 3 | internal/gcp/cloudrun.go |
| 3 | internal/datadog/usage_test.go |
| 3 | internal/api/handlers_s3_object.go |
| 2 | internal/tidb/resources.go |
| 2 | internal/tidb/client.go |
| 2 | internal/snippet/snippet.go |
| 2 | internal/pricecache/pricecache.go |
| 1 | internal/tidb/resources_test.go |
| 1 | internal/cli/gcp.go |
| 1 | internal/aws/s3.go |
| 1 | internal/aws/cloudwatchlogs.go |
| 1 | internal/api/server.go |
| 1 | internal/api/handlers_pricing.go |
| 1 | internal/api/handlers_aws.go |
| 1 | internal/api/errors.go |

errcheck の 36 件の大半は、標準エラー出力や標準出力への書き込みの戻り値を確認していないものである。例えば以下である。

```
internal/cli/run.go:40:15: Error return value of `fmt.Fprintln` is not checked (errcheck)
		fmt.Fprintln(stderr, "interrupted")
internal/cli/gcp.go:378:15: Error return value of `fmt.Fprintln` is not checked (errcheck)
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: object list truncated, narrow down with --prefix")
```

staticcheck の 1 件は `internal/aws/s3.go:127` の `QF1001` (ド・モルガンの法則を適用できる) である。`mise run backend:lint` が実行している `staticcheck ./...` では検出されない。既定で有効な検査の集合が golangci-lint の staticcheck と単体の staticcheck で違うためである。

## なぜ対応が必要か

- ドキュメントと実際の検査内容が食い違っている。`AGENTS.md` は「作業前に必ず参照すること」とされており、記載を信じた判断が誤る。
- `.golangci.yml` が置かれているのに実行されない。設定ファイルの存在が、検査されているという誤解を生む。
- 37 件はいずれも致命的ではないが、実行されない状態が続く限り件数は増え続ける。検査を有効にするかしないかを決めずに放置している状態が問題である。

## 修正方針

どちらに揃えるかを決める必要がある。

案 A: `mise.toml` に `golangci-lint run ./...` を追加し、37 件を解消する。

- `AGENTS.md` の記載どおりになる。
- errcheck の 36 件それぞれに対応方針が必要である。標準エラー出力への書き込みは失敗しても他にできることが無いため、`_ =` を付けて明示的に無視するのが素直である。ただしグローバル `~/.codex/AGENTS.md` は「`_` で握り潰すのは原則禁止 (明確な理由をコメントで残せる場合のみ可)」と定めており、36 箇所すべてに理由のコメントを書くのは現実的ではない。errcheck の除外設定 (`fmt.Fprint` 系を除外する) を `.golangci.yml` に入れるか、コメントを 1 箇所にまとめて書けるようにするかを決める必要がある。
- `internal/gcp` と `internal/tidb` の errcheck には、標準出力への書き込みではないものが含まれる可能性がある。個別に確認すること。

案 B: `AGENTS.md` の記載から golangci-lint を削除し、`backend/.golangci.yml` も削除する。

- 実態に揃う。検査の集合は `go vet` + `staticcheck` + `govulncheck` のままとなる。
- golangci-lint の staticcheck が検出する `QF1001` などが検査されない状態が確定する。

案 C: `mise.toml` に追加し、`.golangci.yml` で errcheck を無効化する。

- staticcheck の追加検出 (`QF1001` 等) だけを取り込める。
- errcheck を無効にする理由を設定ファイルに残す必要がある。既定の集合から意図的に外すことになるため、判断の記録が必要である。

案 A / 案 B / 案 C のいずれを採るかは、errcheck を検査に入れるかどうかの判断であり、実装前に決める必要がある。

## 完了条件

- `AGENTS.md` の記載と `mise.toml` の内容が一致している。
- `.golangci.yml` の存在が実態と合っている (実行しないなら削除する)。
- 検査に入れた場合、`mise run check` が 0 件で通る。
- 除外設定を入れた場合、除外の理由が設定ファイルに書かれている。

## 関連

- docs/issues/0131: 起票元。この issue の作業中に `internal/cli/run.go` の 4 件が新規に生まれ、既存の 33 件と同じ扱いになっていることに気付いた。

## 解決方法

方針は案B (golangci-lint をやめる) を採用した。ユーザーに確認して決定した。

### 実装

- `AGENTS.md` の `mise run backend:lint` の説明を `go vet` + `staticcheck` + `govulncheck` + `golangci-lint` から `golangci-lint` を除いた形に変更し、`mise.toml` の実際の内容 (元々 golangci-lint を呼んでいない) と一致させた。
- golangci-lint の設定ファイルを削除した。本文は `backend/.golangci.yml` と記載しているが、実際にリポジトリに存在したのはリポジトリルート直下の `.golangci.yml` (内容は `version: "2"` の 1 行のみ) であり、`backend/.golangci.yml` は元から存在しなかった。本文のパス記載は誤りと判断し、実際に存在した唯一の設定ファイルであるルートの `.golangci.yml` を削除した。
- `mise.toml` は変更していない (元々 golangci-lint を呼んでいなかったため)。

### 本文の「現状の検出件数」の記載についての訂正

本文は golangci-lint の検出件数を「37 issues (errcheck: 36, staticcheck: 1)」、内訳を「errcheck の 36 件の大半は、標準エラー出力や標準出力への書き込みの戻り値を確認していないもの」と記載しているが、これは golangci-lint の既定の表示上限 (`max-same-issues` 等。削除した `.golangci.yml` はこの既定値を上書きしていなかった) で切り捨てられた見かけの数字だった。上限を外して (`--max-same-issues=0 --max-issues-per-linter=0`) 再実行すると errcheck は 57 件相当になり、実際の内訳は `fmt.Fprint` 系の標準出力・標準エラー出力への書き込みが約 25 件、リソースの `Close()` / `os.Remove` の戻り値未チェックが約 28 件、その他 (`json.Encoder.Encode` / `http.ResponseWriter.Write` / `io.WriteString`) が 4 件だった。「大半が標準出力・標準エラー出力への書き込み」という記述は不正確で、実際には `Close()` 系が同程度かそれ以上を占める。また、既定の表示上限下では同一メッセージのグルーピングが解析の並列実行順序に依存するため、実行ごとに表示される具体的な指摘の組み合わせが変わる (合計件数の 37 自体は安定するが、内訳は変動する)。

いずれの分類も個別に確認した範囲では、エラーの握り潰しによる実害があるものは見つからなかった (`Close()` は正常パスで既にチェック済みの後始末、標準エラー出力への書き込みは失敗時に他に取れる対応が無い、`io.WriteString` の対象は `crypto/md5` の `hash.Hash` で契約上エラーを返さない、HTTP レスポンスへの書き込みはヘッダ送信後で対処不能なタイミング)。今回は案B (golangci-lint を検査に含めない) を採用したためコードの変更は行っていないが、この事実は将来案 A / C に戻す判断をする際の参考として記録する。`CHANGES.md` には、上記の非決定性を踏まえ確定した総数・分類別件数を書かず、定性的な内容のみを記載した。

staticcheck の `QF1001` (`internal/aws/s3.go:127`) は golangci-lint 経由の staticcheck と単体の `staticcheck ./...` で既定の検査集合が異なるため検出されない。この差は golangci-lint を検査に含めない今回の方針の下でも変わらず残る。

### 多観点レビューの結果

5 観点のレビューを 1 ラウンド、および CHANGES.md エントリの文面修正に対する追加レビューを 2 ラウンド実施した。

- 観点 1 (完了条件充足): 指摘なし。完了条件 4 行のうち、案 B で対象となる 2 行 (AGENTS.md と mise.toml の一致、`.golangci.yml` の実態一致) はいずれも満たされていることを確認した。
- 観点 2 (テスト品質): 指摘なし。コード・テストファイルの変更を伴わない chore のため新規テストは無し。`mise run check` が通ることを確認した。
- 観点 3 (堅牢性)
  - [優先度: 中、反映] errcheck 36 件の内訳の実態 (標準エラー出力書き込みは一部で、大半はリソース `Close()` 系であること) を記録しないと、将来案 A / C に戻す判断者が誤解するおそれがあるとの指摘。上記「本文の記載についての訂正」に記録した。
  - [優先度: 中、反映] golangci-lint 経由の staticcheck と単体の staticcheck の既定検査集合の違いが今後も残り続けることを記録すべきとの指摘。上記に記録した。
  - [優先度: 低、対応不要] 本文のパス記載 (`backend/.golangci.yml`) の誤り。実装欄と本節に記録済み。
- 観点 4 (規約準拠)
  - [優先度: 高、反映] `CHANGES.md` への変更履歴の記載が漏れていた。`### misc` セクションに新規エントリを追加した。
- 観点 5 (回帰と整合)
  - [優先度: 中、反映] 追加した `CHANGES.md` エントリの内訳記述が本文の (誤った) 記述と矛盾したまま残っていた。上記の訂正内容に揃えて書き直した。
  - [優先度: 中、反映] 「実害なし」の結論が内訳の一部の分類しかカバーしていなかった。4 分類全てを指す形に書き直した (その後の追加レビューで、この内訳の数値自体が golangci-lint の表示上限による非決定的な値だったことが判明し、確定した数値を書かない形にさらに書き直した)。
  - [優先度: 低、対応不要] 本文のパス記載の誤り (観点 3 と同一の指摘)。
- 追加レビュー 1 回目 (CHANGES.md の内訳修正後)
  - [優先度: 高、反映] 「その他 4 件」が実際には関数の種類数であり、インスタンス数ではなかった。golangci-lint の既定の表示上限 (`max-same-issues` 等) により、そもそも「37 件」自体が全検出数ではなく上限で切り捨てられた見かけの数字であることが判明した。上限を外すと errcheck は 57 件相当になる。同一実行でも `Close()` 系以外の内訳が実行ごとに変動する非決定性も確認した。これを受けて `CHANGES.md` の文面を、確定した数値を書かず定性的な内容のみを記載する形に書き直した。
- 追加レビュー 2 回目 (数値を書かない形への修正後)
  - 指摘なし (CHANGES.md エントリ自体)。issue 本文のパス記載の誤りとの不整合が中優先度で再度挙がったが、これは issue 本文側の記述の問題であり、implement-issues スキルの規則 (既存の記述は書き換えず追記のみ) に従い、本文は書き換えず本節に訂正を記録するにとどめた。文体上の軽微な言い回しの指摘 (低優先度) は対応不要と判断した。

反映後、`mise run check` が通過することを確認した (脆弱性 0 件、backend / frontend の全テスト成功)。
