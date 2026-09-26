// クエリ結果のセルが数値かを判定する純関数。ResultTable の数値ソートと、グラフの Y 列
// 候補の判定 (lib/queryChart.ts) で同じ基準を共有する。片方だけ基準が変わると、
// 表では数値として並ぶ列がグラフでは非数値になる、というずれが生じる。

// isNumericCell はセルが数値として解釈できるかを返す。カンマ区切りは許容し、
// 指数表記と前後の空白は数値としない (ResultTable の従来の判定を変えない)。
export function isNumericCell(v: string): boolean {
  return /^-?[\d,]+(\.\d+)?$/.test(v);
}

// parseNumericCell はセルを数値にする。カンマを除いたうえで Number() が有限値を返す値は
// その結果を、返さない値 (数値でない文字列、オーバーフローした指数表記) は 0 を返す。
// isNumericCell を通らなくても有限値になる値 (指数表記、前後に空白がある値) があるため、
// その除外は呼び出し側が isNumericCell で行う。空文字列 (issues/0196 の変換規則で null に
// 対応する) を欠損として扱うかも呼び出し側が決める。
export function parseNumericCell(v: string): number {
  const n = Number(v.replace(/,/g, ''));
  return Number.isFinite(n) ? n : 0;
}
