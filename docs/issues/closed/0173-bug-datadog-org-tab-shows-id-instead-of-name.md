# Datadog の組織タブのラベルが name ではなく id (UUID) のまま表示される

Created: 2026-09-15
Completed: 2026-09-15
Model: Claude Fable 5.1

## 症状

frontend の Datadog ビューのセッションタブ (と「＋ 組織を追加」ピッカー) に表示される組織の識別子が、表示名 (`attributes.name`) ではなく Datadog 側の組織 ID (UUID) のままになっている。issue 0172 でユーザーから「タブに表示するのは Id ではなく attributes.name にして」と要望があり、その時点では「`ListOrgs` が `attributes.name` を名前解決している実装済みの動作で要望と一致するため追加実装は不要」と判断したが、実環境では引き続き id が表示されている。

## 再現手順

1. `thief datadog auth login` または `DATADOG_API_KEY`/`DATADOG_APP_KEY` で親組織として認証済みの状態にする。
2. frontend の Datadog ビューを開く。
3. セッションタブのラベルと、「＋ 組織を追加」ピッカーの各行の名前が、`GET /api/v2/org` の `included[].attributes.name` ではなく UUID になっている。
4. API サーバのログに `datadog managed org has no matching name in the included resources; using its id as the name` の警告が組織の数だけ出る。

## 原因

`backend/internal/datadog/organizations.go` の `ListOrgs` は、`GET /api/v2/org` のレスポンスの `included[]` (SDK の `[]datadogV2.OrgData`) から `GetId()` と `Attributes.GetName()` で id → name のマップを作り、`managed_orgs.data[]` の各 id をそのマップで名前解決している。

ところが SDK (`github.com/DataDog/datadog-api-client-go/v2 v2.65.0`) の `OrgAttributes.UnmarshalJSON` (`model_org_attributes.go`) は `created_at`/`description`/`disabled`/`modified_at`/`name`/`public_id`/`sharing`/`url` の 8 フィールドをすべて必須として検証し、1 つでも欠ける (または JSON の `null` で `*string` が nil になる) と `required field <name> missing` のエラーを返す。このエラーは `OrgData.UnmarshalJSON` (`model_org_data.go`) で捕まえられ、その `OrgData` 全体が `UnparsedObject` (生の `map[string]interface{}`) に退避されたゼロ値の構造体になる。また `type` が `"orgs"` 以外の場合も `hasInvalidField` 経由で同じく `UnparsedObject` に落ちる。

`UnparsedObject` に落ちた `OrgData` は `GetId()` がゼロ値の UUID (`00000000-0000-0000-0000-000000000000`)、`Attributes.GetName()` が空文字を返すため、`ListOrgs` の名前マップには 1 件も登録されず、`managed_orgs.data[]` の全組織が「included に名前が無い」扱いになって id にフォールバックする。これが frontend で全タブが UUID 表示になる直接の原因である。

issue 0171 の実装時、テスト用フィクスチャの `included[]` が最小形 (`name` と `public_id: null` のみ) だったところ同じ現象で名前解決が失敗し、その時点で「これはフィクスチャの不備であり、実際の Datadog API レスポンスは必須フィールドを常に含む」と結論付けてフィクスチャ側だけを 8 フィールド完備に直した (docs/issues/closed/0171 の「解決方法」7 を参照)。この結論が実環境では成り立っていない (実 API のレスポンスに、SDK が必須とみなすフィールドのうち欠落または `null` になるものがある) ことが、本 issue の症状で判明した。

どのフィールドが欠けているかは、実機のレスポンスをコンテナ内から取得できない (認証情報が無い) ため本 issue の起票時点では特定できていない。ただし修正はフィールドの特定に依存しない形で行う (下記)。

## 完了条件

- `GET /api/v2/org` の `included[]` の組織が SDK の厳格な検証に通らず `UnparsedObject` に退避された場合でも、その生の JSON から `id` と `attributes.name` を読み取って名前解決できることがテストで確認できる (少なくとも「必須フィールドの欠落」「`null` の必須フィールド」「未知の `type`」の 3 パターン)。
- 生の JSON からも名前が得られない組織 (`attributes.name` が無い、空文字、`null`) は従来どおり警告ログを出して id を表示名にフォールバックする。
- SDK の検証に通らなかった組織について、どのフィールドが原因かを運用者が把握できる警告ログを出す (実環境の原因特定のため)。
- SDK の検証に通った組織 (現行のテストが検証している経路) の挙動は変えない。
- frontend 側は変更しない (`DatadogOrgSessionTabs` のタブラベルと `datadogOrgPickerItems` のピッカー名は backend の `name` をすでに使っており、backend が正しい名前を返せば直る)。
- `mise run check` (`fmt` + `lint` + `test`) が通過する。

