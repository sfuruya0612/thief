// クエリ結果からグラフのカテゴリと系列を作る純関数のテスト。数値列の判定は
// ResultTable の numericCols と同じ定義 (非空のセルが 1 つ以上あり、非空のセル全てが
// 数値) であることを、カンマ区切りと欠損だけの列で確認する。
import { describe, expect, it } from 'vitest';
import {
  initialChartSelection,
  isNumericColumn,
  numericColumnIndexes,
  QUERY_CHART_MAX_CATEGORIES,
  queryChartCapabilities,
  queryChartCategories,
  queryChartCategoryCount,
  queryChartOverCategoryLimit,
  queryChartSeries,
} from './queryChart';

const columns = ['name', 'count', 'ratio', 'label'];
const rows = [
  ['a', '1', '1.5', ''],
  ['b', '', '2.5', 'x'],
  ['c', '1,234', '', ''],
  ['d', '5', '', ''],
];

describe('isNumericColumn', () => {
  it('空セルを許容し、全ての非空セルが数値なら数値列とする', () => {
    expect(isNumericColumn(rows, 1)).toBe(true);
    expect(isNumericColumn(rows, 2)).toBe(true);
  });

  it('数値でないセルが混ざる列は数値列としない', () => {
    expect(isNumericColumn(rows, 3)).toBe(false);
    expect(isNumericColumn(rows, 0)).toBe(false);
  });

  it('欠損だけの列は数値列としない', () => {
    expect(isNumericColumn([['', ''], ['']], 0)).toBe(false);
  });
});

describe('numericColumnIndexes', () => {
  it('数値列の index だけを列の順に返す', () => {
    // count と ratio は空セルを許容して数値だけ、label は 'x' が混ざるため非数値。
    expect(numericColumnIndexes(columns, rows)).toEqual([1, 2]);
  });

  it('カンマ区切りを数値として扱う', () => {
    expect(numericColumnIndexes(['n'], [['1,234']])).toEqual([0]);
  });

  it('行が無い結果では数値列が無い', () => {
    expect(numericColumnIndexes(columns, [])).toEqual([]);
  });
});

describe('queryChartCapabilities', () => {
  it('列が無い結果は理由を noColumns にする', () => {
    expect(queryChartCapabilities([], [])).toEqual({
      numericColumns: [],
      disabledReason: 'noColumns',
    });
  });

  it('数値列が無い結果は理由を noNumericColumns にする', () => {
    expect(queryChartCapabilities(['name'], [['a']])).toEqual({
      numericColumns: [],
      disabledReason: 'noNumericColumns',
    });
  });

  it('数値列があれば無効の理由を返さず、候補列を返す', () => {
    expect(queryChartCapabilities(columns, rows)).toEqual({
      numericColumns: [1, 2],
      disabledReason: null,
    });
  });
});

describe('initialChartSelection', () => {
  it('X は先頭の列、Y は先頭の数値列を選ぶ', () => {
    expect(initialChartSelection(columns, rows)).toEqual({ xIndex: 0, yIndexes: [1] });
  });

  it('数値列が無いときは Y を選ばない', () => {
    expect(initialChartSelection(['name'], [['a']])).toEqual({ xIndex: 0, yIndexes: [] });
  });

  it('先頭の列が数値列のときは X と Y が同じ列になる', () => {
    expect(initialChartSelection(['n'], [['1']])).toEqual({ xIndex: 0, yIndexes: [0] });
  });
});

describe('queryChartCategories', () => {
  it('X 列の値を文字列のまま行の順に並べる', () => {
    expect(queryChartCategories(rows, 0)).toEqual(['a', 'b', 'c', 'd']);
  });

  it('欠損のセルは空文字列にする', () => {
    expect(queryChartCategories([['a', ''], ['b']], 1)).toEqual(['', '']);
  });
});

describe('queryChartCategoryCount', () => {
  it('distinct な値の数を返す (重複は 1 つに数える)', () => {
    expect(queryChartCategoryCount([['a'], ['a'], ['b']], 0)).toBe(2);
  });

  it('空文字列 (欠損) も 1 つの値として数える', () => {
    expect(queryChartCategoryCount([[''], [''], ['a']], 0)).toBe(2);
  });

  it('上限ちょうどは超えていない扱い、上限を 1 つ超えると超えている扱いにする', () => {
    const atLimit = Array.from({ length: QUERY_CHART_MAX_CATEGORIES }, (_, i) => [`v${i}`]);
    expect(queryChartOverCategoryLimit(atLimit, 0)).toBe(false);
    expect(queryChartOverCategoryLimit([...atLimit, ['over']], 0)).toBe(true);
  });
});

describe('queryChartSeries', () => {
  it('Y 列を系列にし、欠損 (空文字列) は null にする', () => {
    expect(queryChartSeries(columns, rows, [1, 2])).toEqual([
      { name: 'count', data: [1, null, 1234, 5] },
      { name: 'ratio', data: [1.5, 2.5, null, null] },
    ]);
  });

  it('Y 列の順序を系列の順序として保つ', () => {
    expect(queryChartSeries(columns, rows, [2, 1]).map((s) => s.name)).toEqual(['ratio', 'count']);
  });
});
