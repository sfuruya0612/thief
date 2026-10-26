# TUI の Secrets Manager / Parameter Store に読み取り専用の Value タブを追加する

Created: 2026-10-26
Model: DeepSeek V4.1 Flash 1017

## 背景

`docs/issues/TODO.md` の次の項目に由来する。

> TUI の Drawer の Secrets Manager / Parameter Store の Value タブで、値を読み取り専用で表示し、Secrets Manager と SecureString は初期マスクして `v` で取得 / 表示できるようにしたい

issue 0226 は Drawer のタブ基盤を追加するが、Secrets Manager と Parameter Store の Drawer には `Overview` と `Tags` のタブしかない。Web UI の `frontend/src/components/Drawer/DrawerSecretEdit.tsx` と `DrawerSSMEdit.tsx` は `Value` タブを持ち、`backend/internal/aws/secretsmanager.go` の `GetSecretValue` と `backend/internal/aws/ssm.go` の `GetSSMParameter` で復号済みの値を取得して `DrawerValueEditor` に渡す。

Web UI の Value タブは値を編集できるが、TUI の第 1 段階はリソースの変更操作を扱わない。値をそのまま表示すると、端末のスクロールバックに秘密情報が残る。TUI では読み取り専用の表示に限定し、Secrets Manager と SecureString は初期状態でマスクする。

## 目的

TUI の Secrets Manager と Parameter Store の Drawer に `Value` タブを追加し、メタデータと値を読み取り専用で表示する。Secrets Manager と SecureString の値は初期状態で隠し、`v` を押したときだけ取得して表示する。もう一度 `v` を押すと再び隠す。

## 設計判断

- Value タブは `valueTab` として実装する。メタデータ、値の取得状態、表示 / 非表示の状態、値の取得を開始したかどうかをタブ内で持つ。この issue は issue 0226、0227、0228、0229 の完了後に実装し、最後に TUI の全タブの並びを `DRAWER_TABS` と突き合わせる。
- メタデータは一覧の行から作る。
  - Secrets Manager は Name と Description を表示する。
  - Parameter Store は Name、Type、Tier、Version を表示する。
  - 値の取得に失敗してもメタデータは表示する。
- Secrets Manager と SecureString の値の取得は、タブを開いたときではなく最初に `v` を押したときに行う。`GetSecretValue(ctx, profile, region, name)` または `GetSSMParameter(ctx, profile, region, name, true)` を呼ぶ。
  - Secrets Manager と Parameter Store の SecureString は、取得前は `Press v to reveal` を表示する。
  - 取得後は値を表示し、`v` で `Hidden; press v to reveal` に戻す。再度 `v` を押すと保持している値を再表示し、API を呼び直さない。`v` で再び隠せるようにするのは、値を表示したまま端末を離れたときに秘密情報を見せ続けないためである。
  - Parameter Store の String と StringList は秘密情報として扱わず、タブを開いたときに取得して表示する。`v` は String と StringList では何もしない。
- 値の表示は `viewport` を使い、改行を保ったままスクロールできるようにする。長い値でも Drawer の内側に収め、上下キー、`j` / `k`、PageUp / PageDown、Home / End でスクロールする。
- 値の編集、保存、上書き確認、コピーは扱わない。
- 値はログと TUI が組み立てるエラーメッセージに含めない。取得失敗の理由は取得関数が返したエラー文字列をそのまま表示し、TUI は値を受け取った後に値をエラーへ連結しない。既存の `GetSecretValue` と `GetSSMParameter` のエラーはシークレット名と AWS SDK のエラーだけを含み、値そのものを含まない。`GetSecretValue` は `SecretString` だけを返し、`SecretBinary` は扱わない。バイナリのシークレットは空文字列として表示されることを既知の制約とする。
- タブを開いた時点で取得する案は、Secrets Manager と SecureString では採らない。値を取得する API 呼び出しを `v` の明示的な操作まで遅らせ、端末に秘密情報を出す操作を利用者に選ばせるためである。`v` をタブの外のグローバルキーにする案は、Value タブ以外で誤って押したときに別のタブの値を表示するおそれがあるため採らない。
- `v` は Value タブが処理し、グローバル操作へ流さない。Value タブにはフォーム入力がないため、`q` や数字は従来どおりグローバル操作に届く。
- 取得はタブの context と `loadSeq` で世代管理し、タブ切替、行変更、`r` の再取得の後に届いた古い値を表示にも保持にも使わない。
- 追加の API 呼び出しは `GetSecretValue` と `GetSSMParameter` で、Web UI が既に使う権限の範囲に収まる。新しい IAM 権限は要らない。
- Parameter Store の Type は `SSMParameterResource.Type` の生の値を使い、`SecureString` の比較は大文字と小文字を区別しない。Secrets Manager は常にマスク対象にする。
- PRD の FR-5 は値を Value タブで取得すると定めているため、TUI の Secrets Manager と SecureString は `v` を押すまで取得しない例外を FR-5 に追記する。取得した値は Value タブを開いている間だけタブ内に保持し、タブ切替、行変更、`r` の再取得で破棄する。backend のキャッシュには載せない。この保持期間を FR-5 と非機能要求のキャッシュの記述に追記する。
- この issue では値の編集、TUI からの SSO 再ログイン、値のコピー、複数行の整形、`SecretBinary` の表示を扱わない。

