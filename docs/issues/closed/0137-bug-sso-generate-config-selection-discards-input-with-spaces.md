# sso generate-config の選択入力に空白が混ざると選択全体が黙って破棄される

Created: 2026-08-09
Model: Claude Opus 5
Completed: 2026-08-11

## 背景

issue 0131 (CLI コマンドがシグナル連動 context を持たない) の作業中に、`thief sso generate-config` の対話式選択が `1, 2` のような空白を含む入力を黙って捨てることが判明した。

選択の読み取りは `backend/internal/cli/helper.go` の `promptSelection` が行う (issue 0131 で `fmt.Scanln` から移したもので、読み取りの意味は変えていない)。

```go
	return readWithContext(ctx, func() (string, error) {
		var input string
		if _, err := fmt.Fscanln(in, &input); err != nil {
			// 空入力はそのまま扱う。
			return "", nil
		}
		return input, nil
	})
```

`fmt.Fscanln` は書式指定子 1 つに対して 1 語しか読まない。行に 2 語以上があると `expected newline` を返す。実測は以下である。

| 入力 | n | 読めた値 | err |
| --- | --- | --- | --- |
| `all\n` | 1 | `all` | nil |
| `1,2\n` | 1 | `1,2` | nil |
| `1, 2\n` | 1 | `1,` | `expected newline` |
| `\n` | 0 | 空 | `unexpected newline` |
| `` (EOF) | 0 | 空 | `EOF` |

err が nil でない場合は空文字を返すため、`1, 2` と入力すると選択は空になる。

## 問題

プロンプトの文言は以下である。

```
Select accounts to configure (comma-separated numbers, or 'all' for all accounts):
```

「カンマ区切り」しか指定していないため、`1, 2` は文言に沿った入力である。ところが結果は「1 も 2 も選ばれない」になる。

アカウント選択で空になった場合は `no valid accounts selected` を返して終了する。エラーは出るが、文言は入力の何が悪かったのかを伝えない。利用者は空 Enter を押した場合と同じメッセージを受け取る。

ロール選択で空になった場合はさらに悪い。`selectIndices` が空を返し、そのアカウントのプロファイルが 1 つも作られないまま次のアカウントへ進む。エラーは出ない。全アカウントで同じことが起きた場合にだけ、最後に `no roles selected for any accounts` が出る。一部のアカウントで起きた場合は、指定したはずのロールが黙って設定ファイルから漏れる。

## 再現手順

1. AWS SSO のアクセスポータルにアクセスできるアカウントを 2 つ以上用意する。
2. `thief sso generate-config --url <portal>` を実行する。
3. アカウント選択のプロンプトで `1, 2` と入力する (カンマの後ろに空白を 1 つ入れる)。

期待: 1 番目と 2 番目のアカウントが選択される。

実際: `Error: no valid accounts selected` が表示されて終了する。

手順 3 で `1,2` (空白なし) と入力した場合は期待通りに動く。

ロール選択でも同じことが起きる。この場合はエラーが出ず、そのアカウントのプロファイルが作られないまま処理が続く。

## なぜ対応が必要か

- プロンプトの文言に沿った入力が通らない。カンマ区切りの列挙に空白を入れるのは一般的な書き方である。
- ロール選択の場合はエラーにもならず、生成された `~/.aws/config` から意図したプロファイルが欠ける。利用者が気付くのは、後でそのプロファイルを使おうとして失敗したときになる。
- `1, 2` の読み取り結果が `1,` になっている通り、`fmt.Fscanln` の 1 語しか読まない性質と「1 行を読む」という意図が合っていない。

## 修正方針

`promptSelection` の読み取りを、1 語ではなく 1 行を読む形に変える。`bufio.Scanner` または `bufio.Reader.ReadString('\n')` を使い、読んだ行の前後の空白を `strings.TrimSpace` で落とす。

読み取りは `readWithContext` の中に置いたままにする (issue 0131 で入れた context 連動を壊さないこと)。`internal/cli` の本番コードで context を見ない標準入力の読み取りを 2 箇所に固定している `TestNoContextBlindStdinReadOutsideDesignatedFunctions` の一覧は、`bufio` へ移すと検出対象から外れる。検出対象に `bufio.NewScanner` / `bufio.NewReader` を追加するかを判断する。

行を読む形にすると、以下も併せて決める必要がある。

