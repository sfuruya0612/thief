# S3 / GCS のオブジェクト一覧をフォルダの階層で表示する

Created: 2026-09-28
Model: Claude Fable 5.1

## 背景

TODO.md の次の項目に由来する。

> Object Storage (S3 / GCS) の Object をぜんぶ一覧で表示するのではなく、フォルダごとに階層構造で表示できるようにしたい

現状のオブジェクト一覧は、prefix に前方一致する全オブジェクトを 1 つの平らな表に出す。

- backend の S3 は `backend/internal/aws/s3_object.go` の `ListS3Objects` が `ListObjectsV2Input{Bucket, Prefix}` だけを渡し、`Delimiter` を渡さない。
  `appendS3ObjectsUpToLimit` が蓄積件数を数え、`maxS3ListObjects` (1000) に達した後にまだ追加するオブジェクトがあるときだけ打ち切って `truncated` に true を返す。
  ちょうど 1000 件で終わるときは false である (`backend/internal/aws/s3_object_test.go` の `exactly at limit`)。
- backend の GCS は `backend/internal/gcp/gcs.go` の `ListObjects` が `storage.Query{Prefix}` だけを渡す。
  ループの先頭で `len(objects) >= maxGCSListObjects` を判定してから `it.Next()` を呼ぶため、ちょうど 1000 件のときも次のエントリの有無を確かめずに true を返す。
  この境界の挙動は S3 と食い違っている。
- API の応答は `backend/internal/api/handlers_s3_object.go` の `S3ObjectsResponse{Objects, Truncated}` と `backend/internal/api/handlers_gcp.go` の `GCSObjectsResponse{Objects, Truncated}` である。
  キャッシュキーは `cacheKey("s3-objects", profile, region, bucket, prefix)` と `cacheKey("gcp-gcs-objects", projectID, bucket, prefix)` で、アップロード後はそれぞれ `cacheKey("s3-objects", profile, region, bucket, "")` と `cacheKey("gcp-gcs-objects", projectID, bucket, "")` の前方一致 (`InvalidatePrefix`) でバケット単位に無効化する。
  一覧のハンドラのテストは無い (`handlers_s3_object_test.go` は `sanitizeContentDispositionFilename` と `readS3UploadBody` のテストだけで、GCS のハンドラのテストファイルは存在しない)。
  `gcp.ListObjects` は内部で `storage.NewClient` を作るため、テストで差し替える接ぎ目が無い (`backend/internal/gcp/gcs_test.go` は `bucketFromAttrs` と `objectFromAttrs` のテストだけである)。
- frontend は `frontend/src/components/Drawer/DrawerObjectBrowser.tsx` の `DrawerObjectBrowser` が `committedPrefix` を `useObjects(prefix)` に渡し、返った全オブジェクトを `toTableRow` で行にして `DataTable` で描く。
  prefix の入力欄と検索ボタンがあり、`normalizeSearchPrefix` は前後の空白と先頭の連続する `/` を除く (末尾は加工しない)。
  アップロード先は `normalizeUploadPrefix(prefixInput)` で入力欄の値から決める。
  受け口の型は `ObjectListQuery<TObject>` (`data.objects` と `data.truncated`) と `useObjects: (prefix: string) => ObjectListQuery<TObject>` である。
  取得のフックは `frontend/src/api/queries.ts` の `useS3Objects(profile, region, bucket, prefix)` と `useGcsObjects(projectId, bucket, prefix)` で、`frontend/src/api/endpoints.ts` の `getS3Objects` と `getGcsObjects` を呼び、queryKey に prefix を含む。
  応答の型は `frontend/src/types/common.ts` の `ObjectListEnvelopeRaw<T>` (`objects` と `truncated`) である。
