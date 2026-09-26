# ssoGenerateConfig にテストの差し込み口が無く選択とロール取得の分岐が検証されていない

Created: 2026-08-09
Model: Claude Opus 5
Completed: 2026-08-11

## 背景

issue 0131 (CLI コマンドがシグナル連動 context を持たない) の作業中に、`backend/internal/cli/sso.go` の `ssoGenerateConfig` がテストから一切実行できないことが判明した。

同じファイルの `ssoLogin` は差し込み口を持つ。

```go
type ssoLoginDeps struct {
	getToken  func(ctx context.Context, region, url string) (*SSOTokenCache, error)
	saveCache func(cache *SSOTokenCache) error
}

func ssoLogin(cmd *cobra.Command, args []string) error {
	return ssoLoginWith(cmd, defaultSSOLoginDeps())
}
```

`ssoGenerateConfig` にはこれが無く、外部処理を直接呼ぶ。

- `getSSOToken(ctx, region, startUrl)`: ブラウザの起動と AWS への往復を伴う
- `awsinternal.ListSSOAccountInfos(ctx, region, cache.AccessToken)`: AWS への通信
- `awsinternal.ListSSOAccountRoleNames(ctx, region, cache.AccessToken, account.AccountID)`: AWS への通信
- `getAwsConfigPath()` と `os.WriteFile(configPath, ...)`: `$HOME/.aws/config` への書き込み

`internal/cli/sso_test.go` が `sso generate-config` に関して検証しているのは、切り出された純関数の `selectIndices` と `appendProfiles` だけである。`ssoGenerateConfig` 本体を通るテストは無い。

## 問題

以下の分岐が未検証である。

### 問題 1: アカウント選択の分岐

`promptSelection` の戻り値を `selectIndices` に渡し、0 件なら `no valid accounts selected` を返す。この分岐を通るテストが無い。

### 問題 2: アカウントごとのロール取得のループ

ロールが 0 件のアカウントは `continue` でスキップする。`ListSSOAccountRoleNames` の失敗は `list account roles for %s: %w` でラップする。どちらも未検証である。

### 問題 3: ロールを 1 つも選ばなかった場合

すべてのアカウントでロールが選ばれなければ `no roles selected for any accounts` を返す。未検証である。

### 問題 4: 既存の設定の読み取り失敗の扱い

`readAwsConfig` の失敗は警告を表示して空文字列として続行する。この「失敗しても続ける」という判断が意図どおりかを固定しているテストが無い。

## 検証済みの範囲

context と標準入力の配線そのものは、既に別のテストで固定されている。issue 0131 で追加した 2 つの検査が `internal/cli` の本番コードを走査するためである。

- `promptSelection(ctx, ...)` の `ctx` を `context.Background()` に差し替えると `TestNoRootContextOutsideDesignatedFunctions` が落ちる (ミューテーションで確認済み)。
- `cmd.InOrStdin()` を `os.Stdin` に戻すと `TestNoContextBlindStdinReadOutsideDesignatedFunctions` が落ちる。

したがって本 issue で足りないのは、選択とロール取得の分岐そのものの検証である。

## なぜ対応が必要か

- `ssoGenerateConfig` は `$HOME/.aws/config` を書き換える。失敗の分岐を誤ると利用者の設定ファイルを壊す。分岐が未検証のまま残っている状態が問題である。
- 同じファイルの `ssoLogin` が差し込み口を持っている。同じ性質の関数の一方だけが検証できない形になっており、整合していない。
- グローバル `~/.codex/AGENTS.md` は「ロジック分岐とエラーパスは必ずテストする」と定めている。

## 修正方針

`ssoLoginDeps` と同じ形の `ssoGenerateConfigDeps` を作り、`ssoGenerateConfig` は `defaultSSOGenerateConfigDeps()` を渡して `ssoGenerateConfigWith` を呼ぶだけにする。

```go
type ssoGenerateConfigDeps struct {
	getToken     func(ctx context.Context, region, url string) (*SSOTokenCache, error)
	listAccounts func(ctx context.Context, region, token string) ([]awsinternal.SSOAccountInfo, error)
	listRoles    func(ctx context.Context, region, token, accountID string) ([]string, error)
	configPath   func() (string, error)
	writeConfig  func(path, content string) error
}
```

