# CLI に DynamoDB / API Gateway / NAT Gateway / SQS / WAF の一覧コマンドを追加する

Created: 2026-09-18
Model: deepseek-v4p1-flash
Completed: 2026-09-18

## 背景

Web の AWS ビューは DynamoDB / API Gateway / NAT Gateway / SQS / WAF の一覧を表示できるが、CLI には対応するコマンドが無い。

- backend の取得層は実装済みで、`ListDynamoResources` (`backend/internal/aws/dynamo.go:50`)、`ListAPIGatewayResources` (`backend/internal/aws/apigw.go:33`)、`ListNATGatewayResources` (`backend/internal/aws/natgw.go:31`)、`ListSQSResources` (`backend/internal/aws/sqs.go:57`)、`ListWAFResources` (`backend/internal/aws/waf.go:51`) が存在する。
- API サーバのルートも `backend/internal/api/routes.go` に `/api/aws/profiles/{profile}/dynamo` ほか 5 本がある。
- 一方で `backend/internal/cli/root.go` の `NewRootCmd()` はこれら 5 サービスを登録しておらず、`internal/cli/` にも対応ファイルが無い。

利用者は Web では見られる一覧を CLI から確認できず、シェルやスクリプトから扱えない。

## 目的

CLI から 5 サービスのリソース一覧を取得し、既存コマンドと同じく tab / CSV で出力できるようにする。`--group-by` も既存コマンド同様に使えるようにする。

## 設計判断

- CLI の列は Web の列をそのままは写さず、`internal/cli` の既存慣習に合わせてリージョン列を省く (EC2 / Lambda / Kinesis などもリージョン列を持たない)。列の集合は各 Resource の主要フィールドとする。
  - dynamo: Table(Name) / State / Mode / Items(ItemCount) / Size(SizeBytes) / GSI(GSICount)
  - apigw: API(Name) / State / Type / Stage / Endpoint
  - natgw: Name / State / GatewayID(ID) / VPC(VpcID) / ElasticIP / LaunchTime
  - sqs: Queue(Name) / State / Type / Available(AvailableMessages) / InFlight / Retention(d)(RetentionDays)
  - waf: WebACL(Name) / Description / State / Scope / Rules(RuleCount) / Associated(AssociatedCount)
- SDK 型 → Resource 型の変換は既存の `ListXxxResources` をそのまま使い、CLI 用に別の `Info` 型を作らない。`util.Row` を満たすため、`internal/aws/torow.go` に各 Resource 型の `ToRow()` を追加する。
  - 却下案：Web と同じ列数・列名にする案。リージョンやコスト列は `resource` 構造体に無い項目もあり、既存 CLI の慣習とも合わないため却下。
  - 却下案：`ListXxxInfos` を新設する案。用途が CLI 一覧だけであり、既存 Resource と二重管理になるため却下。
- 既存の `runList` (`internal/cli/helper.go:159`) を使うため、CSV / `--no-header` / `--group-by` は自動で使える。追加の API 呼び出し・権限は不要。

## 完了条件

- `thief dynamo ls` / `thief apigw ls` / `thief natgw ls` / `thief sqs ls` / `thief waf ls` がそれぞれ一覧を tab 出力する。
- 各コマンドに `-o csv` を付けると CSV を出力し、`--no-header` と `--group-by <列>` が効く。
- `thief --help` 相当のコマンド一覧に 5 コマンドが現れる。
- `internal/cli/columns_torow_test.go` の `TestColumnsToRowOrder` に 5 型のケースを追加し、列ヘッダと `ToRow()` の順序が一致することを検証する。
- `mise run check` が通過する。

## 解決方法

- `backend/internal/aws/torow.go` に `DynamoResource` / `APIGatewayResource` / `NATGatewayResource` / `SQSResource` / `WAFResource` の `ToRow()` を追加した。NATGateway の LaunchTime は RFC3339 で整形する。
- `backend/internal/cli/dynamo.go` / `apigw.go` / `natgw.go` / `sqs.go` / `waf.go` を新規作成し、`root.go` の `NewRootCmd()` に `newDynamoCmd` / `newAPIGWCmd` / `newNATGatewayCmd` / `newSQSCmd` / `newWAFCmd` を登録した。各コマンドは `runList` を使うため tab / CSV / `--no-header` / `--group-by` がそのまま使える。
- `columns_torow_test.go` に 5 型のケースを追加した。
- `mise run check` (fmt / lint / test) が通過することを確認した。