## 修正方針

`backend/internal/datadog/organizations.go` の `ListOrgs` で名前マップを組み立てる処理を、SDK が構造体として解釈できた `OrgData` と、`UnparsedObject` に退避された `OrgData` の両方を扱うヘルパー関数に切り出す。

- `OrgData.UnparsedObject` が nil なら現行どおり `GetId()`/`Attributes.GetName()` を使う。
- `UnparsedObject` が non-nil なら、生のマップから `id` (文字列) と `attributes` (マップ) の `name` (文字列) を型アサーションで取り出す。id は現行どおり小文字へ正規化する。
- `UnparsedObject` に落ちた組織については、`attributes` を SDK の `OrgAttributes.UnmarshalJSON` に改めて通してエラー文言 (`required field description missing` 等) を得て、警告ログの属性に含める。SDK の検証結果を自前で再実装せず、SDK 自身の判定理由をそのまま記録する。
- 生の JSON からも名前が取れない組織は現行の「included に一致が無い」経路に合流して id にフォールバックする。

`organizations_test.go` の `managedOrgsBody` ヘルパーを、組織ごとに `attributes` の JSON を差し込める形に拡張し、必須フィールドの欠落、`description: null`、未知の `type` の 3 パターンで名前解決が成功するケースと、`attributes.name` 自体が無い場合に id へフォールバックするケースを追加する。

## 検討したが採らなかった案

- **SDK の生成コードを使わず `GET /api/v2/org` を自前の緩い構造体で直接デコードする案**: SDK の `ListOrgs` が担っている認証ヘッダの付与や `datadogCall` の 403 フォールバック機構との接続をそのまま使えるため、SDK 経由を維持したうえで `UnparsedObject` を読む方が変更範囲が小さい。SDK 側が厳格な検証を修正した場合も、`UnparsedObject` が nil になって現行経路に自然に戻る。
- **SDK を新しいバージョンに更新する案**: `OrgAttributes` の必須フィールド定義は Datadog の OpenAPI 仕様に由来しており、SDK の更新で変わる保証が無い。仕様と実 API の乖離は本 issue の側で吸収する。

## 解決方法

### `backend/internal/datadog/organizations.go`

修正方針の「`UnparsedObject` が non-nil なら生のマップから型アサーションで取り出す」から方式を変え、SDK のレスポンス (`datadogV2.ManagedOrgsResponse`) を `json.Marshal` で一度 JSON に戻し、`ListOrgs` が必要とする部分 (`data.relationships.current_org.data.id`、`data.relationships.managed_orgs.data[].id`、`included[].id`、`included[].attributes.name`) だけを持つ非公開の型 `managedOrgsView` で `json.Unmarshal` し直す方式にした (`managedOrgsViewOf`)。SDK の各モデルの `MarshalJSON` は `UnparsedObject` があればその生の値をそのまま書き出すため、SDK が構造体として解釈できた部分とできなかった部分を同じ経路で読める。

**乖離の理由**: 方針は `included[]` の要素単位の退避だけを想定していたが、実装時に SDK のコードを追ったところ、`included[]` の要素が `id` / `type` / `attributes` のいずれかを欠くと、その要素の `OrgData.UnmarshalJSON` がエラーを返してレスポンス全体 (`ManagedOrgsResponse`) が `UnparsedObject` に退避され、`resp.Data` がゼロ値になって組織一覧が黙って空になることが分かった。要素単位の型アサーションではこの経路を救えず、レスポンス全体の生のマップから `relationships` を辿るコードを別に書く必要が出る。JSON に戻して読み直す方式なら、退避の粒度 (要素単位 / レスポンス全体) を問わず 1 つの経路で扱え、コード量も少ない。

名前解決と id の小文字化、`IsSelf` の判定、included に名前が無い組織を警告ログ付きで id にフォールバックする挙動は従来どおり。

