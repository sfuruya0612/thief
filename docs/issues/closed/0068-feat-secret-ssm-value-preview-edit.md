# Secrets Manager / Parameter Store の値編集を一覧非表示 + Preview からの編集に変更する

Created: 2026-07-23
Completed: 2026-07-23
Model: Claude Opus 4.8

## 背景

issue 0065 で Secrets Manager / SSM Parameter Store の値を Web UI・API・CLI から更新できるようにした。
このとき Web UI では、一覧レスポンスに復号済みの平文値を含め、一覧テーブルの Value 列と Drawer Overview の Value 行に値を表示し、Drawer の「Edit」タブを常時編集モードにした。

`docs/issues/TODO.md` に「docs/issues/closed/0065 の延長で、Secret Manager, Parameter Store の Value の編集の導線を変えたい。一覧には Value を出さず、S3 のように Preview 表示から Edit, Close ボタンで編集ができるようにしたい」という要望がある。

一覧に平文値を常時含める現状は、機密値が backend の 1 時間キャッシュとフロントの react-query キャッシュに常に平文で載る点でも望ましくない。

## 目的

Secrets Manager / SSM Parameter Store の値を、一覧に出さず、Drawer で値をプレビュー表示してから編集する導線に変更する。S3 オブジェクトの Preview から Edit と同じ操作感にする。

## 設計判断

- 一覧レスポンスから値を外す。`SecretResource` / `SSMParameterResource` の `Value` フィールドを削除し、一覧取得時の per-item / batch な値取得 (GetSecretValue / GetParameters) をやめる。これにより一覧が軽くなり、機密値がキャッシュに常時載ることもなくなる。
- 値は Drawer を開いたときにオンデマンドで取得する。取得エンドポイントは name をクエリで受ける (name は階層名を含みうるため。0065 の更新系が name をボディで受けるのと同じ理由)。
  - `GET .../secretsmanager/value?name=...` (新規)
  - `GET .../ssm/parameters/value?name=...` (新規、既存の `aws.GetSSMParameter` を復号ありで再利用)
- Drawer の値タブを S3 の `DrawerObjectPreview` と同じ Preview / Edit / Close 構成に作り替える。初期はプレビュー (read-only) で、「Edit」で編集、「Cancel」で取り消し、「Save」で上書き確認の上更新、「Close」で Drawer を閉じる。
- 一覧テーブルの Value 列 (`columns.tsx` の ssmColumns / secretColumns) と Overview の Value 行 (`overviewRows.tsx`) を削除する。
- タブ名を「Edit」から「Value」に変更する (参照が主で編集は副次的になるため)。
- 既存の未使用エンドポイント `GET .../ssm/parameters/{name}` (handleSSMGet、フロント未使用・階層名非対応) は破壊的変更を避けるため残置する。

## 完了条件

- backend: 一覧から値を外し、値取得エンドポイント (secretsmanager/value、ssm/parameters/value) を追加する。
- frontend: Value 列・Overview の Value 行を削除し、値タブを Preview / Edit / Close 構成にする。値はオンデマンド取得にする。
- 日本語・英語の UI 文字列を i18n に追加する。
- `mise run check` が全て通過する。

## 解決方法

### backend

- `SecretResource` / `SSMParameterResource` から `Value` フィールドを削除した。
- `ListSecretResources` の per-item な `getSecretValue` 呼び出し (N+1) と、`ListSSMParameters` の `fillSSMValues` (GetParameters のバッチ呼び出し) を廃止し、一覧はメタデータのみを返すようにした。これにより機密値が一覧キャッシュに常時載ることもなくなった。
- 値のオンデマンド取得のため、非公開だった `getSecretValue` を公開関数 `GetSecretValue(ctx, profile, region, name)` に変更し、SSM は既存の `GetSSMParameter` を復号ありで再利用した。
- 値取得エンドポイント `GET .../secretsmanager/value?name=` (handleSecretValue) と `GET .../ssm/parameters/value?name=` (handleSSMValue) を追加した。name は階層名を含みうるためクエリで受ける。レスポンスは `ValueResponse` ({value})。
- 既存の未使用エンドポイント `GET .../ssm/parameters/{name}` (handleSSMGet) は破壊的変更を避けるため残置した。
- CLI の一覧テーブル出力 (`torow.go`) から Value 列を除いた。

### frontend

- `SSMParamRaw` / `SSMParamRow` / `SecretRaw` / `SecretRow` から `value` を削除し、`ssmFromRaw` / `secretFromRaw` の値マッピングを外した。
- 一覧テーブルの Value 列 (`columns.tsx` の ssmColumns / secretColumns) と Overview の Value 行 (`overviewRows.tsx`) を削除し、列幅を再配分した。
- Drawer のタブ名を「Edit」から「Value」に変更した (`Drawer.tsx`)。
- `DrawerValueEditor` を S3 の `DrawerObjectPreview` と同じ Preview / Edit / Close 構成に作り替えた。初期はプレビュー (read-only) で、「編集」で textarea 編集に切り替え、保存前に上書き確認を挟み、「Close」で Drawer を閉じる。
- 値は `useSecretValue` / `useSSMValue` (新規、`GET .../value` を叩く) でオンデマンド取得する。機密値をキャッシュに常時載せないため staleTime を設けず、タブを開くたびに取得する。
- 更新成功後は一覧クエリ (メタデータ) と値クエリの両方を invalidate して最新化する。
- i18n (`drawerAws.json` の ja / en) の valueEditor に edit / cancel を追加し、title を「値」/「Value」に変更した。

### test

- `DrawerValueEditor.test.tsx` を新しいプレビュー → 編集フローに合わせて書き直した (Close ボタンと onClose のテストを追加)。

### 確認

- `mise run check` (backend / frontend の fmt + lint + test) が全て通過することを確認した。