`ListSSOAccountInfos` と `ListSSOAccountRoleNames` の実際のシグネチャは `backend/internal/aws/` 側を確認して合わせること。

`defaultSSOGenerateConfigDeps` の全フィールドが埋まっていることは、`TestDefaultSSOLoginDepsIsFullyWired` と同じ形のテストで固定する。

## 完了条件

- `ssoGenerateConfig` が差し込み口を経由する形になっている。
- 問題 1 から問題 4 の 4 つの分岐をテストで検証している。
- `defaultSSOGenerateConfigDeps` の全フィールドが埋まっていることをテストで検証している。
- 設定ファイルへの書き込みがテストで実ファイルを触らない。
- `mise run check` が通る。

## 関連

- docs/issues/0131: 起票元。CLI コマンドへのシグナル連動 context の導入。
- docs/issues/0134: 同じ性質 (テストの差し込み口が無い) の issue。`internal/util/selecter.go` が対象。
- docs/issues/0137: `promptSelection` が空白を含む入力を捨てる問題。本 issue の差し込み口があれば `ssoGenerateConfig` 側から挙動を固定できる。
- `backend/internal/cli/sso.go` の `ssoLoginDeps`: 同じ形の差し込み口の先例。

## 解決方法

### 実装

`backend/internal/cli/sso.go` に `ssoLoginDeps` と同じ形の `ssoGenerateConfigDeps` を追加した。修正方針が示した 5 フィールド (`getToken` / `listAccounts` / `listRoles` / `configPath` / `writeConfig`) に加えて `readConfig` フィールドを追加している (方針セクションの例には無かった)。問題 4 (既存設定の読み取り失敗時の扱い) をテストで検証するには `readAwsConfig` そのものを差し替えられる必要があり、`os` パッケージへの直接依存を持つこの関数を書き込み側の差し替えだけでは再現できないため追加した。差し込み口を並べた構造体という方式自体は変えていないため、方式を保ったままの実装詳細の乖離として記録する。

- `ssoGenerateConfigDeps` (`sso.go`): `getToken` / `listAccounts` / `listRoles` / `configPath` / `readConfig` / `writeConfig` の 6 フィールド。
- `defaultSSOGenerateConfigDeps` (`sso.go`): 本番実装 (`getSSOToken` / `awsinternal.ListSSOAccountInfos` / `awsinternal.ListSSOAccountRoleNames` / `getAwsConfigPath` / `readAwsConfig` / `os.WriteFile`) を配線する。
- `ssoGenerateConfig` (`sso.go`): `ssoGenerateConfigWith(cmd, args, defaultSSOGenerateConfigDeps())` を呼ぶだけの薄いラッパーに変更した。
- `ssoGenerateConfigWith` (`sso.go`): 従来 `ssoGenerateConfig` の本体だった処理をそのまま移し、外部呼び出しを全て `deps.*` 経由に置き換えた。分岐の順序・条件・エラーメッセージの文言・書き込みパーミッション (`0600`) は変えていない。

### 完了条件との対応

- 「`ssoGenerateConfig` が差し込み口を経由する形になっている」: `git diff` で `ssoGenerateConfigWith` が唯一の実処理であることを確認し、`TestSSOGenerateConfigWith_*` 群が `deps` 経由の呼び出しを実際に駆動して通ることで裏付けた。
- 「問題 1 から問題 4 の 4 つの分岐をテストで検証している」:
  - 問題 1 (アカウント選択 0 件): `TestSSOGenerateConfigWith_NoValidAccountsSelected`
  - 問題 2 (ロール取得ループのエラーラップと 0 件スキップ): `TestSSOGenerateConfigWith_ListRolesFailureIsWrapped` (エラーラップと `errors.Is` でのチェーン保持)、`TestSSOGenerateConfigWith_AccountWithNoRolesIsSkipped` (0 件スキップの出力確認)、`TestSSOGenerateConfigWith_SkippedAccountDoesNotBlockLaterAccounts` (`continue` が後続アカウントの処理を妨げないことと `accounts[accountIndex]` / `roles[roleIndex]` の対応関係を検証)
  - 問題 3 (ロール 0 件選択): `TestSSOGenerateConfigWith_NoRolesSelectedForAnyAccount`
  - 問題 4 (既存設定の読み取り失敗): `TestSSOGenerateConfigWith_ExistingConfigReadFailureFallsBackToEmpty`
