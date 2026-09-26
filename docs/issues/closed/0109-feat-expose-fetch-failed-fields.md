# 取得失敗で縮退した AWS リソースのフィールドを API レスポンスと UI で明示する

Created: 2026-08-05
Model: Claude Fable 5
Completed: 2026-08-05

## 背景

docs/issues/0091 の調査で決定した設計を実装する。
設計の経緯と採らなかった案の詳細は docs/issues/0091 の「決定した設計」にあるが、本 issue は単体で実装に着手できるよう必要な事実を再掲する。

`handleIgnoredErr` (`backend/internal/aws/errors.go:84-93`) は、権限不足やスロットリング等で詳細値の取得に失敗したとき、slog.Warn に記録した上で 0 件または空に縮退させ、一覧取得全体は成功させる。
この縮退方針自体は維持する (docs/issues/closed/0088 で確定済み)。
問題は、縮退した値が真の 0 と見た目上区別できないことである。

`handleIgnoredErr` の呼び出し箇所は次の 10 箇所で、構造体とフィールドの組では 7 つになる。

| # | 箇所 | リソース種別 | 縮退するフィールド | JSON タグ |
| --- | --- | --- | --- | --- |
| 1 | `internal/aws/sqs.go:89` | SQS | `Tags` | `tags` |
| 2 | `internal/aws/waf.go:164` | WAF (REGIONAL) | `AssociatedCount` | `associated_count` |
| 3 | `internal/aws/waf.go:179` | WAF | `Tags` | `tags` |
| 4 | `internal/aws/waf.go:233` | WAF (CLOUDFRONT スコープ、クライアント生成失敗) | `AssociatedCount` | `associated_count` |
| 5 | `internal/aws/waf.go:240` | WAF (CLOUDFRONT スコープ、ListDistributions) | `AssociatedCount` | `associated_count` |
| 6 | `internal/aws/iam.go:131` | IAM (User) | `MFAEnabled` | `mfa_enabled` |
| 7 | `internal/aws/iam.go:142` | IAM (User) | `Groups` | `groups` |
| 8 | `internal/aws/iam.go:153` | IAM (User) | `Policies` | `policies` |
| 9 | `internal/aws/iam.go:195` | IAM (Role) | `Policies` | `policies` |
| 10 | `internal/aws/dynamo.go:99` | DynamoDB | `Tags` | `tags` |

縮退値の性質のうち実装に影響するものは次のとおり。

- WAF の REGIONAL スコープは、リソース種別ごとの `ListResourcesForWebACL` (`waf.go:153-171`) の一部だけが失敗すると、合算 (`sumResourceARNs`、`waf.go:218-224`) が失敗分を 0 と数え、値はあるが過小になる。
- IAM の `MFAEnabled` は初期値 false のまま残り (`iam.go:128-131`)、「MFA 無効」と同じ値になる。一覧列 (`columns.tsx:1417-1426`) は無効を赤のバツで表示するため、取得失敗が「無効」の断定として見える。
- SQS の `Tags` は失敗時に空 map (`sqs.go:85`)、成功時に SDK の戻り値をそのまま代入する (`sqs.go:88`)。SDK が nil の map を返すと成功時だけ JSON が null になり、失敗時 (`{}`) と表現が逆転し得る。

## 目的

取得に失敗して縮退したフィールドを API レスポンスのフラグで機械可読にし、UI の一覧と Drawer で「取得できなかった」ことをユーザーが判別できるようにする。

## 設計判断

方式は docs/issues/0091 で決定済みである。
採らなかった案 (汎用の `fetch_warnings: string[]`、null による表現、グレーアウトのみ、DrawerError 方式の一覧拡張) と却下理由は docs/issues/0091 の「決定した設計」を正とし、ここでは要点だけ記す。
`fetch_warnings` は文字列キーと表示列の緩い結合になり型検査で追随できないこと、null 表現は IAM の nil スライスが成功 0 件と同形で部分失敗も表現できないこと、グレーアウトは 0 の誤読を解消しないこと、DrawerError はサブタブ全体の失敗用でフィールド単位と粒度が合わないことが理由である。
新しい AWS API の呼び出しと権限の追加は無い。

