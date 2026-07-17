import { describe, expect, it } from 'vitest';
import { parseCsv } from './parseCsv';

describe('parseCsv', () => {
  it('単純な CSV を行列に分解する', () => {
    expect(parseCsv('a,b,c\n1,2,3')).toEqual([
      ['a', 'b', 'c'],
      ['1', '2', '3'],
    ]);
  });

  it('クォート内のカンマを分割しない', () => {
    expect(parseCsv('a,b\n"1,000",2')).toEqual([
      ['a', 'b'],
      ['1,000', '2'],
    ]);
  });

  it('クォート内の改行をフィールドとして保持する', () => {
    expect(parseCsv('a,b\n"line1\nline2",2')).toEqual([
      ['a', 'b'],
      ['line1\nline2', '2'],
    ]);
  });

  it('"" エスケープをクォート 1 文字に戻す', () => {
    expect(parseCsv('a\n"say ""hi"""')).toEqual([['a'], ['say "hi"']]);
  });

  it('末尾の改行の有無で行数が変わらない', () => {
    expect(parseCsv('a,b\n1,2\n')).toEqual([
      ['a', 'b'],
      ['1', '2'],
    ]);
    expect(parseCsv('a,b\n1,2')).toEqual([
      ['a', 'b'],
      ['1', '2'],
    ]);
  });

  it('空フィールドを空文字として保持する', () => {
    expect(parseCsv('a,,c\n1,,3')).toEqual([
      ['a', '', 'c'],
      ['1', '', '3'],
    ]);
  });

  it('CRLF 改行を LF と同様に扱う', () => {
    expect(parseCsv('a,b\r\n1,2\r\n')).toEqual([
      ['a', 'b'],
      ['1', '2'],
    ]);
  });

  it('空文字列の入力は 0 行を返す', () => {
    expect(parseCsv('')).toEqual([]);
  });
});
