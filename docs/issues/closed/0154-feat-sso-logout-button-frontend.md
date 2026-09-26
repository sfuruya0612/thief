# アクティブセッションカードに SSO ログアウトボタンを追加する

Created: 2026-08-26
Model: Claude Fable 5
Completed: 2026-08-27

## 背景

docs/issues/TODO.md の次の項目に由来する。

> Frontend に SSO のログアウトボタンがほしい
>     - issue 0149 の手動確認で `thief sso logout` コマンドを使ってログアウト状態を作った際に、Frontend からログアウトできると便利だと気付いた

この要望の実現は 2 つの issue に分割した。本 issue はその第 2 段階で、第 1 段階 (docs/issues/0153-feat-sso-logout-api.md) で追加した `POST /api/aws/profiles/{profile}/sso/logout` を呼ぶボタンを frontend に置く。本 issue は 0153 の close 後に着手する。

frontend で SSO を扱う UI は 2 箇所あり、どちらにもログアウトの導線は無い。

- frontend/src/components/SSOExpiredBanner.tsx: 401 `SSO_TOKEN_EXPIRED` のときに表示され、Login ボタンが `useSSOLogin` (frontend/src/api/queries.ts) でデバイス認可フローを実行する。
- frontend/src/components/session/AwsActiveSessionCard.tsx: サイドバーのアクティブセッションカード。profile 単位で認証種別 (`profileAuthLabel`)、SSO 有効期限 (`formatSsoExpiry`)、期限切れや未ログイン時の再認証コマンド `aws sso login --profile <profile>` のコピー導線 (`needsReauth` 分岐) を表示する。

profile 一覧の SSO 状態 (`valid` / `expired` / `not_logged_in`) は backend/internal/aws/profiles.go の `applySSOStatus` が決める。
backend/internal/aws/sso_cache.go の `readSSOCacheStatuses` で読んだキャッシュファイルの `startUrl` と profile の start URL を `normalizeStartURL` で突き合わせる。

frontend にはログアウト API を呼ぶ `endpoints.ts` / `queries.ts` の関数も、それを押す UI も無い。
これが要望が満たされていない理由である。

## 目的

Frontend のアクティブセッションカードにログアウトボタンを置き、ターミナルを開かずにその profile の SSO ログイン状態を解除できる。
ログアウト後はカードの表示が未ログインに変わり、backend のリソースキャッシュも破棄されて AWS リソースの取得が `SSO_TOKEN_EXPIRED` の経路に入り、`SSOExpiredBanner` の Login ボタンから再ログインできる。

## 設計判断

### ボタンの配置と表示条件

- ボタンは `AwsActiveSessionCard` に置き、`meta.authType === 'sso'` かつ `meta.ssoStatus === 'valid'` のときだけ表示する。`ssoStatus` が欠落している profile (backend/internal/aws/profiles.go の `applySSOStatus` が判定不能で空のまま残した場合) はログイン済みかどうか分からないため表示しない。`profileBadge(meta)?.tone !== 'warn'` を条件にすると判定不能 (`profileBadge` が `null`) のときも表示されてしまうため採らない。
- `needsReauth` は `badge?.tone === 'warn'` または有効期限 30 分未満 (`isExpiringSoon`) で真になるため、`ssoStatus === 'valid'` で期限が迫っている間はログアウトボタンと再認証コマンドのコピー導線が同時に表示される。この両立は意図したものとする (期限が迫っていてもログアウトはできてよく、再ログインの案内も出したままにする)。ログアウトボタンは `session-card-reauth` の外、`session-card-meta` の下に独立した行として置く。
- 採らなかった案: `TopBar` に置く。`TopBar` は profile の文脈を持たない (プロファイル選択は Sidebar 側) ため却下する。
- 採らなかった案: `SSOExpiredBanner` に置く。バナーは未ログイン時にしか表示されず、ログイン済みの状態でログアウトする導線にならないため却下する。

### API 呼び出しと後処理

