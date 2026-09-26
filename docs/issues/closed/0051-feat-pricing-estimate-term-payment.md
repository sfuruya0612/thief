# Pricing の見積もり明細に RI/SP の期間と購入タイプを表示する

Created: 2026-07-19
Completed: 2026-07-19
Model: Claude Opus 4.8

## 背景

Pricing 画面の単価表 (`components/pricing/RateGroupSection.tsx`) は、Reserved Instances と Savings Plans について、期間 (`term.lease`: 1yr / 3yr)、オファリングクラス (`term.offeringClass`: standard / convertible)、支払オプション (`term.payment`: No Upfront / Partial Upfront / All Upfront) をセレクタで先に 1 条件へ絞り込んでから、該当する行だけを表に出す。
これは条件の組み合わせが多く、行を羅列すると行数が爆発するためである。

チェックした行は右レールの見積もり (`components/pricing/Estimator.tsx`) に明細として並ぶ。
しかし明細のラベルは `rate.label` (インスタンスタイプ / OS / エンジン等) のみで、どの期間と購入タイプで選んだ行なのかが表示されない。
同一インスタンスタイプで期間や支払条件の異なる RI/SP を複数チェックすると、見積もり上で行を区別できない。

## 現状

- `Estimator` の明細行 (`pr-estimator-line`) は `rate.label`、数量入力、3 系統の金額 (継続 / 前払い / 実効) を表示する。期間と購入タイプは表示しない。
- `PriceRateRow` (`types/aws.ts`) は `model` (`on_demand` / `reserved` / `savings_plan`) と `term` (`{ lease, offeringClass, payment }`) を持つが、`Estimator` はこれらを表示に使っていない。
- 単価表側は選んだ条件をセレクタが暗黙に保持するため行に条件を書かないが、見積もりに移すとその文脈が失われる。

## 目的

見積もり明細で RI/SP の行に期間と購入タイプを表示し、条件の異なる行を区別できるようにする。

## 対応方針

フロントのみの変更で完結する。バックエンドは既に `term` を返している。

- `Estimator` の明細行に、`model` が `reserved` または `savings_plan` の場合、`term.lease` (期間) と `term.payment` (購入タイプ) を表示する。
- On-Demand (`model === 'on_demand'`) は期間と購入タイプを持たない (`term.lease` / `term.payment` が null) ため表示しない。
- 表示は単価表のセレクタの表現 (`1yr` / `No Upfront` 等) と揃える。
- EC2 の Reserved Instances はオファリングクラス (`term.offeringClass`: standard / convertible) でも行が分かれる。期間と支払オプションだけでは EC2 RI の standard と convertible を区別できないため、`offeringClass` が非 null の場合はこれも併記するかどうかを実装時に判断する (TODO の要望は「期間と購入タイプ」だが、区別の目的からは offeringClass も候補になる)。
- 表示位置は `pr-estimator-line-label` の近傍とし、既存の 3 系統金額の並びを崩さない。CSS クラスは既存の `pr-` 接頭辞に揃える。

## 完了条件

- 見積もり明細で RI/SP の行に期間と購入タイプが表示される。
- 同一インスタンスタイプで期間または支払条件の異なる RI/SP 行を、見積もり上で見分けられる。
- On-Demand の行には期間と購入タイプが表示されない。
- `components/pricing/Estimator.test.tsx` に、RI/SP 行の期間と購入タイプの表示、および On-Demand 行に表示されないことの検証を追加する。
- `CHANGES.md` の `## develop` に `[ADD]` エントリを追記する (種別順 UPDATE → ADD → CHANGE → FIX を守り、次行に 2 文字インデントで `- @sfuruya0612` を付ける)。
- `mise run check` が全て通過する。

## 検証

- frontend: `mise run frontend:lint` / `mise run frontend:test`。`Estimator` に RI (1yr / All Upfront 等) と SP と On-Demand を混在させ、RI/SP のみに期間と購入タイプが出ることを検証する。
- 実ブラウザで、単価表から同一インスタンスタイプの異なる期間 / 支払条件の RI/SP を複数チェックし、見積もりで各行を区別できることを確認する。