- 行の型は S3 が `frontend/src/components/Drawer/DrawerS3Objects.tsx` の `S3ObjectTableRow` (`S3ObjectRow` に `id: key` と `state: ''` を足したもの)、GCS が `DrawerGCSObjects.tsx` の `GcsObjectTableRow` (`GcsObjectRow` の `id` は `lib/normalizeGcp.ts` の `gcsObjectFromRaw` が `${bucket}/${name}#${index}` で作る) である。
  列は `frontend/src/components/tables/columns.tsx` の `s3ObjectColumns` (`key`、`size`、`lastModified`、`storageClass`) と `gcpColumns.tsx` の `gcsObjectColumns` (`name`、`size`、`contentType`、`storageClass`、`updated`) で、各 `cell` は行の値をそのまま描く。
  `DrawerObjectBrowser` は全行に対して `previewKeyOf`、`sizeOf`、`downloadHref`、`rowClassName` (プレビュー不可の行に `preview-ineligible`) を呼ぶ。
- 共有部品 `frontend/src/components/DataTable.tsx` は、全行の先頭にチェックボックスの列を描き、チェック状態を内部 state `checked` に持つ。
  外から制御する props は無く、Objects タブではこの列は何にも使われていない。
  ソートは `sortValue` が `row[key]` を読み、値が無い行を末尾に置く。
- 打ち切りの文言 `drawerObjectBrowser.truncatedNotice` (`frontend/src/i18n/locales/{ja,en}/drawerStorage.json`) は「prefix で絞り込んで再検索してください」と助言する。

要望が満たされていない理由は、区切り文字 (delimiter) を使わないことにある。
`logs/2024/01/…` のように深い階層を持つバケットでは、ルートを開くと全階層のオブジェクトが混ざり、1000 件で打ち切られる。
どのフォルダがあるかは、利用者が prefix を自分で打ち込むまで分からない。

API の事実は次のとおりである。

- S3 の ListObjectsV2 は、`Delimiter` を指定すると `Prefix` から次の区切り文字までのキーを `CommonPrefixes` に畳み込んで返す (`github.com/aws/aws-sdk-go-v2/service/s3` の `ListObjectsV2Output.CommonPrefixes`)。
  畳み込まれたキーは `Contents` には含まれない。
  `MaxKeys` は `Contents` と `CommonPrefixes` を合わせて数える。
- GCS は、`storage.Query.Delimiter` を指定すると `ObjectIterator.Next` が合成ディレクトリエントリを `ObjectAttrs.Prefix` だけを埋めた `ObjectAttrs` として返す (`cloud.google.com/go/storage@v1.67.1` の `ObjectAttrs.Prefix` のコメント)。
  そのエントリでは `Name` など他のフィールドが空である。
- どちらも一覧の権限 (`s3:ListBucket`、`storage.objects.list`) の範囲であり、追加の API 呼び出しや権限は要らない。
- キーの末尾が `/` の 0 バイトのオブジェクト (コンソールのフォルダ作成が置くプレースホルダ) は、親の階層では区切り文字を含むため `CommonPrefixes` に畳み込まれる。
  そのフォルダの中を開いたとき (prefix がそのキーと等しいとき) だけ、`Contents` にキーが prefix と等しいオブジェクトとして現れる。

契約の検査 (`backend/internal/contract/contract.go` の `Registry`) に登録されているのは `S3ObjectResource` だけで、エンベロープ (`S3ObjectsResponse`、`GCSObjectsResponse`) は登録されていない。
`example/seed.sh` は `thief-example-data` バケットの `reports/`、`notes/`、`config/` にオブジェクトを置く。

## 目的

S3 / GCS の Objects タブで、バケットをフォルダの階層として 1 階層ずつ辿れるようにする。
1 階層ずつ取得するため、深いバケットでも 1000 件の打ち切りに当たりにくくなる。

## 設計判断

### 階層は API の区切り文字で作る

backend が `Delimiter` を `/` にして API を呼び、フォルダ相当の prefix を応答に足す。

採らなかった案。

- frontend が平らな 1000 件を `/` で分割してツリーにする。
  打ち切られた 1000 件に含まれないフォルダが見えないため、深い階層を持つバケットでは要望を満たせない。
- backend が全オブジェクトを再帰的に取得してツリーを返す。
  オブジェクトが膨大なバケットで取得時間と応答の大きさに上限が無くなり、1000 件で打ち切る方針 (`docs/prd/thief.md` の FR-6) と矛盾する。

### API は既存のエンドポイントに `delimiter` を足す

