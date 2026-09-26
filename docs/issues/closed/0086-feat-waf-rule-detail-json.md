# WAF のルールを選択して Statement を含むルール定義の JSON を Drawer で参照できるようにする

Created: 2026-07-27
Completed: 2026-07-27
Model: Claude Fable 5

## 背景

TODO.md の次の項目に対応する。

> WAF の個々のルールの詳細もみれるようにしたい

docs/issues/closed/0075 で Drawer の Rules タブ (`frontend/src/components/Drawer/DrawerWAFRules.tsx`) が追加され、Web ACL のルール一覧 (name, priority, action, statement) を参照できるようになった。
ただし `statement` は `wafRuleStatement` (`backend/internal/aws/waf.go:277-316`) による種別名への要約であり、マッチ条件の中身 (どのヘッダをどの値と比較するか等) は backend の `WAFRule` (`backend/internal/aws/waf.go:187-192`) に含まれず捨てられている。
`DrawerWAFRules.tsx` の DataTable は行選択が無効 (`onSelect={() => {}}`、46 行) で、利用者はルールの種別名までしか確認できない。

docs/issues/closed/0075 は「Statement の中身を丸ごと JSON で返す」案を「入れ子の巨大な構造体になりテーブル表示に向かない」として却下し、マッチ条件の詳細表示をスコープ外と明記した。
この却下はテーブルのセルに丸ごと表示する前提の判断であり、テーブルの外に選択式の詳細表示を設ける本 issue には当てはまらない。
本 issue でその積み残しを扱う。

## 目的

Rules タブでルールを選択すると、そのルールのマッチ条件 (Statement) を含むルール定義全体が参照できる。

## 設計判断

- backend は `WAFRule` に `rule_json` (JSON タグ `rule_json` の string) を追加し、`newWAFRule` で SDK の `Rule` 全体から生成する。Statement だけでなく Action, OverrideAction, VisibilityConfig, RuleLabels, CaptchaConfig, ChallengeConfig を含むルール定義全体を返す。
  - 却下案: Statement のみを返す。要望の「個々のルールの詳細」にはメトリクス設定 (VisibilityConfig) やラベルも含まれ、Statement だけに絞る理由が無いため却下。
- 生成は純関数 `wafRuleJSON(rule waftypes.Rule) string` に切り出す。一度 `map[string]any` に落とし、null 値のみを再帰的に除去してから再度 Marshal する。
  - 却下案: SDK 構造体をそのまま `json.Marshal` する。未設定の nil フィールドが null として大量に出力され、実際の設定が null に埋もれて読み取りにくくなるため却下。
  - 空オブジェクトは、元から空のものも null 除去で空になったものも保持する。WAFv2 の API では空オブジェクトであること自体が設定の判別子になっている (`NoneAction` (`types/types.go:3126`) と `UriPath` (同 5626) はフィールドを持たない空構造体、`AllowAction` (同 76), `BlockAction` (同 422), `CountAction` (同 985) も nullable フィールド 1 つだけを持つ: いずれも aws-sdk-go-v2 wafv2 v1.74.1)。空オブジェクトまで除去すると `{"Action":{"Block":{}}}` や `{"OverrideAction":{"None":{}}}` や `{"FieldToMatch":{"UriPath":{}}}` が出力から消え、ルールの動作とマッチ条件が読み取れなくなる。`NoneAction` のドキュメントコメント (`types/types.go:3126` 付近) には `JSON specification: "None": {}` と記されており、空オブジェクトが API の JSON 仕様上の表現であることは SDK ソースで確認できる。
    - 却下案: null 除去の結果空になったオブジェクトも再帰的に除去する。上記のとおり Action, OverrideAction, FieldToMatch の判別子が消え、本 issue の目的 (マッチ条件の参照) を損なうため却下。
  - `map[string]any` への変換は `json.Decoder` の `UseNumber()` を使い、数値を float64 に落とさず元の表記のまま保つ。
  - キーは Go のフィールド名そのまま (PascalCase) になる (wafv2 の SDK 構造体は json タグを持たないため、`json.Marshal` はフィールド名をキーに使う)。PascalCase は WAFv2 API の JSON 仕様上の表記 (`NoneAction` のドキュメントコメントの `JSON specification: "None": {}` や `aws wafv2 get-web-acl` の出力と同じ) であり、コンソールや CLI の出力と直接見比べられるため、そのまま採用する。
    - 却下案: camelCase や snake_case へ変換する。API の JSON 仕様と表記が変わって見比べにくくなる上、キー変換の実装と保守が増えるため却下。
  - キーの並び順は、`map[string]any` を経由するため `encoding/json` の仕様で辞書順になり、SDK 構造体のフィールド順とは異なる (`Name` と `Statement` が離れる等)。JSON オブジェクトのメンバ順序に意味は無い (RFC 8259) ため、辞書順を受容する。
    - 却下案: 構造体のフィールド順を保って出力する。`json.Decoder` のトークン単位の処理で順序付きの中間表現を自前実装する必要があり、順序に意味が無い以上、複雑さに見合わないため却下。
  - 値のうち `[]byte` のフィールド (`ByteMatchStatement.SearchString` 等) は `encoding/json` の仕様により base64 文字列になる。base64 のまま返す。
    - 却下案: UTF-8 にデコードして素の文字列で返す。`SearchString` は任意のバイト列を取り得て、デコードできない値の扱いが別途必要になるため却下。
  - 配列は要素内の null 除去のみを行い、要素自体は取り除かない (空オブジェクトになった要素も残す)。空配列もそのまま残す。要素を取り除くと残りの要素の位置がずれ、コンソールや `aws wafv2 get-web-acl` の出力と要素の対応が取れなくなるため。
  - Marshal に失敗した場合は空文字を返し、frontend はダッシュ表示にする (1 ルールの変換失敗で一覧全体を落とさない)。
