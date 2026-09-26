# frontend のクエリキャッシュ設定を見直して再取得とロード表示を減らす

Created: 2026-07-25
Completed: 2026-07-26
Model: Claude Fable 5

## 背景

TODO.md の次の項目のうち、「frontend はキャッシュの利用頻度をあげ API コールによるユーザの体験向上」の部分に対応する。

> Frontend, Backend の処理の最適化をしたい。
> - 具体的には処理の共通化、frontend はキャッシュの利用頻度をあげ API コールによるユーザの体験向上、backend は処理速度の向上を目指す

処理の共通化は docs/issues/0078、backend の処理速度は docs/issues/0081 で扱う。

frontend の TanStack Query の現状は次のとおり (2026-07-25 調査時点)。
docs/issues/0075 が query を 1 本 (`useWAFRules`、`staleTime: 60_000`) 追加する予定で、候補 2 の整理対象に含まれる。
docs/issues/0079 が追加するのは mutation (`useCacheInvalidate`) のみで、候補 2 の対象外。

- `QueryClient` のグローバル既定 (`frontend/src/main.tsx`) は `retry: 1` と `refetchOnWindowFocus: false` の 2 つだけで、`staleTime` / `gcTime` は未設定 (TanStack Query の既定値のまま)。
- `frontend/src/api/queries.ts` の `use*` フック 68 本のうち query は 54 本 (useQuery 50 + useInfiniteQuery 4。残り 14 本は mutation)。query 54 本の `staleTime` は、33 本が `60_000`、6 本が `Infinity`、4 本が `5 * 60 * 1000`、3 本が `QUERY_HISTORY_STALE_TIME` (= `15_000`)、1 本が `30_000`、2 本が `0`、1 本が呼び出し側指定 (`usePricing`、既定引数 `Infinity`)、4 本が未指定 (= 0) という分布になっている。このほか `components/Sidebar.tsx` と `views/GcpSidebar.tsx` に件数バッジ用の `useQuery` が 2 本あるが、前者は `queryFn: skipToken`、後者は `enabled: false` で fetch せず、一覧が埋めたキャッシュを読むだけの観測専用になっている。
- 未指定の 4 本は `useSecretValue` / `useSSMValue` / `useS3ObjectPreview` / `useGcsObjectPreview` で、「開くたびに取得する」意図のコメントが付いている (Secret / SSM を束ねるセクションコメント、`useS3ObjectPreview` 直前のコメント、`useGcsObjectPreview` の「S3 と対称」コメントの 3 つ)。
- 再取得はマウント時 (`refetchOnMount` 既定) と `invalidateQueries` 時に起きる。エラー時の再試行ポーリング (profiles / health / GCP プロジェクト) と、Athena / BigQuery のジョブ実行中の 1 秒間隔ポーリング (`pollWhileActive`) を除き、定期再取得は無い。`refetchOnMount` は既定でもデータが stale のときしか再取得しないため、`staleTime` 内の再マウントでは再取得は発生しない。
- `gcTime` の個別指定は 0 件であり、全 query が既定値 5 分でガベージコレクトされる。ビューやサービスを離れて 5 分を超えて戻ると、キャッシュが消えているため旧データの即時表示ができず、ロード表示になる。このとき `staleTime` (基本 60 秒) も切れているため再取得は `gcTime` の値に関係なく発生する。`gcTime` が効くのは「取得中に旧データを出せるか、ロード表示になるか」の差である。
- backend のリソース一覧キャッシュの TTL は 1 時間 (`backend/internal/api/server.go` の `cacheTTL`) であり、`staleTime` 経過後の再取得の多くは backend の同じキャッシュ済みデータを受け取るだけになっている。

## 目的

各候補の効果を計測し、効果が確認できたものだけを採用して、サービスやビューを行き来する操作での再取得リクエストとロード表示を減らす。

## 計測手段