`GET /api/aws/profiles/{profile}/s3/{bucket}/objects` と `GET /api/gcp/gcs/{bucket}/objects` にクエリパラメータ `delimiter` を足す。
受け付ける値は空と `/` の 2 つだけとし、それ以外は 400 にする。
以下、`delimiter` が `/` の呼び出しを**階層モード**、空の呼び出しを**フラットモード**と呼ぶ。
応答のエンベロープに `prefixes` (フォルダの完全な prefix、末尾 `/` 付き、API が返した順) を足す。
フラットモードは現状どおり平らな一覧で、`prefixes` は空配列にする (null にしない)。

- 打ち切りの境界は S3 の現状 (`appendS3ObjectsUpToLimit`) に揃える。
  オブジェクトとフォルダを合わせた蓄積件数が 1000 に達した後、まだ追加するエントリ (同じページの残り、または次のページ) があるときだけ打ち切って `truncated` に true を返す。
  ちょうど 1000 件で終わるときは false である。
  GCS もこの境界に揃える (1000 件に達したら `it.Next()` を 1 回呼び、`iterator.Done` なら false、エントリが返れば true)。
  現状の GCS はちょうど 1000 件で true を返すため、この点だけ挙動が変わる。
  GCS の境界の判定は、`it.Next` に相当する関数 (`func() (*storage.ObjectAttrs, error)`) と上限を受け取る純関数 (`collectGCSObjects`) に切り出し、`storage.Client` 無しでテストする。
- キャッシュキーには `delimiter` を含める。
  アップロード後の無効化は `cacheKey("s3-objects", profile, region, bucket, "")` と `cacheKey("gcp-gcs-objects", projectID, bucket, "")` の前方一致であり、どちらも `delimiter` を prefix の後ろに置けば両モードのキャッシュが無効化される。
- `ListS3Objects` と `gcp.ListObjects` は `delimiter` 引数を足し、戻り値に `prefixes []string` を足す。
  CLI (`thief s3 objects`、`thief gcp gcs objects`) は空の `delimiter` を渡して現状の平らな一覧のままにする。
- ハンドラのテストのため、一覧関数は `Server` のフィールド (`ec2Resources` と同じ形の `s3Objects`、`gcsObjects`) 経由で呼び、テストで差し替える。
  `NewServer` は実装 (`ListS3Objects`、`gcp.ListObjects`) を、`newTestServer` (`backend/internal/api/server_test.go`) は呼ばれたらエラーを返すダミーを入れる (`ec2Resources` と同じ規約)。

採らなかった案。

- 新しいエンドポイント (`/objects/tree` など) を足す。
  一覧、キャッシュ、無効化のコードが二重になる。
- 打ち切りを「1000 件に達した時点」で行う。
  ちょうど 1000 件のフォルダで打ち切りの通知が出る。
  S3 の現状のテスト (`exactly at limit` は false) とも食い違う。

### frontend は「今いるフォルダ」を持ち、フォルダ行とパンくずで辿る

`DrawerObjectBrowser` の state を次の 4 つにする。

- `currentPrefix`: 今いるフォルダ。空か末尾 `/` 付き。
- `mode`: `hierarchy` (階層モード) か `flat` (フラットモード)。初期値は `hierarchy`。
- `prefixInput`: 検索欄の入力。
- `committedInput`: 検索ボタンで確定した入力 (`normalizeSearchPrefix` を通した値)。

取得に渡す prefix は `currentPrefix` と `committedInput` の連結、`delimiter` は `mode` が `hierarchy` なら `/`、`flat` なら空である。

- 行の型を `ObjectBrowserRow<TRow>` にする。
  オブジェクト行は `TRow` に `kind: 'object'` を足したもの、フォルダ行は `{ kind: 'folder', id, state: '', prefix, name, key }` である。
  `prefix` はフォルダの完全な prefix (末尾 `/` 付き) で、`name` と `key` にも同じ値を入れる (S3 の名前の列は `key`、GCS は `name` を `row[key]` として読むため、どちらでもソートが効き、ソートの値がオブジェクト行の完全なキーと同じ形になる)。
  表示する相対名 (prefix から `currentPrefix` を除いたもの、末尾 `/` 付き) は名前の列の `cell` が計算する。
  `id` は `folder:` と prefix の連結にする。
  オブジェクト行の `id` (S3 はキー、GCS は `${bucket}/${name}#${index}`) は同じ一覧の中で `folder:` から始まって `/` で終わることが無いため衝突しない。