backend のフラグ追加と frontend の警告表示は、docs/issues/0091 の「対象範囲」の決定に従い分割しない。
`omitempty` の後方互換により backend だけを先に close することは可能だが、フラグは表示されて初めて目的 (取得失敗による 0 と真の 0 の判別) を満たすため、backend 単独の close は利用者に届かない中間状態を作るだけである。
フラグの命名と意味の調整は表示側の実装から生じやすく、同一 issue で扱うことで両者の食い違いによる手戻りを防ぐ。

backend の変更は次のとおり。

- 4 構造体に `omitempty` 付きの bool フィールドを 7 つ追加する。
  - `WAFResource`: `associated_count_fetch_failed`、`tags_fetch_failed`
  - `IAMResource`: `mfa_enabled_fetch_failed`、`groups_fetch_failed`、`policies_fetch_failed`
  - `SQSResource`: `tags_fetch_failed`
  - `DynamoResource`: `tags_fetch_failed`
- フラグの意味は「対象フィールドの取得の一部または全部に失敗し、表示値が不完全または既定値である」とする。WAF の REGIONAL スコープの部分失敗 (値はあるが過小) も true にする。
- `omitempty` により、失敗が無いレスポンスは現在と同一形状になる (後方互換)。
- 呼び出し側が縮退の発生を機械的に判定できるよう、`handleIgnoredErr` を包む形で「縮退が起きたか (bool) と伝播すべきエラー」を返すヘルパーを `errors.go` に追加し、10 箇所の呼び出しをこのヘルパー経由に置き換えてフラグへ代入する。キャンセル (context の打ち切り) は従来どおりエラーとして伝播し、フラグを立てる縮退と混同しない。ヘルパーに単体テストを書く。
- WAF の REGIONAL スコープは、リソース種別ごとのループ (`waf.go:153-171`) の中で縮退が複数回起き得るため、各回のヘルパーの戻り値を OR で集約して ACL の `associated_count_fetch_failed` に代入する。
- エラーが nil のまま成功分岐に入らないケース (戻り値が nil の場合。WAF のタグではガードが複合条件のため、戻り値の `tagsOut` は非 nil でも `TagInfoForResource` フィールドが nil の場合を含む。いずれも SDK の契約上は想定しない) は、REGIONAL スコープ (`waf.go:162` の `resOut != nil` ガード) だけでなく、WAF のタグ (`waf.go:177` の `tagsOut != nil && tagsOut.TagInfoForResource != nil` ガード) と SQS のタグ (`sqs.go:87` の `tagsOut != nil` ガード) にも同じ形で存在する。フラグの判定は「成功分岐 (取得した値を代入する分岐) に入らなかったこと」で統一し、この 3 箇所ではエラーが nil でも成功分岐に入らなければフラグを true にする。`handleIgnoredErr` はエラーが nil のとき何もせず nil を返すため (`errors.go:85-86`)、このケースはヘルパーでは検出できず、呼び出し側の分岐で直接フラグを立てる。IAM の 4 箇所 (Role のポリシー取得 `iam.go:195` を含む) と DynamoDB は成功条件がエラーの nil 判定だけで戻り値をガードしないため、このケースは生じない。
- WAF の CLOUDFRONT スコープ (`waf.go:226-249` の `applyCloudFrontAssociatedCounts`) は、失敗が対象の全 ACL に及ぶ。関数の戻り値を `error` から `(bool, error)` (縮退の発生と伝播すべきエラー) に変え、呼び出し元で bool が true のとき CLOUDFRONT スコープの全 ACL に `associated_count_fetch_failed` を一括代入する。ページ取得の途中で失敗した場合に、取得済みのページの集計を使わず全 ACL を 0 のままにする既存の挙動は変えず、フラグの付与だけを加える。この一括代入は、既存の `applyCloudFrontCounts` (`waf.go:267-271`) が生成後の ACL の `AssociatedCount` を直接書き換えるのと同じ、生成後のフィールド代入で行う。CLOUDFRONT スコープの `associated_count_fetch_failed` は `newWAFResource` の引数には含めない。`newWAFResource` のシグネチャ変更が運ぶのは、REGIONAL スコープでのみ判定する `associated_count_fetch_failed` と、両スコープの ACL で判定する `tags_fetch_failed` である。タグ取得 (`waf.go:172-181`) は関連カウントと違いスコープの分岐を持たず REGIONAL と CLOUDFRONT の両方で実行されるため、`tags_fetch_failed` は CLOUDFRONT スコープの ACL でも `newWAFResource` 経由で設定する。
- フラグの受け渡しに伴い、`newWAFResource` (`waf.go:195`)、`newIAMUserResource` (`iam.go:165`)、`newIAMRoleResource` (`iam.go:207`) のシグネチャ変更が必要になる。直接の呼び出し元とそのテストの追随はスコープ内とする。
- `IAMResource` は User と Role が共用する構造体だが、Role には MFA とグループの概念が無く、`newIAMRoleResource` は `MFAEnabled` と `Groups` を設定しない。Role では取得自体を試みないため、`mfa_enabled_fetch_failed` と `groups_fetch_failed` は `newIAMRoleResource` の引数に含めず、Role の行では常に false (JSON にキーが現れない) のままとする。`newIAMRoleResource` に加える引数は `policies_fetch_failed` だけである。UI の警告も Role の行の MFA と Groups には現れない。
- `sqs.go:88` の成功パスを WAF や DynamoDB と同じ非 nil の map に正規化し、`SQSResource.Tags` の JSON が null にならないようにする。これはフラグの `omitempty` による後方互換とは別の、成功時のレスポンス表現の変更 (SDK が nil の map を返した場合の null が `{}` になる) であり、独立した判断としてここに明記する。別 issue に切り出さず本 issue に含めるのは、変更対象が本 issue と同じ `SQSResource.Tags` の同じ代入箇所であり、フラグが示す「失敗時は空」の表現は成功時の表現が非 nil に揃って初めて失敗時と区別できるため、分離すると本 issue の完了条件が成立しないからである。
- IAM の `Groups` と `Policies` の nil スライスは正規化しない。成功 0 件と失敗が同じ null になる曖昧さはフラグが解消するうえ、null を `[]` に変えると既存のレスポンス形状 (frontend の `IAMRaw` は `string[] | null` を許容する、`aws.ts:488-489`) を無用に変えるためである。SQS だけを正規化するのは、同一フィールドの JSON 表現が成功時と失敗時で逆転し得るという SQS 固有の不整合を解消するためである。
- `dynamo.go:92-99` で `TableArn` が nil の場合はタグの取得を試みないままとし、フラグを立てない。取得の失敗ではないためである。

