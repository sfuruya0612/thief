# Pricing の単価表が大量行の場合に仮想化 (windowing) を導入するか判断する

Created: 2026-07-21
Completed: 2026-07-22
Model: Claude Opus 4.8

## 背景

issue 0058 (Pricing 画面のパフォーマンス改善) の計測で、`RateGroupSection` に EC2 On-Demand 相当の 400 行を描画した際のマウント (初回描画) コストを React Profiler (`actualDuration`) で計測した。

- マウント (400 行): 44.807ms
- フィルタ適用後の再レンダー (約 180 行に絞り込み): 6.063ms
- さらに絞り込んだ再レンダー (約 17 行に絞り込み): 0.691ms

マウントの 44.8ms は、同じ計測で確認した `matchesInstanceFilter` の小文字化コスト (6 キーストロークで 0.674ms、キーストロークあたり約 0.11ms) より 2 桁大きい。issue 0058 では、この差から「支配的コストは文字列処理ではなく DOM ノードの生成/差分検出である」と判断し、`matchesInstanceFilter` の事前計算による最適化は見送った。

一方でマウントコストそのものは、単価表の行を仮想化 (windowing) して可視範囲のみ DOM に描画すれば下げられる可能性が高い。ただし、この対応には次の設計判断が伴う。

## 保留にする理由

- **新規依存の追加を伴う**：`react-window` 等の仮想化ライブラリを追加する必要があるか、手書きの windowing で足りるかを検討しなければならない。本リポジトリの frontend 依存方針 (YAGNI、AGENTS.md「frontend (React)」節) では、新規パッケージ追加は「既存/標準手段で代替できない理由」の明示を伴う合意が必要であり、独断で追加できない。
- **総行数が今後の実装順序に依存する**：issue 0058 の「他 issue との相互作用」節に記載の通り、issue 0055 (Savings Plans のサービス分離) と issue 0056 (EC2 Spot 追加) により、1 サービスあたりの並行フェッチ数と単価表の総行数は増加する。仮想化の要否と設計 (グループ単位か単価表全体か) は、この総行数が確定してから再計測して判断するのが妥当。
- **チェック状態・スクロール位置の保持との整合**：仮想化を導入すると、行のチェック状態 (`selection`) やフィルタ入力時のスクロール位置の扱いに実装上の考慮が増える。現状の `RateGroupSection`/`ServiceCard` の設計 (issue 0057 で追加した `onDemandHourlyByLabel` の Map 参照等) と矛盾しない形で組み込めるか、着手前に設計を詰める必要がある。

## 計測データ (再計測時の比較対象として保持)

- 計測方法: `@testing-library/react` の `render`/`rerender` を `<Profiler>` で包み、`onRender` の `actualDuration` を記録 (jsdom 環境、実ブラウザでの paint コストは含まない)。
- 対象: `RateGroupSection` に 400 行 (EC2 On-Demand 相当、`m5.0xlarge`〜`m5.23xlarge` を繰り返し生成) を渡し、フィルタ未入力→`m5.1`→`m5.12` の順に再レンダー。
- 結果:
  - マウント (400 行、フィルタなし): 44.807ms
  - 更新 (フィルタ `m5.1`、約 180 行に絞り込み): 6.063ms
  - 更新 (フィルタ `m5.12`、約 17 行に絞り込み): 0.691ms
- このスクリプトは使い捨てで、リポジトリには含めていない (issue 0058 の計測記録として本ファイルに残す)。

## 完了条件 (再開時)

- issue 0055 / 0056 適用後の実際の総行数 (代表的なリージョンでの EC2 On-Demand 行数等) を確認する。
- 仮想化ライブラリの追加要否 (`react-window` 等 vs 手書き windowing) を検討し、結論を記録する。
- 依存を追加する場合、追加理由 (標準/既存手段で代替できない理由)、メンテナンス状況、ライセンスを明示する。
- 導入する場合は before/after を React Profiler で計測し、`CHANGES.md` に記録する。
- 導入しない場合も、その判断根拠を本ファイルに追記して `issues/closed/` に移動する。

## 判断結果

仮想化 (windowing) は導入しない。以下の実測と根拠に基づく。

### 実測した最大描画行数

issue 0055 / 0056 適用後の実 AWS で、Pricing API (`GET /api/aws/profiles/{profile}/pricing?service=ec2`) のレスポンス行数を計測した (`refresh=true` でキャッシュを介さず取得)。

- us-east-1: total 907 行 (on_demand 780 / reserved 127)
- ap-northeast-1: total 907 行 (on_demand 780 / reserved 127)

