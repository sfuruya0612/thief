# スニペット保存の応答が rename と stat の間の別の上書き保存の更新日時を返す

Created: 2026-08-28
Model: Claude Fable 5
Completed: 2026-08-28

## 症状

スニペット保存 (`POST /api/snippets/{service}`、`Store.Save`) の応答で、`sql` は自分が保存した本文、`updated_at` は同じ名前に対する別リクエストの後続の保存の更新時刻という組が返ることがある。エラーにはならず、次の一覧取得で解消する一時的な不整合である。issue 0158 (`Store.List` の同種の不整合) の実装レビュー (観点 5) で見つかった。

## 再現手順

タイミング依存のため手動での確実な再現は難しい。次のコードパスで発生する。

1. リクエスト A が `Store.Save("athena", "q", "A")` を呼び、一時ファイルを `os.Rename` で `q.sql` に置く。
2. リクエスト A が `os.Stat(p)` を呼ぶ前に、リクエスト B が `Store.Save("athena", "q", "B")` の `os.Rename` を完了する。
3. リクエスト A の `os.Stat(p)` は B の版の `ModTime` を返し、A の応答は `SQL: "A"` と B の更新時刻の組になる。

## 原因

`backend/internal/snippet/snippet.go` の `Store.Save` は、`os.Rename(tmp.Name(), p)` の後に `os.Stat(p)` でパスを再解決して `UpdatedAt` を取得する。

```go
if err := os.Rename(tmp.Name(), p); err != nil {
	return Snippet{}, fmt.Errorf("rename snippet %s: %w", name, err)
}
info, err := os.Stat(p)
if err != nil {
	return Snippet{}, fmt.Errorf("stat snippet %s: %w", name, err)
}
return Snippet{Name: name, SQL: sql, UpdatedAt: info.ModTime().UTC()}, nil
```

`Rename` と `Stat` は別のシステムコールで、その間に同じパスへの別の `Rename` が入ると `Stat` は別の版を指す。issue 0158 の `List` と同じ構造である。

## 修正方針

`os.Rename` の前に、書き込みと `Close` を終えた一時ファイルを `os.Stat(tmp.Name())` (または `Close` 前の `tmp.Stat()`) して `ModTime` を取得し、`Rename` 後の `os.Stat(p)` を廃止する。`rename(2)` は inode を変えないため、一時ファイルの `ModTime` は rename 後の `q.sql` の `ModTime` と同じ値である。一時ファイルは自分だけが参照するため競合しない。

採らなかった案:

- rename 後に `os.Stat(p)` の結果と自分の書き込み内容を突き合わせて再試行する: 再試行の回数の判断が新たに必要になり、`List` (issue 0158) が採った「同じ実体から取る」方針と揃わない。

## 完了条件

- `Store.Save` が `UpdatedAt` を、rename 前の一時ファイル (自分が書き込んだ実体) から取得しており、rename 後にパスを再解決する `os.Stat` の呼び出しが無い。
- `Save` の返す `UpdatedAt` が、直後の `List` が返す同じスニペットの `UpdatedAt` と一致するテストが通る (既存テストで同等の検証があれば流用する)。
- `mise run check` が通過する。

## 調査結果 (implement-issues、2026-08-28)

原因を修正前のコードに対するテストで確認した。`Save` の rename 関数を差し替え可能な変数 `renameFile` (`snippet.go`) にし、テスト `TestStoreSaveReturnsOwnUpdatedAtWhenOverwrittenBetweenRenameAndStat` (`snippet_test.go`) で、rename の直後に更新日時 2023-11-14T22:13:20Z の別の版で `q.sql` を上書きする関数を差し込むと、修正前の `Save` は `SQL: "mine"` と `UpdatedAt: 2023-11-14 22:13:20 +0000 UTC` の組を返した。修正方針は本文の「## 修正方針」のとおりで変更しない。`ModTime` の取得は `Close` 前の `tmp.Stat()` (書き込み済みの記述子から取る) とした。

issue 0158 と同じ確率的な同時上書きによる再現は採らなかった。macOS 26.5 (Darwin 25.5.0、APFS) では、同じ宛先への同時 rename で `rename(2)` が、ソースファイルが存在するにもかかわらず ENOENT を返すことがあり、同時実行のテストでは `Save` 自体が `rename snippet q: ... no such file or directory` で失敗して検証に到達しないため。この挙動は同名スニペットの同時保存で `Save` が失敗する別のバグとして issue 0161 に登録した。

## 解決方法

`backend/internal/snippet/snippet.go` の `Store.Save` で、rename 後の `os.Stat(p)` を廃止し、書き込みを終えた一時ファイルの記述子に対する `tmp.Stat()` (`Close` の前) から `ModTime` を取るようにした。rename は inode を変えないため、この値は rename 後の保存先の更新日時と同じであり、rename と stat の間に別リクエストが同じ名前を上書きしても、自分が保存した本文と別の版の更新日時の組を返すことはない。`Stat` が失敗した場合は一時ファイルを閉じてから `stat snippet <name>: %w` で返す (ラップの文言は従来の rename 後の `Stat` と同じ)。このときの `Close` の戻り値は使わない。記述子の解放だけが目的で、報告すべき失敗は既に `Stat` が示しているためで、その理由はコードのコメントにも書いた。あわせて rename の呼び出しを差し替え可能な変数 `renameFile` (`os.Rename` が既定) に置き、テストから rename 直後の上書きを差し込めるようにした。パッケージ変数を既定の実装で初期化しテストだけが差し替えるという形は `sso_cache.go` の `ssoCacheRemove` と同じだが、`renameFile` のコメントにはそれに加えて、本番コードから代入しないことと、差し替えるテストと `Save` を呼ぶテストを `t.Parallel` で並列に走らせるとこの変数へのデータ競合になることを明記した。

再現と回帰テストは `snippet_test.go` の `TestStoreSaveReturnsOwnUpdatedAtWhenOverwrittenBetweenRenameAndStat` で行った。`renameFile` を、一時ファイルの `ModTime` を記録してから本来の rename を行い、直後に更新日時 2023-11-14T22:13:20Z の別の版で保存先を上書きする関数に差し替え、`Save` の返す `UpdatedAt` が別の版の値ではなく一時ファイルの `ModTime` と一致すること、保存先の本文が後続の版になっていることを検証する。修正前のコードに対してこのテストは失敗し (`Save returned updated_at 2023-11-14 22:13:20 +0000 UTC of the other version`)、修正後は `-race` 付き 3 回連続で通過した。完了条件の 2 項目目は `TestStoreSaveReturnsUpdatedAtMatchingList` として、`Save` の返す `UpdatedAt` が直後の `List` の同じスニペットの `UpdatedAt` と一致し、UTC であることを検証する (既存テストに同等の比較は無かった)。

issue 0158 と同じ確率的な同時上書きによる再現は採らなかった。macOS 26.5 では同じ宛先への同時 rename で `rename(2)` が ENOENT を返して `Save` 自体が失敗するため。この挙動は issue 0161 に登録した。

完了条件の確認結果:

- `Save` が `UpdatedAt` を rename 前の一時ファイルから取得し、rename 後にパスを再解決する `os.Stat` の呼び出しが無い: `Save` 内の `os.Stat` の呼び出しを削除し、`tmp.Stat()` のみが更新日時を返す。
- `Save` の返す `UpdatedAt` が直後の `List` の同じスニペットの `UpdatedAt` と一致するテストが通る: `TestStoreSaveReturnsUpdatedAtMatchingList` を追加し通過した。
- `mise run check` を通過した (最終形のコードに対して exit 0、失敗テストはベースラインと同じくゼロ)。
