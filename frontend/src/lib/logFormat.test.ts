import { describe, expect, it } from 'vitest';
import { formatLogClock, jsonFieldsOf, rowsToCsv, rowsToJson } from './logFormat';

describe('formatLogClock', () => {
  it('RFC3339 を HH:MM:SS.mmm 形式にする', () => {
    expect(formatLogClock('2026-07-18T03:04:05.678Z')).toMatch(/^\d{2}:\d{2}:\d{2}\.\d{3}$/);
  });

  it('パース不能な入力はそのまま返す', () => {
    expect(formatLogClock('not-a-time')).toBe('not-a-time');
  });
});

describe('rowsToCsv', () => {
  it('ヘッダーとデータ行を改行区切りにする', () => {
    const csv = rowsToCsv(
      ['a', 'b'],
      [
        ['1', '2'],
        ['3', '4'],
      ],
    );
    expect(csv).toBe('a,b\n1,2\n3,4');
  });

  it('カンマ・引用符・改行を含む値をエスケープする', () => {
    const csv = rowsToCsv(['x'], [['a,b'], ['he said "hi"'], ['line1\nline2']]);
    expect(csv).toBe('x\n"a,b"\n"he said ""hi"""\n"line1\nline2"');
  });
});

describe('rowsToJson', () => {
  it('2 スペースインデントの JSON にする', () => {
    expect(rowsToJson([{ a: 1 }])).toBe('[\n  {\n    "a": 1\n  }\n]');
  });
});

describe('jsonFieldsOf', () => {
  it('JSON オブジェクトを key/value 配列にする', () => {
    const fields = jsonFieldsOf('{"level":"error","latency":812}');
    expect(fields).toEqual([
      { key: 'level', value: 'error' },
      { key: 'latency', value: '812' },
    ]);
  });

  it('文字列以外の値は JSON 文字列化する', () => {
    const fields = jsonFieldsOf('{"nested":{"a":1},"arr":[1,2]}');
    expect(fields).toEqual([
      { key: 'nested', value: '{"a":1}' },
      { key: 'arr', value: '[1,2]' },
    ]);
  });

  it('オブジェクトでない (配列・プリミティブ・不正) 場合は null', () => {
    expect(jsonFieldsOf('[1,2,3]')).toBeNull();
    expect(jsonFieldsOf('"just a string"')).toBeNull();
    expect(jsonFieldsOf('plain text')).toBeNull();
    expect(jsonFieldsOf('42')).toBeNull();
  });
});
