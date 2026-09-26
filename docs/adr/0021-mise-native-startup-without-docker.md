# 0021. Docker による起動をやめ、mise のタスクでネイティブに起動し、ポートを 8088 と 8089 にする

Created: 2026-09-26
Model: Claude Opus 5.5
Status: Accepted
Decided: 2026-07-18

## 状況

thief は Docker でも起動できたが、Docker による起動は要件から外れた (`docs/issues/TODO.md`)。
使わない起動の経路が残ると、手順と設定が二重になる。
タスクは以前から `mise.toml` に置いており (コミット 80bbf3e、2025-01-26 から)、履歴に Makefile は無い。
また、以前のポート (backend 8080、frontend 8082) は他のツールと衝突しやすかった (issue 0032)。

## 決定

- thief の起動は、mise のタスク (`mise run backend:run`、`mise run frontend:run`) でネイティブに行う。
- thief 本体の Docker による起動をやめ、関連のファイルを削除する (issue 0031、コミット cb2da77)。
- `example/` の floci (AWS のエミュレータ) は残し、単一のコンテナの構成にする。
  backend は `HOME` を `example/home` に差し替えて起動し、ホストの `~/.aws` から隔離する (floci の環境の導入は issue 0024、`HOME` の差し替えによる隔離は issue 0031)。
- backend のポートを 8089、frontend のポートを 8088 にする (issue 0032)。

## 検討した代替案

issue 0031 は、`example/` の floci も含めて Docker を完全にやめる案を採らず、thief 本体の Docker による起動だけをやめた。
タスクランナーとして mise 以外 (Makefile など) を比べた記録は無い。
mise への統一は 2026-07-18 より前からあり、この ADR の判断には含めない。

## 結果

- 起動は `mise run backend:run` と `mise run frontend:run` の 2 つになる。
- ポートを変えるときは、backend の待ち受け、frontend の API の既定の送り先、Vite のポート、WebSocket の許可 Origin の 4 か所を同時に変える (issue 0032)。

## 根拠資料

- `docs/issues/closed/0024`、`0031`、`0032`
- コミット cb2da77 (2026-07-18)
- コミット 80bbf3e (2025-01-26、`mise.toml` の追加)
- `mise.toml`、`AGENTS.md` の「タスクランナー (mise run)」、`example/README.md`
