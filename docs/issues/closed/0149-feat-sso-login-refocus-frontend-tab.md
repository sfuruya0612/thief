# SSO ログインの認可完了後に Frontend のタブへフォーカスを戻す

Created: 2026-08-24
Model: Claude Fable 5
Completed: 2026-08-25

## 背景

docs/issues/TODO.md の次の項目に由来する。

> Frontend で SSO Login をして別タブで開いたリダイレクト先の AWS のページでの認証が通ったあと Frontend のタブに切り替わるようにしたい

この要望の実現は 3 つの issue に分割した。本 issue はその第 3 段階で、frontend を新しいデバイス認可 API へ切り替え、認可タブの制御でフォーカスを Frontend のタブへ戻す。前提となる共有パッケージの抽出とフォーカス復帰の PoC は第 1 段階 (docs/issues/0147-refactor-extract-sso-device-auth-package.md)、backend のエンドポイント追加は第 2 段階 (docs/issues/0148-feat-sso-device-auth-api-endpoints.md) で完了している。

現状の SSO ログインのフローは次のとおり。

1. AWS リソース取得が 401 `SSO_TOKEN_EXPIRED` で失敗すると、`frontend/src/components/SSOExpiredBanner.tsx` が表示される。
2. バナーの Login ボタンが `useSSOLogin` (frontend/src/api/queries.ts:591) を起動し、`POST /api/aws/profiles/{profile}/sso/login` (frontend/src/api/endpoints.ts:170、backend/internal/api/routes.go:38) を呼ぶ。
3. backend の `handleSSOLogin` (backend/internal/api/handlers_sso.go:15) が `aws sso login --profile <profile>` を起動し、ブラウザでの認可完了まで待つ。
4. AWS CLI がユーザの既定ブラウザで認可ページを新しいタブとして開く。ユーザが認可を完了すると `aws sso login` が終了し、backend が 204 を返し、frontend は `['aws']` クエリを invalidate する。

認可ページのタブを開くのは AWS CLI のプロセスであり、frontend のスクリプトはそのタブへの参照 (`WindowProxy`) を持たない。ブラウザには他のタブからフォーカスを奪う API が無いため、認可完了後もフォーカスは AWS の完了ページのタブに残り、ユーザは手動で Frontend のタブへ切り替える必要がある。これが要望が満たされていない構造的な理由であり、frontend 自身が認可タブを開いて閉じる形に変えることで解消する。

## 目的

SSO ログインのブラウザ認可が完了したとき、ユーザが手動でタブを切り替えることなく Frontend のタブが前面に戻る。ユーザは Login ボタンを押して認可を完了するだけで Frontend の操作へ復帰できる。認可タブを自動で閉じるのはそのための手段であり、要望の本体はフォーカスの復帰である。

## 設計判断

- `SSOExpiredBanner` の Login ボタンの click ハンドラで、まず同期的に `window.open('', '_blank')` で空タブを開いて `WindowProxy` を保持する (ボタン押下直後の同期呼び出しなので、既定設定のブラウザではポップアップブロックにかからない。`start` の応答を待ってから `window.open` する形は、await を挟むと transient activation が失効しうるため採らない)。
- `POST /api/aws/profiles/{profile}/sso/login/start` (0148) の応答後にそのタブの `location` を `verification_uri_complete` へ差し替え、`POST /api/aws/profiles/{profile}/sso/login/complete` を待つ。`complete` が成功したら保持している `WindowProxy` の `close()` で認可タブを閉じ (スクリプトが `window.open` で開いたタブはスクリプトから閉じられる)、`['aws']` クエリを invalidate する。認可タブが閉じると、主要ブラウザではフォーカスが opener である Frontend のタブへ戻る (0147 の PoC で検証済みであることが本 issue の前提)。
- 既存の `POST /api/aws/profiles/{profile}/sso/login` (`aws sso login` の exec 方式) と `handleSSOLogin` は、frontend の切り替えと同時に本 issue で削除する。frontend と backend は同一リポジトリで同時に更新される内部 API であり、利用箇所が無くなった時点で後方互換を残す理由が無い。

エッジケースの扱い。

- `window.open` が `null` を返す (ポップアップブロック等) 場合: バナー内に `verification_uri_complete` をリンクとして表示し、ユーザが自分で開けるフォールバックにする。この経路ではタブの自動クローズは行われないが、ログイン自体は完了できる。
- `verification_uri_complete` が空の場合 (RFC 8628 §3.2 で OPTIONAL): CLI 実装と同じく、`verification_uri` と `user_code` をバナー内に表示するフォールバックにする。開いてしまった空タブは閉じる。
- ユーザが認可を拒否 (deny) した場合: `complete` が `access_denied` のエラーコード (0148) を返す。ユーザは deny の操作を完了しているので「認可の途中」ではなく、frontend は認可タブを閉じてよい。バナーには失敗表示を出す。
- `complete` がその他の失敗またはタイムアウトで終わった場合: 認可タブは閉じない (ユーザがまだ認可の途中かもしれない)。バナーに既存の失敗表示 (`sso.failed`) を出す。
- 複数のブラウザタブで Frontend を開いて同じ profile のログインを開始した場合: 各タブのフローはセッション ID 単位で独立し、干渉しない (0148 の設計)。片方のタブで認可が完了すると、もう片方のタブもリソース再取得の成功でバナーが消えるが、そのタブが開いた認可タブと進行中の `complete` は自動では片付けない (放置されたセッションは有効期限で失効する)。
- `start` の応答時点でユーザが開いておいた空タブを既に手動で閉じていた場合 (実装レビューの指摘で追加): 閉じたタブの `location` 操作はブラウザによって例外になりうるため触らず、ポップアップブロック時と同じフォールバックリンクの表示に切り替える。セッションは backend が保持しており、ユーザがリンクから認可を完了すれば `complete` は成功する。

