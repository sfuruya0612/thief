# CLI の SSO ログインのエラーラップが %v でチェーンを切り、文言も二重に表示される

Created: 2026-08-08
Completed: 2026-08-08
Model: Claude Opus 5

## 背景

issue 0125 (`sso_oidc.go` のエラーラップ文言の棚卸し) の調査中に、呼び出し元の CLI 側で 2 つの独立した問題を見つけた。

`backend/internal/cli/sso.go` の `getSSOToken` (336-370 行) は 4 箇所すべてで `%v` を使ってエラーを包んでいる。

```
internal/cli/sso.go:339   fmt.Errorf("failed to register client: %v", err)
internal/cli/sso.go:344   fmt.Errorf("failed to start device authorization: %v", err)
internal/cli/sso.go:348   fmt.Errorf("failed to open browser: %v", err)
internal/cli/sso.go:356   fmt.Errorf("failed to get token: %v", err)
```

### 問題 1: %v がエラーチェーンを切る

`%v` は元のエラーを文字列へ展開するだけで包まない。`errors.Is` / `errors.As` は `Unwrap()` を辿るため、`%v` を通した時点で元のエラー型と番兵値に到達できなくなる。

`internal/cli` パッケージ全体では `fmt.Errorf(..., %w, err)` が 51 箇所、`%v` が 4 箇所である。その 4 箇所はすべてこの `getSSOToken` に集中しており、パッケージ内で唯一の例外になっている。

AGENTS.md の「backend (Go)」節は「ラップは `fmt.Errorf` の `%w` 動詞を使う」「比較は `errors.Is`、型取り出しは `errors.As` を用いる」と定めており、これに反する。

実害として、`WaitForSSOToken` が返す SSO OIDC の型付き例外 (`ssooidctypes.AuthorizationPendingException` 等) や、AWS SDK の `smithy.APIError` へ `errors.As` で到達できない。`internal/api/errors.go` の `writeAWSError` は `errors.As` で smithy のエラーコードを取り出して HTTP ステータスへマップしているため、同じ判定を CLI 側の経路でも行いたくなったときに機能しない。

### 問題 2: 呼び出し先と同じ文言で二重に包んでいる

`getSSOToken` が呼ぶ 2 つの関数は、すでに同じ文言でラップしたエラーを返している。

| 呼び出し元 (cli/sso.go) | 呼び出し先 (aws/sso_oidc.go) |
| --- | --- |
| 339: `failed to register client: %v` | 60: `failed to register client: %w` |
| 344: `failed to start device authorization: %v` | 84: `failed to start device authorization: %w` |

結果として、ユーザーが目にするメッセージは次の形になる。

```
failed to register client: failed to register client: operation error SSO OIDC: RegisterClient, ...
```

同じ語句が 2 回続くだけで情報は増えていない。Go のエラー文言はラップのたびに `: ` で連結されるため、各層は自分の層で分かることだけを足すのが前提である。

なお 356 行の `failed to get token` は呼び出し先の `token creation failed` (sso_oidc.go:128) とは異なる文言のため二重にはならないが、`%v` の問題は同じく該当する。348 行の `openBrowser` は同一パッケージ内の関数で、二重ラップには該当しない。

## 再現手順

1. 有効でない SSO の start URL を指定して `thief sso login` 相当のコマンドを実行する (`internal/cli/sso.go` の `getSSOToken` を通す)。
2. `RegisterClient` が失敗する条件 (到達不能なリージョン指定など) を作る。
3. 表示されるエラーメッセージに `failed to register client:` が 2 回現れる。

## 影響

- ユーザーが目にするエラーメッセージに同じ語句が重複し、読みにくい。
- SSO ログイン経路のエラーが `errors.Is` / `errors.As` で判別できない。SSO 特有の例外に応じた処理を CLI 側で足せない。
- 読み取り専用の認証フローであり、データの破壊や外部への影響はない。

## 修正方針