改善前後で同一の操作シナリオを実行し、ブラウザ開発者ツールの Network タブで `/api/` へのリクエスト数と、ロード表示の回数を記録して比較する。
`/api/` リクエストのうちレスポンスヘッダ `X-Cache-Status` が `MISS` の件数も併せて記録する (backend キャッシュを貫通して AWS を呼んだ回数を表し、`serveCached` が全リソース一覧に付与済み)。
ロード表示は「`AccountView` のリソーステーブル領域にプレースホルダが表示された回数」と数え、Drawer 内と Sidebar の件数バッジは数えない。
シナリオには Athena / BigQuery のクエリ実行を含めない (ジョブポーリングのリクエストが計測に混入するため)。
シナリオは少なくとも次の 2 つを含め、サービス A / B は固定する (例: A = EC2、B = RDS)。

1. AWS のサービス A を表示、別のサービス B へ切替、6 分放置してから A に戻る (gcTime 既定 5 分の破棄を跨ぐ)。
2. AWS ビューと Pricing ビューを 1 分以内に往復する (staleTime 内では再取得が発生しないという前提の確認で、before / after で差が出ないことを期待する)。このシナリオは前提の確認専用で、候補の採否判定には使わない。

候補と採否基準は次のとおり。
計測が要る候補は、基準を満たさなければ実装を変えず、結果だけを本 issue に記録する (docs/issues/closed/0058 の方針)。

- 候補 1: `gcTime` のグローバル既定を引き上げる。採否基準はシナリオ 1 でロード表示回数が減ること (リクエスト削減は狙わない)。値はシナリオ 1 のロード表示回数が 0 になる最小値を候補とし、ブラウザのメモリ使用量に問題が出ない範囲で決める。次のフックは内容の機密性とメモリ量から `gcTime` を現状 (既定 5 分) 以下に個別指定して維持する: `useSecretValue` / `useSSMValue` / `useS3ObjectPreview` / `useGcsObjectPreview` (機密値とプレビュー本文)、`useBQQueryResults` / `useAthenaResults` / `useGcpLogEntries` / `useCWLogEvents` (ページを蓄積する `useInfiniteQuery` で、クエリ結果とログ本文を持つ)。
- 候補 2: `staleTime` のグローバル既定を `60_000` にし、個別指定を既定と異なる query だけに減らす。`queries.ts` の 54 本は個別指定の整理で挙動が変わらない。件数バッジ 2 本は fetch しない観測専用クエリのため、`staleTime` の既定変更でも挙動が変わらない。挙動の変わる query が無いため計測に依らず実施できる (型チェックとテストの通過で足りる)。未指定の 4 本は明示的に `staleTime: 0` を書いて意図を保存する。
- 候補 3: リソース一覧の `staleTime` を backend のキャッシュ TTL (1 時間) に近づける。採否基準はシナリオ 1 で `X-Cache-Status: MISS` の件数が減ること (`/api/` リクエスト数は backend キャッシュに当たるだけの安価な再取得も数えるため基準にしない)。MISS が減らない場合は、表示の鮮度を後退させる対価が無いため採用しない。前提は docs/issues/0079 の完了条件のうち、破棄 POST 後の GET が `X-Cache-Status: MISS` を返すハンドラのテストが通過していること (番号順で本 issue より先に close される)。自動再取得の間隔が延びて表示の鮮度は後退するが、Refresh で即時に取り直せることと引き換えに許容する。

Drawer タブの prefetch は本 issue では扱わない。
上記シナリオでは判定できず、どのタブをいつ prefetch するかはグローバル設定値の変更とは別の設計判断になるため。
`retry: 1` などキャッシュ以外の `QueryClient` 設定も本 issue では扱わない (エラー時ポーリング 3 本とジョブポーリング 2 本の `refetchInterval` にも触れない)。

## 完了条件

