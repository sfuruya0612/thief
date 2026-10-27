# TUI の S3 Objects タブに階層移動と prefix 検索を追加する

Created: 2026-10-26
Model: DeepSeek V4.1 Flash 1017
Completed: 2026-10-27

## 背景

`docs/issues/TODO.md` の次の項目に由来する。

> TUI の Drawer の S3 Objects タブで、フォルダの階層移動、prefix 検索、階層表示とフラット表示の切替ができるようにしたい

issue 0226 は Drawer のタブ基盤を追加するが、S3 の Objects タブは追加しない。Web UI の `frontend/src/components/Drawer/DrawerS3Objects.tsx` と `frontend/src/components/Drawer/DrawerObjectBrowser.tsx` は、`backend/internal/aws/s3_object.go` の `ListS3Objects(ctx, profile, region, bucket, prefix, delimiter)` を使って次の操作を提供している。

- `delimiter` が `/` の階層モードで、今いるフォルダ直下のフォルダとオブジェクトを表示する。
- フォルダ行を選ぶとその prefix へ移動し、パンくずで親へ戻れる。
- prefix の検索欄に入力した文字列を、今いるフォルダの prefix の後ろに連結して再取得する。
- 階層モードとフラットモードを切り替える。フラットモードは今いる prefix 以下の全階層を平らに表示する。
- Web UI はプレビュー、アップロード、ダウンロード、DuckDB の SQL 検索も提供する。

backend の一覧取得は、オブジェクトとフォルダの合計を 1000 件で打ち切り、`truncated` を返す。

TUI には S3 の Objects タブ自体が無く、バケットの中身を確認できない。まず Web UI の階層ブラウザと同じ一覧操作を TUI に追加する。

## 目的

TUI の S3 のバケット行から Objects タブを開き、フォルダを辿り、prefix で絞り込み、階層表示とフラット表示を切り替えて、バケット内のオブジェクトを確認できるようにする。

## 設計判断

- この issue は issue 0226 の完了後に実装する。S3 Objects タブは issue 0226 の `Tab`、`TabSpec`、`TabProvider`、`TabIntent` と、`Drawer` のキー委譲を使う。
- S3 Objects タブは共通の `contentTab` ではなく、S3 専用の `s3ObjectsTab` として実装する。現在の prefix、検索で確定した prefix、表示モード、一覧、読み込み状態、検索入力の状態をタブ内で持つ。
  - `contentTab` は読み取り専用の表をスクロールするだけで、フォルダ行の選択、prefix の入力、モード切替を持てない。S3 専用のタブにする。
  - パンくずを別の行に置く案は、Drawer の高さを消費し、`..` 行と同じ移動を二重に持つため採らない。`..` 行で親へ戻る。
- 一覧の取得は `aws.ListS3Objects(ctx, profile, region, bucket, listPrefix, delimiter)` を呼ぶ。
  - 階層モードは `delimiter="/"`、フラットモードは `delimiter=""` とする。
  - `listPrefix` は「今いるフォルダの prefix」と「検索で確定した prefix」の連結にする。Web UI の `DrawerObjectBrowser` と同じ `normalizeSearchPrefix` の規則で、検索語の前後の空白を取り除き、先頭の連続する `/` をすべて取り除く。
  - フォルダへ移動したときは今いる prefix をフォルダの完全な prefix にし、検索で確定した prefix を空にする。これにより、フォルダ移動と検索が混ざらない。