採らなかった案とその却下理由。

- 認可完了後に frontend が `window.focus()` で自タブへフォーカスを戻す案。背景で述べたとおり他のタブからフォーカスを奪う API は無く、ユーザ操作 (transient activation) を伴わない自己フォーカスもモダンブラウザで無視される。
- 現行の exec 方式のまま、AWS CLI が開いたタブを閉じる案。別プロセスが開いたタブへの参照を frontend のスクリプトが得る手段が無い。
- 旧エンドポイントを残したまま新旧を並存させる案。利用箇所の無い API を残すと、SSO ログインの経路が 2 つあるように見えて保守の混乱を生む。

追加の AWS API 呼び出しや権限は不要である (backend 側の呼び出しは 0148 で追加済み)。

認証フロー (SSO) の挙動変更は AGENTS.md が事前質問の対象とするが、本 issue はユーザの TODO 要望そのものであり、この起票がその確認にあたる。

## 未確定論点

- 認可タブを `close()` したときにフォーカスが opener のタブへ戻る挙動は、主要ブラウザで一般的だが Web 標準で保証されてはいない。0147 の PoC は最小ページでの検証であり、実アプリのフロー (認可ページへの遷移を挟む) での挙動は本 issue の完了条件で確認する。フォーカスが戻らないことが確認された場合は、要望 (Frontend のタブに切り替わる) が未達なので本 issue はそのまま close せず、追加対応を設計して本 issue の完了条件を更新するか、追加対応を別 issue に切り出して本 issue と紐づける。

## 完了条件

- `SSOExpiredBanner` の Login ボタン押下で認可ページが同一ブラウザの新規タブ (ブラウザ設定によっては新規ウィンドウ) として開き、認可完了後にそれが自動で閉じ、Frontend のタブが前面に戻ることを macOS の既定ブラウザで手動確認し、結果を issue に記録する。フォーカスが戻らない場合は未確定論点に書いたとおり本 issue をそのまま close しない。
- `window.open` が失敗した場合、認可 URL がバナー内にリンクとして表示され、それを開いて認可すればログインが完了する。
- `complete` の成功後に `['aws']` クエリが invalidate され、リソース再取得の成功で `SSOExpiredBanner` が消える。
- 既存の `POST /api/aws/profiles/{profile}/sso/login` のルーティング (backend/internal/api/routes.go:38) と `handleSSOLogin` (backend/internal/api/handlers_sso.go) が削除され、frontend に旧エンドポイントの呼び出しが残っていない。
- `SSOExpiredBanner` の分岐にコンポーネントテストが追加される: `complete` 成功時に保持している `WindowProxy` の `close()` が呼ばれること (`window.open` をモックして検証)、`access_denied` 失敗時に `close()` が呼ばれること、その他の失敗時に `close()` が呼ばれないこと、ポップアップブロック時のリンク表示、失敗表示。
- バナーに追加する文言 (認可 URL のリンク、`user_code` の案内) は既存の `session` または `errors` ネームスペースの方針に合わせて i18n リソースに追加される。
- (実装レビューの指摘で追加) `start` の応答時点で空タブが手動で閉じられていた場合に、閉じたタブの `location` を操作せずフォールバックリンクを表示することと、フォールバック表示が再クリックでリセットされることのコンポーネントテストが追加される。
- 扱わない範囲: フォーカスが opener に戻らないブラウザへの追加対応 (未確定論点の判断に従い、必要なら別途設計する)。macOS の既定ブラウザ以外での検証。
- `mise run check` が通る。

## 関連

- docs/issues/0147-refactor-extract-sso-device-auth-package.md: 同じ TODO 項目の第 1 段階。フォーカス復帰の前提を PoC で検証しており、本 issue はその結果を前提とする。
- docs/issues/0148-feat-sso-device-auth-api-endpoints.md: 同じ TODO 項目の第 2 段階。本 issue はこのエンドポイントを利用するため、0148 の完了後に着手する。
- docs/issues/closed/0141-bug-sso-login-aborts-when-the-browser-cannot-be-opened.md: `sso login` のブラウザ起動失敗を警告に落とした対応。本 issue の frontend 側フォールバック (リンク表示) は同じ思想である。
- docs/issues/closed/0030-bug-aws-sso-expiry-not-refreshed-after-login.md: SSO ログイン後に期限表示が更新されないバグの修正。ログイン完了後のクエリ invalidate の経路はこの対応で確立したものを踏襲する。

