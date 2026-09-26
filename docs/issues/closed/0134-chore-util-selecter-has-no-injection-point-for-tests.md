# util.Select にテストの差し込み口が無く、実行失敗と未選択の分岐が検証されていない

Created: 2026-08-08
Model: Claude Opus 5
Completed: 2026-08-11

## 背景

issue 0130 (`internal/util` のエラーラップを `%w` と「動詞 + 対象」に揃える) の作業中に、`backend/internal/util/selecter.go` の `Select` が単体テストで到達できない分岐を 3 つ持っていることが判明した。

```go
func Select(items []Item, prompt string) (Item, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no items to select")
	}

	initialModel := model{
		items:  items,
		cursor: 0,
		prompt: prompt,
	}

	p := tea.NewProgram(initialModel)
	m, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("run bubble tea program: %w", err)
	}

	finalModel := m.(model)
	if finalModel.selected == nil {
		return nil, fmt.Errorf("no item selected")
	}

	return finalModel.selected, nil
}
```

`tea.NewProgram` と `p.Run()` が関数の中に直接書かれているため、`Run` を差し替えられない。`Run` は端末を掴むため、テストから呼ぶと環境 (TTY の有無) に結果が左右される。

`internal/util/selecter_test.go` が検証しているのは `TestSelect_EmptyItems` (0 件の分岐) と `model` のメソッド (`Init` / `Update` / `View`) だけである。`Select` の残り 3 分岐は未検証である。

## 問題

### 問題 1: 実行失敗の分岐が検証されていない

`p.Run()` が失敗したときのラップ (`run bubble tea program: %w`) を通るテストが無い。issue 0130 でこの行を `%v` から `%w` に変えたが、`errors.As` でチェーンが保たれることを検証できていない。将来 `%v` に差し戻されても、どのテストも落ちない。

同じ性質の変更を `internal/util/executer.go` に加えた issue 0128 では、差し込み口があったため `errors.As` による検証を伴えた。`selecter.go` にはそれが無い。

### 問題 2: 未選択の分岐が検証されていない

利用者が選択せずに終了した場合 (`q` や `Ctrl-C`) に `no item selected` を返す分岐を通るテストが無い。`model.Update` が `tea.Quit` を返すことは検証されているが、それを受けて `Select` が何を返すかは検証されていない。

### 問題 3: 型アサーションが panic 経路になっている

`finalModel := m.(model)` はカンマ ok 形式ではない。`p.Run()` が `model` 以外を返した場合に panic する。

現在の実装では `tea.NewProgram(initialModel)` に渡すのが `model` なので、bubbletea が別の型を返すことは実際には起こらない。ただし AGENTS.md は「リクエスト処理中の `panic` は禁止」と定めており、`Select` は CLI の対話処理の中で呼ばれる。差し込み口を入れると、この経路がテストから到達可能になる。

## なぜ対応が必要か

`Select` は `internal/cli` の対話式リソース選択で使われる。ここが失敗したときに利用者が受け取るのはラップしたエラーであり、その形が検証されていない。

グローバル `~/.codex/AGENTS.md` は「品質は自動テストが全て決める」と定めている。issue 0130 で本番コードを変更したにもかかわらず、その正しさをテストで担保できていないのは方針に反する。

## 修正方針

issue 0127 が `internal/aws/sso_oidc.go` に施したのと同じ形で、プログラムの実行を関数値として切り出す。

```go
// selectRunner は対話式プログラムの実行を抽象化する。
type selectRunner func(tea.Model) (tea.Model, error)

// runTeaProgram は bubbletea のプログラムを組んで実行する本番の実装。
func runTeaProgram(m tea.Model) (tea.Model, error) {
	return tea.NewProgram(m).Run()
}

func Select(items []Item, prompt string) (Item, error) {
	return selectWith(items, prompt, runTeaProgram)
}

// selectWith は実行を差し替えられる Select のコア。
func selectWith(items []Item, prompt string, run selectRunner) (Item, error) { ... }
```

あわせて `m.(model)` をカンマ ok 形式にし、型が合わない場合はエラーを返す。

`no items to select` と `no item selected` は `fmt.Errorf` に書式指定子が無く、実質 `errors.New` である。呼び出し側が `errors.Is` で判別する必要があるかを検討し、必要ならセンチネル化する。不要なら `errors.New` に変える。

調査結果：`util.Select` の呼び出し元は `internal/cli/ec2.go` (`select instance: %w` でラップして返すのみ) と `internal/cli/ecs.go` (`select cluster: %w` でラップして返すのみ) の 2 箇所のみで、いずれも `errors.Is` / `errors.As` による分岐を行わず、そのまま上位へ伝播させている。現時点でセンチネル化を要する呼び出し元は無いため、`errors.New` に変える方を採る。

## 完了条件

- `selectWith` が実行の関数値を受け取り、`Select` が本番の実装を渡す形になっている。
- `p.Run()` の失敗が `run bubble tea program` でラップされ、`errors.As` で元のエラーへ到達できることをテストで検証する (`%v` に差し戻すと落ちる)。
- 未選択で終了した場合に `no item selected` を返すことをテストで検証する。
- 選択に成功した場合に選択された `Item` を返すことをテストで検証する。
- 型アサーションが panic せず、エラーとして返ることをテストで検証する。
- テストが TTY の有無に依存しない。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0130: 起票元。`selecter.go` の `%v` を `%w` に変えたが、その正しさを検証するテストを書けなかった。
- docs/issues/closed/0127: `internal/aws/sso_oidc.go` に同じ形の差し込み口を作った issue。
- docs/issues/closed/0128: `internal/util/executer.go` の `%v` を `%w` に変え、差し込み口があったため `errors.As` の検証を伴えた issue。

