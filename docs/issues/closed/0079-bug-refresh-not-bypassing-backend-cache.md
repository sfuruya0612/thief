# TopBar の Refresh が backend のキャッシュを貫通せず古いデータが表示され続けるのを修正する

Created: 2026-07-25
Completed: 2026-07-26
Model: Claude Fable 5

## 症状

TopBar の Refresh ボタンを押しても表示データが最新にならず、最大 1 時間前のキャッシュが表示され続ける。
docs/issues/0080 の調査で見つかった問題を切り出したもの (TODO.md 由来ではない)。

## 再現手順

1. thief の AWS ビューで EC2 の一覧を表示する (backend のキャッシュが作られる)。
2. AWS 側でリソースの状態を変える (例: EC2 インスタンスを停止する)。
3. TopBar の Refresh を押す。
4. 期待: 変更後の状態が表示される。実際: 変更前の表示のままになる。ブラウザ開発者ツールの Network タブで、再取得リクエストのレスポンスヘッダが `X-Cache-Status: HIT` を返す (キャッシュ TTL の 1 時間以内)。

## 原因

`frontend/src/App.tsx` の `handleRefresh` は、表示中の `AppView` に対応する queryKey を無効化するだけで、backend に `refresh=true` を渡していない。

```tsx
const handleRefresh = useCallback(() => {
  void queryClient.invalidateQueries({ queryKey: [view] });
  // BigQuery のクエリキーは歴史的経緯で 'gcp' ではなく 'bigquery' 始まりのため合わせて更新する
  if (view === 'gcp') {
    void queryClient.invalidateQueries({ queryKey: ['bigquery'] });
  }
}, [queryClient, view]);
```

backend のリソース一覧キャッシュの TTL は 1 時間 (`backend/internal/api/server.go` の `cacheTTL`) であり、`?refresh=true` によるキャッシュ迂回の仕組みは `serveCached` 経由の全キャッシュに配線済みだが、frontend でこれを使うのは Pricing と GCP プロジェクト一覧の 2 経路だけになっている (`getResources` は `opts.refresh` を受け取れるが `useResources` が渡していない)。
このため Refresh 起因の再取得は backend のキャッシュ HIT を受け取るだけになる。

docs/issues/0080 で frontend のキャッシュ保持を延ばすとこの問題はさらに悪化するため、先に修正する。

## 修正方針

- backend に `POST /api/cache/invalidate?view=<view>` を新設する。`view` は frontend の `AppView` と同じ `aws` / `gcp` / `datadog` / `tidb` の 4 値で、それ以外は 400。
- 破棄対象はプレフィックスの列挙表ではなく、振り分け規則で決める (新サービス追加時に表の追記漏れで Refresh が効かなくなる事態を避ける)。
  - `gcp` はキー先頭 `gcp-` と `bq-` (BigQuery の queryKey は frontend でも歴史的経緯で `bigquery` 始まりであり、backend のキーは `bq-` 始まり)、`datadog` は `dd-`、`tidb` は `tidb-`。`aws` は「上記のどれにも該当せず、除外集合にも含まれないすべてのキー」。
  - `handlers_aws.go` の SSO セッション確認が使う `sso` キーは除外しない。`view=aws` の破棄に巻き込まれるが、次のアクセスで再評価されるだけで課金も無く、SSO 状態の表示更新にはむしろ望ましい (docs/issues/closed/0030 と同じ領域)。
  - 除外集合: `cost` / `cost-forecast` (Cost Explorer API はリクエスト課金で、`useCost` の queryKey は `['aws', ...]` に含まれるため、除外しないと Refresh のたびに実課金が発生する)、`regions` / `gcp-projects` (24 時間 TTL で保持する明示的な設計があり、GCP プロジェクトには専用の手動更新 mutation が別にある)、`dynamo-items` (Query / Scan の再実行は RCU を消費する。再検索はクエリパネルの操作で明示的に行える)。`dynamo-items` を除外する代償として、同一条件の Query / Scan 結果は Refresh では取り直せず、クエリパネルからの再実行だけが取り直しの手段になる。
  - 実現手段: 振り分け規則は純関数 `viewOwnsCacheKey(view, key string) bool` にする。キーの第 1 セグメント (先頭から最初の `:` まで) で判定し、除外集合は完全一致、provider プレフィックス (`gcp-` / `bq-` / `dd-` / `tidb-`) は前方一致、`aws` はどれにも該当しない残り全部とする。`cache` パッケージの無効化系メソッドは `Invalidate` / `InvalidatePrefix` のみで全消去や述語指定は無いため、`InvalidateFunc(pred func(key string) bool)` を追加する。述語は write lock 下で評価されるため、ロックを取らない純関数に限る (この契約を godoc に書く)。
  - 振り分け規則の既知の限界: 新 provider 追加時は規則に分岐を足すまでそのキーは `aws` に分類される。既知キーの分類を固定するテーブル駆動テスト (完了条件参照) が追記の手掛かりになる。
  - 却下案 1: 全エントリの破棄。上記の課金と 24 時間キャッシュを巻き込むため却下。
  - 却下案 2: view ごとにキャッシュキーのプレフィックスを列挙する対応表。キャッシュキーの第 1 セグメントは 2026-07-25 調査時点で全 59 種 (provider プレフィックス 14 種 + その他 45 種) あり、`view=aws` の対象は除外集合 4 種を引いた 41 種になる。`add-aws-service` スキルの手順にも表への追記が無いため、対応表では新サービスで漏れるサイレントな劣化が構造的に入る。振り分け規則ならキーの命名に従って自動的に対象になる。
  - 却下案 3: 全 query に `refresh=true` を配線する。AWS 一覧は `useResources` 1 本だが、Drawer のサブタブ系フックが多数あり、`getGcpResources` には refresh 引数自体が無い。Refresh 押下時の 1 回だけ `refresh=true` にする状態管理も要るため、backend 側の 1 エンドポイントで完結する方式を採る。
  - 却下案 4: queryKey に更新カウンタを含める。押すたびに別キーのキャッシュが積まれ、TanStack Query 側のメモリを浪費するため却下。