frontend の変更は次のとおり。

- Raw 型 (`types/aws.ts` の `WAFRaw`、`IAMRaw`、`SQSRaw`、`DynamoRaw`) にフラグを optional の boolean で追加し、Row 型と `lib/normalize.ts` の変換で `?? false` により必須の boolean に正規化する。
- フィールド単位の警告表示プリミティブ (alertTriangle アイコン + title 属性のツールチップ) を 1 つ作り、一覧列と Drawer で共用する。コンポーネント名は `FetchFailedWarning` とし、既存のプリミティブと同じ `components/primitives/` に置く。alertTriangle は `Icons.tsx:385` に定義済み (現在の利用は `ErrorBanner.tsx:17` のみ)。title 属性によるツールチップは `DrawerECSTasks.tsx:120` に i18n 込みの先例がある。
- 一覧列では、該当列 (WAF の Associated `columns.tsx:1358-1363`、IAM の MFA `columns.tsx:1417-1426`、IAM の Policies と Groups `columns.tsx:1434-1447`) の値の隣に警告を表示する。IAM の MFA はフラグが立った行では赤のバツを表示せず警告アイコンにする。
- Drawer では `overviewRows.tsx` の該当行 (wafOverviewRows 245-254 行、iamOverviewRows 167-177 行) に同じ警告を付す。`DrawerTags` はフラグが立った場合「Tags (0)」ではなく取得失敗の明示表示にする (docs/issues/closed/0075 の「エラーは空表示ではなく明示する」規則の適用)。
- `Drawer.tsx:277` はサービス共通の `BaseRow` (`common.ts:97-103`) 越しに `<DrawerTags tags={resource.tags} />` を描画するため、`BaseRow` に optional の `tagsFetchFailed` を追加し、WAF と SQS と DynamoDB の Row がこれを埋める。`DrawerTags` に optional の `fetchFailed` prop を新設し、`Drawer.tsx` から渡す。`DrawerCFNTags` は別 API から取得する CloudFormation スタックのタグ表示であり、本 issue の対象外である。
- `Tags`/`Groups`/`Policies`/`MFAEnabled` の一覧列は新設しない。現状の表示面に警告を重ねるだけとする。
- ツールチップの文言は ja の i18n リソースに新規追加する。AWS 由来メッセージの転記ではないため、docs/issues/closed/0066 の英語ハードコード方針の対象外である。