- `DataTable` に渡す並びは、フォルダ行を先頭に、オブジェクト行をその後ろにする (ソートしていない初期表示の並び)。
  ソートは `DataTable` の規則のままとし、名前の列でソートするとフォルダ行とオブジェクト行が完全なキーの順に混ざり、値の無い列 (サイズなど) ではフォルダ行が末尾に回る。
- 列は `baseColumns` を `ObjectBrowserRow<TRow>` の列に包んで作る。
  名前の列 (`baseColumns` の先頭) は、フォルダ行ではフォルダのアイコンと相対名をクリックできる形で描き、オブジェクト行では階層モードなら `currentPrefix` を除いた相対名、フラットモードなら完全なキーを描く。
  名前以外の列は、フォルダ行では何も描かない。
  列フィルタ (`filterValue`) は行の種別で分ける。
  フォルダ行は、名前の列では相対名、他の列では空文字を返す。
  オブジェクト行は、名前の列では表示している名前 (階層モードは相対名、フラットモードは完全なキー)、他の列では元の列の判定 (元の `filterValue` があればその値、無ければ `row[key]` の文字列化) をそのまま返す。
- Actions 列 (Preview、Query、Download) はフォルダ行では何も描かない。
  `previewKeyOf`、`sizeOf`、`downloadHref`、`rowClassName` はオブジェクト行にだけ呼び、フォルダ行は `kind` で先に分岐する。
- 階層モードでは、キーが `currentPrefix` と等しいオブジェクト (フォルダのプレースホルダ) を一覧から除く。
  相対名が空になり、行として意味が無い。
- フォルダ行のクリックで `currentPrefix` をそのフォルダの prefix にし、`prefixInput` と `committedInput` を空にする。
  見出しの下にパンくず (バケットのルート、各階層) を出し、クリックでその階層に戻る (同じく入力を空にする)。
- 検索欄は「今いるフォルダの中で名前の前方一致」に意味を変える (AWS コンソールと同じ)。
  入力が `/` で終わらないとき (ルートで `rep` など)、フォルダ行 `reports/` とオブジェクト行 `reports.txt` が混在するが、どちらも `currentPrefix` からの相対名なので表示は揃う。
- アップロード先は `currentPrefix` に `normalizeUploadPrefix(prefixInput)` を連結したものにする。
  今いるフォルダへのアップロードと、検索欄に打った新しいフォルダ名へのアップロードの両方ができる。
- 表示モードのトグル (階層 / フラット) を検索欄の横に置く。
  トグルは `currentPrefix` と入力を変えない。
  フラットモードは現状の挙動 (delimiter 無し) で、今いるフォルダ以下の全階層のオブジェクトを平らに出す。
  フラットモードは、issue 0208 で階層をまたいだオブジェクトをまとめて選ぶために要る。
- 打ち切りの通知は階層モードでは文言を変え、フォルダに潜るか検索欄で絞り込むよう助言する (`truncatedNoticeHierarchy`)。
  フラットモードは現状の文言のままにする。
- `DataTable` の既存のチェックボックス列は変えない。
  現状どおり全行に出て (フォルダ行にも出る)、何にも使われない。
  この列の制御化は issue 0208 で行う。
- 今いるフォルダとモードは `DrawerObjectBrowser` の state に置き、永続化しない。
  region の変更で作り直す既存の挙動 (`DrawerS3Objects` の `key={region}`) は変えない。
- `ObjectListQuery<TObject>` の `data` に `prefixes: string[]` を足し、`useObjects` を `(prefix: string, delimiter: string) => ObjectListQuery<TObject>` にする。
  `useS3Objects` と `useGcsObjects`、`getS3Objects` と `getGcsObjects` は `delimiter` 引数を足し、フックは queryKey に含める。
- 文言 (パンくずのルート、トグルのラベル、フォルダ行の title、階層モードの打ち切り通知) は `frontend/src/i18n/locales/{ja,en}/drawerStorage.json` に足す。

採らなかった案。