- メソッドは POST、成功時は 204 を返す (`handleSSMPut` / `handleSecretsPut` と同形)。`view` の presence 検証は docs/issues/0078 の `requireQueryParam` を使う。`/api/cache/` は provider 単位でない新しい第 1 階層になる (既存の非 provider は `/api/health` / `/api/snippets` / `/api/bigquery`)。
- frontend は `api/endpoints.ts` に `postCacheInvalidate(view)`、`api/queries.ts` に `useCacheInvalidate` (useMutation) を追加し、`handleRefresh` の処理列を `frontend/src/lib/refreshView.ts` の関数に切り出す。破棄 POST の完了を待ってから `invalidateQueries` を実行する (先に invalidate すると再取得が破棄前の backend に届きうるため)。POST が失敗した場合でも `invalidateQueries` は実行する (backend 停止時に Refresh が完全な no-op になるのを避ける)。実行中の再入は無視し、TopBar のボタンは実行中は無効化する。`view === 'gcp'` のとき `['bigquery']` も無効化する既存分岐は維持する。
- 既知の限界: `cache.Load` は in-flight の取得に singleflight で合流し、取得完了時に値をキャッシュへ書き戻す。破棄の直前に始まった取得は破棄後に古い値を書き戻すため、破棄をすり抜けるレースがある。これは既存の `refresh=true` 経路と同等の限界であり、本 issue では扱わない。
- 破棄エンドポイントは AWS API を呼ばないため、SSO 期限切れ (401) の経路とは無関係。追加の AWS API 呼び出し、権限は不要。
- Pricing のディスクキャッシュ (`/tmp/thief/price/v2`) は破棄の対象外とする。Pricing は `refresh=true` の配線が既にあり、Pricing パネル側の操作で取り直せる。
- 破棄は view 単位で、全 profile / region のエントリをまとめて消す。表示中の profile / region に絞る案は、キーの第 2 セグメント以降の構造に依存する割に、消しすぎの実害が次アクセス時の再取得だけであるため採らない。
- AGENTS.md の「トップバーの Refresh ボタンは現在表示中の `AppView` に対応する `queryKey` のみを invalidate する」という記述を、backend キャッシュの破棄を含む挙動に更新する。あわせて、AGENTS.md の `AppView` の値域の記述を、`bigquery` を含む記述から実装 (`aws` / `gcp` / `datadog` / `tidb`) に合わせて直す。

## 完了条件

- `POST /api/cache/invalidate?view=aws` が振り分け規則に該当するエントリを破棄して 204 を返し、除外集合 (`cost` / `cost-forecast` / `regions` / `gcp-projects` / `dynamo-items`) のエントリは破棄されない。`gcp` で `gcp-` と `bq-` の両方が破棄される。4 値以外の `view` と欠落に 400 を返す (presence 検証は docs/issues/0078 の `requireQueryParam` を使い、直書きを増やさない)。以上を検証するハンドラのテストがある。
- `viewOwnsCacheKey` にテーブル駆動テストがあり、実装時点の既知のキャッシュキー第 1 セグメント全種の分類期待値を固定している。全種は `grep -roh 'cacheKey("[a-z0-9-]*"' backend/ | sort -u` で抽出する (2026-07-25 調査時点で 59 種、却下案 2 の内訳と同数)。テストのケース数が実装時点の抽出結果と一致することを確認して本 issue に記録する。`view=aws` の破棄後に provider プレフィックスのエントリが残ることも検証している。
- 破棄 POST 後の同一 GET が `X-Cache-Status: MISS` を返すことをハンドラのテストで検証している (既存の `server_test.go` の HIT / MISS 検証と同型)。実環境での確認結果は任意で記録する。
- `cache` パッケージに `InvalidateFunc` が追加され、テストがある。
- `lib/refreshView.ts` のテストがあり、(a) 破棄 POST のレスポンス受領後に `invalidateQueries` が実行される、(b) POST が reject されても `invalidateQueries` が実行される、(c) 実行中の再入が no-op になる、(d) `view === 'gcp'` で `['bigquery']` も無効化される、の 4 点を検証している。
- singleflight のレースと新 provider 追加時の分類漏れ (既知の限界)、および DELETE の CORS 論点 (未確定論点) は本 issue では扱わない。
- AGENTS.md の Refresh に関する記述と `AppView` の値域の記述が更新されている。
- `CHANGES.md` の `## develop` に `[FIX]` エントリと担当者行が記載されている。
- `mise run check` が通過する。