## 解決方法

`backend/internal/util/selecter.go` の `Select` を修正方針のとおり分割した。

- 対話式プログラムの実行を `type selectRunner func(tea.Model) (tea.Model, error)` として切り出し、本番の実装 `runTeaProgram` (`tea.NewProgram(m).Run()` を呼ぶだけ) を用意した。
- `Select(items, prompt)` は `selectWith(items, prompt, runTeaProgram)` を呼ぶだけの薄いラッパーにした。
- `selectWith` がコアの実装を持ち、`run` 引数で実行を差し替えられるようにした。
- `finalModel := m.(model)` をカンマ ok 形式 (`finalModel, ok := m.(model)`) にし、`ok` が偽なら `fmt.Errorf("unexpected select program result type %T", m)` を返すようにした (panic しない)。
- `no items to select` と `no item selected` は、調査結果のとおり呼び出し元 (`internal/cli/ec2.go` / `internal/cli/ecs.go`) がどちらも `errors.Is` で分岐しないため、センチネル化せず `fmt.Errorf` (書式指定子無し) から `errors.New` に変えた。

方針セクションからの乖離は無い。

完了条件の各行への対応は次のとおり。

- 「`selectWith` が実行の関数値を受け取り、`Select` が本番の実装を渡す形になっている」: 上記のとおり実装済み。
- 「`p.Run()` の失敗が `run bubble tea program` でラップされ、`errors.As` で元のエラーへ到達できることをテストで検証する (`%v` に差し戻すと落ちる)」: `selecter_test.go` に追加した `TestSelectWith_RunFailureIsWrapped` で、`run` が独自型 `fakeSelectRunnerError` を返したときに `errors.As` でその型を取り出せることを検証する。`%w` を一時的に `%v` に戻して同テストが失敗することを確認済み (ミューテーション確認)。
- 「未選択で終了した場合に `no item selected` を返すことをテストで検証する」: `TestSelectWith_NoSelectionReturnsError` で、`run` が `selected` を設定しないまま model を返したときに `no item selected` を含むエラーが返ることを検証する。
- 「選択に成功した場合に選択された `Item` を返すことをテストで検証する」: `TestSelectWith_ReturnsSelectedItem` で、`run` が `selected` を設定した model を返したときにその `Item` がそのまま返ることを検証する。
- 「型アサーションが panic せず、エラーとして返ることをテストで検証する」: `TestSelectWith_UnexpectedResultTypeReturnsErrorWithoutPanic` で、`run` が `model` ではない `fakeTeaModel` を返したときに panic せずエラーが返ることを検証する。
- 「テストが TTY の有無に依存しない」: 新設した 4 テストはいずれも `run` に fake 関数を注入し `tea.NewProgram` を呼ばないため、TTY に依存しない。既存の `TestSelect_EmptyItems` も items が空の場合は `run` を呼ぶ前に return するため元々 TTY に依存しない。
- 「`mise run check` が通る」: 実行して確認した (Step 1 のベースラインからの新たな失敗は無い)。

多観点レビュー (5 観点 × 1 ラウンド) の結果と反映は次のとおり。

- 観点1 (完了条件充足)：指摘なし。
- 観点2 (テストの品質)：低優先度の指摘 2 件のうち 1 件を反映した。
  - `TestSelectWith_UnexpectedResultTypeReturnsErrorWithoutPanic` がエラーの有無しか検証せず、文言 (`unexpected select program result type %T`) を変えても検出できない指摘を受け、`strings.Contains` でメッセージと実際の型名 (`fakeTeaModel`) が含まれることを検証するよう追加した。
  - `Select` が `selectWith` に本番の `runTeaProgram` を正しく結線しているかを検証するテストが無い指摘は却下した。理由は、`runTeaProgram` は `tea.NewProgram(m).Run()` を呼ぶ本物の実装であり、これを呼ぶテストは TTY を掴むため、完了条件「テストが TTY の有無に依存しない」と両立しない。
- 観点3 (堅牢性)：低優先度の指摘 2 件はいずれも却下した。
  - テストの fake run 実装内 (`finalModel := m.(model)`) がカンマ ok 形式でない指摘は、この型アサーションは常にテスト自身が組み立てた `model` 型の値を渡す経路であり型不一致が起こり得ないため、実害が無いと判断した。
  - `selectRunner` に `nil` が渡された場合に呼び出しで panic する指摘は、`selectWith` は非公開でパッケージ内 (`Select` とテスト) からしか呼ばれず、いずれも `nil` を渡さないため、完了条件にも修正方針にも無いスコープ外の堅牢化と判断した。
- 観点4 (規約準拠)：指摘なし。
- 観点5 (回帰と整合)：低優先度の指摘 1 件を反映した。`CHANGES.md` のエントリが `no items to select` / `no item selected` を `errors.New` に変えた点に触れておらず、「## 解決方法」に明記した変更点との突き合わせで抜けがあるとの指摘を受け、`CHANGES.md` のエントリにこの変更とその理由 (呼び出し元がセンチネル判別をしないため、エラー文字列自体は変わらない) を追記した。

いずれの観点にも優先度「高」「中」の指摘は無かった。反映した指摘はテスト検証の追加と `CHANGES.md` の記述追加にとどまり、`selecter.go` の実装は変更していないため、反映後に `go test -race ./internal/util/...` と `mise run check` を再実行して全通過を確認し、変更点に限った追加レビューは行わなかった。
