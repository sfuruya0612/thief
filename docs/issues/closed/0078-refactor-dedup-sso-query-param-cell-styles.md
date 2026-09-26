# SSO 期限切れ判定、必須クエリパラメータ検証、列セル部品の重複を共通化する

Created: 2026-07-25
Completed: 2026-07-26
Model: Claude Fable 5

## 背景

TODO.md の次の項目のうち、「処理の共通化」の部分に対応する。

> Frontend, Backend の処理の最適化をしたい。
> - 具体的には処理の共通化、frontend はキャッシュの利用頻度をあげ API コールによるユーザの体験向上、backend は処理速度の向上を目指す

frontend のキャッシュ利用の見直しは docs/issues/0080、backend の処理速度の向上は docs/issues/0081 で扱う。
TODO 由来の機能 issue (docs/issues/0074〜0077) を先に出すため、本 issue は番号順でその後に置く。

現状、次の重複がある。

- SSO 期限切れ判定: `error instanceof ApiError && error.code === 'SSO_TOKEN_EXPIRED'` という判定が frontend の 5 箇所に散っている。`views/AthenaView.tsx` と `views/CloudWatchLogsView.tsx` はバイト単位で同一の関数 `isSSOExpired` を定義し、`views/AccountView.tsx` / `views/CostExplorerPanel.tsx` / `views/PricingPanel.tsx` は同等のインライン判定を持つ。
- 必須クエリパラメータ検証: 「クエリパラメータを取り出し、空なら `writeBadRequest` で `<name> query parameter is required` を返す」という同型ブロックが `backend/internal/api/handlers_aws.go` に 7 箇所 (`handleRDSParameters` / `handleRDSClusterParameters` / `handleElastiCacheParameters` / `handleELBListeners` / `handleELBRules` / `handleELBTargetGroups` / `handleELBTargetHealth`)、`handlers_athena.go` に 1 箇所、`handlers_secrets_ssm.go` に 2 箇所、`handlers_session.go` に 1 箇所の計 11 箇所ある (すべて同一パッケージ `backend/internal/api`)。docs/issues/0075 が同型の直書きを 3 箇所追加する予定で、番号順に実装すると本 issue の置き換え対象は 14 箇所になる。
- 列セル部品: `Dash` コンポーネントが `components/tables/columns.tsx` / `gcpColumns.tsx` / `nonAwsColumns.tsx` の 3 ファイルにバイト単位で同一に定義され、そのスタイル定数 `dashStyle` (3 ファイル)、`mutedMono` (3 ファイル)、`monoStyle` (2 ファイル) も重複している。`dashStyle` は `gcpColumns.tsx` / `nonAwsColumns.tsx` では `Dash` からのみ参照され、`columns.tsx` では `Dash` に加えて 701 行目 (`<span style={dashStyle}>private</span>`) でも直接使われている。

調査の過程で見つかったが本 issue では扱わない重複と、その理由は次のとおり。

- `noopSave` (2 箇所): テストファイルのフィクスチャであり、プロダクションコードの重複ではないため扱わない。
- `copyRaw` (2 箇所): 1 行の関数で共通化の便益がない。既存の共有フック `useCopy` (`components/logviewer/useCopy.ts`) に統合すると「コピー済み」表示が付いて挙動が変わり、refactor の範囲を超えるため扱わない。
- `DrawerSecretEdit.tsx` / `DrawerSSMEdit.tsx`: 共通本体 `DrawerValueEditor` が既に存在し、両ファイルは差分 (使用フックと `infoRows`) だけを持つ薄いアダプタになっているため、これ以上の共通化は行わない。
- `DrawerValueEditor.tsx` のローカル関数 `errorText`: docs/issues/0075 が `components/Drawer/drawerError.tsx` の `DrawerError` として共有化するため、本 issue では扱わない。
- `ErrorBanner` (`components/ErrorBanner.tsx`) と docs/issues/0075 の `DrawerError`: 表示する情報 (ステータス、コード、メッセージ) は同等だが、ビュー全体幅のバナーと Drawer サブタブ内の表示でレイアウトが異なる。統合しない判断と理由は docs/issues/0075 の設計判断に記載済みで、本 issue では扱わない。
- `MAX_SERIES` (2 箇所): 同値の定数だが `MonthlyCostPanel` と `CostExplorerPanel` で意味が独立しており、片方だけ変えたい変更がありうるため共有しない。
- `FILTER_SNIPPETS` (AWS 版と GCP 版) と `useProfiles` (`hooks/useProfiles.ts` と `api/queries.ts`) は名前が同じだけで中身や役割が異なり、重複ではない。

