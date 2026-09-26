// クエリ結果 (columns / rows の string[][]) を QueryResultChart が描ける形へ変換する
// 純関数群。数値の判定は lib/numericCell.ts を ResultTable と共有する。DuckDB や React に
// 触れないため単体テストで固定できる。
import { isNumericCell, parseNumericCell } from './numericCell';

// QUERY_CHART_MAX_CATEGORIES は X 軸に置ける distinct な値の上限。docs/issues/0196 の結果上限
// (10000 行) の範囲でも、カテゴリ 10000 本の棒グラフは判読できないため、上限を設けて
// SQL での集計 (GROUP BY) または LIMIT を促す。
export const QUERY_CHART_MAX_CATEGORIES = 5000;

// QueryResultChartType は描けるグラフの種類。値は ECharts の系列型名をそのまま使う。
export type QueryResultChartType = 'bar' | 'line';

// QueryChartSeries は 1 つの Y 列から作る系列。値の無いセルは欠損として null にする。
export interface QueryChartSeries {
  name: string;
  data: (number | null)[];
}

// QueryChartSelection は軸の選択。xIndex はカテゴリ軸に置く列、yIndexes は値にする列で、
// 配列の順が系列の順になる。
export interface QueryChartSelection {
  xIndex: number;
  yIndexes: number[];
}

// QueryChartDisabledReason は Chart に切り替えられない理由。日本語の文言は呼び出し側が
// i18n のキー (drawerObjectQuery.chart.*) に対応させて選ぶ。
export type QueryChartDisabledReason = 'noColumns' | 'noNumericColumns';

// QueryChartCapabilities は結果に対して Chart が使えるかの判定結果。
export interface QueryChartCapabilities {
  // numericColumns は Y 列の候補になる列の index (列の順)。
  numericColumns: number[];
  // disabledReason は Chart に切り替えられない理由 (切り替えられる場合は null)。
  disabledReason: QueryChartDisabledReason | null;
}

// isNumericColumn は列 index が数値列かを返す。非空のセルが 1 つ以上あり、非空のセル全てが
// isNumericCell を通る列を数値列とする (ResultTable の numericCols と同じ定義)。
// 空文字列は null に対応するため判定から除き、欠損だけの列は数値列にしない。
export function isNumericColumn(rows: string[][], index: number): boolean {
  let hasValue = false;
  for (const row of rows) {
    const v = row[index] ?? '';
    if (v === '') continue;
    if (!isNumericCell(v)) return false;
    hasValue = true;
  }
  return hasValue;
}

// numericColumnIndexes は数値列の index を列の順に返す。
export function numericColumnIndexes(columns: string[], rows: string[][]): number[] {
  const indexes: number[] = [];
  for (let i = 0; i < columns.length; i += 1) {
    if (isNumericColumn(rows, i)) indexes.push(i);
  }
  return indexes;
}

// queryChartCapabilities は Y 列の候補と、Chart に切り替えられない理由をまとめて返す。
// 列が無い結果と数値列が無い結果ではグラフを描けない (高さを取れる列が存在しないため)。
export function queryChartCapabilities(
  columns: string[],
  rows: string[][],
): QueryChartCapabilities {
  if (columns.length === 0) return { numericColumns: [], disabledReason: 'noColumns' };
  const numericColumns = numericColumnIndexes(columns, rows);
  return {
    numericColumns,
    disabledReason: numericColumns.length === 0 ? 'noNumericColumns' : null,
  };
}

// initialChartSelection は結果を表示したときの初期選択を返す。X 列は先頭の列、Y 列は
// 先頭の数値列 1 つ。列が 1 個でそれが数値列のときは X と Y が同じ列になるが、X は値を
// カテゴリとして、Y は同じ値を高さとして描くため、そのまま許容する。
export function initialChartSelection(columns: string[], rows: string[][]): QueryChartSelection {
  const numericColumns = numericColumnIndexes(columns, rows);
  return { xIndex: 0, yIndexes: numericColumns.length > 0 ? [numericColumns[0]] : [] };
}

// queryChartCategories は X 列の値をカテゴリ軸のラベルとして行の順に並べる。値は文字列の
// まま使う (数値や日付への解釈は行わない)。
export function queryChartCategories(rows: string[][], xIndex: number): string[] {
  return rows.map((row) => row[xIndex] ?? '');
}

// queryChartCategoryCount は X 列の distinct な値の数を返す。同じ値が繰り返す結果は
// カテゴリ数が distinct 数に収まるため、行数ではなく distinct 数で上限を判定する。空文字列
// (欠損) も 1 つの値として数える。カテゴリ軸には空のラベルとして並ぶためである。
export function queryChartCategoryCount(rows: string[][], xIndex: number): number {
  return new Set(queryChartCategories(rows, xIndex)).size;
}

// queryChartOverCategoryLimit は X 列の distinct な値の数が上限を超えているかを返す。
// 超えている結果はグラフを描かず、SQL での集計または LIMIT を促す。
export function queryChartOverCategoryLimit(rows: string[][], xIndex: number): boolean {
  return queryChartCategoryCount(rows, xIndex) > QUERY_CHART_MAX_CATEGORIES;
}

// queryChartSeries は Y 列を系列へ変換する。値は parseNumericCell で数値にし、空文字列は
// 欠損として null にする (0 を描くと「値が 0 だった」と誤読させる)。
export function queryChartSeries(
  columns: string[],
  rows: string[][],
  yIndexes: number[],
): QueryChartSeries[] {
  return yIndexes.map((yIndex) => ({
    name: columns[yIndex] ?? '',
    data: rows.map((row) => {
      const v = row[yIndex] ?? '';
      return v === '' ? null : parseNumericCell(v);
    }),
  }));
}