- 既存の `TestNewWAFRule` (`backend/internal/aws/waf_test.go:116`) は `WAFRule` 全体を `reflect.DeepEqual` で比較しており、フィールド追加で全ケースの期待値に影響する。比較を `rule_json` 以外のフィールド単位に変え、JSON の内容は `wafRuleJSON` の専用テストで検証する。
  - 却下案: 各ケースの期待値に JSON リテラルを追記する。ケースを足すたびに巨大なリテラルの保守が要るため却下。
- 新しいエンドポイントは作らず、既存の `GET /api/aws/profiles/{profile}/waf/rules` のレスポンス拡張で済ませる。
  - 却下案: ルール単位の詳細エンドポイント。`GetWebACL` (aws-sdk-go-v2 wafv2 v1.74.1) は Web ACL 単位でしか取得できず、サーバ側で同じレスポンスから 1 ルールを切り出すだけになる。エンドポイントとキャッシュキーが増える割に AWS API 呼び出しは減らないため却下。
  - レスポンス肥大について: ルール一覧は Drawer で Web ACL を選択したときだけ取得される遅延取得であり、一覧 (`WAFResource`) には埋め込まないという docs/issues/closed/0075 の判断は変えない。レスポンスの増分は 1 ルールあたり数百バイトから数 KB (マネージドルールグループ参照で `RuleActionOverrides` を多く持つ場合に大きくなる) の概算で、対象が選択した 1 Web ACL のルールに限られるため受容する。
