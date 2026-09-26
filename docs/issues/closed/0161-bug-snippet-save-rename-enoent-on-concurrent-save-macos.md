# macOS で同名スニペットの同時保存が rename の ENOENT で失敗する

Created: 2026-08-28
Model: Claude Fable 5
Completed: 2026-08-29

## 症状

macOS 26.5 (Darwin 25.5.0、APFS) で、同じ名前のスニペットに対する `Store.Save` (`POST /api/snippets/{service}`) が同時に実行されると、一方の `os.Rename(tmp.Name(), p)` が、一時ファイルが存在するにもかかわらず `no such file or directory` (ENOENT) を返し、`Save` が次のエラーで失敗することがある。

```
rename snippet q: rename .../athena/.tmp-3021914655 .../athena/q.sql: no such file or directory
```

`handlers_snippets.go` の `writeSnippetError` はこのエラーを既定の分岐で HTTP 500 (`SNIPPET_ERROR`) にする。失敗した側の一時ファイルは `defer os.Remove(tmp.Name())` で削除され、保存先には他方の版が残る。issue 0160 の実装中に、確率的な同時上書きで再現テストを書こうとして見つけた。Linux では未確認。

## 再現手順

タイミング依存のため、次のプログラムで再現する。1 つの goroutine が事前に用意した 5000 個のファイルを同じ宛先 `q.sql` へ rename し続け、その間にメイン goroutine が `Save` と同じ手順 (`os.CreateTemp` → 書き込み → `Close` → `Chmod` → `os.Rename`) を繰り返す。

1. 次のプログラムを `go run` する (Go 1.26.6 darwin/arm64 で実施)。

   ```go
   package main

   import (
   	"errors"
   	"fmt"
   	"os"
   	"path/filepath"
   )

   func main() {
   	dir, _ := os.MkdirTemp("", "rr")
   	defer os.RemoveAll(dir)
   	target := filepath.Join(dir, "q.sql")
   	tmps := make([]string, 5000)
   	for i := range tmps {
   		tmps[i] = filepath.Join(dir, fmt.Sprintf(".tmp-overwrite-%d", i))
   		os.WriteFile(tmps[i], []byte("other"), 0o644)
   	}
   	done := make(chan struct{})
   	go func() {
   		for _, tmp := range tmps {
   			os.Rename(tmp, target)
   		}
   		close(done)
   	}()
   	ok, enoent, srcGone := 0, 0, 0
   	for {
   		select {
   		case <-done:
   			fmt.Println("ok", ok, "enoent", enoent, "srcGone", srcGone)
   			return
   		default:
   		}
   		tmp, _ := os.CreateTemp(dir, ".tmp-*")
   		tmp.WriteString("mine")
   		tmp.Close()
   		os.Chmod(tmp.Name(), 0o644)
   		if err := os.Rename(tmp.Name(), target); err != nil {
   			if errors.Is(err, os.ErrNotExist) {
   				enoent++
   				if _, serr := os.Stat(tmp.Name()); serr != nil {
   					srcGone++
   				}
   			}
   			os.Remove(tmp.Name())
   			continue
   		}
   		ok++
   	}
   }
   ```

2. 期待される観測結果: 2 回の実行で `ok 6 enoent 3 srcGone 0` と `ok 6 enoent 4 srcGone 0`。`srcGone 0` は、ENOENT を返した rename のソース (一時ファイル) がその時点で存在していたことを示す。
3. 同じ現象は `backend/internal/snippet` のテストとして書いた場合も `-race` 付き 3 回の実行すべてで発生した (`Save: rename snippet q: rename ... no such file or directory`)。

## 原因

`rename(2)` の宛先 `q.sql` を別スレッドが同時に rename で置き換えている間に、macOS のカーネルが宛先の解決に失敗して ENOENT を返す。ソースが存在すること (`srcGone 0`) から、ENOENT はソースではなく宛先側の解決に由来する。POSIX は rename の宛先が存在する場合に置き換えを求めているが、宛先が同時に置き換えられる場合の挙動は規定していない。Linux での発生の有無と、macOS 側の条件 (APFS 限定か、Darwin のバージョン依存か) は未確認。

## 未確定論点