- 上記 2 シナリオの before / after のリクエスト数と `X-Cache-Status: MISS` の件数とロード表示回数が本 issue に記録されている。
- 候補 1 / 3 は採否基準の計測結果とともに採否が記録され、基準を満たした場合のみ実装されている。候補 2 は実装されている。全候補の結果が記録されていれば、候補 1 / 3 が「変更なし」でも close できる。
- 計測が実施できない場合 (実環境の AWS profile が使えない等) は、計測に依らない候補 2 のみを実施し、候補 1 / 3 を見送った理由を本 issue に記録して close する。この場合、上記 2 条件の計測の記録は要らない (docs/issues/0081 の「実環境で採取できない場合」のフォールバックと対応する)。
- `useSecretValue` / `useSSMValue` / `useS3ObjectPreview` / `useGcsObjectPreview` に `staleTime: 0` が明示され、`gcTime` のグローバル既定を変えた場合は候補 1 に列挙した 8 本に既定 5 分以下の個別指定がある。
- 候補 2 / 3 の実装で AGENTS.md の「`staleTime` はバックエンドのキャッシュ TTL に合わせて `60_000`(60秒)を基本とする」の記述と実装が食い違う場合、AGENTS.md が更新されている。
- Drawer タブの prefetch と、`retry` / `refetchInterval` などキャッシュ以外の設定は本 issue では扱わない。
- `CHANGES.md` の `## develop` に `[UPDATE]` エントリと担当者行が記載されている (候補 2 の整理は `### misc`)。
- `mise run check` が通過する。

## 未確定論点

- 候補 1 の `gcTime` の具体値。シナリオ 1 のロード表示回数が 0 になる最小値を候補に、ブラウザのメモリ使用量を見て決め、採用時に値と根拠を本 issue に記録する。

## 関連

- docs/issues/0078 / docs/issues/0081: 同じ TODO 項目からの分割。本 issue は frontend のキャッシュ設定のみを扱う。
- docs/issues/0079: TopBar の Refresh が backend のキャッシュを貫通しない問題。本 issue でキャッシュ保持を延ばすと悪化するため、番号順で先に修正する。候補 3 の前提条件。
- docs/issues/closed/0058: 「候補は仮説とし、効果が無ければ変更せず記録する」方針の出典。

## 解決方法

完了条件のフォールバック条項 (計測が実施できない場合は候補 2 のみを実施して close する) を適用した。

### 候補 1 / 3 を見送った理由

候補 1 (`gcTime` 引き上げ) と候補 3 (リソース一覧の `staleTime` 引き上げ) の採否基準は、実環境の AWS profile を使ってブラウザ開発者ツールで操作シナリオを計測すること (ロード表示回数、`X-Cache-Status: MISS` 件数) を前提とする。本セッションは自律実行であり、実環境の AWS profile を用いたブラウザ操作と目視計測が実施できないため、両候補とも実装せず見送った。基準を満たす計測結果が得られた場合は、本 issue の計測手段と採否基準に従って別 issue として再提案できる。

### 候補 2 の実装内容

挙動を変えない整理のみを実施した。

- `frontend/src/main.tsx`: `QueryClient` の `defaultOptions.queries` に `staleTime: 60_000` をグローバル既定として追加した (backend のリソースキャッシュ TTL に合わせた値であることをコメントに記載)。
- `frontend/src/api/queries.ts`: グローバル既定と同値になった 34 箇所の `staleTime: 60_000` 個別指定を削除した (issue 調査時点の 33 箇所 + docs/issues/0075 で追加された `useWAFRules` の 1 箇所)。
- `frontend/src/api/queries.ts`: 暗黙の既定値 0 に依存していた `useSecretValue` / `useSSMValue` / `useS3ObjectPreview` / `useGcsObjectPreview` の 4 フックに `staleTime: 0` を明示し、「開くたびに取得する」意図のコメントを明示指定に合わせて更新した。
- `components/Sidebar.tsx` (`queryFn: skipToken`) と `views/GcpSidebar.tsx` (`enabled: false`) の観測専用クエリ 2 本は fetch しないため変更していない。`Infinity` / `5 * 60 * 1000` / `QUERY_HISTORY_STALE_TIME` / `30_000` / `0` の既存個別指定はそのまま残した。
- `AGENTS.md`: staleTime の記述を「`main.tsx` のグローバル既定 60 秒 + 例外のみ個別指定」の実装に合わせて更新した。
- `CHANGES.md`: 挙動を変えない整理のため `### misc` にエントリを追加した。

`mise run check` 通過 (frontend 67 ファイル 579 テスト、backend lint / test 含む)。計測記録は完了条件のフォールバック条項によって不要。