- 一覧の表は Web UI の `s3ObjectColumns` と同じ Key、Size、Last modified、Storage class の 4 列にする。フォルダ行の Key は `ListS3Objects` が返す `prefixes` の値（末尾 `/` 付き）をそのまま表示し、Size、Last modified、Storage class は `—` にする。末尾に `/` を追加しない。
- 今いる prefix が空でないときは、一覧の先頭に `..` 行を置く。`enter` を押すと 1 つ上のフォルダへ移動する。`esc` または `backspace` でも親へ移動する。prefix が空のときの `esc` と `backspace` は何もしない。
- `enter` を押すと、フォルダ行ではその prefix へ移動し、オブジェクト行では何もしない。オブジェクトのプレビューはこの issue では扱わない。
- `f` で検索入力を開く。`enter` で確定し、`esc` で取り消す。検索入力中は `enter`、`esc`、`ctrl+c` 以外の印刷可能なキーと `tab`、`backspace` をすべて入力として扱う。`q`、`r`、`j`、`k`、`f`、`m`、`s`、`p`、`R`、`S`、`[`、`]`、`/`、数字も文字として入力する。`ctrl+c` は従来どおり TUI の終了にする。検索入力が開いている間は `backspace` で親へ移動しない。
- `m` で階層モードとフラットモードを切り替える。モードを変えたら同じ prefix で取得をやり直す。検索入力中は `m` を文字として扱う。
- 検索入力の `esc` は、入力中の値を捨てて入力欄を閉じる。確定済みの検索 prefix と一覧は変えない。`f` で開き直したときは、確定済みの検索 prefix を初期値にする。
- 読み込み中、失敗、空、1000 件での打ち切りを区別して表示する。打ち切りのときは一覧の末尾に `Showing the first 1000 entries; narrow the prefix to see more` を表示する。
- 一覧の取得はタブの context と `loadSeq` で世代管理し、フォルダ移動、検索、モード切替の後に届いた古い結果を捨てる。入力中の一覧は前回の結果を残し、確定後に取得を始める。
- この issue ではプレビュー、アップロード、ダウンロード、DuckDB の SQL 検索、複数選択、フォルダの作成と削除を扱わない。これらはオブジェクトの内容取得や書き込みを伴い、TUI の第 2 段階以降で扱う。
- 追加の API 呼び出しは `ListObjectsV2` とバケットリージョンの解決だけで、Web UI が既に使う権限の範囲に収まる。新しい IAM 権限は要らない。
- S3 のバケット一覧の行では、`Row.Name()` と `Row.ID()` がともにバケット名である。一覧の取得には `s3Row.bucketRegion()` が返すバケットのリージョンを使う。選択中のリージョンとバケットのリージョンが異なる場合も、Web UI と同じくバケットのリージョンで取得する。
- ローカルの floci では `THIEF_S3_PATH_STYLE=true` が必要である。`example/README.md` の既存の注意を維持する。

## 完了条件

- TUI の S3 の行の Drawer に `Objects` タブがあり、`ListS3Objects` をバケット名、選択中の prefix、`delimiter` で呼んで一覧を表示する。
- 階層モードでフォルダ行を選んで `enter` を押すとそのフォルダへ移動し、`..` 行を選ぶか、`esc` または `backspace` を押すと親フォルダへ戻る。prefix が空のときは `..` 行を表示しない。
- `f` で prefix 検索を開き、`enter` で確定すると、今いるフォルダの prefix と、前後の空白を除いて先頭の連続する `/` を取り除いた検索語を連結した prefix で再取得する。`esc` で検索を取り消す。
- 検索入力中は `enter`、`esc`、`ctrl+c` 以外の印刷可能なキーと `tab`、`backspace` が文字入力として扱われ、親移動、モード切替、タブ切替、一覧の再取得を起こさない。`r`、`s`、`p`、`R`、`S`、`[`、`]`、`/` を含む。
- 検索入力の `esc` は入力中の値を捨てて入力欄を閉じ、確定済みの検索 prefix と一覧を変えない。`f` で開き直したときは確定済みの検索 prefix が初期値になる。
- 階層モードで prefix が空のときの `esc` と `backspace` は何もしない。
- `m` で階層モードとフラットモードを切り替えると、同じ prefix で `delimiter` を変えて再取得する。
- 一覧の列は Key、Size、Last modified、Storage class で、フォルダ行とオブジェクト行が区別できる。オブジェクトのサイズは Web UI の `formatBytes` と同じ表示にする。
- 読み込み中、取得失敗、空、1000 件での打ち切りの各状態が区別して表示される。打ち切りのときは `Showing the first 1000 entries; narrow the prefix to see more` が表示される。
- フォルダ移動、検索の確定、モード切替、タブ切替、行変更、`r` の再取得の後に届いた古い一覧が表示にも保持にも使われない。
- S3 のバケットのリージョンが選択中のリージョンと異なるとき、`s3Row.bucketRegion()` のリージョンで取得する。
- `backend/internal/tui` の単体テストに、初期表示、フォルダ移動、親へ戻る、検索、`reports` の `r` を含む検索入力がグローバル再取得を起こさないこと、検索の `esc` が確定済み prefix を変えないこと、モード切替、空、失敗、打ち切り、古い結果の破棄、バケットのリージョン解決の各ケースがある。
- `example/README.md` に、floci の `thief-example-data` バケットで `reports/` などの実在するフォルダへの階層移動、`reports` の prefix 検索、階層 / フラット切替を確認する手順が追加されている。`THIEF_S3_PATH_STYLE=true` の注意が残っている。
- `CHANGES.md` の `## develop` に、TUI の S3 Objects タブの [ADD] エントリが 1 つある。担当者行は `- @sfuruya0612` とする。
- `mise run check` が通る。
- 扱わない範囲: オブジェクトのプレビュー、アップロード、ダウンロード、DuckDB の SQL 検索、複数選択、フォルダの作成と削除、prefix の永続化。