- 修正の方式。候補は次の 2 つで、どちらを採るかは設計判断が要る。
  - `Save` で `os.Rename` が `fs.ErrNotExist` を返し、かつ一時ファイルが存在する場合に限り、回数を限って rename を再試行する。再試行の回数と、`Save` の他の失敗経路との切り分け (一時ファイルが存在しないなら再試行しない) を決める必要がある。
  - `Store` に名前単位の排他 (`sync.Mutex` または名前ごとのロック) を入れ、同一プロセス内の同名スニペットの `Save` を直列化する。別プロセス (手動配置や別インスタンス) との競合は防げない。
- Linux での再現の有無。Linux で発生しないなら、再試行は macOS 限定の回避策として位置付ける。

## 完了条件

- 上記の再現プログラムと同じ手順を `backend/internal/snippet` のテストにしたものが、macOS で `-race` 付き 3 回連続で `Save` のエラーなしに通過する。
- 同時保存が失敗しなくなった後の保存先 `q.sql` の本文が、いずれかの `Save` が書いた本文と一致し、部分書き込みや消失が無い。
- `mise run check` が通過する。

## 関連

- issue 0160: 同じ `Store.Save` の rename 後の `os.Stat` による更新日時のずれ。0160 の実装中に本 issue を見つけた。

## 調査結果 (implement-issues、2026-08-28)

未確定論点の 2 項目をいずれも実測で決着させた。

### Linux での再現の有無 (解消済み)

再現手順のプログラムを Linux コンテナ (Docker、golang:1.25、kernel 7.0.12-linuxkit aarch64) で実行した結果、ENOENT は 1 回も発生しなかった。

| 実行環境 | 成功した rename | ENOENT |
| --- | --- | --- |
| overlayfs (コンテナの /tmp)、3 回の実行 | 2863 / 2795 / 2756 | 0 / 0 / 0 |
| tmpfs (--tmpfs /tmp)、2 回の実行 | 1783 / 1742 | 0 / 0 |

同じプログラムの macOS での成功数が 1 回の実行あたり 1 から 75 にとどまるのに対し Linux は 1700 以上であり、macOS の rename は同一宛先の競合下で著しく遅い。この差自体は本 issue の対象外とする。以上より、ENOENT は macOS 固有の挙動として扱う。tmpfs の 1 回目の実行はビルド成果物を noexec の /tmp に置いたことによる `fork/exec ... permission denied` で計測に至らなかったため、表から除いている。

### ENOENT に対する即時再試行の収束 (解消済み)

再現手順のプログラムを、ENOENT かつソースが存在する場合に上限 64 回まで即時再試行するよう変更し、成功までに要した試行回数を macOS で計測した。13 回の実行、合計 268 回の保存で、上限の枯渇は 0 回、要した試行回数の最大は 5 回 (再試行 4 回) だった。

| 要した試行回数 | 1 | 2 | 3 | 5 |
| --- | --- | --- | --- | --- |
| 回数 | 253 | 8 | 5 | 4 |

即時再試行はバックオフなしで収束する。敵対的な writer (事前に用意した 5000 個のファイルを同じ宛先へ連続で rename する goroutine) の下での値であり、実際の同時保存 (2 つの HTTP リクエスト) はこれより緩い競合になる。

## 修正方針

`Save` の rename を、ENOENT に対してソースの存在を確認したうえで回数を限って再試行するヘルパー (`renameSnippetFile`) に置き換える。

- 再試行の判別条件は「エラーが `fs.ErrNotExist` に一致し、かつソースの一時ファイルが存在する」の両方を満たす場合とする。一時ファイルは保存先と同じディレクトリに作るため、ディレクトリごと消えた場合は一時ファイルも存在せず再試行しない。ソースの `os.Stat` が ENOENT 以外で失敗した場合も存在を確認できていないため再試行しない。これにより、宛先側の一時的な解決失敗以外の ENOENT を再試行で隠さない。
- 試行回数の上限は 16 回 (初回 + 再試行 15 回) とする。上記の計測で要した試行回数の最大が 5 回であり、その約 3 倍の余裕を持たせた値。上限に達した場合は試行回数を添えてエラーを返す (運用時に、ディレクトリ欠落による ENOENT と競合による ENOENT を切り分けられるようにする)。
- バックオフは入れない。上記の計測で即時再試行が収束しており、待機を入れると通常経路の遅延とテストの時間依存を持ち込むため。
- build tag による macOS 限定にはしない。判別条件が宛先側の一時的な失敗に限定されているため、ENOENT が発生しない Linux では再試行の経路に入らず、挙動が変わらない。