- API 呼び出しは frontend/src/api/endpoints.ts に `postSSOLogout(profile)` を追加し、frontend/src/api/queries.ts に `useSSOLogout(profile)` を `useMutation` で追加する。成功時は frontend/src/lib/refreshView.ts の `createViewRefresher(deps)` が返す関数を `'aws'` で呼び、`POST /api/cache/invalidate?view=aws` で backend のリソースキャッシュを破棄してから `['aws']` を 1 回 invalidate する。TanStack Query の `invalidateQueries` は既定で前方一致するため、`useProfiles` の `['aws', 'profiles']` (profile 一覧の SSO 状態) もこの 1 回で無効化され、別途 `['aws', 'profiles']` を invalidate しない。
- backend のリソースキャッシュ (`resourceCache`、backend/internal/api/server.go の `cacheTTL = time.Hour`) は `serveCached` が `refresh` 無しの GET でヒットさせ、ヒット時は AWS API を呼ばない (backend/internal/cache/cache.go の `Load`)。frontend の通常のクエリ (`getResources`) は `refresh` を付けない。
- このため TanStack Query だけを invalidate しても、再取得は TTL 内の backend キャッシュを受け取り、SSO トークンキャッシュを削除済みでも AWS API が呼ばれず `SSO_TOKEN_EXPIRED` にならない。古いリソース一覧が最長 1 時間表示され続ける。
- TopBar の Refresh が backend 破棄 → Query 無効化の順序を取るのはこの理由で (`refreshView.ts` 冒頭コメント、docs/issues/closed/0079)、ログアウトも同じ順序を `createViewRefresher` の再利用で守る。
- `createViewRefresher` は backend 側の破棄に失敗しても Query の無効化を続ける (backend 停止時に no-op にならないための既存の判断)。ログアウトでもその挙動を引き継ぐ。
- 採らなかった案: TanStack Query の `['aws']` だけを invalidate し `POST /api/cache/invalidate` を呼ばない。`useSSOLogin` の成功時 (frontend/src/api/queries.ts) はこの形だが、ログインは「取れなかったものを取り直す」方向で backend キャッシュのヒットが問題にならないのに対し、ログアウトは「取れていたものを見せなくする」方向で、上記のとおり backend キャッシュのヒットが目的を無効にするため却下する。
- 採らなかった案: 0153 の `handleSSOLogout` が backend 側で `resourceCache` を破棄する。SSO のハンドラがリソースキャッシュの内部構造に依存することになり、CLI の `thief sso logout` には同じ処理が無い (CLI はキャッシュを持たない) ため対称性も無い。frontend に既存の `createViewRefresher` 経路があるため却下する。
- ログアウトボタンは `SSOExpiredBanner` の Login ボタンと同じ規約で、mutation の `isPending` の間は `disabled` にして二重押しを防ぐ。`isError` のときはボタンの隣に失敗の文言 (`session.json` の `awsActiveSessionCard` に追加するキー) を表示し、`ApiError` の `message` は表示しない (`SSOExpiredBanner` が `sso.failed` の固定文言だけを出す規約に揃える。backend のエラーメッセージは英語で i18n の対象外)。失敗後もボタンは再度押せる。
- 文言 (ボタン、title、失敗) は frontend/src/i18n/locales/{ja,en}/session.json の `awsActiveSessionCard` に追加する。
- 同じ start URL を共有する複数 profile は 1 つのキャッシュファイルを共有するため、1 つの profile でログアウトすると同じ start URL の他の profile も未ログインになる (0153 の設計判断)。ボタンの title 属性にその旨を記載する。

### 権限

- frontend の変更のみで、IAM 権限と依存の追加は無い。

## 完了条件

- frontend/src/api/endpoints.ts に `postSSOLogout(profile)`、frontend/src/api/queries.ts に `useSSOLogout(profile)` が追加されている。
- `AwsActiveSessionCard` に、`authType === 'sso'` かつ `ssoStatus === 'valid'` の profile のときだけログアウトボタンが表示され、押下で `POST .../sso/logout` が呼ばれ、成功後に `createViewRefresher` が返す関数を通して `POST /api/cache/invalidate?view=aws` が先に呼ばれ、その完了後に `['aws']` が 1 回 invalidate され (前方一致で `['aws', 'profiles']` も無効化される)、`['aws', 'profiles']` を個別に invalidate する呼び出しは追加しない。
- ログアウトボタンの `title` 属性に、同じ start URL を共有する他の profile も未ログインになる旨の文言 (`session.json` のキーを経由) が設定されている。
- `AwsActiveSessionCard.test.tsx` に、ログアウト成功時に `POST /api/cache/invalidate?view=aws` が `['aws']` の invalidate より先に 1 回呼ばれること、ログアウト失敗時 (500) には呼ばれないことのテストが追加されている。
- `AwsActiveSessionCard.test.tsx` に、`ssoStatus === 'valid'` でボタンが表示されること、`expired` / `not_logged_in` / `ssoStatus` 欠落 / 非 SSO の 4 通りで表示されないこと、`ssoStatus === 'valid'` かつ有効期限 30 分未満のときログアウトボタンと再認証コマンドのコピー導線が両方表示されること、押下で `POST .../sso/logout` が呼ばれることのテストが追加されている。
- ログアウトボタンが mutation の `isPending` の間 `disabled` になり、`isError` のとき失敗の文言が表示され、`ApiError` の `message` は表示されない。
- `AwsActiveSessionCard.test.tsx` に、pending 中にボタンが `disabled` になること、失敗時に失敗の文言が表示されボタンが再度押せることのテストが追加されている。
- ja / en の `session.json` にボタン文言、title 文言、失敗文言が追加されている。
- backend の変更は行わない。
- `mise run check` が通過する。