## 未確定論点

- CORS ミドルウェア (`backend/internal/api/middleware.go`) の `Access-Control-Allow-Methods` は `GET, POST, OPTIONS` で、DELETE を含まない。
- 一方で DELETE のルートは 3 本ある (Athena クエリの停止、BigQuery ジョブのキャンセル、スニペットの削除)。
- frontend の `api/client.ts` のベース URL は `http://127.0.0.1:8089` 固定で、dev サーバ (ポート 8088) からの呼び出しは常にクロスオリジンになる。
- クロスオリジンの DELETE は preflight の許可メソッドに DELETE が要るため、これらのルートは機能していない可能性が高い (少なくともスニペットの削除は、`SnippetDropdown` の削除操作から `views/AthenaView.tsx` / `views/nonaws/BigQueryView.tsx` の `onDelete={snippets.remove}` を経由して `useDeleteSnippet` に配線済みで、呼ばれる経路がある)。
- この論点は次バッチで bug issue として起票する。
- 本 issue のエンドポイントは POST のため、この論点の影響を受けない。

## 解決方法

修正方針の通りに実装した。

- backend
  - `backend/internal/cache/cache.go` に `InvalidateFunc(pred func(key string) bool)` を追加した。述語は write lock 下で評価されるためロックを取らない純関数に限る契約を godoc に明記し、`cache_test.go` にテーブル駆動テスト (`TestCacheInvalidateFunc`) を追加した。
  - `backend/internal/api/handlers_cache.go` を新設し、振り分け規則の純関数 `viewOwnsCacheKey(view, key)` (第 1 セグメント判定、除外集合は完全一致、`gcp-`/`bq-`/`dd-`/`tidb-` は前方一致、`aws` は残り全部) と `handleCacheInvalidate` (presence 検証は `requireQueryParam`、4 値以外は 400、成功時 204) を実装した。ルートは `POST /api/cache/invalidate` として `routes.go` に登録した。
  - `backend/internal/api/handlers_cache_test.go` に以下を追加した。
    - `TestViewOwnsCacheKey`: 既知のキャッシュキー第 1 セグメント全種の分類を固定するテーブル駆動テスト。`grep -roh 'cacheKey("[a-z0-9-]*"' backend/ | sort -u` の 2026-07-26 時点の抽出結果は **60 種** で、issue 作成時 (2026-07-25) の 59 種から `rds-cluster-parameters` が増えている (docs/issues/0076・0077 の実装で追加されたもの)。テストはケース数 60 との一致も固定している。
    - `TestHandleCacheInvalidate`: `view=aws` が振り分け対象のみ破棄し除外集合 (`cost` / `cost-forecast` / `regions` / `gcp-projects` / `dynamo-items`) と他 provider のエントリを残すこと、`view=gcp` が `gcp-` と `bq-` の両方を破棄すること、`view` の欠落と 4 値以外が 400 になること、破棄 POST 後の同一キーの `serveCached` GET が `X-Cache-Status: MISS` に戻ること (MISS → HIT → 破棄 → MISS、loader 呼び出し 2 回) を検証する。
- frontend
  - `api/endpoints.ts` に `postCacheInvalidate(view)`、`api/queries.ts` に `useCacheInvalidate` (useMutation) を追加した。
  - `lib/refreshView.ts` を新設し、Refresh の処理列 (破棄 POST の完了を待ってから `invalidateQueries`、POST 失敗時も `invalidateQueries` は実行、実行中の再入は no-op、`view === 'gcp'` は `['bigquery']` も無効化) を `createViewRefresher(deps)` に切り出した。`lib/refreshView.test.ts` で完了条件の 4 点 (a)〜(d) を検証している。
  - `App.tsx` の `handleRefresh` を `createViewRefresher` 経由に置き換え、実行中は `TopBar` の Refresh ボタンを `disabled` にした (`refreshing` prop を追加)。
- ドキュメント
  - AGENTS.md の Refresh の記述を backend キャッシュ破棄を含む挙動に更新し、`AppView` の値域を `aws`/`gcp`/`datadog`/`tidb` に修正した。
  - CHANGES.md の `## develop` に `[FIX]` エントリと担当者行を追加した。
- `mise run check` 通過を確認した。実環境での MISS 確認は未実施 (完了条件上は任意)。

## 関連

- docs/issues/0080 (frontend のキャッシュ設定): 本 issue の発見元。docs/issues/0080 の候補 3 (staleTime の引き上げ) は本 issue の完了が前提で、番号順 (本 issue → docs/issues/0080) に実装する。
- docs/issues/0078 (重複の共通化): `view` の presence 検証に `requireQueryParam` を使う。
- docs/issues/0077 (RDS クラスターパラメータグループ名): 本 issue が close されると、キャッシュ TTL 内の stale なグループ名を Refresh で即時に取り直せるようになる。
