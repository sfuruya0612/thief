# S3 アップロード後、Objects 一覧にアップロードしたファイルが出てこない

Created: 2026-07-10
Completed: 2026-07-10
Model: Claude Sonnet 5

## 再現手順

1. `mise run backend:run` / `mise run frontend:run` (または docker compose) で起動する。
2. S3 バケットの Drawer を開き、Objects タブでファイルをアップロードする。
3. アップロードは成功し、AWS Console から確認すると S3 に実際にオブジェクトが作成されている。
4. しかし Objects タブの一覧には、TTL (60 秒) が切れるかリロード後の再取得タイミングまでアップロードしたファイルが表示されない。

## 原因

`backend/internal/api/handlers_s3_object.go` の `handleS3ObjectUpload` は、アップロード成功後に以下のコメントを残しているが、対応する実装コードが存在しない。

```go
// アップロード成功後、対象バケット配下のオブジェクト一覧キャッシュを無効化する。
// prefix ごとにキーが分かれるため prefix 部分をワイルドカード相当で扱えないので
// バケット単位で invalidate する簡易実装として、対象キーを含む prefix 群は
// 次回アクセス時に refresh=true が来るまで stale の可能性がある。
// (現状 cache パッケージにワイルドカード invalidate API がないため許容)
writeJSON(w, map[string]string{"status": "ok", "key": objectKey})
```

`handleS3Objects` (一覧取得) は `cacheKey("s3-objects", profile, region, bucket, prefix)` で `prefix` ごとに異なるキーを持つ `resourceCache` (60 秒 TTL) から一覧を返す。アップロード時に該当エントリを `Invalidate` していないため、TTL が切れるまで古い一覧がキャッシュから返り続ける。

加えて、フロントエンド側にも同種の問題がある。`frontend/src/api/queries.ts` の `useS3Upload` は `onSuccess` で

```ts
void queryClient.invalidateQueries({
  queryKey: ['aws', 's3-objects', profile, region, bucket],
});
```

を実行しているが、`frontend/src/api/endpoints.ts` の `uploadS3Object` はアップロードリクエストに `prefix` を渡していない。TanStack Query の `queryKey` は `['aws', 's3-objects', profile, region, bucket, prefix]` の完全一致 (前方一致) で invalidate されるため、この呼び出し自体は `bucket` までの前方一致で該当 `bucket` の全 `prefix` 分の queryKey を正しく invalidate できる。ただし、フロントが再フェッチしてもバックエンドの `resourceCache` 側が古い値を返す限り、表示は更新されない。

## 影響

- S3 バケットへファイルをアップロードした直後、Objects 一覧に反映されない (最大 60 秒の TTL の間、stale な一覧が表示される)。
- ユーザーはアップロードが失敗したと誤解する可能性がある。実際には S3 側には正しく書き込まれている。

## 解決方法

`backend/internal/cache/cache.go` の `Cache[V]` に `InvalidatePrefix(prefix string)` を追加した。指定した prefix で前方一致するキーを一括で削除するメソッドで、`prefix` クエリパラメータの違いによってキーが分岐する `s3-objects` キャッシュ全体を、実際に使われている prefix 値を知らなくても無効化できる。

`backend/internal/api/handlers_s3_object.go` の `handleS3ObjectUpload` で、アップロード成功後に

```go
s.resourceCache.InvalidatePrefix(cacheKey("s3-objects", profile, region, bucket, ""))
```

を呼び出し、対象バケットの全 `s3-objects` キャッシュエントリ (prefix 違いを含む) を無効化するようにした。これにより、アップロード後の一覧取得は必ずキャッシュミスとなり、S3 から最新の一覧を再取得する。

`backend/internal/cache/cache_test.go` に `TestCacheInvalidatePrefix` を追加し、prefix 一致するキーのみが削除され他のキーが残ることをテーブル駆動テストで検証した。`mise run backend:fmt` / `mise run backend:lint` / `mise run backend:test` がすべて成功することを確認した。