- frontend は `WAFRuleRaw` に `rule_json`、`WAFRuleRow` に `ruleJson` を追加し、`wafRuleFromRaw` (`frontend/src/lib/normalize.ts`) で `raw.rule_json ?? ''` として写す。
- `DrawerWAFRules.tsx` の DataTable の `onSelect` で選択行を state に持ち、テーブルの下に選択ルールの `ruleJson` を `JSON.stringify(JSON.parse(ruleJson), null, 2)` で整形して表示する。パースに失敗した場合は文字列をそのまま表示し、`ruleJson` が空文字の場合はダッシュ表示にする。詳細表示の直上に、選択中のルール名の見出しを既存の `Rules (n)` 見出しと同じ `<h3>` で置く。列フィルタで選択行が表示から外れても (後述のとおり選択は残す)、表示中の JSON がどのルールのものかを見出しから読み取れるようにするため。
  - 却下案: backend が `json.MarshalIndent` で整形済みの文字列を返す。frontend の parse と整形は不要になるが、インデント幅などの整形様式は表示の関心事であり、表示都合の変更のたびに backend の変更になる上、インデントの分だけレスポンスが増えるため却下。
  - パース失敗の分岐は、backend が `json.Marshal` の出力か空文字を返す契約の下では通常到達しない防御である。それでも完了条件でテストするのは、backend の Marshal 失敗分岐 (SDK の型から失敗入力を構成できず自動テストの対象外) と異なり、モックで不正な文字列を与えるだけで検証できるため (対象外の線引きは到達可能性ではなく失敗入力の構成可能性で行う)。
  - 表示は `<pre className="logbox">` とし、`.logbox` の既定の高さ制限 (`frontend/src/app.css:1689-1700` の `max-height: 200px` と内部スクロール) を保つ。本タブは一覧と詳細の併置 (後述) を採るため、`<pre>` を無制限に伸ばすと長いルール定義で一覧が画面外へ押し出され、行を切り替えながら見比べる目的を損なう。全文は `<pre>` 内のスクロールで読める。
    - 却下案: `DrawerObjectPreview.tsx:39` と `DrawerValueEditor.tsx:124` に合わせて `style={{ maxHeight: 'none' }}` で高さ制限を外し、Drawer 本体のスクロールに委ねる。この 2 つの前例は `<pre>` がその表示の唯一の内容で、見え続けさせたい一覧が無い構成のため、本タブには当てはまらず却下。
  - 行の識別はルール名で行う。docs/issues/closed/0075 の実装が DataTable の行キー `id` にルール名を充てており、「ルール名は Web ACL 内で一意のためそのまま使う」という前提は `frontend/src/types/aws.ts:775` のコメントに明記されている。SDK ドキュメントが一意性を明記しているのは `Priority` のみでルール名には無いが、本 issue は既存実装が置いたこの前提を踏襲する (行キーの変更は本 issue のスコープ外)。選択はルール名で state に持ち、`DataTable` の `selectedId` にも渡して選択行の強調 (`selected` クラス、`frontend/src/components/DataTable.tsx:177-179`) を既定の仕組みで得る。再取得で該当名のルールが無くなった場合は選択を解除して詳細表示を消す。
  - `DataTable` の列フィルタで選択行が表示から外れた場合は、選択と JSON 表示をそのまま残す。列フィルタは表示の絞り込みであり、選択の解除条件にしない (一覧ビューでも絞り込みで選択行が隠れたとき Drawer が開いたまま残る挙動を受容している)。
  - テーブルの下に詳細を併置する構成はリポジトリに前例が無い (`DrawerObjectBrowser` はプレビュー時に一覧をプレビュー表示へ丸ごと差し替える方式: `frontend/src/components/Drawer/DrawerObjectBrowser.tsx:156-174` の早期 return)。行を切り替えながら JSON を見比べる用途では一覧が見え続ける必要があるため、差し替えではなく併置を新規に採る。
    - 却下案: `DrawerObjectBrowser` と同じ差し替え方式。JSON 表示中に一覧が隠れ、閉じる操作を挟まないと次のルールへ移れないため却下。
  - 選択の操作は行の切り替えのみとする。同じ行の再選択で閉じるようなトグルは付けない。`DataTable` の行クリック (`DataTable.tsx:180`) は選択状態に関わらず常に `onSelect` を呼ぶ単純通知で、トグルには呼び出し側で選択状態と比較する追加実装が要る。閉じる操作の需要は観測されていないため実装しない (YAGNI)。
- 追加の AWS API 呼び出しと権限は不要 (`GetWebACL` のレスポンスに含まれる情報のみを使う)。
- CLI には WAF コマンドが存在しないため、CLI 側の変更は不要。

## 完了条件