- 4 箇所の `%v` を `%w` に変更する。エラーチェーンを繋ぎ、パッケージ内の他の 51 箇所と揃える。
- 二重になる 2 箇所 (339, 344) は、呼び出し元側のラップ文言を、この層で分かることだけを足す形に変更する。呼び出し先が「どの API で失敗したか」をすでに述べているため、呼び出し元は「SSO ログインのどの段階か」を述べる。`sso login: %w` のような段階を示す文言に統一するか、あるいは呼び出し元のラップ自体を削って呼び出し先の文言をそのまま伝播させるかを、実装時に比較して決める。
- 348 行と 356 行も同じ方針で文言を見直す。`openBrowser` は同一パッケージのため二重ではないが、"failed to" 形式は `internal/aws` の棚卸し (issue 0123 / 0125) と揃えて除く。
- 変更後の文言を文字列比較しているコードやテストがないことを、backend と frontend の両方で確認してから変更する。

## 追記 (2026-08-08): ラップは 3 層あり、issue 0125 で内側の文言が変わった

issue 0125 のレビューで、ユーザーの端末に出る文字列が本 issue の起票時に想定していたより 1 層多いことが分かった。

| 層 | 場所 | 付ける文言 |
| --- | --- | --- |
| 1 (最内) | `internal/aws/sso_oidc.go` | `register sso oidc client` 等 (0125 で変更済み) |
| 2 | `internal/cli/sso.go` の `getSSOToken` | `failed to register client` 等 (`%v`。本 issue の対象) |
| 3 | `internal/cli/sso.go` の `ssoLogin` (132 行) / `ssoGenerateConfig` (196 行) | `get token: %w` |
| 4 (表示) | Cobra | `Error: ` を前置 (`SilenceErrors` は未設定) |

issue 0125 で最内層の文言を変更したため、現在の表示は次のようになっている。

```
RegisterSSOClient 失敗時:
  0125 前: Error: get token: failed to register client: failed to register client: <sdk error>
  0125 後: Error: get token: failed to register client: register sso oidc client: <sdk error>

StartSSODeviceAuthorization 失敗時:
  0125 前: Error: get token: failed to start device authorization: failed to start device authorization: <sdk error>
  0125 後: Error: get token: failed to start device authorization: start sso oidc device authorization: <sdk error>

WaitForSSOToken (CreateToken) 失敗時:
  0125 前: Error: get token: failed to get token: token creation failed: <sdk error>
  0125 後: Error: get token: failed to get token: create sso oidc token: <sdk error>
```

一字一句同じ文言が 2 回続く状態 (問題 2) は 0125 で解消された。ただし意味の重複は残っており、`failed to register client: register sso oidc client:` のように文法的に読み取りにくい連結になっている。

また、層 3 の `get token: %w` は `RegisterClient` の失敗にも `StartDeviceAuthorization` の失敗にも一律に付くため、実際には「トークン取得のどの段階か」を示せていない。層 2 の `failed to get token` (356 行) とも重複する。

本 issue の修正時は、層 2 だけでなく層 3 も含めて棚卸しし、各層が自分の層で分かることだけを足す形に整理すること。層 1 が「どの API で失敗したか」を述べている以上、層 2 と層 3 の両方が「トークン取得で失敗した」と述べる必要はない。

## 完了条件

- `internal/cli/sso.go` の `getSSOToken` の 4 箇所が `%w` でラップしている。
- `internal/cli` に `fmt.Errorf(..., "%v", err)` 形式のラップが残っていない。
- 呼び出し先と同じ語句が 2 回続くエラーメッセージが生じない。
- `errors.Is` / `errors.As` が `getSSOToken` の戻り値から元のエラーへ到達できることを検証するテストがある。
- `mise run check` が通る。

## 解決方法

### 完了条件 1 から意図的に逸脱した

完了条件 1 は「`getSSOToken` の 4 箇所が `%w` でラップしている」だが、実装は 3 箇所のラップを削除し、`openBrowser` の 1 箇所だけを `%w` でラップした。

修正方針が「あるいは呼び出し元のラップ自体を削って呼び出し先の文言をそのまま伝播させるかを、実装時に比較して決める」と明示的に選択肢を与えているため、この逸脱は方針の範囲内である。比較した結果、削除を選んだ根拠は次のとおり。

| 呼び出し | 層 1 (`internal/aws`) が述べていること | 層 2 が足せること | 判断 |
| --- | --- | --- | --- |
| `RegisterSSOClient` | `register sso oidc client` (どの API で失敗したか) | なし | ラップを削る |
| `StartSSODeviceAuthorization` | `start sso oidc device authorization` | なし | ラップを削る |
| `WaitForSSOToken` | `create sso oidc token` | なし | ラップを削る |
| `openBrowser` | 何も述べない (`exec` の裸のエラー) | 「ブラウザを開こうとした」 | `open browser: %w` でラップする |

