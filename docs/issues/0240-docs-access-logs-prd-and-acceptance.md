# Access Logs の PRD と受け入れ確認を更新する

Created: 2026-11-02
Model: DeepSeek V4.1 Flash 1017

## 背景

ADR 0037 と issue 0235 から 0239 で、Access Logs のバックエンドとフロントエンドを追加する。
PRD `docs/prd/thief.md` は、AWS の 23 サービスの一覧と詳細、ログの調査、セキュリティの要求を定めているが、Access Logs の要求を持たない。
成功指標の「AWS サービスの網羅」は 23 サービスを前提にしており、サービスを 24 に増やすと更新が必要になる。
PRD の「やらないこと」は、外部サービスのリソースの作成、削除、設定変更と、書き込みの SQL の実行を定めている。
Access Logs が Athena のテーブルを自動で作らないことと、WAF のルールを変更しないことを、この方針に沿って明記する必要がある。

## 目的

PRD に Access Logs のユースケース、機能要求、受け入れ基準、成功指標、スコープを追加し、実装と PRD の記述を一致させる。
あわせて `CHANGES.md` の `## develop` に Access Logs のエントリを追加し、`mise run check` で最終確認する。

## 設計判断

- PRD の「ユースケース」に UC-15 を追加する。
  - 場面: 不正なアクセスの初動調査
  - 行動: AWS のプロファイルとリージョンを選び、ALB、CloudFront、WAF のソースと期間を選び、IP と URI の集計を見る。WAF のブロック、4xx、5xx、センシティブパスで絞り込む。
  - 対応する機能要求: FR-32
- PRD の「機能要求」に FR-32 を追加する。
  - Access Logs を Security グループに置く。
  - ALB と CloudFront は S3 のログを Athena で、WAF は CloudWatch Logs のログを Logs Insights で、WAF の S3 のログを Athena で集計する。
  - ソースを検出し、出力先、ログの有効、テーブルの有効、配信の遅延、権限の状態を表示する。
    ログが無効なリソースも一覧に残し、有効化の手順を表示する。
  - 複数の ALB、CloudFront、WAF が同じ S3 のバケットとプレフィックス、または同じ CloudWatch Logs のロググループを共有する場合に、`source_key` でリソースごとの内訳を保つ。
  - IP と URI の組、IP 単独の集計を表示する。
  - CloudFront の origin の ALB を IP の集計から外し、WAF をオーバーレイとして扱う。
  - Athena のテーブルは利用者が選び、DDL のテンプレートをコピーできる。thief は DDL を実行しない。
  - 期間、ソース数、結果行数、スキャン量の制限を掛け、結果をキャッシュしない。
  - ジョブを取り消せる。
  - ソース間の重複は排除せず、ソース別の内訳を表示する。
  - リアルタイムの検知、自動修復、アラート、CLI、TUI は対象外とする。
- PRD の「成功指標」の「AWS サービスの網羅」を 24 サービスに更新する。
  `frontend/src/lib/serviceMeta.ts` の `SERVICES` の 24 サービスすべてに、一覧の表示先か専用ビューがあることを確認する。
  Access Logs は専用ビューとして数える。
- PRD の「スコープ」の「やること」に Access Logs を追加する。
  「やらないこと」に、Access Logs からの WAF のルール変更、IP のブロック、Athena のテーブルの自動作成、リアルタイムの検知、アラート、ソース間の重複排除を追加する。
- PRD の「非機能要求」のセキュリティに、IP、URI、User-Agent をログに出さないこと、Athena のクエリを読み取り専用にすること、テーブル名とカラム名を検証することを追加する。
- `CHANGES.md` の `## develop` に、Access Logs の `[ADD]` エントリを 1 つ追加する。
  - 担当者は `@sfuruya0612` とする。
  - 変更の内容に、ADR 0037 と FR-32 を参照する。
- `docs/adr/README.md` の索引に ADR 0037 を追加する。ADR 0036 の索引の欠落も直す。
- この issue では新しい機能の実装を含めない。
  受け入れ確認は、issue 0235 から 0239 の完了後に、`mise run check` と手動の確認項目で行う。

## 完了条件

- `docs/prd/thief.md` のユースケースに UC-15 がある。
- `docs/prd/thief.md` の機能要求に FR-32 があり、ALB、CloudFront、WAF のログの出力先とクエリエンジン、ログが無効なリソースの表示、共有された出力先の `source_key` による内訳、IP の attribution、WAF のオーバーレイ、テーブルの選択と DDL のコピー、コストと期間の制限、ジョブの取り消し、ソース間の重複の扱い、v1 の対象外が書かれている。
- `docs/prd/thief.md` の成功指標の「AWS サービスの網羅」が 24 サービスになっている。
- `docs/prd/thief.md` のスコープの「やること」と「やらないこと」が Access Logs に言及している。
- `docs/prd/thief.md` の非機能要求のセキュリティが、IP、URI、User-Agent をログに出さないことと、読み取り専用のクエリと識別子の検証を定めている。
- `CHANGES.md` の `## develop` に Access Logs の `[ADD]` エントリがあり、担当者行が 2 文字分インデントされている。
- `docs/adr/README.md` に ADR 0036 と ADR 0037 がある。
- `mise run check` が通る。
- 手動の受け入れ確認として、ALB、CloudFront legacy、CloudFront v2 JSON、WAF の CloudWatch Logs、WAF の S3 の各ソースが、検出、テーブルの選択、集計、結果の表示のいずれかで状態を表示することを確認する。実 AWS の環境が無い場合は、フェイクのクライアントを使ったテストで代替し、その旨を「## 解決方法」に書く。

## 関連

- ADR 0037
- issue 0235、0236、0237、0238、0239
- `docs/prd/thief.md`
- `CHANGES.md`
- `docs/adr/README.md`
