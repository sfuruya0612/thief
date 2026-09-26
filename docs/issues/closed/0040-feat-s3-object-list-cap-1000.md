# S3 オブジェクト一覧の取得を最大 1000 件で打ち切り prefix で絞り込めるようにする

Created: 2026-07-18
Completed: 2026-07-18
Model: Claude Opus 4.8

## 背景

`internal/aws/s3_object.go` の `ListS3Objects` は `s3.NewListObjectsV2Paginator` を `HasMorePages` が false になるまで回し、全オブジェクトを取得する。
取得件数に上限がないため、オブジェクトが膨大なバケットでは取得に時間がかかり、レスポンスも肥大化する。
ユーザが必要とするのは通常一部のオブジェクトであり、全件を取得する必要はない。

prefix は既に対応済みである。
`ListS3Objects` は引数 `prefix` を受け取り、frontend の `useS3Objects` (`api/queries.ts`) も `queryKey` に prefix を含むため、prefix を変更すると再取得が走る。
不足しているのは取得件数の上限と、prefix を入力する UI である。

## 目的

オブジェクト一覧の取得を最大 1000 件で打ち切る。
上限に達したことをユーザに提示し、prefix で絞り込んで再取得できるようにする。

## 対応内容

### backend

- `ListS3Objects` のオブジェクト取得を、蓄積件数が 1000 に達した時点でループを抜ける形で打ち切る (ページネータの既定ページサイズに依存しないため 1 ページ止めより堅牢)
- 上限で打ち切ったかどうかを示すフラグをレスポンスに含める (例: truncated)
- 現状 `handleS3Objects` は `ListS3Objects` の結果を素の JSON 配列として返す。truncated を加えると `{objects, truncated}` のエンベロープ形式へ変わるため、戻り値型・ハンドラ・frontend の `getS3Objects` (`apiGetList`) と `useS3Objects` の map (`api/queries.ts`) を連動して変更する

### frontend

- prefix の入力欄を追加し、入力値でオブジェクト一覧を再取得する (既存の queryKey により再取得は自動で走る)
- 上限で打ち切られた場合、その旨と prefix での絞り込みを促す表示を出す

## 設計上の論点

- GCS の `internal/gcp/gcs.go` の `ListObjects` も同様に上限なしで全件取得している。本 issue は TODO の記載に合わせ S3 を対象とするが、GCS へ同じ対応を広げるかは要判断とする。

## 完了条件

- オブジェクトが 1000 件を超えるバケットで取得が 1000 件で打ち切られる
- 打ち切りが発生したことがユーザに提示される
- prefix を指定すると絞り込んだ結果を再取得できる

## 解決方法

設計上の論点は以下の方針とした (ユーザ判断)。

- GCS の `internal/gcp/gcs.go` の `ListObjects` にも同じ 1000 件上限を同時に導入した。
- prefix 入力での再取得は、入力の都度ではなく明示的な「検索」ボタン押下 (または Enter) で
  サーバへ再取得を要求する方式にした。

### backend

- `internal/aws/s3_object.go` に `maxS3ListObjects = 1000` を追加し、`ListS3Objects` の戻り値を
  `(resources, truncated, err)` に変更した。蓄積件数の判定はページ単位ではなくオブジェクト単位で
  行う `appendS3ObjectsUpToLimit` ヘルパーに切り出し、ページの途中でも打ち切れるようにした
  (`ListObjectsV2` の既定ページサイズに依存しない)。
- `internal/gcp/gcs.go` の `ListObjects` にも同様に `maxGCSListObjects = 1000` を導入し、戻り値を
  `(objects, truncated, err)` に変更した。
- `handleS3Objects` / `handleGCPGCSObjects` のレスポンスを `{objects, truncated}` エンベロープに
  変更した (`S3ObjectsResponse` / `GCSObjectsResponse`)。
- CLI (`internal/cli/gcp.go` の `gcpRunObjects`) の呼び出し側を新しいシグネチャに追従させ、
  打ち切り時は stderr に警告を出すようにした。
- `appendS3ObjectsUpToLimit` の境界値 (ちょうど上限、上限超過、ページ途中で上限到達等) を
  テーブル駆動テストで検証した。GCS 側の `ListObjects` はイテレータ内に判定を直接書いており、
  ネットワーク呼び出しのモックが必要になるため、既存のテスト方針 (フィールド変換関数のみ
  テスト対象) に合わせてテストは追加していない。

### frontend

- `types/common.ts` に `ObjectListEnvelopeRaw<T>` を追加し、`getS3Objects` / `getGcsObjects` が
  このエンベロープを返すようにした。
- `useS3Objects` / `useGcsObjects` は `{objects, truncated}` を返すよう変更した。
- `DrawerObjectBrowser` の prefix フィルタをフロントエンド側フィルタから、検索ボタン押下で
  `queryKey` (prefix) を確定させサーバへ再取得させる方式に変更した。`useObjects` フックを
  `data`/`isLoading`/`error` の直接 props の代わりに注入する形に変えた (`useUpload`/`usePreview`
  と同じ hook-as-prop パターン)。
- 打ち切り発生時は一覧上部に通知を表示する。

### 完了条件の確認

- `appendS3ObjectsUpToLimit` の単体テストで 1000 件ちょうど・1001 件・ページ途中到達の各ケースを
  確認した。
- `DrawerS3Objects.test.tsx` に、prefix 入力だけでは再取得されないこと、検索ボタン押下で
  prefix 付きの再取得が走ること、打ち切り時に通知が表示されることのテストを追加し、全て通過を
  確認した。