## 解決方法

設計判断の方式どおりに実装した。

- frontend: `SSOExpiredBanner` の Login ボタンの click ハンドラで同期的に `window.open('', '_blank')` を呼んで `WindowProxy` を保持し、`useSSOLogin` (frontend/src/api/queries.ts) をデバイス認可フロー全体 (`start` で認可 URL 取得 → 認可タブを `location.replace` で遷移 → `complete` でトークン取得を待機 → `close()` でタブを閉じる) を実行するミューテーションに書き換えた。成功時は従来どおり `['aws']` クエリを invalidate する。`start` の応答は `SSOLoginStartRaw`/`SSOLoginStartRow` (frontend/src/types/aws.ts) と `ssoLoginStartFromRaw` (frontend/src/lib/normalize.ts) の 2 層で扱う。
- フォールバック: ポップアップブロック時 (`window.open` が null) と、開いた空タブが `start` 応答時点で手動で閉じられていた場合 (`onTabUnavailable` コールバック) は、バナー内に認可 URL のリンクを表示する (`tabUnavailable` state)。`verification_uri_complete` が空の場合は `verification_uri` と `user_code` を表示し、開いた空タブは閉じる。文言は `errors` ネームスペースの `sso.fallbackLink` / `sso.userCode` として ja/en に追加した。
- 失敗時のタブの扱い: `start` 失敗時は認可前なので空タブを閉じる。`complete` の `SSO_LOGIN_ACCESS_DENIED` (deny) はユーザが操作を終えているので閉じる。その他の失敗とタイムアウトは認可の途中かもしれないので閉じない。
- backend: 旧 `POST /api/aws/profiles/{profile}/sso/login` のルーティングと `handleSSOLogin` / `writeSSOLoginResult` を削除した。旧ルートが未登録で 404 を返すことは `TestOldSSOLoginRouteRemoved` (backend/internal/api/handlers_sso_test.go) で検証する。

実装詳細の乖離 (方式は保ったまま)。

- i18n の `Trans` コンポーネントのプレースホルダ名は `<link>` が HTML の void 要素と衝突して子テキストを注入できないため `<authLink>` とした。
- `start` 応答時点で空タブが手動で閉じられていた場合の分岐 (エッジケース一覧に追記済み) は実装レビューの指摘で追加した。閉じたタブの `location` 操作はブラウザによって例外になりうるため触らず、フォールバックリンク表示に切り替える。

完了条件の手動確認 (2026-08-25、macOS の既定ブラウザで実施)。

- `thief sso logout` でログアウトした状態から、バナーの Login ボタンで認可ページが新規タブとして開き、認可完了後にタブが自動で閉じ、Frontend のタブへフォーカスが戻ることを確認した (完了条件 1 充足。未確定論点だった実アプリのフローでのフォーカス復帰は成立)。
- 確認の途中で `start` が 404 を返す事象があったが、0148 のエンドポイント追加より前に起動したままの API サーバプロセスが原因で、サーバ再起動後に解消した (コードの不具合ではない)。

多観点レビュー (5 観点 × 1 ラウンド + 変更点限定の追加 2 ラウンド) の主な反映。

- 認可タブへの遷移前に `authWindow.closed` を確認する分岐と `onTabUnavailable` によるフォールバック切り替え (観点 3 と追加ラウンド観点 1 の指摘)。
- 旧ルート削除テストを未登録 (空パターン) と 404 応答の両方の検証に強化 (観点 2)。
- 再クリックで `tabUnavailable` がリセットされることのテストを、フォールバックリンクの表示を確認してから失敗させる deferred 方式で 2 経路 (ポップアップブロック、closed 検知) とも追加 (追加ラウンド観点 2 と観点 3)。
- `handleSSOLoginComplete` のコメントから削除済み `handleSSOLogin` への参照を除去 (観点 4)。

却下した指摘とその理由。

- `complete` がその他の失敗で終わった認可タブを後から回収する仕組み: 認可の途中かもしれないタブを自動で閉じない設計は本 issue のエッジケースの扱いのとおりで、backend のセッションと goroutine は 0148 の失効掃除で回収される。
- `verification_uri_complete` のスキーム検証: backend は同一ホストのローカル自前プロセスで AWS OIDC の応答をそのまま写すだけであり、悪意ある値の混入は backend 自体の侵害が前提になるため、フロント側の検証は防御にならない。
- unmount 後の `setStarted` 呼び出しへのガード: React 18 では unmount 後の setState は安全に無視される。
- ポップアップブロック時に `start` 応答が届くまでの間の文言: 応答は通常 1 秒未満で切り替わり、ギャップ期間の文言のためだけの state 追加は複雑化に見合わない。
- `onTabUnavailable` 呼び出しの try/catch 保護: 呼び出し元はバナーの setState のみで、既存の `onStarted` と同じパターンの踏襲であり新規リスクの上乗せではない。