タグ変換 (`tagsToMapFunc`)、クライアント生成 (`NewClient`)、キャッシュハンドラ本体 (`serveCached`、docs/issues/closed/0004)、オブジェクトブラウザ (docs/issues/closed/0014)、コスト集計 (docs/issues/closed/0012) は共通化済みであり、対象外。

## 目的

SSO 期限切れ判定、必須クエリパラメータ検証、列セル部品 (`Dash` とスタイル定数) の定義がそれぞれ 1 箇所になり、変更が 1 箇所の修正で済む。
挙動は変えない。

## 設計判断

- SSO 期限切れ判定は `frontend/src/lib/ssoError.ts` に純関数 `isSSOExpiredError(err: unknown): boolean` として抽出する。現行の `isSSOExpired` は可変長引数 (`...errors: unknown[]`) のため、5 箇所すべての呼び出し形が `errors.some(isSSOExpiredError)` などに変わるが、判定式は等価のまま保つ。`lib/` の純関数群 (`format.ts` 等) にテストを併設する既存整理に合わせ、`ssoError.test.ts` を置く。
  - 却下案: `api/client.ts` に置く。client は HTTP 呼び出しの層であり、エラー判定ユーティリティの置き場として `lib/` の慣行から外れるため却下。
- backend は必須クエリパラメータの検証をヘルパー `requireQueryParam(w http.ResponseWriter, r *http.Request, name string) (string, bool)` として `backend/internal/api` パッケージに抽出し、直書き 14 箇所 (docs/issues/0075 実装後) をすべて置き換える。エラーメッセージは現行の `<name> query parameter is required` を維持する。
  - 却下案 1: ハンドラ全体のジェネリック化。`serveCached` の抽出 (docs/issues/closed/0004) 後に残る差分は小さく、これ以上の抽象化はハンドラの見通しを悪くするため却下。
  - 却下案 2: `handlers_aws.go` 内だけの置き換え。残りも同一パッケージ内で同じヘルパーを使えるため、部分置き換えにする理由がなく却下。
  - presence 以外の検証 (docs/issues/0075 の `scope` 値検証など) はヘルパーの対象外。
- 列セル部品は共有モジュール `components/tables/cells.tsx` に `Dash` コンポーネントと `mutedMono` / `dashStyle` / `monoStyle` を移し、3 ファイルから import する。インラインスタイルのまま値を動かさないため DOM は変わらない。`columns.tsx` にのみ存在する同族の `dimMono` も、定義の分裂を避けるため同じモジュールへ移す。
  - 却下案 1: `app.css` のクラスに置き換える。style 属性から class への変更で DOM が変わり、既存のテストと詳細度に影響が及ぶため、挙動を変えない本 issue では採らない。
  - 却下案 2: スタイル定数だけを移して `Dash` を 3 ファイルに残す。`Dash` 自体が 3 ファイルにバイト単位で重複しており、定数だけを移してもコンポーネント定義の重複が残って「変更が 1 箇所で済む」という目的を満たさないため却下。

## 完了条件

- `frontend/src` で `SSO_TOKEN_EXPIRED` の文字列が、`lib/ssoError.ts` とそのテスト、およびコメント 2 箇所 (`components/SSOExpiredBanner.tsx` 冒頭、`api/queries.ts` の `useSSOLogin` 付近) 以外に存在しない。grep で判定する。
- `AthenaView` / `CloudWatchLogsView` / `AccountView` / `CostExplorerPanel` / `PricingPanel` の 5 箇所が `isSSOExpiredError` を使う。置き換えは判定式の等価な置換であることを diff で確認する。
- `isSSOExpiredError` にユニットテストがある (`ApiError` の該当コード / 他コード / `ApiError` 以外 / null のケース)。
- `backend/internal/api` に `<name> query parameter is required` を `writeBadRequest` で直書きしている箇所が存在しない。`grep -rn 'query parameter is required' backend/internal/api --include='*.go' | grep -v _test.go` のヒットが `requireQueryParam` の実装内の 1 件のみであることで判定する (2026-07-25 調査時点の直書きは 11 件。期待値として文言を持つ `_test.go` はコマンドで除外済み)。
- 代表 2 ハンドラ (例: `handleRDSParameters`、`handleELBListeners`) について、パラメータ欠落時に 400 と現行メッセージを返すことを検証するテストが `backend/internal/api/` に追加されている。
- `Dash` / `mutedMono` / `dashStyle` / `monoStyle` / `dimMono` の定義が `components/tables/cells.tsx` の 1 箇所になり、`frontend/src` に重複定義が存在しない。grep で判定する。
- `noopSave` / `copyRaw` / `DrawerSecretEdit` と `DrawerSSMEdit` の統合、`MAX_SERIES`、`errorText` の共通化 (docs/issues/0075 で扱う)、および backend の N+1 構造の整理 (docs/issues/0081 で扱う) は本 issue では扱わない。
- `CHANGES.md` の `## develop` の `### misc` に変更が記載されている (担当者行を含む)。
- `mise run check` が通過する。