SDK が拒否した部分の警告ログは `warnSDKRejections` に切り出した。`included[]` の要素単位で退避された組織については、その `attributes` を SDK の `OrgAttributes` で改めてデコードして SDK 自身のエラー文言 (`required field description missing` など) を `reason` に載せる。`attributes` が SDK の要求を満たす場合は `type` が `orgs` でないこと、それも満たす場合は `id` が UUID として解釈できないことを理由として記録する (SDK はこれらの理由をエラーとして返さないため、消去法で補う)。レスポンス全体が退避された場合は、その旨の警告を 1 行出したうえで `included[]` の各要素を `OrgData` で再デコードし、SDK が拒否する要素だけ同じ形式の警告を出す。

### `backend/internal/datadog/organizations_test.go`

`managedOrgsBody` の `included` 引数を `map[string]string` (id → name) から `[]includedOrg` (id / type / attributes の JSON 断片) に変え、組織ごとに `type` と `attributes` を差し込めるようにした (map の反復順が不定だった点も slice で解消)。SDK の検証に通る 8 フィールド完備の attributes は `fullOrgAttributes` / `fullOrg` ヘルパーに集約した。

`TestListOrgs` に次の 5 ケースを追加した。

- 必須フィールド (`created_at` 等) を欠く attributes の組織でも `attributes.name` で名前解決できる。
- `description` が JSON の `null` の組織でも名前解決できる (`null` は欠落と同じく SDK に拒否される)。
- `type` が `orgs` 以外の組織でも名前解決できる。
- `attributes` キー自体が無い組織が含まれ、SDK がレスポンス全体を退避した場合でも、`current_org` / `managed_orgs` を生の JSON から読んで `IsSelf` と一覧を正しく返す (その組織自身は名前が無いので id へフォールバック)。
- 生の attributes に `name` が無い (`null`) 組織は id へフォールバックする。

### frontend

方針どおり変更なし。`DatadogOrgSessionTabs` のタブラベル (`names.get(id) ?? id`) と `datadogOrgPickerItems` のピッカー名は backend の `name` をすでに使っている。

## 完了条件の充足

- `UnparsedObject` に退避された組織の名前解決: `TestListOrgs` の「必須フィールドの欠落」「`null` の必須フィールド」「未知の `type`」の 3 ケースと、レスポンス全体が退避される 1 ケースで確認した。
- 生の JSON からも名前が得られない組織の id フォールバック: 「生の attributes に `name` が無い」ケースと既存の「included に無い」ケースで確認した。
- 原因フィールドを把握できる警告ログ: テスト実行時のログで `reason="required field created_at missing"`、`reason="required field description missing"`、`reason="resource type is not orgs"`、`reason="attributes missing"` が出ることを確認した。
- SDK の検証に通った組織の挙動は不変: 既存 5 ケース (IsSelf と小文字化、included 欠落の id フォールバック、自組織のみ、空一覧、API エラー) がそのまま通る。
- frontend は変更なし。
- `mise run check` 相当の通過: 下記。

## テスト結果

- `mise run backend:fmt`: 差分なし。
- `mise run backend:lint` (`go vet` + `staticcheck` + `govulncheck`): 指摘なし (自コードに起因する脆弱性 0 件)。
- `mise run backend:test` (`go test -race -cover ./...`): 全 18 パッケージ `ok`。`internal/datadog` のカバレッジは 91.2%。
- `mise run frontend:lint`: エラー 0、警告は本変更と無関係な既存 10 件のみ。
- `mise run frontend:test`: 88 テストファイル、926 テストすべて成功 (frontend は無変更で、件数も issue 0171 完了時と同一)。
- 補足: コンテナ内では frontend のテストが `Cannot find native binding` (`@rolldown/binding-wasm32-wasi`) で起動に失敗した。直前の vite メジャーアップデート (rolldown ベース) 以降、ホスト (macOS) で `npm install` した `node_modules` に linux 用のバインディングが無かったためで、本変更とは無関係。`mise run frontend:setup` で linux 用を導入し、ホスト側が壊れないよう darwin 用 (`@rolldown/binding-darwin-arm64@1.2.8`) も `npm install --no-save --force` で戻した。`package.json` / `package-lock.json` に変更は無い。

## 関連

- docs/issues/closed/0171 (`GET /api/v2/org` への切り替え。「解決方法」7 でフィクスチャ側の同じ現象を記録し「本番コードの不具合ではない」と結論付けていた)
- docs/issues/closed/0172 (ユーザーの元の要望「タブに表示するのは Id ではなく attributes.name にして」。実装済みと判断して追加対応を見送っていた)