- 空白のみの行を空入力 (選択なし) として扱うか。現在の `\n` と同じ扱いにするのが自然である。
- `selectIndices` に渡す前に、カンマで分割した各要素を `TrimSpace` するか。`promptSelection` で行全体の空白を落とすだけでは `1, 2` は `1, 2` のまま渡るため、`selectIndices` 側の分割でも空白を落とす必要がある。`selectIndices` の現在の実装を確認して決める。
- EOF を空入力として扱う現在の挙動は変えないこと。パイプ経由で入力を渡した場合に依存している可能性がある。

`selectIndices` が不正な要素をどう扱っているか (黙って捨てるか、警告を出すか) も確認し、空白による取りこぼしが起きなくなったことをテストで固定する。

## 完了条件

- `1, 2` と `1 , 2` のように空白を含むカンマ区切りの入力が、`1,2` と同じ選択になる。
- `all` の前後に空白がある場合も `all` として扱われる。
- 空行と空白のみの行が選択なしとして扱われる (現在の挙動を維持する)。
- EOF が選択なしとして扱われる (現在の挙動を維持する)。
- context のキャンセルで入力待ちが打ち切られる (issue 0131 で入れた挙動を維持する)。
- 上記をテーブル駆動テストで検証する。
- `TestNoContextBlindStdinReadOutsideDesignatedFunctions` の検出対象と一覧を、`bufio` を使う形に合わせて更新する。
- `mise run check` が通る。

## 関連

- docs/issues/0131: 起票元。`fmt.Scanln` から `promptSelection` へ移した際に、この読み取りの性質を挙動として固定した (`internal/cli/helper_test.go` の `TestPromptSelection` の `space inside the selection` の行)。

## 解決方法

`backend/internal/cli/helper.go` の `promptSelection` を、`fmt.Fscanln` (1 語しか読まない) から `*bufio.Reader.ReadString('\n')` (1 行読み) + `strings.TrimSpace` (前後の空白除去) に変更した。シグネチャも `io.Reader` から呼び出し元が保持する `*bufio.Reader` に変更した。`selectIndices` (`backend/internal/cli/sso.go`) は各要素の `TrimSpace` と `"all"` 判定を既に正しく行っており (`TestSelectIndices` の `with spaces` ケースで確認済み)、変更していない。

### 実装中に見つけて併せて修正した欠陥

方針どおりに実装したところ、`promptSelection` を呼ぶたびに `bufio.NewReader(cmd.InOrStdin())` を新規に作る形では、`bufio.Reader` が下層の Reader から先読みした分が使い捨てられた Reader の内部バッファに閉じ込められたまま失われ、`ssoGenerateConfig` がアカウント選択の直後に呼ぶロール選択 (2 回目以降の `promptSelection` 呼び出し) が実際にはまだ入力が残っているのに空扱いになる欠陥があることに気付いた (5 観点レビューの観点1 で高優先度の指摘として検出)。この issue が解決しようとした症状 (入力が黙って破棄される) を、原因を変えて再発させるものだったため、issue の完了条件を満たすために必要な修正として同じ issue の中で対応した。`sso.go` の `ssoGenerateConfig` で `*bufio.Reader` を 1 つだけ構築し、アカウント選択とロール選択の両方の呼び出しへ使い回す形に変更した。

さらに、観点3 (堅牢性) のレビューで、`promptSelection` が EOF 以外の読み取りエラーを空文字へ握り潰していたこと (`bufio.Reader` は下層のエラーを内部に保持し以降の呼び出しでも返し続けるため、一度エラーが起きると以後の選択がすべて理由なく空になる) を指摘され、`fmt.Errorf("read selection: %w", err)` で呼び出し元へ伝播させる形に修正した。呼び出し元 (`sso.go` の 2 箇所) は元々 `if err != nil { return err }` で即座に return する作りだったため、この変更に伴う呼び出し側の修正は不要だった。

### 完了条件の充足状況