- 「`defaultSSOGenerateConfigDeps` の全フィールドが埋まっていることをテストで検証している」: `TestDefaultSSOGenerateConfigDepsIsFullyWired`
- 「設定ファイルへの書き込みがテストで実ファイルを触らない」: 全テストで `writeConfig` をダミー関数に差し替え、実ファイルへの書き込みを行わない。`configPath` もダミーの `"/dummy/config"` を返す。
- 「`mise run check` が通る」: `mise run check` を実行し、backend 全パッケージ (`internal/cli` 含む) と frontend が通過することを確認した。

### 多観点レビューの結果

- 観点2 (テストの品質) で、`TestSSOGenerateConfigWith_AccountWithNoRolesIsSkipped` が `continue` を削除するミューテーションに対して red にならないこと (実際に踏んでいるのは後段の `len(profiles) == 0` ガードであり `continue` そのものではないこと)、また複数アカウントを跨いだ `continue` の影響と `accounts[accountIndex]` / `roles[roleIndex]` のインデックス対応関係を検証するテストが無いことが優先度「高」で指摘された。`TestSSOGenerateConfigWith_SkippedAccountDoesNotBlockLaterAccounts` を追加して対応した。`continue` を外すミューテーションでこの新規テストが red になることを確認し、元に戻して `git diff --stat` で復元を確認した。
- 観点2で、`TestSSOGenerateConfigWith_NoValidAccountsSelected` のコメントが「AWS へ問い合わせる前に no valid accounts selected を返す」と実装 (`listAccounts` は既に呼ばれている) に反していたため、「ロール取得に進む前に」に修正した (優先度「低」)。
- 観点2で、`TestDefaultSSOGenerateConfigDepsIsFullyWired` のようなフィールド列挙型の nil チェックはフィールド追加への追随漏れを検知できない (優先度「中」) との指摘があったが、既存の `TestDefaultSSOLoginDepsIsFullyWired` と同じ形式であり、リポジトリ内で確立された既存パターンとの一貫性を優先し、この issue のスコープでは変更しないこととして却下した。
- 観点2で、`TestSSOGenerateConfigWith_ListRolesFailureIsWrapped` が `errors.Is` のみで `errors.As` による型復元を検証していない (優先度「低」) との指摘があったが、issue の完了条件はチェーンの保持のみを求めており、`ssoStageError` に復元すべき追加フィールドも無いため、このテストの検証範囲としては十分と判断し却下した。
- 観点2で、`newSSOGenerateConfigCmd` と既存の `newSSOLoginCmd` のフラグ設定ロジックがほぼ同一で重複している (優先度「低」) との指摘があったが、この issue の完了条件を超えるリファクタであり、グローバル規約「Premature Optimization is the Root of All Evil」に照らし、完了条件に無い変更として却下した。
- 観点1 (完了条件の充足) で、完了条件「`mise run check` が通る」について backend のみ実行し frontend の再実行で裏取りしていない (優先度「低」) との指摘があったが、変更ファイルが backend のみであることを `git diff --stat` で確認済みであり、実際には `mise run check` (frontend 込みのフルタスク) を実行して確認済みであるため却下した。
- 観点3 (堅牢性) は指摘なし。
- 観点4 (規約準拠) では、この「## 解決方法」の下書き自体の表記の誤り (全角と半角の間の半角スペース欠落、完了条件の検証方法の記述精度) が優先度「中」で指摘され、修正した (この最終版に反映済み)。
- 観点5 (回帰と整合) では、レビュー時点で `CHANGES.md` への追記が未反映だったことが優先度「中」で指摘されたが、これは close 処理 (Step 8) の一部としてこの後に反映する手順上の順序によるものであり、実装そのものの不備ではないため、close 処理の完了をもって解消済みとする。
