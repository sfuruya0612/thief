# util.Parser のエラーラップがチェーンを切り、文言がリポジトリの慣習に従っていない

Created: 2026-08-08
Model: Claude Opus 5
Completed: 2026-08-08

## 背景

issue 0128 (`internal/cli/ec2.go` の実行エラーが `errors.As` で到達できない不具合) の作業中に、`backend/internal/util/parser.go` に 2 つの問題を見つけた。

該当箇所は 9-16 行である。

```go
func Parser(i interface{}) ([]byte, error) {
	bytes, err := json.Marshal(i)
	if err != nil {
		return nil, fmt.Errorf("json Marshal error: %v", err)
	}

	return bytes, nil
}
```

### 問題 1: `%v` でエラーチェーンを切っている

`json.Marshal` は失敗時に `*json.UnsupportedTypeError`、`*json.UnsupportedValueError`、`*json.MarshalerError` のいずれかを返す。`%v` は文字列に展開するため `Unwrap()` の連鎖が切れ、呼び出し側は `errors.As` でこれらの型を取り出せない。

AGENTS.md の「backend (Go)」節は「ラップは `fmt.Errorf` の `%w` 動詞を使う」「比較は `errors.Is`、型取り出しは `errors.As` を用いる」と定めており、これに反する。

issue 0126 は `internal/cli` の `%v` を棚卸ししたが `internal/util` はスコープに含めていなかった。issue 0128 は `internal/util/executer.go` の `%v` を扱ったが、同じパッケージの `parser.go` は対象外だった。

### 問題 2: 文言がリポジトリの慣習に従っていない

`json Marshal error` は名詞句で始まり、`Marshal` が文中で大文字始まりになっている。issue 0125 と 0117 で `internal/aws` 全体を「動詞 + 対象」の形式 (`describe ssm instance information`、`create sso oidc token` など) に揃えたが、`internal/util` はその対象外だった。

また `error` という語自体が冗長である。`fmt.Errorf` の戻り値は必ずエラーであり、呼び出し側は `Error()` の文字列を自分の接頭辞の後ろに連結する。

## 到達可能性

本番コードからこのエラー経路には到達しない。`util.Parser` の呼び出しは 3 箇所のみで、いずれも文字列フィールドだけを持つ構造体を渡している。

| 呼び出し元 | 渡している値 |
| --- | --- |
| `internal/cli/session_plugin.go:14` | `struct { SessionId, StreamUrl, TokenValue string }` |
| `internal/cli/ec2.go:177` | `struct { Target string }` |
| `internal/cli/ecs.go:220` | `struct { Target string `json:"Target"` }` |

`json.Marshal` が失敗するのはチャネル・関数・複素数・循環参照・`NaN` / `Inf` を含む場合であり、string だけの構造体では起こり得ない。したがってユーザーに影響する不具合ではなく、実装の一貫性の問題である。

`Parser` は `interface{}` を受け取る汎用関数であり、今後の呼び出し元が到達可能な値を渡す余地はある。

## 再現手順

`internal/util/parser_test.go` の `TestParser_Invalid` が循環参照を渡してこの経路を通している。

1. 同テストの `Parser(m1)` の戻り値に対し `errors.As(err, new(*json.UnsupportedValueError))` を評価する。
2. `false` になる。`%v` でラップされているため元の型に到達できない。

## 影響

- `util.Parser` の呼び出し側が `json.Marshal` の失敗理由を型で判別できない。ただし現在の 3 呼び出し元はこの経路に到達しない。
- エラー文言が `internal/aws` および `internal/cli` で統一した形式から外れている。

## 修正方針

- `%v` を `%w` に変更する。
- 文言を「動詞 + 対象」の形式に変える。`Parser` は呼び出し側から見て「渡した値を JSON にする」関数であるため `marshal json` とする。呼び出し元 3 箇所はいずれも自前の接頭辞 (`marshal session:` / `marshal start session input:` / `marshal target:`) を前置しており、`Parser` 側が「何を」marshal しようとしたかを述べる余地は無い。
- `internal/util/parser_test.go` の `TestParser_Invalid` は `strings.Contains(err.Error(), "json Marshal error")` で旧文言を固定しているため、新文言に合わせて更新する。あわせて `errors.As` で `json.Marshal` の型に到達できることを検証するアサーションを追加する。
- `internal/util` の他のファイルにも同種の `%v` ラップが残っていないか棚卸しし、あれば同じ方針で揃える。

