# サイドバーの件数バッジ用オブザーバが共有クエリの queryFn を skipToken で上書きし SSO 期限切れ時に Missing queryFn になる

Created: 2026-09-18
Model: Claude Fable 5.1
Completed: 2026-09-18

## 症状

EC2 など AWS サービスの画面で SSO セッションが期限切れになると、本来は `SSOExpiredBanner` (再ログインボタン付き) が表示されるべきところ、代わりに `Missing queryFn: '["aws","ecr","<profile>","ap-northeast-1"]'` のようなエラーが `ErrorBanner` に表示され、再ログイン導線が出ない。

backend は 401 と `{"code":"SSO_TOKEN_EXPIRED"}` を返している (ログ例)。

```
WARN http method=GET path=/api/aws/profiles/<profile>/identity status=401 ... body="{\"error\":\"... the SSO session has expired or is invalid\",\"code\":\"SSO_TOKEN_EXPIRED\"}"
```

同じ profile / region のリソース一覧クエリ (例: `["aws","ecr","<profile>","ap-northeast-1"]`) の error が ApiError ではなく `Error: Missing queryFn` になるため、`isSSOExpiredError` が false になり `SSOExpiredBanner` が表示されない。

## 再現手順

1. `mise run backend:run` と `mise run frontend:run` を起動する。
2. SSO トークンを期限切れにする (`~/.aws/sso/cache/*.json` の `expiresAt` を過去にするなど)。
3. AWS プロファイルを選択してサービス (ecr など) の一覧を開く。
4. 期待: `SSOExpiredBanner` が表示され、再ログインできる。
5. 実際: `Missing queryFn: '["aws","ecr",...]'` が表示され、再ログイン導線が出ない。

自動テストでは、`Sidebar` (件数バッジ) と同じ queryKey のリソース一覧クエリを同居させ、`queryClient.invalidateQueries({ queryKey: ['aws'] })` を実行すると、リソース一覧クエリの error が `Missing queryFn: '["aws","ecr","test","ap-northeast-1"]'` になることを確認できる。

## 原因

`frontend/src/components/Sidebar.tsx` の `SvcItem` は、件数バッジを「他所で埋まったキャッシュを読むだけ」で表示するために、リソース一覧 (`frontend/src/api/queries.ts` の `useResources`) と同一の queryKey で `queryFn: skipToken` の読み取り専用オブザーバを作っている。

TanStack Query v5 では、`QueryObserver.setOptions` が `this.#currentQuery.setOptions(this.options)` を呼び、**同一 queryKey のクエリの options をオブザーバごとに上書きする**。`SvcItem` のオブザーバは `queryClient.defaultQueryOptions` により `enabled: false` と `queryFn: skipToken` を持つため、`Sidebar` が再描画される (例: `useRegions` の取得完了で `Sidebar` 全体が再描画される) と、共有クエリの `options.queryFn` が `skipToken` に、`options.enabled` が `false` になる。

この状態で `invalidateQueries` が `query.fetch()` をオプション無しで呼ぶと、クエリ自身の `options.queryFn` (`skipToken`) で取得しようとし、`ensureQueryFn` が `Missing queryFn` で reject する (`@tanstack/query-core` の `utils.js`)。

`query.js` の `fetch` には「`options.queryFn` が無いときに他オブザーバの options から補う」フォールバックがあるが、`skipToken` は truthy なため発動しない。`enabled: false` だけの旧実装 (issue 0048 以前、`GcpSidebar` は現在もこの形) ではフォールバックが効き、この問題は起きない。issue 0048 で dev 用の `No queryFn was passed` console.error を消すために `skipToken` へ変えたことが、この機能不具合を持ち込んだ。

SSO 期限切れが直接の引き金ではない。どのエラーでも、`Sidebar` の再描画後に `invalidateQueries(['aws'])` (Refresh、SSO ログイン成功、SSO ログアウト等) が走ればリソース一覧クエリが `Missing queryFn` になる。SSO 期限切れ時にこれが起きると、本来表示すべき再ログイン導線が隠れる。

## 修正方針

`SvcItem` が `QueryObserver` を作らないようにする。件数バッジは `useQuery` をやめ、`queryClient` のクエリキャッシュを `useSyncExternalStore` で直接購読する読み取り専用フック `useCachedQueryData` に置き換える。

- `frontend/src/hooks/useCachedQueryData.ts` を新設する。`queryKey` から `hashKey` を求め、`queryCache.subscribe` でその hash のイベントだけを `onStoreChange` に流し、`queryCache.get(hash)?.state.data` を snapshot として返す。
- `SvcItem` は `useCachedQueryData<unknown[]>(['aws', svc, profile, region])` を使う。`skipToken` と `useQuery` の import を削除する。

これにより、件数バッジは従来どおり他所で埋まったキャッシュを購読して表示し、fetch も console.error も発生させない。共有クエリの options には一切触れないため、`invalidateQueries` は本来の queryFn で再取得し、SSO 期限切れは 401 `SSO_TOKEN_EXPIRED` として届き `SSOExpiredBanner` が表示される。