## 完了条件

- 上記 4 構造体に `omitempty` 付きの 7 つのフラグが追加されている。
- ヘルパーの単体テストが、縮退時に true を返すことと、キャンセルをエラーとして伝播すること (フラグを立てないこと) を検証する。
- 10 箇所全てについて、各サービスのテスト (`sqs_test.go`、`waf_test.go`、`iam_test.go`、`dynamo_test.go`) がエラー注入で縮退を起こし、対応するフラグが true になることを検証する。
- WAF の REGIONAL スコープは、一部のリソース種別だけが失敗するケースでもフラグが true になることを検証する。
- エラーが nil のまま成功分岐に入らないケースを持つ 3 箇所 (WAF の REGIONAL スコープ、WAF のタグ、SQS のタグ) は、エラーと戻り値がともに nil のケースでフラグが true になることを検証する。WAF のタグはガードが複合条件のため、`tagsOut` が nil のケースに加え、`tagsOut` は非 nil だが `TagInfoForResource` が nil のケースでもフラグが true になることを検証する。
- WAF の CLOUDFRONT スコープは、クライアント生成の失敗 (`waf.go:233` の経路) と `ListDistributions` のページ取得の途中失敗 (`waf.go:240` の経路) のそれぞれについて、CLOUDFRONT スコープの全 ACL の `associated_count` が 0 のままであることと、フラグが true になることを検証する。
- WAF のタグ取得はスコープの分岐を持たないため、CLOUDFRONT スコープの ACL でもタグ取得の失敗で `tags_fetch_failed` が true になることを検証する。
- DynamoDB は、`TableArn` が nil でタグの取得を試みないケースで、フラグが false のままであることを検証する。
- IAM の Role は、`mfa_enabled_fetch_failed` と `groups_fetch_failed` が常に false のままであることを検証する。
- JSON エンコードのテストが、7 つのフラグ全てについて、false のときキーがレスポンスに現れず、true のとき現れることを検証する。
- `SQSResource.Tags` が成功時も失敗時も JSON で null にならないことをテストが検証する。
- frontend の Raw、Row、normalize がフラグを受け取り、`?? false` で正規化している。normalize のユニットテストが、フラグが省略された Raw から false の Row が生成されることと、true の Raw から true の Row が生成されることを検証する。
- 警告表示の全箇所が、`components/primitives/` に新設した `FetchFailedWarning` コンポーネントを import して描画している (grep で `FetchFailedWarning` を検索して確認できる)。
- 一覧列 (WAF の Associated、IAM の MFA と Policies と Groups) と Drawer (overviewRows の該当行、DrawerTags) に警告表示がある。
- DrawerTags はフラグが立った場合、「Tags (0)」の件数表示の代わりに取得失敗を示す i18n の文言を表示する。
- ツールチップの文言が ja の i18n リソースに追加されている。
- 警告表示のある全ての表示箇所について、コンポーネントテストが、フラグが立った行で警告が表示され、立っていない行では表示されないことを検証する。IAM の MFA は、フラグが立った行で赤のバツが表示されないことも検証する。
- 実 AWS 環境での縮退の再現 (権限剥奪等) は本 issue の完了条件に含めない。検証はテストで行う。
- `mise run check` が通る。