## 関連

- docs/issues/0153-feat-sso-logout-api.md: 本 issue が呼ぶエンドポイントを追加する第 1 段階。0153 の close が本 issue の前提である。この前提は手動確認のための順序であり、自動テストの技術的な依存ではない。本 issue の自動テストは API クライアントをモックして 0153 無しでも書けるが、ボタンから実際にログアウトできることの手動確認は 0153 のエンドポイントが無いと行えない。
- docs/issues/closed/0149-feat-sso-login-refocus-frontend-tab.md: TODO 項目の発端となった手動確認。`useSSOLogin` の mutation と `SSOExpiredBanner` の pending / error 表示の規約。

## 解決方法

frontend/src/api/endpoints.ts に `postSSOLogout(profile)` を追加した。`apiPost<void>` で `POST /api/aws/profiles/{profile}/sso/logout` を呼ぶだけの関数で、profile 名は `encodeURIComponent` でエンコードする (既存の `postSSOLoginStart` と同じ)。

frontend/src/api/queries.ts に `useSSOLogout(profile)` を追加した。`useMutation` の `mutationFn` が `postSSOLogout(profile)` を呼び、`onSuccess` で `createViewRefresher` (frontend/src/lib/refreshView.ts) が返す関数を `'aws'` で呼ぶ。refresher には `postCacheInvalidate` (endpoints.ts) と `queryClient.invalidateQueries({ queryKey })` を渡し、`useMemo` で `queryClient` ごとに 1 つだけ作る。これにより `POST /api/cache/invalidate?view=aws` が先に呼ばれ、その完了後に `['aws']` が 1 回 invalidate される。`['aws', 'profiles']` は前方一致で同時に無効化されるため個別の invalidate は書いていない。`onSuccess` が refresher の Promise を返すため、TanStack Query は無効化の完了まで `isPending` を保ち、その間ボタンは `disabled` のままになる。 `queryClient.invalidateQueries` は既定で再取得の失敗を reject に載せないため、ログアウト成功後に `onSuccess` の失敗で `isError` に転ぶ経路は無い (コメントに明記)。同じコンポーネントが別の profile を表示するようになったときは `useEffect` のクリーンアップで `mutation.reset()` を呼び、前の profile の `isPending` / `isError` を次の profile のカードに持ち越さない (レビュー観点 3 の指摘で追加。Sidebar は `AwsActiveSessionCard` を `key` 無しで描画するため profile を切り替えても再マウントされない)。この refresher は TopBar の Refresh (frontend/src/App.tsx) の refresher とは別インスタンスで再入ガードを共有しないため、両方を同時に押すと backend の破棄と `['aws']` の無効化が重複して走りうるが、どちらも冪等なので実害は無い (レビュー観点 5 の指摘でコメントに明記)。

frontend/src/components/session/AwsActiveSessionCard.tsx に `canLogout = meta?.authType === 'sso' && meta.ssoStatus === 'valid'` を追加し、真のときだけ `session-card-meta` の直下、`session-card-reauth` の外に `session-card-logout` 行を描画する。行にはログアウトボタン (`btn sm ghost`、`title` は `awsActiveSessionCard.logoutTitle`、`disabled={logout.isPending}`、pending 中は `logoutPending` の文言) と、`logout.isError` のときの固定文言 `awsActiveSessionCard.logoutFailed` (`session-card-error`) を置く。`ApiError` の `message` は表示しない。`useSSOLogout(profile)` は Hooks の規則に従い条件分岐の外で常に呼ぶ。

frontend/src/i18n/locales/ja/session.json と en/session.json の `awsActiveSessionCard` に `logout`、`logoutPending`、`logoutTitle` (同じ start URL を共有する他の profile も未ログインになる旨)、`logoutFailed` を追加した。frontend/src/app.css に `.session-card-logout` (`.session-card-reauth` と同じ flex 行) と `.session-card-error` (`var(--err)`、10px) を追加した。