- `1, 2` と `1 , 2` のような空白を含むカンマ区切りの入力が `1,2` と同じ選択になる: `TestPromptSelection` の `space inside the selection` / `space around each element` で検証。
- `all` の前後に空白がある場合も `all` として扱われる: `TestPromptSelection` の `all with surrounding whitespace` で検証。
- 空行と空白のみの行が選択なしとして扱われる (現在の挙動を維持): `TestPromptSelection` の `empty line` / `whitespace only line` で検証。
- EOF が選択なしとして扱われる (現在の挙動を維持): `TestPromptSelection` の `eof` / `no trailing newline` 系のケースで検証。
- context のキャンセルで入力待ちが打ち切られる (issue 0131 の挙動を維持): `TestPromptSelectionStopsWaitingWhenContextIsCanceled` で検証 (変更なし)。
- 上記をテーブル駆動テストで検証する: `TestPromptSelection` に加え、複数回呼び出しの相互作用 (`TestPromptSelectionReusesTheSameReaderAcrossCalls`) とエラー伝播 (`TestPromptSelectionPropagatesNonEOFReadErrors`) は単一シナリオの検証のためテーブル化せず個別のテスト関数にした。
- `TestNoContextBlindStdinReadOutsideDesignatedFunctions` の検出対象と一覧を、`bufio` を使う形に合わせて更新する: 当初、この検査が使う `isContextBlindRead` (`run_test.go`) は既に `bufio.NewReader` / `NewReaderSize` / `NewScanner` の構築呼び出しを検出するロジックを持っており、`promptSelection` は一覧に変更なしで通ると判断した。実際、`promptSelection` 単体の変更だけならその判断で正しかった。ところが上記の「実装中に見つけて併せて修正した欠陥」で `bufio.NewReader` の構築位置を `helper.go:promptSelection` から `sso.go:ssoGenerateConfig` へ移したため、検出される関数名がここで初めて変わり、`want` リストを `"helper.go:promptSelection"` から `"sso.go:ssoGenerateConfig"` へ更新する対応が実際に必要になった。
- `mise run check` が通る: 通過を確認した。

### 多観点レビューの結果

- **観点1 (完了条件充足)**: 高優先度の指摘 1 件 (上記の「実装中に見つけて併せて修正した欠陥」) を反映。中優先度の指摘 2 件は、(1) `selectIndices` の `"all"` 判定自体には `TrimSpace` が入っておらず `promptSelection` 側の `TrimSpace` に暗黙に依存している、(2) 完了条件 1 行目を入力から最終選択結果まで一貫して直接検証するテストが無い、という指摘で、いずれも現状の実装で完了条件自体は満たされており、テストの粒度に関する示唆として記録するに留め、今回は反映しなかった (低優先度相当の改善提案として扱った)。
- **観点2 (テスト品質)**: 高優先度の指摘 1 件。`ssoGenerateConfig` 自体 (アカウント選択 → ロール選択の複数回呼び出しパターン) を直接検証する統合テストが無く、`helper.go` 単体のテストだけでは `sso.go` 側の配線 (`stdin` 変数の共有) が壊れても検知できない、という指摘。`ssoGenerateConfig` は `getSSOToken` / `ListSSOAccountInfos` / `ListSSOAccountRoleNames` を直接呼んでおり依存注入の口が無いため、統合テストを書くには依存注入の導入が要る。これは docs/issues/0140 (`ssoGenerateConfig` に差し込み口が無い) が既に登録済みでまさにこの用途のために `ssoGenerateConfigDeps` を導入する計画を持っており、0140 の関連セクションにも本 issue が明記されている。本 issue の完了条件を超える設計変更であり、0140 の設計と重複・競合する実装を先取りするべきではないため、却下 (0140 に委ねる) とした。
- **観点3 (堅牢性)**: 中優先度の指摘 1 件 (EOF 以外のエラーの握り潰し) を反映 (上記参照)。低優先度の指摘 1 件 (`*bufio.Reader` の使い回しが「エラー時は即座に return する」という呼び出し元の実装に暗黙に依存しており契約が明文化されていない) を反映し、`sso.go` の `stdin` 構築箇所にコメントを追記した。
- **観点4 (規約準拠)**: 指摘なし。
- **観点5 (回帰と整合)**: 中優先度の指摘 1 件 (CHANGES.md のエントリが EOF 以外のエラー伝播という挙動変更に触れていなかった) を反映し、エントリに追記した。低優先度の指摘 3 件のうち、CHANGES.md の記述粒度の指摘 (`TestNoContextBlindStdinReadOutsideDesignatedFunctions` の一覧変更に触れていない) は同じ追記で解消した。残り 2 件 (docs/issues/0140 が `ssoGenerateConfigWith` へ分離した場合に `want` リストの追従が別途必要になる可能性がある、および docs/issues/closed/0131 の「2 箇所に限られる」という記述が現状のコードと厳密には食い違う) はいずれも実害の無い情報共有であり、前者は 0140 着手時の留意点として、後者は closed issue のスナップショットとして書き換えないことが妥当と判断し、いずれも変更は行わなかった。

反映した指摘はすべて再テスト (`go build ./...`, `go vet ./...`, `gofmt -l .`, `go test -race ./...`) を通過することを確認済み。