## 完了条件

- `internal/util/parser.go` が `%w` でラップしている。
- `internal/util` 全体に `fmt.Errorf` の `%v` ラップが残っていない。残す場合はコメントで理由を述べている。
- `internal/util/parser_test.go` が `errors.As` により `json.Marshal` が返した型へ到達できることを検証している。
- 新しい文言が「動詞 + 対象」の形式になっており、`error` という語を含まない。
- 呼び出し元 3 箇所の接頭辞と新文言が重複していない。
- `mise run check` が通る。

## 関連

- docs/issues/closed/0128: 起票元。`internal/util/executer.go` の `%v` を扱った issue。同じパッケージの `parser.go` はスコープ外として残した。
- docs/issues/closed/0126: `internal/cli` の `%v` ラップを棚卸しした issue。`internal/util` はスコープ外だった。
- docs/issues/closed/0125: `internal/aws` のラップ文言を「動詞 + 対象」に揃えた issue。

## 解決方法

### 1. `parser.go` のラップを `%w` に変え、文言を `encode value` にした

```go
// 失敗は %w でラップして返す。encoding/json が返す *json.UnsupportedTypeError、
// *json.UnsupportedValueError、*json.MarshalerError に呼び出し側が errors.As で
// 到達できるようにするためである。*json.MarshalerError は自身も Unwrap を持つため、
// MarshalJSON が返した元のエラーまでたどれる。
//
// 文言に json を含めないのは、呼び出し元がいずれも marshal を含む接頭辞を前置しており、
// encoding/json 自身のエラーも json: で始まるためである。どちらとも重複しない語を選ぶ。
func Parser(i interface{}) ([]byte, error) {
	bytes, err := json.Marshal(i)
	if err != nil {
		return nil, fmt.Errorf("encode value: %w", err)
	}

	return bytes, nil
}
```

### 2. 文言は本 issue の修正方針から変えた

修正方針は `marshal json` を提案していたが、これは本 issue の完了条件「呼び出し元 3 箇所の接頭辞と新文言が重複していない」に自ら違反する。実測した連結結果は次のとおり。

| 候補文言 | 連結結果 | 判定 |
| --- | --- | --- |
| `marshal json` | `marshal target: marshal json: json: unsupported type: chan int` | `marshal` が 2 回、`json:` が隣接 |
| `encode json` | `marshal target: encode json: json: unsupported type: chan int` | `json:` が隣接 |
| `encode value` | `marshal target: encode value: json: unsupported type: chan int` | 重複なし |

`encoding/json` 自身のエラー文言が `json: ` で始まるため、`json` を名乗ると必ず隣接重複が生じる。`value` という対象語は `internal/cli/helper.go:58` の `read value from stdin: %w` に既存の前例があり、孤立した語彙ではない。この判断の理由は `parser.go` のコメント、CHANGES.md のエントリ、および専用テストの 3 箇所に記録した。

### 3. `selecter.go` の `%v` も同じ方針で揃えた

完了条件「`internal/util` 全体に `fmt.Errorf` の `%v` ラップが残っていない」を満たすため、`selecter.go:78` を `failed to start bubble tea program: %v` から `run bubble tea program: %w` に変更した。「外部ライブラリ名をそのまま対象に含める」書き方は `internal/cli/ec2.go:233` / `ecs.go:233` の `execute session-manager-plugin command: %w` に前例がある。

`selecter.go:66` の `no items to select`、`selecter.go:83` の `no item selected`、`formatter.go:108` の `column %q not found` は、下位のエラーを受け取ってラップしているのではなくその場で生成する終端エラーであり、`%w` で包む対象が存在しない。呼び出し元 (`ec2.go:249` / `ecs.go:166`) はいずれも `errors.Is` で判別せずそのまま上位へ伝播しているため、センチネル化の実需も無い。YAGNI に従い変更しなかった。

`executer.go` は `fmt.Errorf` の呼び出し自体が無く (issue 0128 で `call.Run()` のエラーをラップせずそのまま返す設計にした)、対象外である。

### 4. テストは 3 段構えにした