## 関連

- docs/issues/0091: 設計の決定元。棚卸しと採らなかった案の詳細を持つ。
- docs/issues/0110: 本 issue が追加するフィールドを契約テストの対象に含める後続 issue。本 issue の完了後に行われ、フィールドの追加や改名はゴールデンの再生成を伴う。
- docs/issues/closed/0088: 縮退方針を維持することの確定元。
- docs/issues/closed/0075: 「エラーは空表示ではなく明示する」表示規則の確立元。

## 解決方法

設計判断のとおり実装した。

### backend

- `backend/internal/aws/errors.go` に `handleIgnoredErrFlag(err error, msg string, attrs ...any) (degraded bool, propagated error)` を追加した。縮退(エラーを無視して 0 件・空に倒す)が起きたかを bool で返しつつ、キャンセル (`context.Canceled` を含むラップされたエラー) は従来どおり `propagated` として呼び出し元に伝播させ、縮退フラグは立てない。単体テスト (`errors_test.go`) でキャンセル時に `degraded=false` かつエラーがそのまま伝播することを確認した。
- `WAFResource` に `AssociatedCountFetchFailed` / `TagsFetchFailed`、`IAMResource` に `MFAEnabledFetchFailed` / `GroupsFetchFailed` / `PoliciesFetchFailed`、`SQSResource` に `TagsFetchFailed`、`DynamoResource` に `TagsFetchFailed` を `omitempty` 付きの bool フィールドとして追加した (計 7 フィールド)。
- `waf.go` / `iam.go` / `sqs.go` / `dynamo.go` の該当 10 箇所すべてを `handleIgnoredErrFlag` 経由に置き換えた。
  - WAF の REGIONAL スコープはリソース種別ごとのループ内で複数回起こり得る縮退を OR で集約して `AssociatedCountFetchFailed` に代入する。
  - エラーが nil のまま成功分岐に入らないケース (WAF の REGIONAL スコープの `resOut` nil ガード、WAF タグの `tagsOut` / `TagInfoForResource` nil ガード、SQS タグの `tagsOut` nil ガード) は `handleIgnoredErrFlag` では検出できないため、呼び出し側の分岐で直接フラグを立てる実装にした。
  - WAF の CLOUDFRONT スコープ (`applyCloudFrontAssociatedCounts`) は戻り値を `error` から `(bool, error)` に変更し、クライアント生成失敗と `ListDistributions` のページ取得失敗のいずれでも、CLOUDFRONT スコープの全 ACL に `AssociatedCountFetchFailed` を一括代入するようにした。
  - `newWAFResource` / `newIAMUserResource` / `newIAMRoleResource` のシグネチャにフラグ引数を追加し、直接の呼び出し元とテストを追随させた。`newIAMRoleResource` は設計判断のとおり `MFAEnabledFetchFailed` と `GroupsFetchFailed` を持たず、常に false (JSON にキーが現れない) のままにした。
  - `sqs.go` のタグ取得の成功パスを、SDK が nil map を返した場合でも非 nil の空 map に正規化するよう変更した。これにより `SQSResource.Tags` は成功時・失敗時のいずれも JSON で null にならない。
  - `dynamo.go` は `TableArn` が nil のときフラグを立てないままとした (取得を試みていないため縮退ではない)。
- `waf_test.go` / `iam_test.go` / `sqs_test.go` / `dynamo_test.go` に、10 箇所それぞれの縮退シナリオ (WAF REGIONAL の一部リソース種別のみ失敗、WAF タグの `tagsOut`/`TagInfoForResource` nil、WAF CLOUDFRONT のクライアント生成失敗と `ListDistributions` 途中失敗、IAM Role の MFA/Groups フラグが常に false、DynamoDB の `TableArn` nil 時にフラグを立てない、等) と、JSON エンコードで 7 フラグが `omitempty` により false 時は非表示・true 時は表示されることを検証するテストを追加した。

