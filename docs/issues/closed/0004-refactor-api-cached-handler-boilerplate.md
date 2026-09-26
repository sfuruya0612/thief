# API ハンドラのキャッシュ応答ボイラープレートを共通化する

Created: 2026-07-16
Completed: 2026-07-16
Model: Claude Fable 5 claude-fable-5

## 根拠

`backend/internal/api/` の HTTP ハンドラ約 44 箇所が、以下の同一パターンをコピペしている。

```go
entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
    return awsinternal.ListXxxResources(r.Context(), profile, region)
})
if err != nil {
    writeAWSError(w, err)
    return
}
writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
writeJSON(w, entry.Value)
```

該当箇所:

- `handlers_aws.go`: handleEC2 / handleRDS / handleElastiCache / handleLambda / handleECS / handleECSServices / handleECSTasks / handleECSContainers / handleECR / handleECRImages / handleS3 / handleIAM / handleSSO / handleSSMList / handleSecretsList / handleCFN / handleKinesis / handleCloudFront / handleELB / handleELBListeners / handleELBRules / handleELBTargetGroups / handleELBTargetHealth / handleDynamo / handleAPIGW / handleNATGW / handleSQS / handleWAF
- `handlers_gcp.go`: handleGCPProjects / handleGCPCloudRun / handleGCPGCS / handleGCPGCSObjects / handleGCPIAM / handleGCPServiceAccounts
- `handlers_cost.go`: handleCost / handleCostForecast
- `handlers_datadog.go`: handleDatadogHistorical / handleDatadogEstimated
- `handlers_tidb.go`: handleTiDBProjects / handleTiDBClusters / handleTiDBCost
- `handlers_bigquery.go`: handleBQDatasets / handleBQTables / handleBQSchema
- `handlers_dynamo.go`: handleDynamoSchema / handleDynamoItems
- `handlers_s3_object.go`: handleS3Objects
- `handlers_regions.go`: handleRegions

新しいハンドラを追加するたびに同じ 8 行が複製され、エラー writer の選択ミス (writeAWSError と writeInternalError の取り違え) を招きやすい。

## 対応方針

`Server` にヘルパを 1 つ追加し、各ハンドラはキー組み立てと loader の指定のみを行う。

```go
func (s *Server) serveCached(w http.ResponseWriter, r *http.Request,
    key string, ttl time.Duration, onErr func(http.ResponseWriter, error),
    load func() (any, error))
```

制約:

- エラー writer は必ず呼び出し側から渡す。AWS リソース系は `writeAWSError` (SSO 期限切れで 401 SSO_TOKEN_EXPIRED)、cost / gcp / datadog / tidb / bq は `writeInternalError` (常に 500) を使い分けており、統一するとエラーレスポンスの JSON が変わるため。
- TTL も引数で渡す (`cacheTTL` と `regionsCacheTTL` の使い分けを保持する)。

## API レスポンス / 画面表示への影響

なし。キャッシュキー・TTL・エラー writer・レスポンス生成ロジックをすべて保持する純粋な共通化。

## 解決方法

- `backend/internal/api/server.go`: `Server.serveCached(w, r, key, ttl, onErr, load)` を追加した。resourceCache.Load → エラー時 onErr → writeCacheHeaders → writeJSON の共通フローを 1 箇所に集約する。
- `backend/internal/api/errors.go`: `writeInternalFromError(w, err)` を追加した。string 引数の `writeInternalError` を error 引数の serveCached エラー writer として使うためのアダプタ。
- 対象 44 ハンドラ (handlers_aws.go / handlers_gcp.go / handlers_cost.go / handlers_datadog.go / handlers_tidb.go / handlers_bigquery.go / handlers_dynamo.go / handlers_s3_object.go / handlers_regions.go) を serveCached 呼び出しに置換した。エラー writer は AWS リソース系が `writeAWSError`、それ以外が `writeInternalFromError` の使い分けを保持し、TTL も従来どおり (regions / gcp-projects のみ regionsCacheTTL)。
- `backend/internal/api/server_test.go` を新規追加し、MISS → HIT → refresh=true での再ロード、loader エラー時に onErr が呼ばれキャッシュヘッダが付かないことを検証した。
- `mise run check` 全通過を確認した。