### 採らなかった案

- `Store` に名前単位の排他 (名前ごとの `sync.Mutex`) を入れて同名スニペットの `Save` を直列化する案。却下の理由は 3 つ。(1) 完了条件 1 が求める再現テストは敵対的な writer が `Store` を経由せず生の `os.Rename` で宛先を置き換えるため、プロセス内の排他では通らない。(2) 別プロセス (手動配置、別インスタンスの thief、AWS CLI) との競合を防げず、堅牢性の観点で再試行に劣る。(3) 名前ごとの Mutex を保持する map は寿命管理 (到達不能になったエントリの除去) を要し、状態を増やす。
- 再試行を無条件 (ソースの存在確認なし) にする案。ディレクトリ欠落のような本物の ENOENT を上限回数まで再試行してから返すことになり、失敗の原因が分かりにくくなるため却下した。

新しい API や権限の追加は無い。

## 解決方法

`Store.Save` の rename を、修正方針どおりヘルパー `renameSnippetFile` (`backend/internal/snippet/snippet.go`) 経由にした。

- `renameSnippetFile` は rename が失敗した場合、エラーが `fs.ErrNotExist` に一致し、かつソースの `os.Stat` が成功する (ソースが存在する) 場合に限り、上限 `renameAttemptLimit` = 16 回 (初回 + 再試行 15 回) まで即時に再試行する。`fs.ErrNotExist` 以外のエラー、ソースが存在しない場合、ソースの `os.Stat` が ENOENT 以外で失敗して存在を確認できない場合は再試行せずそのまま返す。
- 上限に達した場合は `give up after 16 rename attempts: %w` で試行回数を添えて返し、ディレクトリ欠落による ENOENT (初回で返る) と競合による ENOENT (上限まで再試行して返る) を運用時に切り分けられるようにした。
- `Save` の rename 呼び出しを `renameFile` の直接呼び出しから `renameSnippetFile` に差し替えた。差し替え可能な `renameFile` の宣言のコメントは、issue 0161 の ENOENT の注入にも使うことと、データ競合の当事者が `renameSnippetFile` 内の読み取りになったことを反映して更新した。
- バックオフと build tag による macOS 限定は入れていない (理由は修正方針に記載)。

テストは `backend/internal/snippet/snippet_test.go` に 4 つ追加した。

- `TestStoreSaveSucceedsUnderConcurrentRenameToSameDestination`: 完了条件 1 と 2 に対応する。再現手順のプログラムと同じ手順 (事前に用意した 5000 個のファイルを同じ宛先へ rename し続ける goroutine の下で `Save` を繰り返す) をテストにしたもの。`Save` が 1 度も失敗しないことと、最終的な `q.sql` の本文が `mine` か `other` のどちらか (部分書き込みや消失が無いこと) を検証する。writer の終了を `select` で見る前に `Save` を実行するループ構造にすることで、writer が先に終わりきった環境でも保存が 1 回は行われる (保存が 0 回のまま何も検証せずに通ることがない) ことを構造で保証しており、保存回数を検査するアサーションは置いていない。writer の終了を待ってから判定するため、一時ディレクトリの削除が writer の rename と競合しない。
- `TestStoreSaveAbsorbsRenameEnoentFromDestinationContention`: `Save` が rename を `renameSnippetFile` 経由で呼び、宛先の競合に由来する ENOENT を吸収することを決定的に検証する。`renameFile` を差し替えて 1 回目だけ ENOENT を返し、`Save` が成功を返すこと、`renameFile` の呼び出しが 2 回であること、`List` が保存内容を返すことを検査する。
- `TestRenameSnippetFile`: 再試行の判別条件をテーブル駆動で決定的に検証する。初回成功、ENOENT を 3 回返した後の成功 (呼び出し 4 回)、上限到達 (呼び出し 16 回、メッセージに試行回数を含む)、ソースが消えている場合の非再試行 (呼び出し 1 回)、EACCES の非再試行 (呼び出し 1 回) の 5 ケース。`renameFile` を差し替えるため `t.Parallel` は使っていない。
- `TestRenameSnippetFileDoesNotRetryWhenSourceCannotBeStatted`: ソースの `os.Stat` が `fs.ErrNotExist` 以外で失敗する場合に再試行しないことを検証する。Stat を失敗させる理由には、実行ユーザに依存しない ENAMETOOLONG (NAME_MAX を超える単一コンポーネント) を使い、ディレクトリの権限を落とす方法 (root では Stat が成功してしまう) を避けた。テーブルに入れず独立したテストにしたのは、ソースを作成しない点で他のケースと前提が異なるため。

