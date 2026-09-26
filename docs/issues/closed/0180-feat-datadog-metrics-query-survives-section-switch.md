# Datadog Metrics のクエリと期間をセクション切り替えをまたいで保持する

Created: 2026-09-16
Model: Claude Opus 5
Completed: 2026-09-17

## 背景

`docs/issues/TODO.md` の次の項目に対応する。

> - [ ] Datadog Metrics 画面の入力クエリを Cost/Dashboards セクションの切り替えをまたいで保持したい
>     - issue 0170 のレビューで判明。`DatadogMetricsView` はセクション切り替えで条件レンダリングによりアンマウントされるため、入力中のクエリが失われ最後に Dashboards から引き継いだ値に戻る

`frontend/src/views/nonaws/DatadogView.tsx` の `DatadogView` は、表示中のセクションを `const [section, setSection] = useState<Section>('cost')` で持つ (`Section` は `'cost' | 'dashboards' | 'metrics'`)。
Metrics の描画は条件レンダリングである。

```tsx
{section === 'metrics' && <DatadogMetricsView orgId={orgId} initialQuery={metricsQuery} />}
```

`display: none` による非表示ではないため、他のセクションへ切り替えると `DatadogMetricsView` はアンマウントされ、そのローカル state が破棄される。

`frontend/src/views/nonaws/DatadogMetricsView.tsx` の `DatadogMetricsView` が持つ state は次の 3 つで、すべてローカルである。

```tsx
export function DatadogMetricsView({ orgId, initialQuery = '' }: DatadogMetricsViewProps) {
  const [queryInput, setQueryInput] = useState(initialQuery);
  const [runningQuery, setRunningQuery] = useState(initialQuery.trim());
  const [spanSeconds, setSpanSeconds] = useState(DEFAULT_METRICS_SPAN_SECONDS);
```

`initialQuery` は `useState` の初期値としてのみ読まれ、再同期する `useEffect` は無い。
`DatadogView` 側で `initialQuery` の元になる `metricsQuery` を更新するのは、Dashboards のウィジェットから引き継ぐ `onOpenQuery` コールバックだけであり、Metrics 画面での入力を親へ書き戻す経路は無い。

したがって Metrics から他のセクションへ移って戻ると、入力欄の文字列 (`queryInput`)、実行済みのクエリ (`runningQuery`)、選択していた期間 (`spanSeconds`) の 3 つがすべて失われ、`metricsQuery` (Dashboards から最後に引き継いだ値、無ければ空文字) と既定の期間 `DEFAULT_METRICS_SPAN_SECONDS` に戻る。

TanStack Query のキャッシュは復元の役に立たない。
`frontend/src/api/queries.ts` の `useDatadogMetricsQueries` のキーは `['datadog', 'metrics', org, query, range.from, range.to]` で、`frontend/src/main.tsx` の既定 `staleTime: 60_000` が効く。
同じクエリと同じ分の時間窓であれば再取得は起きないが、`runningQuery` が空文字に戻る以上、そもそもクエリが発行されない。取得結果ではなく入力の state が失われることが問題である。

組織との関係は既に決まっている。
`frontend/src/App.tsx` は `<DatadogView key={datadogOrg} orgId={datadogRequestOrg} />` と `key` を指定しており、組織タブを切り替えると `DatadogView` 自体が再マウントされる。
`DatadogView` の state はすべて組織ごとにリセットされる設計になっている。

既存のテストはこの挙動を固定していない。
`frontend/src/views/nonaws/DatadogView.test.tsx` はセクションの切り替えと Dashboards からのクエリ引き継ぎを検証するが、`DatadogMetricsView` をスタブ化しており内部 state を見ていない。
`frontend/src/views/nonaws/DatadogMetricsView.test.tsx` はクエリ入力、Run ボタン、Enter キー、空白のトリム、期間変更、`initialQuery` の初期反映を検証するが、セクション切り替えをまたいだ保持を検証するテストは無い。

## 目的

Metrics で入力したクエリと選択した期間が、Cost や Dashboards へ移って Metrics に戻ったときにそのまま残る。
入力し直さずに続きから操作できる。

## 設計判断

### 採用する方式: state を `DatadogView` へリフトアップする

`queryInput`、`runningQuery`、`spanSeconds` の 3 つを `DatadogView` の state に移し、`DatadogMetricsView` へ値と更新関数を props で渡す。
`DatadogMetricsView` は state を持たない表示部品になり、アンマウントされても値は `DatadogView` に残る。

この方式を選ぶ理由は次の 3 つである。

- 保持の範囲が要望と一致する。`DatadogView` は組織タブの切り替えで `key` により再マウントされるため、組織をまたいだクエリの持ち越しは起きない。組織ごとにキーを分ける仕組みを別途作る必要がない。
- 変更が `DatadogView` と `DatadogMetricsView` の 2 ファイルに収まる。`frontend/src/lib/storage.ts` の `PersistedState` に手を入れない。
- 既存の `metricsQuery` を置き換える形で書ける。Dashboards からの引き継ぎ (`onOpenQuery`) は、リフトアップ後の `queryInput` と `runningQuery` を同時に更新する操作になる。`initialQuery` という初回だけ読む props が消え、値の出所が 1 つになる。

