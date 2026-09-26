# 0004. 依存は標準ライブラリを優先して最小に保つ

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-02-20

## 状況

依存を増やすと、脆弱性の対応、更新、ライセンスの確認の手間が増える。
thief は少人数で保守するローカルツールであり、依存の保守に割ける手間は小さい。

## 決定

依存の追加は次の優先順で判断する (`AGENTS.md` の backend 「依存関係の方針」)。

1. Go 標準ライブラリ (`net/http`、`encoding/json`、`log/slog`)
2. `golang.org/x/...` と準標準の軽量ライブラリ
3. CLI の定番ライブラリ (`spf13/cobra`)
4. クラウドの公式 SDK (AWS SDK for Go v2、Google Cloud、Datadog)
5. その他 (上記で代替できない場合だけ。理由、保守状況、ライセンスを明記する)

ORM、リッチなロガー (zap、zerolog)、DI コンテナは導入しない。
frontend も同じ考え方で、状態管理ライブラリ、ルーター、外部のアイコンパッケージを導入しない。

この方針に沿った個別の判断は次のとおりである。

- 設定の読み込みを Viper から自前の軽量な実装に替えた (コミット 7cf03c5、2026-02-20)。
- Pricing の数百行の表 (EC2 On-Demand で最大 780 行、issue 0061 の実測) の仮想化に `react-window` を入れず、自前の windowing (`frontend/src/lib/windowedRows.ts` `computeVisibleRange`) を書いた (issue 0061、2026-07-22)。

## 検討した代替案

- issue 0061: `react-window` の導入を検討し、いったん「導入しない」と判断した後、reopen して自前の windowing を採った。
  CSS の `content-visibility: auto` は table の内部要素に効かないため採らなかった。
- 方針そのものの代替案 (依存を制限しない) を比べた記録は無い。

## 結果

- `backend/go.mod` の直接依存は、公式 SDK のほか、cobra、yaml.v3、bubbletea、coder/websocket、google/uuid、go-cmp、golang.org/x/sync などであり、ORM、リッチなロガー、DI コンテナは無い。
- 標準ライブラリに無い機能は自前で書く。
  書いたコードの保守とテストは thief が負う。
- 例外として、ブラウザで SQL を動かすために `@duckdb/duckdb-wasm` と `apache-arrow` を追加した (ADR 0018)。
  標準の API に SQL エンジンが無いことを理由とした。

## 根拠資料

- コミット 7cf03c5 (2026-02-20)
- `docs/issues/closed/0061`
- `AGENTS.md` の backend 「依存関係の方針」と frontend 「基本方針」