テストは frontend/src/components/session/AwsActiveSessionCard.test.tsx に `describe('SSO ログアウト')` として追加した。fetch モック `mockLogoutFetch` は `POST .../sso/logout` と `POST /api/cache/invalidate?view=aws` だけに応答し、それ以外 (STS 補完) は解決させず、呼ばれたメソッドと URL を配列 `calls` に積む。順序の検証では `qc.invalidateQueries` を `vi.spyOn` で差し替えて同じ配列に `invalidateQueries:["aws"]` を積み、`['POST .../sso/logout', 'POST /api/cache/invalidate?view=aws', 'invalidateQueries:["aws"]']` と完全一致することを確認する (backend 破棄が 1 回先行し、invalidate が `['aws']` の 1 回だけであることを同時に固定する)。 比較は mutation の完了 (ボタンが有効に戻る) を待ってから行い、完了後に追加される invalidate も取りこぼさない。テストケースは次のとおり。

- `ssoStatus === 'valid'` でボタンが表示され、`title` が `logoutTitle` の文言になる
- `expired` / `not_logged_in` / `ssoStatus` 欠落 / 非 SSO (`access_key`) の 4 通りで表示されない (`it.each`)
- `valid` かつ有効期限 10 分後 (30 分未満) でログアウトボタンと再認証コマンドとコピー導線が両方表示される
- 押下で `POST .../sso/logout` が呼ばれ、成功後の呼び出し順が上記と一致する
- ログアウトの応答が返らない間、ボタンが `ログアウト中…` の文言で `disabled` になる
- ログアウトが 204 で成功した後、`POST /api/cache/invalidate` の応答が返らない間もボタンが `disabled` のまま (レビュー観点 1 の指摘で追加。`onSuccess` の Promise 解決まで `isPending` が続くことを固定する)
- 500 (`SSO_LOGOUT_FAILED`、backend の `ErrorResponse` と同じく `error` に `permission denied` を含むメッセージ) のとき、`POST /api/cache/invalidate` が呼ばれず、`invalidateQueries` も呼ばれず、固定文言 `ログアウトに失敗しました` が表示され、`permission denied` は表示されず、ボタンが有効に戻り、再押下で 2 回目の `POST .../sso/logout` が送られて再度失敗文言とボタンの復帰に至る (実装を `logout.error?.message` の表示に変えるとこのテストが落ちることを一時変更で確認した)
- profile A の失敗表示が出た状態で同じカードを profile B に切り替えると、失敗文言が消えボタンが有効になる (観点 3 の指摘で追加。`reset` を外すとこのテストが落ちることを一時変更で確認した)
- 失敗表示が出た状態で同じ profile のまま再レンダーしても失敗文言は消えない (追加レビュー観点 2 の指摘で追加。`useEffect` の依存配列を外して毎レンダーで `reset` が走るようにするとこのテストが落ちることを一時変更で確認した)
- 一覧に無い profile (`meta` が `undefined`) ではボタンが出ない (既存テスト「一覧に無いプロファイルでも名前だけで描画できる」に検査を追加)

backend は変更していない。手動確認 (ボタンから実際にログアウトし、カードが未ログインに変わり `SSOExpiredBanner` から再ログインできること) は 0153 のエンドポイントで行える状態だが、この実行では行っておらず、自動テストで代えている。

完了条件の確認結果は次のとおり。

- `postSSOLogout` と `useSSOLogout` の追加: 差分で確認
- 表示条件、押下時の POST、backend 破棄の先行と `['aws']` の 1 回 invalidate、`['aws', 'profiles']` の個別 invalidate 無し: テスト「押下で POST .../sso/logout が呼ばれ、成功後は backend キャッシュ破棄が 1 回先行してから [aws] を 1 回無効化する」で検証。`useSSOLogout` の実装に `['aws', 'profiles']` の invalidate は無い
- `title` 属性の文言: テスト「ssoStatus が valid の SSO profile にはログアウトボタンが出る」で検証
- 成功時の順序と失敗時に破棄が呼ばれないこと: 上記 2 テストで検証
- 表示 / 非表示 4 通り / 期限間近の両立 / 押下で POST: 上記テストで検証
- pending 中の `disabled`、`isError` の固定文言、`message` の非表示: 上記テストで検証
- ja / en の `session.json`: 差分で確認
- backend の変更無し: 差分に backend 配下のファイルが無い
- `mise run check`: 通過 (frontend 754 テスト PASS、lint は 0 エラー。警告 10 件はベースラインと同じ)
