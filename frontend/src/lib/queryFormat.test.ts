import { describe, expect, it } from 'vitest';
import {
  cliHintSql,
  estimateBQCostUSD,
  formatApproxUSD,
  formatDurationClock,
  formatDurationSeconds,
  formatTimestampShort,
  s3Dir,
  shortId,
  toCsv,
} from './queryFormat';

describe('cliHintSql', () => {
  it('改行と連続空白を 1 スペースへ潰す', () => {
    expect(cliHintSql('SELECT *\n  FROM t')).toBe('SELECT * FROM t');
  });
  it('長い SQL は切り詰めて省略記号を付ける', () => {
    const long = 'SELECT ' + 'x, '.repeat(40);
    expect(cliHintSql(long, 20)).toBe(long.slice(0, 20) + '…');
  });
});

describe('s3Dir', () => {
  it('オブジェクトパスからディレクトリを取り出す', () => {
    expect(s3Dir('s3://bucket/prefix/exec.csv')).toBe('s3://bucket/prefix/');
  });
  it('スラッシュが無ければそのまま返す', () => {
    expect(s3Dir('abc')).toBe('abc');
  });
});

describe('formatDurationSeconds', () => {
  it('10 秒未満は小数 1 桁の秒で表示する', () => {
    expect(formatDurationSeconds(2300)).toBe('2.3s');
    expect(formatDurationSeconds(0)).toBe('0.0s');
  });
  it('60 秒未満は整数秒で表示する', () => {
    expect(formatDurationSeconds(45_000)).toBe('45s');
  });
  it('60 秒以上は分と秒で表示する', () => {
    expect(formatDurationSeconds(65_000)).toBe('1m 5s');
  });
  it('負値や NaN は空文字列を返す', () => {
    expect(formatDurationSeconds(-1)).toBe('');
    expect(formatDurationSeconds(NaN)).toBe('');
  });
});

describe('formatDurationClock', () => {
  it('mm:ss 形式でゼロ埋めする', () => {
    expect(formatDurationClock(6000)).toBe('00:06');
    expect(formatDurationClock(62_000)).toBe('01:02');
  });
  it('1 時間以上は h:mm:ss 形式になる', () => {
    expect(formatDurationClock(3_723_000)).toBe('1:02:03');
  });
  it('負値は空文字列を返す', () => {
    expect(formatDurationClock(-5)).toBe('');
  });
});

describe('formatTimestampShort', () => {
  it('ローカル時刻の MM-DD HH:mm で表示する', () => {
    const d = new Date(2026, 6, 16, 22, 4, 0);
    expect(formatTimestampShort(d.toISOString())).toBe('07-16 22:04');
  });
  it('不正な入力は空文字列を返す', () => {
    expect(formatTimestampShort('')).toBe('');
    expect(formatTimestampShort('not-a-date')).toBe('');
  });
});

describe('estimateBQCostUSD / formatApproxUSD', () => {
  it('TiB あたり 6.25 USD で概算する', () => {
    expect(estimateBQCostUSD(1024 ** 4)).toBeCloseTo(6.25);
    expect(estimateBQCostUSD(0)).toBe(0);
  });
  it('1 セント未満は 3 桁で表示する', () => {
    expect(formatApproxUSD(0.0063)).toBe('$0.006');
  });
  it('100 未満は 2 桁で表示する', () => {
    expect(formatApproxUSD(1.238)).toBe('$1.24');
  });
  it('100 以上は整数で表示する', () => {
    expect(formatApproxUSD(129.6)).toBe('$130');
  });
});

describe('shortId', () => {
  it('長い ID を短縮する', () => {
    expect(shortId('job_ab12cd34ef56gh78')).toBe('job_ab12…78');
  });
  it('短い ID はそのまま返す', () => {
    expect(shortId('abc')).toBe('abc');
  });
});

describe('toCsv', () => {
  it('ヘッダ行と data 行を CRLF で連結する', () => {
    expect(toCsv(['a', 'b'], [['1', '2']])).toBe('a,b\r\n1,2');
  });
  it('カンマ・引用符・改行を含むセルを引用する', () => {
    expect(toCsv(['a'], [['x,y'], ['say "hi"'], ['line1\nline2']])).toBe(
      'a\r\n"x,y"\r\n"say ""hi"""\r\n"line1\nline2"',
    );
  });
  it('行が無い場合はヘッダのみを返す', () => {
    expect(toCsv(['a', 'b'], [])).toBe('a,b');
  });
});
