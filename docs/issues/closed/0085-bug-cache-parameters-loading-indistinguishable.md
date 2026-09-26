# ElastiCache の Parameters タブで一覧キャッシュ未取得時に「No parameter group.」と誤表示されるのを修正する

Created: 2026-07-26
Completed: 2026-07-26
Model: Claude Fable 5

## 症状

ElastiCache クラスタの Drawer の Parameters タブ (`DrawerCacheParameters`) で、パラメータグループ名の解決元である一覧 query (`useResources('cache', ...)`) の data が未取得の間、「No parameter group.」が表示される。パラメータグループを本当に持たないクラスタの表示と見分けが付かず、一覧キャッシュの取得が終わると表示が突然パラメータ一覧に切り替わる。
docs/issues/0076 が RDS の Parameters タブで直した「未取得と 0 件の区別」と同型の問題。docs/issues/closed/0082 の完了条件で次バッチでの起票を予告したもの。

## 再現手順

1. `mise run backend:run` と `mise run frontend:run` を起動する。
2. ElastiCache の一覧を表示してクラスタを選択し、Drawer の Parameters タブを開く。
3. トップバーの Refresh などで一覧キャッシュを破棄した直後 (一覧 query が再取得中) に Parameters タブを見る。
4. 期待: グループ名の解決中であることが分かるローディング表示。実際: 「No parameter group.」が表示される。

## 原因

`DrawerCacheParameters` が `useResources` から `data` のみを取り出し、`isLoading` を見ずに `group` の空文字判定 (`!group`) で「No parameter group.」に分岐している。`data` が undefined の間 (未取得・取得中・取得エラー) も `group` は `''` になるため、「グループ無し」と「未解決」が同じ表示になる。

```tsx
const { data } = useResources<CacheRaw, CacheRow>('cache', profile, region, cacheFromRaw);
const group = useMemo(
  () => data?.find((r) => r.name === cluster)?.parameterGroup ?? '',
  [data, cluster],
);
```

## 修正方針 (案)

- `useResources` の `isLoading` (または `data === undefined`) を参照し、一覧未取得の間は「No parameter group.」ではなく `DrawerLoading` を表示する。
- 一覧 query のエラー表示は docs/issues/closed/0082 の方針どおり一覧ビュー側 (`SSOExpiredBanner` / `ErrorBanner`) の責務とし、本 issue では扱わない (未取得時の誤表示の解消のみ)。
- docs/issues/0076 が RDS 側 (`DrawerRDSInstanceParameters` / `DrawerRDSClusterParameters`) で確立したパターンがあればそれに揃える。

## 完了条件

- 一覧キャッシュ未取得の間に「No parameter group.」が表示されない (ローディング表示になる)。
- パラメータグループを持たないクラスタでは従来どおり「No parameter group.」が表示される。
- 分岐を検証するテストがある (`DrawerCacheParameters.test.tsx` を新設する)。
- `CHANGES.md` の `## develop` に `[FIX]` エントリと担当者行が記載されている。
- `mise run check` が通過する。

## 解決方法

- `DrawerCacheParameters` で `useResources` の `isLoading` を `listLoading` として取り出し、グループ名が未解決 (`!group`) かつ一覧取得中は「No parameter group.」ではなく `DrawerLoading` を表示するようにした。一覧取得完了後にグループ名が空のままの場合のみ従来どおり「No parameter group.」を表示する。
- 一覧 query のエラー時 (`listLoading` が false で `data` が undefined) は従来どおり「No parameter group.」になるが、一覧のエラー表示は一覧ビュー側 (`SSOExpiredBanner` / `ErrorBanner`) の責務とする docs/issues/closed/0082 の方針のとおり本 issue では扱わない。
- `DrawerCacheParameters.test.tsx` を新設し、(1) 一覧未解決の間は Loading… が表示され「No parameter group.」が出ないこと、(2) パラメータグループを持たないクラスタでは「No parameter group.」が表示されること、(3) グループが解決できたらグループ名見出しとパラメータ一覧が表示されることを検証した。
- `CHANGES.md` の `## develop` に `[FIX]` エントリと担当者行を追記した。
- `mise run check` 通過 (frontend 69 ファイル 585 テスト)。

## 関連

- docs/issues/closed/0082 (Drawer サブタブのエラー表示): 本 issue の起票元。取得エラーの表示はそちらで対応済み。
- docs/issues/closed/0076 (RDS の Parameters タブ分割): 同型の「未取得と 0 件の区別」を RDS 側で解決した先行例。
