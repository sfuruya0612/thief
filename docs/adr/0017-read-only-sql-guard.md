# 0017. Athena と BigQuery のクエリは、書き込みの語を含むものを拒否する

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-07-08

## 状況

thief は Athena と BigQuery のクエリを、利用者の権限で実行する。
thief は閲覧のためのツールであり、利用者の権限が書き込みを許していても、thief から書き込みのクエリを実行させない。

## 決定

- クエリを外部サービスに送る前に、`backend/internal/sqlguard` の `ValidateReadOnly` で検査する。
- 正規表現 `(?i)\b(INSERT|UPDATE|DELETE|MERGE|CREATE|DROP|ALTER|TRUNCATE|GRANT|REVOKE|UNLOAD)\b` に一致したら拒否する。
- この検査は API サーバの全クエリ (Web UI の Athena と BigQuery) と、CLI の `thief athena query` に掛ける。
- 例外として、CLI の `thief bq query` は検査しない。
  旧 CLI との互換のため、`backend/internal/bigquery/query.go` の `ExecuteQueryUnrestricted` で任意の SQL を実行する (コミット 2020b79、2026-07-16)。
  この関数は API サーバから呼ばない。

## 検討した代替案

記録なし。
同じ語の一覧の検査は、最初の実装 (コミット 2aac22f、2026-07-08) で BigQuery 用に `backend/internal/bigquery/query.go` に入った。
コミット bee07ba (2026-07-17) は、検査を `backend/internal/sqlguard` に切り出して Athena にも掛け、`UNLOAD` を語に加えた。
SQL を構文解析する案や、読み取り専用の IAM 権限だけに頼る案を比べた記録は無い。

## 結果

- 語の一致で判定するため、コメントや文字列リテラルの中の語でも拒否する (誤検知)。
  例: `SELECT 'drop' AS x`。
- 語の一致による検査であり、書き込みを確実に防ぐものではない。
  この評価は ADR の起草者の判断であり、資料に記録は無い。
- `thief bq query` からは書き込みのクエリを実行できる。
  閲覧のためのツールという前提と食い違うため、この例外を残すかどうかを `docs/prd/thief.md` の「未確定論点」に残した。
- 誤検知の扱いは `docs/prd/thief.md` の「未確定論点」に残した。

## 根拠資料

- コミット 2aac22f (2026-07-08)
- コミット bee07ba (2026-07-17)
- コミット 2020b79 (2026-07-16)
- `backend/internal/sqlguard/sqlguard.go`
- `backend/internal/bigquery/query.go` の `ExecuteQueryUnrestricted`
- `backend/internal/cli/bq.go`