## 関連

- PRD `docs/prd/thief.md` の FR-29
- `docs/issues/0226`
- `frontend/src/components/Drawer/DrawerS3Objects.tsx`、`DrawerObjectBrowser.tsx`
- `backend/internal/aws/s3_object.go` の `ListS3Objects`
- `docs/prd/thief.md` の「スコープ」にあるオブジェクトのプレビューと SQL 検索の扱い

## 解決方法

- `backend/internal/tui/s3.go` の `s3Service` に `ListS3Objects` の取得関数を追加し、`Tabs()` に Objects タブを追加した。取得には `s3Row.bucketRegion()` のリージョンを使い、選択中のリージョンと異なるバケットでも正しいリージョンで呼ぶ。
- `backend/internal/tui/tab_s3.go` に `s3ObjectsTab` を追加した。階層モードは `delimiter="/"`、フラットモードは `delimiter=""` で取得し、`..` 行、フォルダ行、オブジェクト行を表示する。`enter` でフォルダへ移動し、`..` 行、`esc`、`backspace` で表示中の prefix の親へ戻る。prefix が空のときの `esc` と `backspace` は何もしない。
- `f` で prefix 検索を開き、前後の空白を除いて先頭の `/` を取り除いた検索語を、今いるフォルダの prefix に連結して再取得する。`esc` は入力中の値を捨て、確定済みの prefix と一覧を変えない。`f` で開き直すと確定済みの prefix が初期値になる。
- 検索入力中は `r`、`s`、`p`、`R`、`S`、`[`、`]`、`/`、`m`、`tab`、`backspace` を入力欄が消費し、タブ切替、モード切替、再取得を起こさない。`tab` は S3 の prefix にタブ文字を入れる用途が無いため、入力欄が消費するだけにした。
- `m` で階層表示とフラット表示を切り替え、同じ prefix で `delimiter` を変えて再取得する。`r` は現在の prefix と表示モードのまま取り直す。
- 読み込み中、取得失敗、空、1000 件での打ち切りを区別して表示する。サブフォルダで `..` 以外に表示するものが無いときも `No objects` を表示する。
- 一覧の取得はタブの context と `loadSeq` で世代管理し、フォルダ移動、検索、モード切替、`r` の後に届いた古い結果を捨てる。
- `backend/internal/tui` の単体テストに、初期表示、フォルダ移動、`..` 行、親へ戻る、prefix 検索、`f` の初期値、検索入力中のグローバルキー隔離、`m` の切替、空、失敗、打ち切り、古い結果の破棄、バケットのリージョンと profile / bucket の引き渡し、prefix の正規化と親の計算を追加した。モデルレベルで検索入力中の `r`、`q`、`s`、`p`、`R`、`S`、`[`、`]`、`/`、`tab` が再取得やタブ切替を起こさず、`ctrl+c` だけが終了することを確認している。
- `example/README.md` に floci の `thief-example-data` バケットでフォルダ移動、prefix 検索、階層 / フラット切替を確認する手順を追加し、`THIEF_S3_PATH_STYLE=true` を起動コマンドに含めた。`CHANGES.md` に [ADD] エントリを追加した。
- `mise run check` が通る。