最大描画行数を決めるのは条件セレクタを持たない group であり、EC2 On-Demand (`model == on_demand`) が最大で 780 行。これがフィルタ未入力時に一度に DOM 描画されうる行数の実測上限である。EC2 Reserved Instance は条件セレクタ (lease/offeringClass/payment) で 1 条件に絞ってから描画するため、同時描画行数はこれより小さい。

補足: 計測初回に ap-northeast-1 で total 815 という内部不整合 (on_demand 780 + reserved 127 = 907 と一致しない) が観測されたが、これはキャッシュ書き込み途中のレスポンスを拾った測定タイミングのブレであった。`refresh=true` で取り直すと 907 で整合し、レスポンスの `region` フィールドも指定リージョン単独であることを確認した。リージョンフィルタおよびキャッシュキー (`awsServiceCode|region`) に不具合はない。

### 判断根拠

- 780 行のマウントコストは issue 0058 の計測 (400 行 = 44.807ms) から線形外挿して約 87ms。これはフィルタ未入力かつ EC2 On-Demand を開いた初回描画の 1 回に限定される。instanceType を 1 文字入力すれば約 180 行 / 6.063ms、さらに絞り込めば 1ms 未満 (issue 0058 実測) であり、恒常的な負荷ではない。
- 仮想化の導入は便益に見合うコストを持たない。`react-window` は新規依存であり frontend の YAGNI・依存最小方針に反する。手書き windowing でも sticky thead、パネル全体での横スクロール同期、上位の `useReducer` が保持する選択状態、スクロール位置保持との整合で実装が複雑化し、堅牢性のリスクを増やす。
- 「性能より堅牢性」「性能最適化はプロファイリングで裏付けがある場合にのみ行い、可読性・保守性・正確性を犠牲にしない」という原則に照らし、初回 1 回・最悪ケースのみの 87ms のために依存追加または複雑な自前実装を入れる判断は採らない。

### 再検討のトリガー

以下のいずれかが観測された場合に再検討する。

- 実ブラウザでの初回描画が体感上の問題として報告される。
- 1 サービスあたりの総行数が大幅に増加する (複数サービスの同時展開等)。
- フィルタ未入力での初回表示が主要な利用導線になる。

再検討時も第一候補は依存追加ではなく UI 側の緩和 (デフォルト折りたたみ、上位 N 件表示 +「もっと見る」) とし、それでも不足する場合に手書き windowing を検討する。

## 再オープン (2026-07-22)

上記「導入しない」の判断を覆し、手書き windowing を導入する方針に変更したため reopen する。

### reopen の理由

利用者の明示的な指示により、当初検討した `content-visibility: auto` が table 内部要素 (tbody/tr/td) には CSS Containment 仕様上 containment が効かず無効であることが判明したうえで、依存追加を避ける手書き windowing (上記「再検討のトリガー」で第二候補としていた手段) を採用することにした。「導入しない」という前回の結論自体は当時の便益・コスト評価としては妥当だったが、方針が変わった以上 closed のまま放置すると記録と実装が矛盾するため、issues/ に戻して実装 issue として扱う。

### 実装済みの内容

- `frontend/src/lib/windowedRows.ts`: 可視範囲を求める純関数 `computeVisibleRange` (DOM 非依存)。
- `frontend/src/hooks/useWindowedRows.ts`: スクロール祖先の探索・行高の実測・scroll/resize 購読を担うフック。Pricing の縦スクロールは複数グループを包む単一領域 (.pr-stack) で発生するため、スクロール領域単位の共有 ResizeObserver で sibling のレイアウト変化にも追従する。
- `frontend/src/components/pricing/RateGroupSection.tsx`: 60 行以上のグループで tbody を上下スペーサー行 + 可視スライス描画に変更。初回から推定行高で windowing を効かせ、実測値へ補正する。
- 依存追加なし。選択状態は上位 `useReducer` 集約のまま保持。
- テスト: `windowedRows.test.ts` (純関数の境界・不変条件)、`RateGroupSection.test.tsx` (windowing 挙動)。lint / test (524 passed) / build いずれも通過。

### 効果確認と close

実ブラウザ (Pricing 画面の EC2 On-Demand、約 780 行) で、windowing 導入後は初回描画・スクロールが体感で明確に高速化したことを利用者が確認した。React Profiler での定量値 (before/after の actualDuration) は取得していないが、目的である「フィルタ未入力時の大量行の初回描画コスト削減」は体感で達成できているため close とする。

必要になった場合の定量計測手順は本 issue の履歴に記載した方法 (グループの折りたたみ→展開で tbody の再マウントを isolate し、`WINDOW_THRESHOLD` を一時的に `Infinity` にして before を測る) で再現できる。
