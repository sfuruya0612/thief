# スニペット一覧が読み取りと stat の間の上書き保存で SQL と UpdatedAt の組が食い違う

Created: 2026-08-27
Model: Claude Fable 5
Completed: 2026-08-28

## 症状

`GET /api/snippets/{service}` の一覧取得中に、あるスニペットが別リクエストの `Save` で上書きされると、そのスニペットの `sql` は上書き前の本文、`updated_at` は上書き後の更新時刻という組で返ることがある。エラーにはならず、次の一覧取得で解消する一時的な不整合である。

issue 0152 の多観点レビュー (観点 3) で発見した既存の問題であり、0152 の変更で入ったものではない。

## 再現手順

タイミング依存のため確実な手動再現は難しい。コード上の経路は次のとおり。

1. `backend/internal/snippet/snippet.go` の `List` が、エントリごとに `os.ReadFile(filepath.Join(s.dir(service), e.Name()))` で本文を読む。
2. 続けて `e.Info()` で更新時刻を取る。Unix では `os.ReadDir` が返す `DirEntry` の `Info` は呼び出し時に `Lstat` を実行する (Go 1.26 の `os/file_unix.go` の `unixDirent.Info`)。
3. 1 と 2 の間に `Save` (一時ファイルへの書き込みと `os.Rename` による置き換え) が同じパスを上書きすると、1 で読んだ本文は旧ファイルのもの、2 の `ModTime` は新ファイルのものになる。

## 原因

本文の読み取り (`os.ReadFile`) と更新時刻の取得 (`e.Info()`) が別のシステムコールであり、その間の上書きに対して TOCTOU の窓がある。1 つのファイルの内容と属性を別々の呼び出しで取るため、両者が同じバージョンのファイルを指す保証が無い。

## 修正方針

`os.ReadFile` と `e.Info()` の組を、1 つの `*os.File` に対する `Open` → `Stat` → `ReadAll` に置き換える。開いたファイル記述子に対する `Stat` と読み取りは同じ inode を参照するため、`Rename` による置き換えが間に入っても旧ファイルの本文と旧ファイルの更新時刻の組になり、食い違わない。`Open` が `fs.ErrNotExist` を返した場合は issue 0152 と同じくスキップする。

検討して採らなかった案:

- `Info` を先に呼び、`ReadFile` の後にもう一度 `Lstat` して一致を確認する案: システムコールが 1 回増え、一致しない場合の再試行回数の上限という新しい判断が要る。記述子経由の `Stat` で同じ結果を追加コストなく得られるため却下。

新しい API や権限は不要。

## 完了条件

- `List` が本文と更新時刻を同じ開いたファイルから取得している (`os.ReadFile` と `e.Info()` の組を使っていない) ことをコードで確認できる
- 開いたファイルが読み取り前に消えた場合のスキップ (issue 0152 の回帰テスト) が引き続き通る
- `mise run check` が通過する

## 解決方法

`backend/internal/snippet/snippet.go` の `Store.List` で、`os.ReadFile` と `DirEntry.Info` の 2 回のパス解決を、新設した `readSnippetFile` による 1 回の `os.Open` に置き換えた。`readSnippetFile` は開いたファイル記述子に対して `File.Stat` と `io.ReadAll` を順に呼び、本文と更新日時を同じ inode から取得する。`Save` は一時ファイルの `os.Rename` で上書きするため、`Open` の後に上書きが起きても記述子は旧版を指し続け、本文と更新日時が別の版に属することはない。`Open` が `fs.ErrNotExist` を返した場合は issue 0152 と同じく一覧から外し、それ以外のエラーは `read snippet <file>: %w` でラップして返す (従来の `stat snippet <file>` のラップは、`Stat` と `ReadAll` が `Open` 済みの記述子に対して失敗する経路に集約されたため廃止した)。副次的な変化として、シンボリックリンクのエントリの更新日時は従来の `DirEntry.Info` (Lstat、リンク自体) からリンク先のものになる。本文は従来からリンク先を読んでいたため、本文と更新日時が同じ実体を指す方向の変化である。

再現は `snippet_test.go` の `TestStoreListReturnsSQLAndUpdatedAtFromSameVersionUnderConcurrentOverwrite` で行った。別 goroutine が `Save` と同じ一時ファイル + `os.Rename` の手順で 1000 版 (本文は連番、更新日時は `os.Chtimes` で 1 秒刻み) を上書きし続ける間に `List` を繰り返し、返った `SQL` と `UpdatedAt` が同じ版に属することを検証する (書き込み goroutine は停止要求と完了通知のチャネルを持ち、書き込み側のエラーと検証側の失敗のどちらでも goroutine を待ち切ってから終了する)。あわせて issue 0152 のテスト `TestStoreListSkipsEntryRemovedAfterReadDir` のコメントにあった `ReadFile` への言及を `Open` に改めた。このテストは開いたファイルへの rename の挙動が保証されない Windows ではスキップする。修正前のコードに対してこのテストは 1 回目の実行で失敗し、修正後は `-race` 付きで 3 回連続通過した (コミットしていない実装時の別ハーネスで版数を 20000 に増やした場合は、修正前で約 15000 件の観測のうち約 14000 件が不一致、修正後は 0 件だった)。

完了条件の確認結果:

- `List` が本文と更新日時を同じ開いたファイルから取得する: `readSnippetFile` のみが本文と更新日時を返し、`List` 内に `os.ReadFile` と `e.Info()` の呼び出しは無い。
- issue 0152 の回帰テスト (`TestStoreListSkipsEntryRemovedAfterReadDir`、`TestStoreListReturnsEmptyWhenAllEntriesRemovedAfterReadDir`、`TestStoreListReturnsNonNotExistReadError`) は変更なしで通過する。
- `mise run check` を通過した。