検証の結果。

- 修正前のコードに対して `TestStoreSaveSucceedsUnderConcurrentRenameToSameDestination` は `-race` 付き 3 回の実行すべてで `Save under concurrent rename = rename snippet q: rename ... no such file or directory` で失敗した (再現の確認)。
- 修正後は `go test -race -count=3 -run TestStoreSaveSucceedsUnderConcurrentRenameToSameDestination` が 3 回連続で通過した (完了条件 1)。
- 追加したテストの判別能力を、実装を意図的に壊して確認した。ソースの存在確認を削除すると `TestRenameSnippetFile/does_not_retry_enoent_when_the_source_is_gone` が呼び出し 16 回で失敗し、試行回数の付記を削除すると `TestRenameSnippetFile/gives_up_at_the_attempt_limit` がメッセージの検査で失敗し、存在確認の条件を `errors.Is(serr, os.ErrNotExist)` に狭めると `TestRenameSnippetFileDoesNotRetryWhenSourceCannotBeStatted` が呼び出し 16 回で失敗した。`Save` の rename 呼び出しを `renameSnippetFile` から `renameFile` の直接呼び出しに戻すと `TestStoreSaveAbsorbsRenameEnoentFromDestinationContention` だけが失敗し、`TestRenameSnippetFile` 系は通過したままだった。いずれも確認後に実装を元に戻している。
- `go test -race -count=3 ./internal/snippet/...` と `go test -race ./internal/api/...` が通過した。
- `mise run check` が exit 0 で通過した (完了条件 3)。frontend の lint 警告は 10 件 (0 errors) でベースラインと同一、新たな失敗テストは無い。

`internal/snippet` パッケージのテスト時間は、5000 回の rename を伴う再現テストの追加により、`mise run check` の `go test -race -cover` で 7.773 秒 (issue 0160 の実装時のログ) から 20 秒台から 30 秒台に増えた (この実装での 3 回の実行で 20.121 秒、28.767 秒、32.680 秒)。競合下の rename が完了するまでの時間がタイミングに依存するため、実行ごとの振れ幅が大きく、単一の値では表さない。再現テスト単体の所要時間は、`go test -race -count=5 -v -run TestStoreSaveSucceedsUnderConcurrentRenameToSameDestination ./internal/snippet/` の 5 回の実行で 3.75 秒から 4.30 秒だった。完了条件 1 が再現手順と同じ手順 (5000 回の rename) のテストを求めているため、rename の回数は減らしていない。

再現テストの失敗はタイミング依存で確率的である。次の値は、issue の「## 再現手順」の独立したプログラム (`Store` を介さず生の `os.Rename` を使うもの) ではなく、`backend/internal/snippet` のテストを `-count` で繰り返した計測である。`Save` の rename を `renameSnippetFile` から `renameFile` の直接呼び出しに戻したうえで `go test -race -count=10 -v -run TestStoreSaveSucceedsUnderConcurrentRenameToSameDestination ./internal/snippet/` を実行すると、10 回中 8 回が失敗し 2 回は通過した。回帰の検出は決定的なテストが担う。再試行の判別条件は `TestRenameSnippetFile` と `TestRenameSnippetFileDoesNotRetryWhenSourceCannotBeStatted` が `renameFile` の差し替えで検証し、`Save` が `renameSnippetFile` を通るという配線は `TestStoreSaveAbsorbsRenameEnoentFromDestinationContention` が検証する。この配線の検証を独立したテストにしたのは、`renameSnippetFile` を直接呼ぶテストでは、`Save` の呼び出しが `renameFile` の直接呼び出しへ戻る退行 (この修正そのものの取り消し) を検出できないためである。その退行を入れると `TestStoreSaveAbsorbsRenameEnoentFromDestinationContention` だけが失敗し、他の決定的なテストはすべて通過する。再現テストは、macOS のカーネルが返す実際の ENOENT が `Save` の経路全体で吸収されることの確認として置いている。上記の退行も検出できるが、10 回中 2 回は見逃すため、この退行の検出をこのテストだけに頼らない。