### 採らなかった案

- **`enabled: false` + queryFn なしに戻す (issue 0048 の巻き戻し)**。機能的には正しい (fetch フォールバックが効く) が、issue 0048 で消した dev 用 console.error が再発する。
- **`skipToken` を渡しつつ共有しない別 queryKey にする**。バッジが実データを参照できず、件数表示が壊れる。
- **TanStack Query のバージョンアップで解消する**。手元の 5.102.8 と最新の 5.103.1 はいずれもフォールバックが `!this.options.queryFn` のままで、解消しない。

## 完了条件

- `useCachedQueryData` が、同じ queryKey の実クエリの options (`queryFn` / `enabled`) を変更しないことを検証するテストがある。
- `useCachedQueryData` が、実クエリの取得結果の変化に追従して再描画されることを検証するテストがある。
- `Sidebar` の件数バッジが、他所で埋まったキャッシュを表示し続ける (既存テストが通る)。
- `Sidebar` と実クエリを同居させ、`invalidateQueries(['aws'])` 後に実クエリが `Missing queryFn` にならず SSO 期限切れ (401 `SSO_TOKEN_EXPIRED`) を返すことを検証する回帰テストがある。
- `mise run check` が通る。

## 関連

- `docs/issues/closed/0048-bug-sidebar-svcitem-no-queryfn-console-error.md`: `SvcItem` に `queryFn: skipToken` を導入した issue。本不具合の混入元。
- `docs/issues/closed/0078-refactor-dedup-sso-query-param-cell-styles.md`: `isSSOExpiredError` の共通化。本 issue はその判定に正しい ApiError が届かなくなる問題。
- `docs/issues/closed/0105-bug-cost-handlers-sso-expired-mapping.md`: 別経路で `SSOExpiredBanner` が出ない問題を直した issue。

## 解決方法

### 変更内容

- `frontend/src/hooks/useCachedQueryData.ts` を新設した。`queryKey` を `hashKey` でハッシュ化し、`queryClient.getQueryCache().subscribe` でその hash のイベントだけを `onStoreChange` に流し、`queryCache.get(hash)?.state.data` を `useSyncExternalStore` の snapshot として返す読み取り専用フックである。`QueryObserver` を作らないため、共有クエリの options を一切変更しない。
- `frontend/src/components/Sidebar.tsx` の `SvcItem` を、`useQuery({ queryKey, queryFn: skipToken })` から `useCachedQueryData<unknown[]>(['aws', svc, profile, region])` に置き換えた。`skipToken` と `useQuery` の import を削除し、冒頭コメントを実装に合わせて更新した。件数バッジの表示 (フェッチしない、他所で埋まったキャッシュに追従する) は変わらない。

### 完了条件の検証

- `useCachedQueryData` が共有クエリの options を変更しない: `frontend/src/hooks/useCachedQueryData.test.tsx` に「同じ queryKey の実クエリの options を変更しない」を追加した。`fetchQuery` で実クエリを作り、フックを render した後も `query.options.queryFn` が実クエリの関数のままであることを検証する。
- データ変化への追従: 同テストの「キャッシュ済みデータを返し、setQueryData の更新に追従する」で、初回 undefined → `setQueryData` 後に値が反映されることを検証する。
- 件数バッジの既存動作: `Sidebar.test.tsx` の「他所で埋まったキャッシュを読み取ってバッジ件数に反映し、自身では fetch しない」が無変更で通る。
- `Missing queryFn` にならず SSO 期限切れが届く回帰テスト: `Sidebar.test.tsx` に「Sidebar とリソース一覧クエリの同居」を追加した。`Sidebar` と同じ queryKey の `useResources` を同居させ、401 `SSO_TOKEN_EXPIRED` を返す fetch で、初回取得後と `invalidateQueries(['aws'])` 後の双方でリソース一覧のエラーコードが `SSO_TOKEN_EXPIRED` のままで、`Missing queryFn` を含まないことを検証する。
- `mise run check` の通過: `mise run frontend:lint` (eslint 0 エラー / 9 警告、警告はベースラインと同数。tsc 0 エラー)、`mise run frontend:test` (97 ファイル / 1023 件、ベースラインからの新たな失敗なし)、`mise run frontend:fmt` が通ることを確認した。backend は変更していない。

### 再現確認

修正前の `SvcItem` (`queryFn: skipToken`) で上記の回帰テストを実行すると 3 回連続で失敗し、リソース一覧のエラーが `Missing queryFn: '["aws","ecr","test","ap-northeast-1"]'` となることを確認した (症状の再現)。修正後は同テストが通る (非再現)。

### 方針からの乖離

「## 修正方針」からの乖離は無い。採らなかった案 (enabled: false + queryFn なしへの巻き戻し、別 queryKey 化、TanStack Query のバージョンアップ) も実装しなかった。なお、共有クエリの `enabled` を検証するアサーションは、TanStack Query の型 (`QueryOptions` に `enabled` が無い) に合わせて `queryFn` の検証に絞った。`enabled` が `false` に上書きされる問題自体は、`QueryObserver` を作らない本修正で解消している。