## 関連

- docs/issues/0080 / docs/issues/0081: 同じ TODO 項目からの分割。本 issue は重複定義の解消のみを扱い、キャッシュ設定と処理速度には触れない。N+1 構造の共通ヘルパー化の判断は docs/issues/0081 に送る。
- docs/issues/0074 / docs/issues/0075: `columns.tsx` (セル部品) と `handlers_aws.go` (docs/issues/0075 の直書き 3 箇所) を先に変更する。番号順に実装すれば衝突しない。
- docs/issues/0079 (Refresh の修正): 新設エンドポイントの `view` の presence 検証に本 issue の `requireQueryParam` を使う。
- docs/issues/0073 (AccessDenied の誤マップ修正): backend のエラーマッピングを扱う。本 issue の SSO 期限切れ判定の共通化は frontend のみで、対象が異なる。
- docs/issues/closed/0004, 0012, 0014: 既に共通化済みの範囲の出典。

## 解決方法

- SSO 期限切れ判定: `frontend/src/lib/ssoError.ts` を新設し、純関数 `isSSOExpiredError(err: unknown): boolean` とエラーコード定数 `SSO_TOKEN_EXPIRED_CODE` を定義した。`AthenaView` / `CloudWatchLogsView` のローカル関数 `isSSOExpired` を削除し、5 ビューすべてを `isSSOExpiredError` (複数エラーは `errors.some(isSSOExpiredError)`) に置き換えた。判定式は等価のまま。`lib/ssoError.test.ts` に 4 ケース (該当コードの ApiError / 他コードの ApiError / ApiError 以外 / null) のユニットテストを追加した。
- 完了条件の grep について: 発行後に issue 0073 で追加された `views/AccountView.test.tsx` と既存の `Drawer/DrawerValueEditor.test.tsx` がフィクスチャとして `'SSO_TOKEN_EXPIRED'` リテラルを持っていた。前者は `SSO_TOKEN_EXPIRED_CODE` 定数の import に置き換え (コメント・テスト名のリテラルも言い換え)、後者は SSO と無関係な ApiError 表示のテストのためフィクスチャを `403 ACCESS_DENIED` に変更した。これによりコード文字列リテラルの定義は `lib/ssoError.ts` の 1 箇所になり、残る出現は同ファイル・そのテスト・コメント 2 箇所 (`SSOExpiredBanner.tsx` 冒頭、`api/queries.ts` の `useSSOLogin` 付近) と、`AccountView.test.tsx` の定数名 `SSO_TOKEN_EXPIRED_CODE` の参照のみ。
- 必須クエリパラメータ検証: `backend/internal/api/errors.go` に `requireQueryParam(w, r, name) (string, bool)` を追加し、`handlers_aws.go` (10 箇所) / `handlers_athena.go` (1 箇所) / `handlers_secrets_ssm.go` (2 箇所) / `handlers_session.go` (1 箇所) の直書き 14 箇所をすべて置き換えた。エラーメッセージは `<name> query parameter is required` を維持。値域検証 (WAF の scope) は呼び出し側に残した。`grep -rn 'query parameter is required' backend/internal/api --include='*.go' | grep -v _test.go` のヒットはヘルパー実装 (とその godoc コメント) のみ。代表 2 ハンドラ (`handleRDSParameters` / `handleELBListeners`) のパラメータ欠落で 400 と現行メッセージを返すテスト `TestRequireQueryParamHandlers` を `handlers_aws_test.go` に追加した。
- 列セル部品: `frontend/src/components/tables/cells.tsx` を新設し、`Dash` / `monoStyle` / `mutedMono` / `dimMono` / `dashStyle` を移した。`columns.tsx` / `gcpColumns.tsx` / `nonAwsColumns.tsx` の重複定義を削除して import に置き換えた。インラインスタイルの値は変えていないため DOM は同一。
- `CHANGES.md` の `## develop` `### misc` に記載し、`mise run check` の通過を確認した (backend 全パッケージ ok、frontend 575 テスト成功)。