- 階層モードだけにしてフラットモードを廃止する。
  `year=2024/month=01/` のように分かれた parquet を issue 0208 でまとめて選べなくなる。
- フォルダ行を `DataTable` の外の別の一覧に出す。
  列フィルタとソートの対象から外れ、AWS コンソールと見た目が変わる。
- フォルダ行を `TRow` の形に無理に合わせる (サイズ 0 などの仮の値を入れる)。
  サイズの列に `0 B` が出て、フォルダかオブジェクトかを列の値で区別できない。
- フォルダに潜っても検索欄の入力を残す。
  潜った先で前の入力による絞り込みが掛かったままになり、フォルダの中身が全部は見えない。

### ドキュメント

- `docs/prd/thief.md` の FR-6 を、階層モードとフラットモードの 2 モード、打ち切りの境界 (1000 件を超える残りがあるときだけ)、両モードでの打ち切りの通知の記述に更新する。
  受け入れ基準「1500 件のオブジェクトがある prefix を開くと、1000 件を表示し、打ち切りの通知を表示する」を、階層モードでフォルダとオブジェクトを合わせて 1500 件あるフォルダを開いたときの基準に書き換える。
- Google Cloud の節の「FR-6 と同じ規則で」の列挙に階層の表示を足す。
- 判断 (API の区切り文字で階層を作り、フラットモードを残す) を ADR として `docs/adr/0029` に書く。

## 未確定論点

- floci の `ListObjectsV2` が `Delimiter` と `CommonPrefixes` に対応しているかは確認していない (`example/README.md` の「動作確認できない機能」にも記載が無い)。
  実装時に `mise run example:up` と `mise run example:seed` の後、`example/seed.sh` と同じダミーの認証情報で `AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test AWS_DEFAULT_REGION=ap-northeast-1 aws --endpoint-url http://localhost:4566 s3api list-objects-v2 --bucket thief-example-data --delimiter /` を実行し、`CommonPrefixes` に `reports/`、`notes/`、`config/` が返り、`Contents` が無いことを確かめる。
  対応していなければ、階層モードの確認は実アカウントで行い、その旨を `example/README.md` の「動作確認できない機能」に足す。

## 完了条件

backend。

- `ListS3Objects(ctx, profile, region, bucket, prefix, delimiter)` が、`delimiter` が `/` のとき `CommonPrefixes` を `prefixes []string` として返し、空のときは現状と同じ結果を返す。
- `gcp.ListObjects(ctx, projectID, bucket, prefix, delimiter)` が、`delimiter` が `/` のとき `ObjectAttrs.Prefix` が空でないエントリを `prefixes []string` として返し、`objects` に含めない。
- 打ち切りは、オブジェクトとフォルダを合わせた蓄積件数が 1000 に達した後にまだ追加するエントリがあるときだけ行い、`truncated` に true を返す。
  ちょうど 1000 件で終わるときは false である。
  上限の判定を行う純関数に次のテーブル駆動テストがある。
  フォルダだけでちょうど 1000 件 (false)、フォルダ 1000 件とオブジェクト 1 件 (true)、オブジェクト 600 件とフォルダ 400 件 (false)、オブジェクト 600 件とフォルダ 401 件 (true)。
- GCS の境界の判定が純関数 `collectGCSObjects` に切り出され、ちょうど 1000 件で false、1001 件で true、`ObjectAttrs.Prefix` だけを持つエントリが `prefixes` に入って `objects` に入らないことのテストが `backend/internal/gcp/gcs_test.go` にある。
- `GET …/objects?prefix=<p>&delimiter=/` が `{"objects": [...], "prefixes": [...], "truncated": false}` を返す。
  `delimiter` が空でも `/` でもない値は 400 になる。
  `prefixes` は該当が無いとき `[]` であり、`null` にならない。
  S3 のハンドラのテスト (`backend/internal/api/handlers_s3_object_test.go` に追加) と GCS のハンドラのテスト (`backend/internal/api/handlers_gcp_test.go` を新設) に、`Server` のフィールド (`s3Objects`、`gcsObjects`) を差し替えて、階層モードの応答、`[]` の応答、400 のケースを検証するテストがある。
  `newTestServer` が `s3Objects` と `gcsObjects` を、呼ばれたらエラーを返すダミーで初期化している。