### 採らなかった案

- **`DatadogMetricsView` をアンマウントしない**。`section !== 'metrics'` のときも描画し続け、CSS で隠す案。`frontend/src/hooks/useTerminalSessions.ts` がターミナルの接続を維持するために採った考え方と同じである。state を動かさずに済む一方、隠れている間も `useDatadogMetricsQueries` がマウントされたままになり、TanStack Query が見えない画面のためにクエリを保持し、再フェッチの対象にもなる。Datadog の API 呼び出しを画面に出ていない状態で続ける理由が無いため却下する。
- **`localStorage` に永続化する**。`frontend/src/lib/storage.ts` の `PersistedState` にフィールドを足し、`usePersistedXxx` の形で保存する案。ブラウザを閉じても残るが、要望はセッション内でセクションを移動する間の保持であり、それを超える。加えて組織ごとにキーを分ける設計が新たに必要になる。`frontend/AGENTS.md` の「UI 状態と永続化」は新しい永続化フィールドをこのパターンで追加するよう定めるが、そもそも永続化を必要としないため対象外とする。
- **モジュールレベルの共有ストアを作る**。`frontend/src/hooks/useTerminalSessions.ts` や `frontend/src/hooks/useTweaks.ts` と同じ `useSyncExternalStore` の形にする案。これらは React のツリー上で親子関係に無い部品の間で状態を共有するための仕組みである。`DatadogMetricsView` は `DatadogView` の子であり、親に持たせれば足りる。組織ごとの分離も自前で書くことになるため却下する。
- **`initialQuery` を `useEffect` で再同期する**。props の変化を state へ反映する案。アンマウントで state が消える問題そのものは解決しない。

### 保持する対象と、保持しない対象

保持するのは `queryInput`、`runningQuery`、`spanSeconds` の 3 つとする。
期間 (`spanSeconds`) を含めるのは、クエリだけが残って期間が既定に戻ると、戻ってきた画面が以前と違うグラフを描くためである。

取得結果 (`series`) は保持しない。TanStack Query のキャッシュが担い、`runningQuery` と期間が復元されれば同じキーで引き直される。

### Dashboards からの引き継ぎの扱い

`onOpenQuery(query)` は現在 `setMetricsQuery(query)` と `setSection('metrics')` を呼ぶ。
リフトアップ後は `queryInput` と `runningQuery` の両方を `query` (と `query.trim()`) で上書きし、`section` を `'metrics'` に変える。
Metrics に入力中の値が残っていても、Dashboards から明示的に引き継いだ値で上書きする。利用者が Dashboards のウィジェットを選んだ操作の結果を優先する。
期間 (`spanSeconds`) は引き継ぎで変更しない。`onOpenQuery` はクエリ文字列だけを渡すためである。

## 完了条件

- `frontend/src/views/nonaws/DatadogMetricsView.tsx` の `DatadogMetricsView` が `queryInput`、`runningQuery`、`spanSeconds` を `useState` で持たず、props で受け取る。
- `frontend/src/views/nonaws/DatadogView.tsx` の `DatadogView` がこの 3 つを state として持つ。`metricsQuery` という初期値専用の state と `initialQuery` という props は無くなる。
- `frontend/src/views/nonaws/DatadogView.test.tsx` の `vi.mock('./DatadogMetricsView', ...)` が削除され、`DatadogMetricsView` の実体が描画される。`vi.mock` はファイル単位で効くため、スタブを残したままでは state の保持を検証できない。
- 同ファイルの `vi.mock('../../api/queries', ...)` に `useDatadogMetricsQueries` のオーバーライドが追加され、実体化した `DatadogMetricsView` が `fetch` を発火しない。現在このモックは `useDatadogHistorical` と `useDatadogEstimated` しか差し替えていない。差し替え方は `frontend/src/views/nonaws/DatadogMetricsView.test.tsx` の `vi.hoisted` と `vi.mock('../../api/queries', ...)` の組み合わせに揃える。
- 同ファイルの `vi.mock('../../components/charts/TimeseriesChart', ...)` が追加され、jsdom で描画できない `echarts-for-react` を避ける。`DatadogMetricsView.test.tsx` が同じスタブを持つ。
- スタブの `data-testid="metrics-view-stub"` と `data-initial-query` 属性に依存する既存の 2 テストが、実体の DOM (`aria-label="Query"` の入力欄と `aria-label="Period"` の選択欄) を見る形に書き換えられている。対象は次の 2 つで、いずれも書き換えなしでは必ず失敗する。
  - 「Metrics を押すと表示中の組織で Metrics に切り替わる」
  - 「Dashboards の引き継ぎ導線で Metrics へクエリを渡して切り替わる」