## 完了条件

- TUI の Secrets Manager と Parameter Store の行の Drawer に `Value` タブがあり、Secrets Manager は Name と Description、Parameter Store は Name、Type、Tier、Version を表示する。空の Description と Tier は `—` で表示する。
- Secrets Manager の Value タブは、タブを開いた時点で値を取得せず、`Press v to reveal` を表示する。`v` を押すと `GetSecretValue` を呼び、値が表示される。もう一度 `v` を押すと `Hidden; press v to reveal` に戻り、再度 `v` を押すと API を呼ばずに保持している値を表示する。
- Parameter Store の SecureString は Secrets Manager と同じく初期マスクで、`v` で取得 / 表示 / 非表示を切り替える。String と StringList はタブを開いたときに `GetSSMParameter` を呼び、値を表示する。String と StringList では `v` を押しても表示は変わらない。
- 値の取得中、取得失敗、空文字列、表示中、非表示の各状態が区別して表示される。取得失敗時もメタデータは残る。
- 値は改行を保ったまま表示され、上下キー、`j` / `k`、PageUp / PageDown、Home / End でスクロールできる。
- タブ切替、行変更、`r` の再取得の後に届いた古い値が表示にも保持にも使われない。
- 値はログと TUI が組み立てるエラーメッセージに含まれない。TUI は値を受け取った後に値をエラーへ連結しない。既存の `GetSecretValue` と `GetSSMParameter` が値を含まないエラーを返すことを前提とし、その前提をコードの読み合わせで確認する。
- `backend/internal/tui` の単体テストに、Secrets Manager の初期マスク、`v` の取得と再表示、Parameter Store の SecureString と String の分岐、String で `v` が何もしないこと、取得失敗、空文字列、スクロール、古い結果の破棄、値がエラー表示に混入しないことの各ケースがある。
- `backend/internal/cli/tui.go` の `newTUIDeps` が Secrets Manager と Parameter Store のタブに `aws.GetSecretValue` と `aws.GetSSMParameter` を渡していることを `backend/internal/cli/tui_test.go` で確認している。
- `docs/prd/thief.md` の FR-5 に、TUI の Secrets Manager と SecureString は Value タブで `v` を押すまで取得しない例外が追記されている。取得した値は Value タブを開いている間だけ保持し、タブ切替、行変更、`r` の再取得で破棄し、backend のキャッシュには載せないことが FR-5 と非機能要求に追記されている。FR-5 の受け入れ基準に、TUI の Value タブが初期マスクを表示し、`v` で取得 / 表示 / 非表示を切り替えることを確認する項目が追加されている。
- `docs/prd/thief.md` の FR-29 の全タブの受け入れ基準に、19 サービスの `TabProvider.Tabs()` のタイトル列が `DRAWER_TABS` と一致し、ECR では `Tags` を表示しないことを確認する項目が追加されている。この確認は issue 0227、0228、0229 のタブが揃った後に行う。
- `example/README.md` に、floci の `thief/example/db-credentials` と `/thief/example/db-password` で Value タブを開き、初期マスクと `v` の取得 / 非表示を確認する手順が追加されている。floci が返す値の型と、SecureString の扱いを明記する。
- `CHANGES.md` の `## develop` に、TUI の Value タブの [ADD] エントリが 1 つある。担当者行は `- @sfuruya0612` とする。
- `mise run check` が通る。
- 扱わない範囲: 値の編集と保存、上書き確認、値のコピー、TUI からの SSO 再ログイン、値の複数行の整形、`SecretBinary` の表示。

## 関連

- PRD `docs/prd/thief.md` の FR-29、FR-5
- `docs/issues/0226`
- `frontend/src/components/Drawer/DrawerSecretEdit.tsx`、`DrawerSSMEdit.tsx`、`DrawerValueEditor.tsx`
- `backend/internal/aws/secretsmanager.go`、`backend/internal/aws/ssm.go`