- `WAFRule` の `json.Marshal` 出力に `rule_json` キーが現れることを検証するテストが `backend/internal/aws/waf_test.go` にある。
- `wafRuleJSON` のテーブル駆動テストがあり、次のケースを含む: Statement と Action を持つルールで null フィールドが出力に現れない、`Action: {Allow: {}}` の `"Allow"` キーが空オブジェクトとして残る、`OverrideAction: {None: {}}` が残る、`FieldToMatch: {UriPath: {}}` が残る、null 除去で空オブジェクトになった配列要素が残る、空配列が保持される、`ByteMatchStatement.SearchString` が base64 文字列として出力される、VisibilityConfig の `SampledRequestsEnabled` と `MetricName` のキーが出力に含まれる、非ポインタ型のゼロ値が null 除去で消えずに残る (`Priority` の 0 と `SampledRequestsEnabled` の false。null 以外の falsy な値を巻き添えにしない検証)、`RateBasedStatement.Limit` (int64) に float64 で正確に表現できない値 (例: 9007199254740993) を与えたとき数値がその表記のまま出力される (`UseNumber()` の検証)。
- `wafRuleJSON` の Marshal 失敗パス (空文字を返す分岐) は、SDK の型から Marshal に失敗する入力を構成できないため自動テストの対象外とする (防御的分岐として実装のみ行う)。
- `TestNewWAFRule` が `rule_json` 以外のフィールド単位の比較になっており、既存ケースが通過する。あわせて、戻り値の `RuleJSON` が `wafRuleJSON` の出力と一致するアサーションを含む (フィールドの配線の検証)。
- `wafRuleFromRaw` のテスト (`frontend/src/lib/normalize.test.ts`) に、`rule_json` 欠落時の既定値 (空文字) のケースと、値があるとき `ruleJson` に写るケースが追加されている。既存のケース (`normalize.test.ts:658-672`) は Row 全体の `toEqual` 比較のため、期待値に `ruleJson` を追加して通過させる。
- `DrawerWAFRules.test.tsx` に次のケースが追加されている: 行クリックで選択したルール名の見出しと `<pre>` が現れ、`<pre>` の内容が `JSON.stringify(JSON.parse(rule_json), null, 2)` と一致する。行クリックで該当行に `selected` クラスが付く。選択中の行とは別の行をクリックすると `<pre>` の内容が後から選択した行の `rule_json` の整形結果に置き換わり、`selected` クラスは後から選択した行にだけ付く。`ruleJson` が空文字の行を選択したとき `<pre>` は描画されず、見出しの下にダッシュが表示される。`ruleJson` が JSON として不正な行を選択したとき文字列がそのまま表示される。列フィルタで選択行が表示から外れたとき、見出しと `<pre>` は表示されたまま残る。選択中のルール名が再取得後の一覧から消えたとき `<pre>` が消え、どの行にも `selected` クラスが付かない (再取得は fetch モックの応答を差し替え、同一マウントのまま QueryClient の `invalidateQueries` を呼んで起こす。既存ヘルパー `renderWithQC` は QueryClient を内部生成して返さないため、テストから `invalidateQueries` を呼べるよう QueryClient を返す形に変更する)。
- ルールの編集、JSON のツリー表示等の整形 UI、Rules タブの列構成の変更、行キー (`id` へのルール名充当) の変更、`SearchString` 等の `[]byte` 値の base64 デコード表示 (JSON には base64 のまま載る) は本 issue では扱わない。同一行の再選択で閉じるトグルは実装しないため (設計判断に記載)、その不在を検証するテストも書かない。
- `CHANGES.md` の `## develop` の `[ADD]` 群に、種別順 (UPDATE → ADD → CHANGE → FIX) を保つ位置へ挿入する形で `[ADD]` エントリと担当者行が記載されている。
- `mise run check` が通過する。

## 関連

