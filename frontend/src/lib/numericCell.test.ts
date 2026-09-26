// 数値セル判定のテスト。ResultTable の数値ソートとグラフの Y 列候補の判定が共有する
// 基準 (カンマ区切りを許容し、指数表記と前後の空白は数値としない) を固定する。
import { describe, expect, it } from 'vitest';
import { isNumericCell, parseNumericCell } from './numericCell';

describe('isNumericCell', () => {
  it('整数・小数・負数・カンマ区切りを数値として扱う', () => {
    for (const v of ['0', '10', '-3', '1.5', '-0.25', '1,234', '-1,234.5']) {
      expect(isNumericCell(v)).toBe(true);
    }
  });

  it('指数表記と前後の空白と全角数字は数値としない', () => {
    for (const v of ['1e3', '1E3', ' 1', '1 ', '１']) {
      expect(isNumericCell(v)).toBe(false);
    }
  });

  it('空文字列と数値でない文字列は数値としない', () => {
    for (const v of ['', 'abc', '1a', '.5', '1.', '+1', '1 234', 'NaN', 'Infinity', 'null']) {
      expect(isNumericCell(v)).toBe(false);
    }
  });
});

describe('parseNumericCell', () => {
  it('カンマを除いて数値にする', () => {
    expect(parseNumericCell('1,234.5')).toBe(1234.5);
    expect(parseNumericCell('-3')).toBe(-3);
    expect(parseNumericCell('0')).toBe(0);
  });

  it('数値として解釈できない値は 0 を返す', () => {
    expect(parseNumericCell('abc')).toBe(0);
    // 空文字列は Number('') が 0 (有限値) になるため 0 を返す。解釈できない値と同じ値だが
    // 経路は異なる。欠損として扱うかは呼び出し側が決める。
    expect(parseNumericCell('')).toBe(0);
  });

  it('有限値にならない指数表記は 0 を返す', () => {
    expect(parseNumericCell('1e999')).toBe(0);
  });

  it('isNumericCell を通らなくても有限値になる値はその値を返す', () => {
    // 呼び出し側が isNumericCell で除外する前提の値。ここでは 0 にならないことを固定する。
    expect(parseNumericCell('1e3')).toBe(1000);
    expect(parseNumericCell(' 1')).toBe(1);
  });
});