## 解決方法

`frontend/src/components/pricing/Estimator.tsx` に、`model`/`term` から表示文字列を組み立てる
純関数 `termLabel(rate)` を追加した。

```ts
function termLabel(rate: PriceRateRow): string | null {
  if (rate.model === 'on_demand') return null;
  const parts = [rate.term.lease, rate.term.offeringClass, rate.term.payment].filter(
    (v): v is string => !!v,
  );
  return parts.length > 0 ? parts.join(' / ') : null;
}
```

明細行 (`pr-estimator-line-head` の直後) に `{term && <div className="pr-estimator-line-term">{term}</div>}`
を追加し、`null` (On-Demand) の場合は要素ごと出さないようにした。表示テキストは
`RateGroupSection` のセレクタが使う値 (`term.lease`/`term.offeringClass`/`term.payment`) を
そのまま `' / '` 区切りで連結しており、単価表側の表現と完全に一致する。

対応方針で「実装時に判断する」としていた EC2 RI の `offeringClass` (standard/convertible) は、
**併記する**判断にした。理由: `parts` のフィルタは `model` による分岐ではなく値の有無
(非 null かどうか) だけで行っており、EC2 RI では `offeringClass` が非 null、Savings Plans
では常に null (対応方針の記載通り SP はオファリングクラスを持たない) になるため、分岐を
増やさずに「値がある条件だけを表示する」という一貫したロジックで両方を扱える。

CSS は `app.css` に `.pr-estimator-line-term` を追加した (`pr-estimator-line-label` 直下、
`text-3` 色・10.5px の控えめな表示にし、既存の 3 系統金額の並びは変更していない)。

### 実施した検証

- `frontend/src/components/pricing/Estimator.test.tsx` に 4 テストを追加した: RI 行の
  期間・オファリングクラス・購入タイプ表示、SP 行の期間・購入タイプ表示 (オファリングクラス
  なし)、同一ラベルで期間違いの RI 2 行を両方チェックして両方の条件行が表示されること、
  On-Demand 行に `.pr-estimator-line-term` が存在しないこと。
- 実装前 (`git stash` で `Estimator.tsx` のみ一時的に戻す) に同じテストを実行し、新規 4
  テストのうち 3 件 (RI/SP/複数 RI) が意図通り FAIL することを確認した (On-Demand のテストは
  「表示されない」ことの確認なので変更前後どちらでも通る)。
- 修正後は既存 5 テスト + 新規 4 テストの全 9 テストが PASS することを確認した。
- `mise run check` (backend/frontend の fmt + lint + test) が全て通過することを確認した。
- 実ブラウザ (Playwright + ローカル Chrome) で `mise run frontend:run` を起動し、実際に
  Pricing 画面の EC2 Reserved Instance グループから `a1.2xlarge / Linux / Shared` を
  期間 1yr と 3yr でそれぞれ 1 行ずつチェックした。見積もり明細に
  `1yr / standard / No Upfront` と `3yr / standard / No Upfront` がそれぞれ表示され、
  `rate.label` だけでは区別できない同一ラベルの 2 行を見積もり上で区別できることを実データで
  確認した (スクリーンショットで目視確認済み)。

### 検証中に判明した別件 (このリポジトリの既知事項、issue 0051 の対象外)

実ブラウザ確認の過程で、Pricing 画面の EC2/RDS Savings Plans グループにまだ
`Encountered two children with the same key` 警告 (issue 0049) が出ていることに気付いた。
原因はコードのバグ再発ではなく、(1) ローカルディスクの単価キャッシュが issue 0049 修正前の
古い `rate_id` のままだったこと、(2) 稼働中の backend プロセスが issue 0049 の修正を反映した
バイナリで再起動されていなかったことの 2 点。更新ボタンでキャッシュを再取得しても解消しな
かったことから (2) を確認している。issue 0049 の対応方針に明記した「既存キャッシュは更新
ボタンでのみ再取得」という既知の制約に加え、**コード修正を実機に反映するには backend プロセス
の再起動も必要**という点は、issue 0049 側に追記が必要だが、本 issue の変更範囲外のため
ここでは触れるに留める。
