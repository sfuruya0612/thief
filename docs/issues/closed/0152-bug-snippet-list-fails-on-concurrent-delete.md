# スニペット一覧が列挙と読み取りの間のファイル削除で全体エラーになる

Created: 2026-08-25
Model: Claude Fable 5
Completed: 2026-08-27

## 症状

`GET /api/snippets/{service}` の一覧取得中に、列挙済みの .sql ファイルが別リクエストの削除や手動操作で消えると、一覧全体が HTTP 500 (`INTERNAL`) で失敗する。残っているスニペットも返らない。

issue 0150 の多観点レビュー (観点 3) で発見した既存の問題であり、0150 の変更で入ったものではない。

## 再現手順

タイミング依存のため確実な手動再現は難しい。コード上の経路は次のとおり。

1. `backend/internal/snippet/snippet.go` の `List` が `os.ReadDir(s.dir(service))` でエントリを列挙する。
2. 列挙後、エントリごとに `os.ReadFile(filepath.Join(s.dir(service), e.Name()))` で本文を読む。
3. 1 と 2 の間に対象ファイルが削除されると `os.ReadFile` が `fs.ErrNotExist` を返し、`List` は `fmt.Errorf("read snippet %s: %w", ...)` でエラーを返す。
4. `backend/internal/api/handlers_snippets.go` の `writeSnippetError` は `ErrInvalidService` / `ErrInvalidName` / `ErrNotFound` 以外を 500 にマップするため、レスポンスは 500 になる。

テストでは、`os.ReadDir` の結果を得た後にファイルを削除してから読み取りを行う状況を再現すればよい (例: 一覧対象のディレクトリに読み取り不能あるいは削除済みのエントリを作る)。決定的な再現には `List` の途中に介入できる構造が必要なため、修正方針の実装とあわせて回帰テストの形を確定する。

## 原因

`List` の列挙 (`os.ReadDir`) と本文読み取り (`os.ReadFile`) が別のシステムコールであり、その間のファイル削除に対して TOCTOU (time-of-check to time-of-use) の窓がある。読み取り失敗を一律にエラーとして呼び出し元へ返すため、1 ファイルの消失が一覧全体の失敗になる。

`e.Info()` (stat) も同じ窓を持ち、削除されたエントリに対して `fs.ErrNotExist` を返しうる。

## 修正方針

`List` のエントリごとの処理で、`os.ReadFile` または `e.Info()` が `fs.ErrNotExist` を返した場合はそのエントリをスキップして続行する (列挙と読み取りの間に削除されたものは「もう存在しないスニペット」であり、一覧に載らないのが正しい)。`fs.ErrNotExist` 以外のエラー (権限不足など) は従来どおりエラーとして返す。

検討して採らなかった案:

- ディレクトリ全体をロックして列挙と読み取りを原子化する案: プロセス内ロックでは手動のファイル削除に効かず、ファイルロックは移植性と複雑さのコストが利益に見合わない。スキップで十分。
- 読み取り失敗のエントリを空 SQL で一覧に載せる案: 存在しないスニペットを UI に見せることになり、選択時の挙動が不定になるため却下。

新しい API や権限は不要。

## 完了条件

- `List` の列挙後にファイルが削除された場合、該当エントリがスキップされ、残りのスニペットが正常に返ることを検証するテストがある
- `fs.ErrNotExist` 以外の読み取りエラーが従来どおりエラーとして返ることがテストで確認できる
- `mise run check` が通過する

## 解決方法

`backend/internal/snippet/snippet.go` の `Store.List` で、エントリごとの `os.ReadFile` と `e.Info()` の直後に `errors.Is(err, os.ErrNotExist)` の判定を追加し、該当するエントリは `continue` でスキップして残りの処理を続けるようにした。それ以外のエラーは従来どおり `read snippet %s: %w` / `stat snippet %s: %w` でラップして返す。判定の理由はコード内コメントに記載した。

テストは `backend/internal/snippet/snippet_test.go` に 3 件追加した。シンボリックリンクの作成に管理者権限が必要で、パーミッションビットが所有者の読み取りを制限しない Windows では、リポジトリの他のテストと同じ `runtime.GOOS == "windows"` の判定で 3 件とも `t.Skip` する。このリポジトリは CI 設定を持たず、Windows を対象プラットフォームと定めておらず、同じ判定によるスキップが `internal/util/executer_test.go` などに既にあるため、Windows で 3 件が実行されないことは完了条件の充足を妨げないと判断した。

- `TestStoreListSkipsEntryRemovedAfterReadDir`: 一覧対象ディレクトリに `alive.sql` と、参照先の無いシンボリックリンク `gone.sql` を置く。シンボリックリンクは `os.ReadDir` の列挙には載るが `os.ReadFile` が `fs.ErrNotExist` を返すため、列挙と読み取りの間に削除された状態を決定的に再現できる。`List` がエラーを返さず `alive` (`SELECT 1`) だけを返すことを検証する。修正前はこのテストが `read snippet gone.sql: open .../missing-target.sql: no such file or directory` で失敗することを確認した (再現確認)。
- `TestStoreListReturnsEmptyWhenAllEntriesRemovedAfterReadDir`: 参照先の無いシンボリックリンクだけを置き、列挙された全エントリがスキップされた場合に `List` がエラーや nil ではなく空スライスを返すことを検証する (JSON では `null` ではなく `[]` になる。ディレクトリ自体が無い場合の `TestStoreListMissingDirReturnsEmpty` と同じ契約)。
- `TestStoreListReturnsNonNotExistReadError`: モード 0o000 の `locked.sql` を置き、`List` が `fs.ErrPermission` を `errors.Is` で判別できるエラーとして返し、結果が nil であることを検証する。root は権限ビットを無視して読めるため uid 0 では `t.Skip` する。このテストが通る経路 (`fs.ErrNotExist` 以外のエラーを返す分岐) は今回の変更で書き換えていないため、今回の差分を戻しても失敗しない。検知するのは、`errors.Is(err, os.ErrNotExist)` の判定を `err != nil` のように広げてエラーを握り潰す誤った変更である。

`e.Info()` が `fs.ErrNotExist` を返す経路は、同じパスに対する `os.ReadFile` が先に同じ条件で失敗するため、`List` の途中に介入できる構造を持たない現行実装では決定的に再現できず、自動テストで未検証のまま残る。完了条件 1 のテストが担保するのは `os.ReadFile` 側の分岐である。`e.Info()` 側には `os.ReadFile` 側と同じ `errors.Is(err, os.ErrNotExist)` 判定を置いており、`os.ReadFile` の成功直後に削除された場合も同様にスキップされる。

完了条件の確認結果は次のとおり。

- 列挙後に削除されたエントリのスキップと残りの返却: `TestStoreListSkipsEntryRemovedAfterReadDir` と `TestStoreListReturnsEmptyWhenAllEntriesRemovedAfterReadDir` で検証
- `fs.ErrNotExist` 以外の読み取りエラーの返却: `TestStoreListReturnsNonNotExistReadError` で検証 (今回変更していない分岐が従来どおり残ることの確認)
- `mise run check`: 通過 (ベースラインと同じく失敗テストなし)