- docs/issues/closed/0075 (Rules タブの導入元): マッチ条件の詳細表示をスコープ外と明記しており、本 issue が引き受ける。
- docs/issues/0088 (Associated 件数の修正): 同じ `backend/internal/aws/waf.go` と `waf_test.go` を触る。番号順 (本 issue → docs/issues/0088) に実装すれば衝突しない。
- docs/issues/0087 (WAF の列順変更): 同じ WAF ビューの要望だが、本 issue は Drawer のサブタブと backend を、docs/issues/0087 は一覧の列定義を触るためファイルが重ならず衝突しない。
- docs/issues/0089, docs/issues/0090 (CloudFront の列変更と Behaviors タブ): 同じ `frontend/src/types/aws.ts`、`frontend/src/lib/normalize.ts`、`normalize.test.ts` を触る。編集箇所は型と関数の単位で独立しており、番号順に実装すれば衝突しない。
- docs/issues/0087, docs/issues/0088, docs/issues/0089, docs/issues/0090: `CHANGES.md` の `## develop` は 5 issue 全てが変更する。番号順に直列で実装し、各 issue のエントリを種別順 (UPDATE → ADD → CHANGE → FIX) を保つ位置へ挿入すれば衝突しない。

## 解決方法

- backend: `WAFRule` に `RuleJSON string` (JSON tag `rule_json`) を追加し、`newWAFRule` から `wafRuleJSON(r)` で設定する。`wafRuleJSON` は `waftypes.Rule` を一度 `json.Marshal` した後、`UseNumber()` を設定した `json.Decoder` で再デコードして大きな int64 の精度劣化を防ぎ、`removeJSONNulls` で null 値のみを再帰的に除去して `json.Marshal` し直す。空オブジェクト `{}` や空配列 `[]` (WAFv2 JSON スキーマの型判別子として使われる `NoneAction{}`, `UriPath{}`, `AllowAction{}` 等) はそのまま残す。marshal/decode に失敗した場合は空文字を返す。
- backend: `waf_test.go` に `wafRuleJSON`/`removeJSONNulls` を対象とするテーブル駆動テスト `TestWAFRuleJSON` (null 除去、空オブジェクト/空配列の保持、base64 の `SearchString`、`VisibilityConfig` のキー保持、ゼロ値の保持、`UseNumber()` による大きな int64 の精度保持など 10 ケース) と、`RuleJSON` を含む JSON marshal を確認する `TestWAFRuleJSONHasRuleJSONKey` を追加する。`TestNewWAFRule` は構造体丸ごとの `reflect.DeepEqual` をやめ、フィールドごとの比較 + `wafRuleJSON(tt.rule)` とのクロスチェックに変更する。
- frontend: `WAFRuleRaw`/`WAFRuleRow` にそれぞれ `rule_json`/`ruleJson` を追加し、`wafRuleFromRaw` で `raw.rule_json ?? ''` としてマッピングする。`normalize.test.ts` にマッピングと未定義時のデフォルト値のテストを追加する。
- frontend: `DrawerWAFRules.tsx` に選択状態 (`useState<string | null>`) と、選択中のルール名から `rules` を引き直す `selectedRule` (`useMemo`) を追加し、`DataTable` に `onSelect`/`selectedId` を接続する。選択中のルールがあれば見出しとその下に `RuleJSONDetail` (JSON をパースして整形表示、空文字はダッシュ、パース失敗時は生文字列をそのまま表示) を一覧の下に併置表示する (差し替え方式ではなく併置方式を採用し、一覧を見ながら JSON を確認できるようにする)。選択は行の絞り込みでは消えず (`selectedName` はフィルタと独立)、再取得で選択中のルール名が一覧から消えたときは `selectedRule` が `null` になり詳細表示も自動的に消える (`useEffect` 不要)。
- frontend: `DrawerWAFRules.test.tsx` に行クリックでの選択・詳細表示・`selected` クラス付与、選択行の切り替え、`ruleJson` が空文字/不正 JSON の場合の表示、列フィルタで選択行が表示から外れても詳細表示が残ること、再取得で選択中のルールが消えたときに詳細表示と `selected` クラスが消えることを検証する 7 ケースを追加する。
- `CHANGES.md` の `## develop` に `[ADD]` エントリを追加する。
- `mise run check` (fmt / lint / test) が backend / frontend ともに成功することを確認した。
