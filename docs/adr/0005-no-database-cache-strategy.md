# 0005. データベースを持たず、取得結果をメモリにキャッシュする

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-07-08

## 状況

thief が表示するデータの正は、各クラウドの API にある。
thief 自身が持つべき永続データは、認証のキャッシュ、料金のキャッシュ、Google Cloud のプロジェクト一覧、保存クエリ程度である。
一方で、一覧を開くたびにクラウドの API を呼ぶと、表示が遅く、API のレート制限にも当たりやすい。

## 決定

- データベース (RDB、ORM) を持たない。
- backend はリソースの取得結果をプロセスのメモリにキャッシュする。
  期限はリソースの一覧が 1 時間、リージョンの一覧と Google Cloud のプロジェクトの一覧が 24 時間、CloudFormation のスタックのイベントが 30 秒とする (`backend/internal/api/server.go` `cacheTTL`、`regionsCacheTTL`、`backend/internal/api/handlers_aws.go` `cfnEventsCacheTTL`)。
  メモリのキャッシュと一覧の 1 時間の期限は、最初の実装 (コミット 2aac22f、2026-07-08) からある。
  リージョンの一覧の 24 時間 (コミット 685a2d3、2026-07-08) と CloudFormation のイベントの 30 秒 (コミット 8cc249d、2026-07-17) は後から加わった。
- トップバーの Refresh は、`POST /api/cache/invalidate?view=<AppView>` で表示中のビューの backend のキャッシュを破棄し、その完了後に frontend の TanStack Query のキャッシュを無効にする (`frontend/src/lib/refreshView.ts`、issue 0079)。
- Cost Explorer とその予測、リージョン一覧、Google Cloud のプロジェクト一覧、DynamoDB の Query と Scan は、Refresh の破棄の対象から外す (`backend/internal/api/handlers_cache.go` `cacheInvalidateExcluded`、issue 0079)。
- frontend の `staleTime` は既定 60 秒とし、既定と異なる値が要るクエリだけ個別に指定する (`frontend/src/main.tsx`、issue 0080)。
  シークレットの値とオブジェクトのプレビューは `staleTime: 0` とする。
- 永続が要るものはファイルに置く (ADR 0024)。

## 検討した代替案

- データベースを持つ案を比べた記録は無い。
- issue 0080 は frontend のキャッシュについて 3 つの候補を挙げた。
  - 候補 1: `gcTime` の既定を引き上げる。
    計測できなかったため見送った。
  - 候補 2: `staleTime` の既定を 60 秒にする。
    挙動が変わらないため実施した。
  - 候補 3: リソース一覧の `staleTime` を backend の TTL (1 時間) に近づける。
    計測できなかったため見送った。

## 結果

- backend を再起動するとキャッシュは消える。
- キャッシュの整合 (破棄の範囲、キーの衝突、期限) が設計上の主な課題になる。
  キャッシュのキーは値を `QueryEscape` して組み立て、衝突を防ぐ。
- 最大 1 時間古い一覧が表示されうる。
  利用者は Refresh で取り直す。
- リージョンの一覧と Google Cloud のプロジェクトの一覧は 24 時間保持され、Refresh の破棄の対象外であるため、古い表示が残りうる。

## 根拠資料

- `docs/issues/closed/0079`、`docs/issues/closed/0080`
- コミット 2aac22f (2026-07-08)
- `backend/internal/api/server.go` `serveCached`、`backend/internal/api/handlers_cache.go` `handleCacheInvalidate`、`cacheInvalidateExcluded`
- `frontend/src/lib/refreshView.ts`、`frontend/src/main.tsx`