- `TestParser_Invalid` をテーブル駆動にし、`json.Marshal` が返す 3 型すべてを引き出す入力を並べた。循環参照 → `*json.UnsupportedValueError`、`make(chan int)` → `*json.UnsupportedTypeError`、`MarshalJSON` が失敗する型 → `*json.MarshalerError`。各行で `errors.As` による型到達、`b == nil`、`encode value: ` の接頭辞、`encode value: error` の非出現を検証する。
- `*json.MarshalerError` の行だけは `wantIs` を指定し、`errors.Is(err, errFailingMarshaler)` を検証する。`*json.MarshalerError` は自身も `Unwrap` を持つため、これが通ることは `Parser` の `%w` と `MarshalerError` の `Unwrap` の**両段**が生きていることの証明になる。
- `TestParser_WrapperDoesNotRepeatCallerPrefix` をテーブル駆動にし、本番の呼び出し経路 3 つの接頭辞をすべて回すようにした。`marshal session` (`session_plugin.go:14` を呼ぶ `ec2.go:174` / `ecs.go:217`)、`marshal start session input` (`ec2.go:181`)、`marshal target` (`ecs.go:224`)。各行で `marshal` の出現回数が 1、`json: json:` の非出現、接頭辞越しの `errors.As` を検証する。

### 5. ミューテーション検証

`parser.go` に対して 6 パターンを実際に適用し、すべて検出できることを確認した。

| # | ミューテーション | 結果 | 落ちたアサーション |
| --- | --- | --- | --- |
| M1 | `%w` を `%v` に戻す | 検出 | `errors.As` 3 行 + 接頭辞テスト 3 行 |
| M2 | 文言を `json Marshal error` に戻す | 検出 | `HasPrefix("encode value: ")` 3 行 |
| M3 | 文言を `marshal json` にする | 検出 | 接頭辞 + `Count("marshal") != 1` 計 6 行 |
| M4 | 文言を `encode json` にする | 検出 | 接頭辞 + `json: json:` の隣接 計 6 行 |
| M5 | 文言を `encode target` にする | 検出 | `HasPrefix` 3 行 |
| M6 | エラー時に非 nil のバイト列を返す | 検出 | `b != nil` 3 行 |

`selecter.go` の変更行に対する 2 パターン (`%w` → `%v`、文言の差し戻し) は **いずれも検出できなかった (0/2)**。`selecter_test.go` の 5 テストはどれも `p.Run()` の失敗経路を通らないためである。`Select` は `tea.NewProgram` と `p.Run()` を関数内に直接書いており差し込み口が無く、`Run` は端末を掴むためテストから呼ぶと TTY の有無に結果が左右される。

差し込み口の追加は構造変更であり本 issue の完了条件の範囲外なので、issue 0134 として切り出した。issue 0127 が `internal/aws/sso_oidc.go` に同じ形の差し込み口を作ったのと同じ扱いである。この 0/2 という実測値が 0134 の起票根拠そのものである。

### 6. レビュー

2 観点で並列レビューした。

- 規約・文言の整合: 高・中の指摘なし。完了条件 6 項目すべて充足、`encode value` の判断は妥当 (`marshal json` を採ると完了条件 5 に自ら違反することを実測で確認)、既存文言との整合あり、CHANGES.md の種別・順序・担当者行も規約どおり。低 1 件は本 issue 自身の記述の不正確さ (後述)。
- テスト検出力: 高 2 件・中 1 件。`selecter.go` の検出力ゼロ (→ 0134 に切り出し済み)、`*json.MarshalerError` の未検証 (→ 上記 4 で対応)、接頭辞テストが 3 箇所中 1 箇所しか固定していない (→ 上記 4 でテーブル化して対応)。

### 7. 本 issue 自身の記述の不正確さ

完了条件は「呼び出し元 3 箇所の接頭辞」と書いているが、実際には 3 箇所のうち `session_plugin.go:14` (`sessionManagerSessionJSON`) は接頭辞を付けずに `util.Parser` のエラーをそのまま返し、さらに 1 段上の `ec2.go:174` / `ecs.go:217` で `marshal session: %w` が付く。接頭辞の種類は 3 つで判定結果は変わらないが、経路の構造が本文の記述と異なる。issue 本文は追記のみとし書き換えていないため、ここに記録する。

## 関連 (追記)

- docs/issues/0134: 本 issue から切り出した。`selecter.go` の変更行を守るテストが書けなかったため、`Select` に差し込み口を作る issue。