- キャッシュキーが `delimiter` を含み、アップロード後の無効化で階層モードとフラットモードの両方のキャッシュが消える。
- `thief s3 objects` と `thief gcp gcs objects` は `delimiter` を渡さず、出力の列と行は変わらない。
  `thief gcp gcs objects` のちょうど 1000 件のときの打ち切りの警告 (標準エラー出力) は、境界の変更に従って出なくなる。

frontend。

- `ObjectListEnvelopeRaw<T>` (`objects` と `truncated` の並び) に `prefixes: string[]` を足し、`ObjectListQuery<TObject>` の `data` にも `prefixes: string[]` を足す (backend が `[]` を保証するため null を許さない)。
  `useObjects`、`useS3Objects`、`useGcsObjects`、`getS3Objects`、`getGcsObjects` が `delimiter` を受け取り、フックは queryKey に含める。
- Objects タブを開いた直後は、バケットのルートを階層モードで表示する (フォルダ行と、ルート直下のオブジェクト)。
- ソートしていない初期表示ではフォルダ行が一覧の先頭に並び、名前の列にフォルダ名 (相対名) が出て、他の列と Actions 列には何も出ない。
  名前の列でソートすると、フォルダ行とオブジェクト行が完全なキー (フォルダ行は prefix) の順に並ぶ。
  フォルダ行は `preview-ineligible` にならない。
- 階層モードのオブジェクト行の名前の列は `currentPrefix` を除いた相対名で、フラットモードでは完全なキーである。
- キーが `currentPrefix` と等しいオブジェクトは、階層モードの一覧に出ない。
- フォルダ行をクリックするとそのフォルダに潜り、パンくずをクリックするとその階層に戻る。
  どちらでも検索欄と確定済みの入力が空になる。
- 検索欄の入力は今いるフォルダの中の前方一致になる (`prefix` が `currentPrefix` と入力の連結で送られる)。
- 名前の列の列フィルタは、フォルダ行を相対名、オブジェクト行を表示している名前で絞る。
  名前以外の列の列フィルタは、オブジェクト行で現状どおり効き、フォルダ行には一致しない。
- トグルでフラットモードに切り替えると `delimiter` 無しで取得し、今いるフォルダ以下の全階層のオブジェクトを平らに出す。
  トグルで `currentPrefix` と入力は変わらない。
- アップロード先のキーが `currentPrefix` と `normalizeUploadPrefix(prefixInput)` とファイル名の連結になる。
- 打ち切りの通知が、階層モードでは `truncatedNoticeHierarchy`、フラットモードでは `truncatedNotice` の文言で出る。
- 見出しの件数 (`Objects (N)`) はオブジェクトの件数であり、フォルダを含めない。
- `DrawerS3Objects.test.tsx` と `DrawerGCSObjects.test.tsx` に、フォルダ行の描画 (名前だけが出て他の列と Actions が空)、フォルダへの遷移とパンくずでの復帰 (入力が空になる)、相対名の表示、プレースホルダの除外、検索欄の前方一致、列フィルタ、名前の列のソート、トグルの切り替え、アップロード先のキー、両モードの打ち切り通知のテストがある。
- 文言が ja と en の `drawerStorage.json` にある。

ドキュメントと検証。

- `docs/prd/thief.md` の FR-6 (本文と受け入れ基準) と Google Cloud の節が「設計判断」の「ドキュメント」のとおりに更新されている。
- `docs/adr/0029` が書かれ、`docs/adr/README.md` の表に行がある。
- `CHANGES.md` の `## develop` に追記されている (GCS の境界の変更を含む)。
- `mise run check` が通る。

扱わない範囲。

- CLI の階層表示 (`--delimiter` の追加)。
- フォルダの作成、削除、名前の変更。
- `DataTable` のチェックボックス列の制御化と、複数のオブジェクトの選択と SQL 検索 (issue 0208)。

## 関連

- issue 0208 (複数のオブジェクトの SQL 検索) は本 issue のフラットモードと行の型 (`ObjectBrowserRow`) に依存する。
  本 issue を先に実装する。