- 同ファイルの「Metrics に切り替えると Cost の historical/estimated 取得を止める」は `mocks.useDatadogHistorical` の呼び出し引数だけを見ており、スタブに依存しない。変更せずに通る。
- 同ファイルに次の 4 つのテストが追加されている。
  - Metrics でクエリ文字列を入力し、Cost へ切り替えてから Metrics に戻したとき、`aria-label="Query"` の入力欄の値が入力した文字列のままである。
  - Metrics で期間を既定以外 (`Last 1 day`) に変えて Dashboards へ切り替え、Metrics に戻したとき、`aria-label="Period"` の選択欄の値が変えた値のままである。
  - Metrics でクエリを入力して Run を押し、Cost へ切り替えてから Metrics に戻したとき、`useDatadogMetricsQueries` が最後にそのクエリを含む配列で呼ばれる (未実行の状態に戻らない)。
  - Metrics でクエリを入力するだけで Run を押さず、Cost へ切り替えてから Metrics に戻したとき、入力欄の値は残り、`useDatadogMetricsQueries` には空配列が渡されたままである (入力が勝手に実行されない)。
- `frontend/src/views/nonaws/DatadogMetricsView.test.tsx` の既存テスト (クエリ入力、Run ボタン、Enter キー実行、空白のトリム、期間変更、ローディングとエラー表示) が、props 化した後も同じ観点で通る。props になった値と更新関数を渡すラッパーをテスト側に置いてよい。`initialQuery` を渡す既存テストは、新しい props の形に合わせて書き換える。
- 組織タブを切り替えたときにクエリが持ち越されないことが、`frontend/src/App.tsx` の `key={datadogOrg}` による再マウントで担保されている。この点はコードを変更せず、`DatadogView` の state をこれ以上外へ持ち出さないことで満たす。
- `mise run check` が通る。

### 扱わない範囲

- `localStorage` への永続化は行わない。ブラウザのリロードでクエリは失われてよい。
- Cost セクションの期間 (`startMonth` / `endMonth`) や Dashboards の選択状態は本 issue の対象ではない。これらは既に `DatadogView` の state であり、セクション切り替えで失われない。

## 関連

- `docs/issues/closed/0170-feat-datadog-metrics.md` が Metrics 画面と Dashboards からの引き継ぎを追加した issue で、本 TODO の出所である。同 issue は引き継ぎ導線を満たす最小の実装にとどめ、この未対応点を TODO.md へ送った。

## 解決方法

設計判断のとおり、`queryInput`、`runningQuery`、`spanSeconds` の 3 つの state を `DatadogMetricsView` から `DatadogView` へリフトアップした。

- `frontend/src/views/nonaws/DatadogMetricsView.tsx` は `useState` を使わず、3 つの値と対応する更新関数 (`onQueryInputChange`/`onRunningQueryChange`/`onSpanSecondsChange`) を props で受け取る表示部品にした。`initialQuery` props は削除した。
- `frontend/src/views/nonaws/DatadogView.tsx` は `metricsQuery` を置き換える形で `metricsQueryInput`/`metricsRunningQuery`/`metricsSpanSeconds` (既定値は `DEFAULT_METRICS_SPAN_SECONDS`) を state として持ち、`DatadogMetricsView` へ渡す。`DatadogMetricsView` はセクション切り替えでアンマウントされても、これらの state は親である `DatadogView` に残る。
- Dashboards からの引き継ぎ (`onOpenQuery`) は `queryInput` と `runningQuery` の両方を渡されたクエリで上書きし、`section` を `metrics` に変える。`spanSeconds` は引き継ぎで変更しない。
- 組織タブを切り替えたときにクエリが持ち越されないことは、`frontend/src/App.tsx` の `key={datadogOrg}` による `DatadogView` 自体の再マウントで従来どおり担保される (この点はコードを変更していない)。
- `frontend/src/views/nonaws/DatadogView.test.tsx` の `vi.mock('./DatadogMetricsView', ...)` を削除して実体を描画するようにし、`vi.mock('../../api/queries', ...)` に `useDatadogMetricsQueries` のオーバーライドを、`vi.mock('../../components/charts/TimeseriesChart', ...)` のスタブを追加した。スタブの `data-testid`/`data-initial-query` に依存していた既存 2 テストを実 DOM (`aria-label="Query"`/`aria-label="Period"`) を見る形に書き換え、完了条件が挙げる 4 つの新規テスト (Cost をまたいだクエリ文字列の保持、Dashboards をまたいだ期間の保持、Run 済みクエリの保持、未 Run のクエリが自動実行されないこと) を追加した。
- `frontend/src/views/nonaws/DatadogMetricsView.test.tsx` は、親相当の 3 state を保持して props として渡す `Harness` コンポーネントを追加し、既存テスト (クエリ入力、Run ボタン、Enter キー実行、空白のトリム、期間変更、ローディングとエラー表示) を props 経由の形に書き換えた。
- 多観点レビュー (3 観点: 完了条件との突き合わせ、回帰と整合、成果物の規約整合) を実施し、指摘は 0 件だった。追加レビューは発生していない。
- `mise run check` が通ることを確認した (frontend テストは 993 件から 997 件に増加。lint の警告 10 件・脆弱性 0 件はベースラインと同じ)。