`%v` から `%w` への単純な置換ではなく削除を選んだのは、`%w` にしただけでは完了条件 3 (呼び出し先と同じ語句が 2 回続かない) を満たせないためである。`fmt.Errorf("register client: %w", err)` は文言を変えても「クライアント登録で失敗した」という同じ意味を 2 回述べる。層 1 が API 名を述べている以上、層 2 に足す情報が存在しない。

### 層 3 の `get token: %w` は棚卸しした結果、変更しないと判断した

追記が「層 2 だけでなく層 3 も含めて棚卸しすること」と指示していたため、`ssoLogin` (132 行) と `ssoGenerateConfig` (196 行) の `get token: %w` を検討した。結論は現状維持である。

追記が挙げた 2 つの懸念のうち、「層 2 の `failed to get token` と重複する」は層 2 のラップを削除したことで解消した。残る「`RegisterClient` の失敗にも `StartDeviceAuthorization` の失敗にも一律に付くため、トークン取得のどの段階かを示せていない」については、層 3 の役割がそもそも段階の提示ではないと判断した。

同じファイル内の兄弟のラップは、いずれも「呼び出し元の関数のどの手順で失敗したか」を述べる形で揃っている。

```
sso.go:137  save cache file: %w        (ssoLogin)
sso.go:149  get cache directory: %w    (ssoLogout)
sso.go:158  walk directory: %w         (ssoLogout)
sso.go:166  delete file %s: %w         (ssoLogout)
sso.go:201  list accounts: %w          (ssoGenerateConfig)
```

`get token: %w` はこの並びの一員であり、`ssoLogin` の手順のうちトークン取得で失敗したことを述べている。段階 (どの API か) は層 1 が述べる。各層が自分の層で分かることだけを足すという原則に照らして、層 3 は既に正しい。

`get token: create sso oidc token:` は token という語が 2 回現れるが、前者は CLI の手順名、後者は SSO OIDC の `CreateToken` API 名であり、意味が異なる。残る 3 経路 (`register sso oidc client` / `start sso oidc device authorization` / `open browser`) では語の重複すら生じない。

### 実際にユーザーへ表示される文字列の変化

```
RegisterSSOClient 失敗時:
  修正前: Error: get token: failed to register client: register sso oidc client: <sdk error>
  修正後: Error: get token: register sso oidc client: <sdk error>

StartSSODeviceAuthorization 失敗時:
  修正前: Error: get token: failed to start device authorization: start sso oidc device authorization: <sdk error>
  修正後: Error: get token: start sso oidc device authorization: <sdk error>

openBrowser 失敗時:
  修正前: Error: get token: failed to open browser: exec: "open": executable file not found in $PATH
  修正後: Error: get token: open browser: exec: "open": executable file not found in $PATH

WaitForSSOToken 失敗時:
  修正前: Error: get token: failed to get token: create sso oidc token: <sdk error>
  修正後: Error: get token: create sso oidc token: <sdk error>
```

変更後の文言を文字列比較しているコードがないことを、backend の Go コードと frontend の `src/` の両方で確認した (旧文言 5 種の grep でいずれも 0 件)。

### 依存の差し替えにインターフェースではなく関数値の構造体を使った

テストから AWS への接続とブラウザ起動を回避するため、`getSSOToken` が呼ぶ 5 つの外部処理を `ssoTokenDeps` 構造体の関数型フィールドとして受け取る形にし、本体を `getSSOTokenWith` に分けた。`getSSOToken` の署名は変えていないため呼び出し元 2 箇所は無変更である。

レビューで「`internal/aws` の 20 ファイルはコンシューマ定義インターフェースで統一されており、そちらに揃えるべき」との指摘 (優先度 高) を受けたが、却下した。前提が異なる。

`internal/aws` のインターフェースは、既に存在する具象型 (`*cloudformation.Client` 等) が持つメソッド集合を絞り込むものであり、本番側に書くコードは 0 行である。対して今回の 5 つは元から自由関数であり、絞り込む対象の具象型が存在しない。インターフェース化するには、既存関数へ転送するだけの本番実装を新設する必要がある。

両案を実際に書き下して非空・非コメント行数を測った。

