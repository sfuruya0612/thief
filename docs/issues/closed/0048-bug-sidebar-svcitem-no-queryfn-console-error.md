# サイドバーの SvcItem が queryFn なし useQuery で console.error を出し続ける

Created: 2026-07-18
Completed: 2026-07-19
Model: Claude Sonnet 5

## 症状

開発サーバー (`mise run frontend:run`) でアプリを開くと、サイドバーを描画するたびにブラウザコンソールへ次の形の `console.error` が大量に出力される (SERVICES 配列の全サービス分、React の `<StrictMode>` により通常時は 2 重に出力される)。

```
[["aws","ec2","CT Audit","ap-northeast-1"]]: No queryFn was passed as an option, and no default queryFn was found. The queryFn parameter is only optional when using a default queryFn. More info here: https://tanstack.com/query/latest/docs/framework/react/guides/default-query-function
```

`ec2` 以外にも `lambda` / `ecr` / `ecs` / `s3` / `rds` / `dynamo` / `cache` / `elb` / `cloudfront` / `apigw` / `natgw` / `athena` / `kinesis` / `sqs` / `iam` / `waf` / `secrets` / `ssm` / `cfn` / `cloudwatchlogs` / `costexplorer` / `pricing` の `lib/serviceMeta.ts` の `SERVICES` 全件について同様に出力される。

機能自体 (バッジ件数表示、リソース一覧取得) は正常に動作しており、実害はない。本番ビルド (`NODE_ENV==='production'`) ではこの警告は出ない (TanStack Query 側の dev 専用チェックのため)。

## 再現手順

1. `mise run backend:run` と `mise run frontend:run` を起動する。
2. ブラウザ (または Playwright) で `http://localhost:8088/` を開く。
3. AWS プロファイルのセッションタブを開いてサイドバーを表示する。
4. DevTools のコンソールに `No queryFn was passed as an option` という `console.error` が SERVICES の件数分 (概ね 2 重で) 出力されることを確認する。

## 原因

`frontend/src/components/Sidebar.tsx` の `SvcItem` コンポーネントが次の実装になっている。

```tsx
function SvcItem({ svc, profile, region, active, onService }: SvcItemProps) {
  const meta = SERVICES.find((s) => s.key === svc);
  // fetch は発生させず、他所で埋まったキャッシュを読み取るだけの観測用クエリ
  const { data } = useQuery<unknown[]>({
    queryKey: ['aws', svc, profile, region],
    enabled: false,
  });
  ...
}
```

`AccountView.tsx` の `Sidebar` は `lib/serviceMeta.ts` の `SERVICES` を 1 件ずつ `SvcItem` としてレンダーし、各 `SvcItem` がこの `queryFn` なしの `useQuery` を呼ぶ。狙いは「バッジ件数は選択中サービスのみ実フェッチし、他の未選択サービスは `ServicePanel` (`api/queries.ts` の `useResources`) 側が同じ `queryKey` で埋めたキャッシュを読むだけに留める」という設計 (fetch は起こさない観測専用オブザーバ)。この設計自体は壊れていない。

`node_modules/@tanstack/react-query/build/modern/useBaseQuery.js` に次の dev 専用チェックがあり、`enabled` の値に関係なく `useQuery` を呼ぶたびに無条件で実行される。

```js
if (!defaultedOptions.queryFn) {
  console.error(`[${defaultedOptions.queryHash}]: No queryFn was passed as an option, ...`);
}
```

`enabled: false` は `isActive()`/`fetch()` の判定 (フェッチ実行そのもの) にのみ効き、この警告は render 時点でそれより先に発火するため抑制されない。

## 根拠

`git blame -L 103,120 frontend/src/components/Sidebar.tsx` により、当該実装はコミット `2aac22fb` (2026-07-08、モノレポ移行時点) から変更されておらず、`ec2` 等の既存サービスで以前から発生していたことを確認した。
issue 0045 (AWS Pricing 画面) で `SERVICES` に `pricing` を追加したことで対象サービスが 22 件から 23 件に増えたが、警告の発生源はそれ以前から存在する既存バグであり、`pricing` 追加が原因ではない。

## 対応方針

`@tanstack/react-query` v5 が提供する `skipToken` を `queryFn` に渡す形 (`queryFn: enabled ? realQueryFn : skipToken`) に書き換えると、`queryFn` 自体は存在する状態になり、この dev 専用警告を出さずに「フェッチしない」動作を維持できる。
`SvcItem` は元々 `queryFn` を持たない設計 (バッジ用の読み取り専用オブザーバ) のため、`skipToken` を渡す形に修正するのが対応の候補になる。

## 解決方法

`frontend/src/components/Sidebar.tsx` の `SvcItem` で、`useQuery` の `enabled: false` を
`queryFn: skipToken` に置き換えた (`@tanstack/react-query` から `skipToken` を import)。
`skipToken` は「フェッチしない」ことを示す専用の値で、これを `queryFn` に渡すこと自体で
enabled 相当の抑止が効くため `enabled` オプションは不要になった。ファイル冒頭のコメント
(`enabled: false の読み取り専用オブザーバ...`) も実装に合わせて更新した。

### 実施した検証

- `frontend/src/components/Sidebar.test.tsx` に以下の 2 テストを追加した。
  - `console.error` をスパイし、`SvcItem` のレンダーで `No queryFn was passed` が 1 件も
    出力されないことを確認するテスト。
  - `QueryClient.setQueryData` で `['aws', 'ec2', profile, region]` に事前にデータを
    入れておき、`SvcItem` がそれをバッジ件数として表示すること (キャッシュ読み取りの
    既存動作が壊れていないこと)、かつ `ec2` 向けの実 fetch が発生しないこと
    (`queryFn: skipToken` でも観測専用の性質が保たれていること) を確認するテスト。
- 修正前の実装に対して同じテストを実行し、`No queryFn was passed` が (SERVICES 件数分の)
  23 件検出されて意図通り FAIL することを確認した (テストがこの不具合を検出できることの
  裏付け)。修正後は全 4 テスト (既存 2 件 + 追加 2 件) が PASS する。
- `mise run check` (backend/frontend の fmt + lint + test、471 tests) が全て通過することを
  確認した。
- issue の再現手順通り、`mise run frontend:run` で dev server を起動し、Playwright
  (headless Chrome) で実際に `http://localhost:8088/` を開いて検証した。認証済み AWS
  プロファイル (SSO 有効) の状態でサイドバーが 23 件の nav-item (SERVICES 全件) を描画し、
  収集した `console.error` に `No queryFn was passed` が 0 件であることを確認した (検出された
  2 件のエラーはいずれも別 API の 400 Bad Request で本 issue とは無関係)。スクリーンショットで
  サイドバー・バッジ表示が正常であることも目視確認した。
