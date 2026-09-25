import { describe, expect, it } from 'vitest';
import {
  exceedsMaxBytes,
  isFetchAbortError,
  isNotFoundError,
  isQuotaError,
  parseContentLength,
} from './opfsWriterLimits';

describe('parseContentLength', () => {
  it('10 進の非負整数はその値を返す', () => {
    expect(parseContentLength('0')).toBe(0);
    expect(parseContentLength('1024')).toBe(1024);
    expect(parseContentLength('1073741824')).toBe(1073741824);
  });

  it('ヘッダが無い場合は null', () => {
    expect(parseContentLength(null)).toBeNull();
  });

  it('10 進の非負整数でない場合は null', () => {
    expect(parseContentLength('')).toBeNull();
    expect(parseContentLength('-1')).toBeNull();
    expect(parseContentLength('1.5')).toBeNull();
    expect(parseContentLength('1024, 1024')).toBeNull();
    expect(parseContentLength('abc')).toBeNull();
    expect(parseContentLength(' 1024')).toBeNull();
  });
});

describe('exceedsMaxBytes', () => {
  it('上限ちょうどは超過としない', () => {
    expect(exceedsMaxBytes(1024, 1024)).toBe(false);
  });

  it('上限を 1 バイト超えると超過', () => {
    expect(exceedsMaxBytes(1025, 1024)).toBe(true);
  });

  it('上限未満は超過としない', () => {
    expect(exceedsMaxBytes(0, 1024)).toBe(false);
    expect(exceedsMaxBytes(1023, 1024)).toBe(false);
  });
});

describe('isFetchAbortError', () => {
  it('DOMException の AbortError だけを真とする', () => {
    expect(isFetchAbortError(new DOMException('aborted', 'AbortError'))).toBe(true);
    expect(isFetchAbortError(new DOMException('quota', 'QuotaExceededError'))).toBe(false);
    // name だけを合わせた通常の Error は判定しない (中断以外を中断として扱わないため)
    const err = new Error('aborted');
    err.name = 'AbortError';
    expect(isFetchAbortError(err)).toBe(false);
    expect(isFetchAbortError(null)).toBe(false);
    expect(isFetchAbortError('AbortError')).toBe(false);
  });
});

describe('isQuotaError', () => {
  it('DOMException の QuotaExceededError だけを真とする', () => {
    expect(isQuotaError(new DOMException('no space', 'QuotaExceededError'))).toBe(true);
    expect(isQuotaError(new DOMException('aborted', 'AbortError'))).toBe(false);
    expect(isQuotaError(new Error('QuotaExceededError'))).toBe(false);
    expect(isQuotaError(undefined)).toBe(false);
  });
});

describe('isNotFoundError', () => {
  it('DOMException の NotFoundError だけを真とする', () => {
    expect(isNotFoundError(new DOMException('missing', 'NotFoundError'))).toBe(true);
    expect(isNotFoundError(new DOMException('aborted', 'AbortError'))).toBe(false);
    expect(isNotFoundError(new Error('NotFoundError'))).toBe(false);
  });
});
