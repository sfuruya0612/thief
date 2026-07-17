import { describe, expect, it } from 'vitest';
import { appendLogLines, MAX_LOG_LINES } from './logLines';

describe('appendLogLines', () => {
  it('上限未満なら単純に末尾へ追加する', () => {
    const result = appendLogLines([1, 2, 3], [4, 5], 10);
    expect(result).toEqual([1, 2, 3, 4, 5]);
  });

  it('incoming が空なら lines をそのまま返す (参照も変えない)', () => {
    const lines = [1, 2, 3];
    const result = appendLogLines(lines, [], 10);
    expect(result).toBe(lines);
  });

  it('上限を超えたら古い行 (先頭) から切り捨てる', () => {
    const result = appendLogLines([1, 2, 3], [4, 5], 4);
    expect(result).toEqual([2, 3, 4, 5]);
  });

  it('1 回の追加で上限を超える場合も末尾 maxLines 件だけ残す', () => {
    const result = appendLogLines([], [1, 2, 3, 4, 5], 2);
    expect(result).toEqual([4, 5]);
  });

  it('maxLines 省略時は既定の MAX_LOG_LINES を使う', () => {
    const lines = Array.from({ length: MAX_LOG_LINES }, (_, i) => i);
    const result = appendLogLines(lines, [MAX_LOG_LINES]);
    expect(result).toHaveLength(MAX_LOG_LINES);
    expect(result[0]).toBe(1);
    expect(result[result.length - 1]).toBe(MAX_LOG_LINES);
  });
});
