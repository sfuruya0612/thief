# リソースキャッシュのキーが要素の未エスケープにより衝突する

Created: 2026-07-29
Completed: 2026-07-29
Model: Claude Opus 5

## 背景

issue 0094 (CostExplorer にアカウントとサービスの絞り込みを追加する) の実装中に発見した別バグである。0094 の完了条件には含まれないため、独立した issue として登録する。

`backend/internal/api/server.go` の `cacheKey(parts ...string) string` は、API サーバのリソースキャッシュ (`internal/cache`) のキーを組み立てる共通ヘルパーであり、`internal/api` 配下のテスト以外のコードから 64 箇所で呼ばれている。修正前の実装は各要素をエスケープせず `":"` で単純に連結していた。

```go
func cacheKey(parts ...string) string {
	key := ""
	for i, p := range parts {
		if i > 0 {
			key += ":"
		}
		key += p
	}
	return key
}
```

この実装では、要素の値に区切り文字である `":"` が含まれると、その `":"` が区切りとして解釈された場合と区別できない。結果として異なる引数の組み合わせが同一のキーへ写り、一方のリクエストの結果が他方のリクエストへ返る。

キーの要素にはクエリパラメータやリクエストボディ由来の自由入力が入る箇所が複数ある。

- `handlers_cost.go:34-46` (`costCacheKey`) — Cost Explorer の絞り込み条件 (`ServiceFilter` / `AccountFilter`)
- `handlers_dynamo.go:30` — DynamoDB のスキャン条件 (`PKValue` / `SKValue` / `AttrName` / `AttrValue` の 4 要素が連続する)
- `handlers_s3_object.go:33` — S3 オブジェクトのプレフィックス
- `handlers_gcp.go:96` — GCS オブジェクトのプレフィックス

とくに `handlers_dynamo.go` は自由入力が 4 要素連続するため、隣接要素間で値がずれる組み合わせを作りやすい。

## 再現手順

1. API サーバを起動する (`mise run backend:run`)。
2. DynamoDB のスキャンエンドポイントに対し、`AttrName` と `AttrValue` の組み合わせが異なるが `":"` の位置だけがずれた 2 通のリクエストを送る。
   - リクエスト A: `attr_name=a:b`, `attr_value=c`
   - リクエスト B: `attr_name=a`, `attr_value=b:c`
3. 修正前の `cacheKey` はどちらも同じキー (`... :a:b:c`) を生成するため、先に実行した側の結果が後続のリクエストへキャッシュヒットとして返る。

同じ現象は Cost Explorer の `service` / `account` パラメータでも起きる。

- リクエスト A: `service=a:b`, `account=c`
- リクエスト B: `service=a`, `account=b:c`

最小の再現は Go のテストでも確認できる。修正前の実装では次が成立してしまう。

```go
cacheKey("cost", "a:b", "c") == cacheKey("cost", "a", "b:c") // どちらも "cost:a:b:c"
```

## 影響

- 別の絞り込み条件の結果が表示される。コスト画面では、絞り込んだはずのサービス/アカウントとは異なる条件の金額が表示される。
- DynamoDB のスキャン結果でも同様に、別の属性条件の結果が返る。
- キャッシュはインメモリ (`cache.New[any](5 * time.Minute)`) であり永続化されないため、プロセス再起動で解消する。データの破壊や外部への影響はない。
- AWS サービス名やリージョン名に `":"` は通常含まれないため、既定の操作で踏む可能性は低い。ただし自由入力欄に `":"` を入れれば意図せず踏める。

## 解決方法

`cacheKey` を、各要素を `url.QueryEscape` でエスケープしてから `strings.Join` で `":"` 連結する実装に変更した。

```go
func cacheKey(parts ...string) string {
	escaped := make([]string, len(parts))
	for i, p := range parts {
		escaped[i] = url.QueryEscape(p)
	}
	return strings.Join(escaped, ":")
}
```

`url.QueryEscape` は `":"` を `"%3A"` に、`"%"` を `"%25"` に変換する。エスケープ後の各要素には区切り文字である生の `":"` が残らないため、要素ごとに単射になり、連結結果も引数列に対して単射になる。

以下の性質を保つことを確認した。

- 空文字はエスケープしても空文字のままである。`cacheKey(..., "")` で末尾要素を空文字にして `cache.InvalidatePrefix` 向けの前方一致プレフィックスを組む用法 (`handlers_s3_object.go:202` / `handlers_gcp.go:270`) は維持される。キー生成側とプレフィックス生成側の双方が同じ `cacheKey` を通るため、エスケープ後も前方一致が成立する。
- キーの第 1 要素は全呼び出し箇所でリテラルの識別子文字列 (`"cost"` / `"s3-objects"` / `"gcp-gcs-objects"` 等) であり `":"` や `"%"` を含まない。`handlers_cache.go` の `viewOwnsCacheKey` が `strings.Cut(key, ":")` で先頭セグメントのみを見て view 単位の無効化対象を判定するロジックは影響を受けない。
- `cacheKey` の返り値を分割・パースして使っている呼び出し箇所は存在しない (64 箇所すべてが不透明なマップキーとしてのみ使用)。

あわせて `backend/internal/api/server_test.go` に以下 3 件のテストを追加した。

- `TestCacheKey` — エスケープの基本動作 (`":"` → `"%3A"`、`"%"` → `"%25"`、空要素の保持) をテーブル駆動で固定する。
- `TestCacheKeyDistinctForDifferentParts` — `":"` の位置がずれる 8 通りの引数列が互いに衝突しないことを検証する。
- `TestCacheKeyPrefixForInvalidate` — `InvalidatePrefix` 向けの末尾空文字プレフィックスが、対象キーには前方一致し、別バケット名・`":"` を含むバケット名・別リージョンのキーには一致しないことを検証する。

`url.PathEscape` (`":"` をエスケープしない) に差し替えるミューテーションで 5 件のテストが失敗することを実測し、テストに検出力があることを確認した。

CHANGES.md の `## develop` に `[FIX]` エントリとして記載済み。