### frontend

- `types/aws.ts` の `WAFRaw` / `IAMRaw` / `SQSRaw` / `DynamoRaw` と対応する Row 型に 7 フラグを optional boolean として追加し、`lib/normalize.ts` の `wafFromRaw` / `iamFromRaw` / `sqsFromRaw` / `dynamoFromRaw` で `?? false` により必須 boolean に正規化した。
- `components/primitives/FetchFailedWarning.tsx` を新設し、警告アイコン (`Icons.alertTriangle`) と日本語ツールチップ (title 属性) を一覧列・Drawer で共用できるようにした。ツールチップの文言は `i18n/locales/ja/drawerAws.json` に追加した。
- `components/tables/columns.tsx` の WAF の Associated 列、IAM の MFA / Policies / Groups 列に `FetchFailedWarning` を組み込んだ。IAM の MFA はフラグが立った行で赤のバツ表示を出さず警告アイコンに置き換える。
- `components/Drawer/overviewRows.tsx` の該当行 (WAF の Associated resources、IAM の MFA / Policies attached / Groups) にも同じ警告を表示した。`OverviewEntry` の値型は変更前から既に `ReactNode` であり、今回の変更で型を変えてはいない。
- `components/Drawer/DrawerTags.tsx` に optional の `fetchFailed` prop を追加し、フラグが立った場合は「Tags (0)」の件数表示の代わりに取得失敗を示す文言と警告アイコンを表示する。`Drawer.tsx` は `BaseRow` に追加した optional の `tagsFetchFailed` を経由してこれを渡す。`DrawerCFNTags` (CloudFormation スタックのタグ) は対象外のまま変更していない。
- `lib/normalize.test.ts` に `wafFromRaw` / `iamFromRaw` / `sqsFromRaw` / `dynamoFromRaw` それぞれのフラグ省略時 (false 既定) と true 引き継ぎのテストを追加し、`FetchFailedWarning.test.tsx`、`columns.test.tsx` の `iamColumns` ブロックと `wafColumns` の追加テスト、`overviewRows.test.tsx` の `iamOverviewRows` ブロックと `wafOverviewRows` の追加テスト、`DrawerTags.test.tsx` を新設し、警告表示の全箇所でフラグ true/false それぞれの表示を検証した。

### 検証

`mise run check` (backend: `go vet` / `staticcheck` / `govulncheck` / `golangci-lint` / `go test -race -cover ./...`、frontend: `eslint` / `tsc --noEmit` / vitest) が通ることを確認した。frontend は 73 ファイル 663 件のテストがすべて成功し、既存テストへの回帰は無い。backend の `govulncheck` はコードが呼び出す脆弱性 0 件を報告している (依存モジュール内の未呼び出し脆弱性 2 件は本 issue と無関係の既知事項)。
swarm レビュー Round 1 で指摘された不足を追加で埋めた。WAF のタグ取得 (`resOut`/`tagsOut`/`TagInfoForResource` が nil でエラーも nil のケース) を検証する `TestWAFACLDetail` の追加サブテスト、7 フラグ全てについて `omitempty` の有無を JSON マーシャルで直接検証する `Test{WAF,IAM,SQS,Dynamo}ResourceJSONFetchFailedOmitempty`、CLOUDFRONT スコープの全 ACL 一括代入ロジックを `markCloudFrontAssociatedCountFetchFailed` として抽出したうえでの単体テスト、`applyCloudFrontAssociatedCounts` のクライアント生成失敗経路を実プロファイル名の失敗 (モック不要) で直接検証するテストを `waf_test.go` に追加した。frontend では `columns.test.tsx` の IAM `policies` 列に、既存の true ケースと対になる false ケース (`policiesFetchFailed が false の行は警告アイコンを表示しない`) を追加した。