| 案 | 本番側 | テスト側 | 合計 |
| --- | --- | --- | --- |
| インターフェース | 23 行 | 24 行 | 47 行 |
| 関数値の構造体 (採用) | 11 行 | 0 行 | 11 行 |

差の 36 行はすべて転送のみのボイラープレートである。さらに、インターフェース案のテスト側の fake は「行ごとに 1 段だけ挙動を変える」要件を満たすために関数型フィールドを持つ構造体になる。つまりパターンは消えず、テストコードへ移動した上に転送メソッド 10 個が上乗せされる。

関数値を依存として受け渡す形自体には、このリポジトリに先例がある。

```
internal/cli/helper.go:89     ListConfig[T].Fetch func(ctx, cfg) ([]T, error)   (構造体の関数型フィールド)
internal/cache/cache.go:105   loader func() (V, error)                          (関数引数)
internal/api/server.go:135    onErr func(http.ResponseWriter, error)            (関数引数)
internal/api/server.go:136    load func() (any, error)                          (関数引数)
```

AGENTS.md の「受け取り側はインターフェース、返却側は具象型」は Go の accept interfaces, return structs を指すもので、関数値を引数で受けることを禁じてはいない。

### 測定した検出力

`getSSOTokenWith` のブロック内だけを書き換えてテストが落ちるかを確認した。

| 壊した内容 | 結果 |
| --- | --- |
| `RegisterSSOClient` の失敗を `%v` で包み直す | FAIL (検出) |
| `RegisterSSOClient` の失敗を `%w` だが呼び出し先と同語句で包む | FAIL (検出) |
| `WaitForSSOToken` の失敗を `%v` で包み直す | FAIL (検出) |
| `openBrowser` のラップを `%v` に劣化させる | FAIL (検出) |
| `openBrowser` のラップを削って文脈を失わせる | FAIL (検出) |
| `defaultSSOTokenDeps` の `openBrowser` を nil にする | FAIL (検出) |

`wantMsg` を完全一致で固定しているため、どの段でラップが増減しても落ちる。

### このテストの限界

依存の埋め忘れをコンパイル時に検出できない点はレビューの指摘 (優先度 高) のとおりで、事実として認める。インターフェースなら未実装メソッドがコンパイルエラーになるが、関数型フィールドは nil のままでもビルドが通り、呼んだ時点で panic する。

緩和策として `TestDefaultSSOTokenDepsIsFullyWired` で 5 フィールドすべての非 nil を検証し、実際に `openBrowser` を nil にすると FAIL することを確認した。CI で必ず実行されるため、埋め忘れが取り込まれることはない。コンパイル時ではなくテスト時の検出になる差を、転送コード 36 行と引き換えにする価値はないと判断した。

`ExpiresAt` は `time.Now()` に依存するため検証していない。時刻を注入すれば検証できるが、この issue の対象はエラーラップであり、キャッシュの構築内容の検証は付随的なものにとどめた。

### 採用したレビュー指摘

`ssoTokenDeps` の型コメントが差し替えの目的を「エラーの伝播を検証できるようにするため」の 1 つしか述べていなかったが、`display` はエラーを返さないため説明になっていなかった (優先度 中)。フィールドによって理由が 2 つあること (AWS 接続とブラウザ起動を回避する / テスト出力を汚さない) と、インターフェースを選ばなかった理由をコメントに書き足した。

### スコープ外とした観測

`internal/cli/ec2.go:161` に `fmt.Errorf("terminate session after exec error: %w (original exec error: %v)", termErr, execErr)` が残っている。完了条件 2 の字面には触れるが、これはラップが `%v` なのではなく、`%w` で包んだ `termErr` とは別の独立したエラー `execErr` を文脈として埋め込んでいる形である。起票時の集計でも 4 箇所の `%v` には数えられておらず `%w` 側の 51 箇所に含まれていた。Go 1.20 以降は `%w` を 2 つ書けるため `execErr` も `errors.Is` で到達可能にできるが、`getSSOToken` とは無関係な関数であり本 issue のスコープ外とする。別 issue として起票する。

## 関連

- docs/issues/0125: 起票元。`sso_oidc.go` 側のラップ文言を棚卸しする issue。二重になっている文言のもう一方を扱う。
- docs/issues/closed/0123: `ssm.go` の同種の棚卸しを行った issue。
- docs/issues/0128: 本 issue の作業中に見つけた `internal/cli/ec2.go` の 2 つのエラーの扱いに関する issue。
