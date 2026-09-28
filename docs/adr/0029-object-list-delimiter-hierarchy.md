# 0029. オブジェクトの階層は API の区切り文字で 1 階層ずつ取得し、フラットモードを残す

Created: 2026-09-28
Model: Claude Fable 5.1
Status: Accepted
Decided: 2026-09-28

## 状況

S3 と GCS のオブジェクト一覧は、prefix に前方一致する全オブジェクトを平らな表に出し、1000 件で打ち切る (FR-6)。
`logs/2024/01/` のように深い階層を持つバケットでは、ルートを開くと全階層のオブジェクトが混ざって打ち切られ、どのフォルダがあるかは利用者が prefix を打ち込むまで分からない (`docs/issues/TODO.md`)。

## 決定

- backend が S3 の `ListObjectsV2` と GCS の `storage.Query` に区切り文字 `/` を渡し、フォルダに当たる prefix を応答の `prefixes` として返す。
  既存のエンドポイント (`GET /api/aws/profiles/{profile}/s3/{bucket}/objects`、`GET /api/gcp/gcs/{bucket}/objects`) にクエリパラメータ `delimiter` を足し、受け付ける値は空と `/` の 2 つに限る。
- frontend は「今いるフォルダ」を持ち、フォルダ行とパンくずで 1 階層ずつ辿る (階層モード)。
  区切り文字を渡さない従来の平らな一覧はフラットモードとして残し、トグルで切り替える。
- 打ち切りは、オブジェクトとフォルダを合わせた蓄積件数が 1000 に達した後にまだ追加するエントリがあるときだけ行う。
  ちょうど 1000 件は打ち切らない。
  GCS もこの境界に揃える (これまではちょうど 1000 件でも打ち切っていた)。
- CLI (`thief s3 objects`、`thief gcp gcs objects`) は区切り文字を渡さず、平らな一覧のままとする。

## 検討した代替案

issue 0207 は次の案を採らなかった。

- frontend が平らな 1000 件を `/` で分割してツリーにする。
  打ち切られた 1000 件に含まれないフォルダが見えないため。
- backend が全オブジェクトを再帰的に取得してツリーを返す。
  取得時間と応答の大きさに上限が無くなり、1000 件で打ち切る方針 (FR-6) と矛盾するため。
- 新しいエンドポイント (`/objects/tree` など) を足す。
  一覧、キャッシュ、無効化のコードが二重になるため。
- 階層モードだけにしてフラットモードを廃止する。
  `year=2024/month=01/` のように分かれた parquet をまとめて選ぶ操作 (issue 0208) ができなくなるため。

## 結果

- 1 階層ずつ取得するため、深いバケットでも 1000 件の打ち切りに当たりにくくなる。
- 追加の API 呼び出しや権限は要らない。
  区切り文字の指定は一覧の権限 (`s3:ListBucket`、`storage.objects.list`) の範囲である。
- 応答のエンベロープに `prefixes` が増えた。
  フラットモードでは空配列である。
- GCS のちょうど 1000 件のときの打ち切りの通知が出なくなった。

## 根拠資料

- `docs/issues/closed/0207`
- `backend/internal/aws/s3_object.go`、`backend/internal/gcp/gcs.go`
- `frontend/src/components/Drawer/DrawerObjectBrowser.tsx`
