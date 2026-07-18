import { describe, expect, it } from 'vitest';
import { cwSeverityFromMessage } from './logSeverity';

describe('cwSeverityFromMessage', () => {
  it('ERROR 系のレベル語を err に丸める', () => {
    expect(cwSeverityFromMessage('ERROR upstream timeout')).toBe('err');
    expect(cwSeverityFromMessage('FATAL panic')).toBe('err');
    expect(cwSeverityFromMessage('a critical failure occurred')).toBe('err');
    expect(cwSeverityFromMessage('Exception in thread')).toBe('err');
  });

  it('WARNING 系を warn に丸める', () => {
    expect(cwSeverityFromMessage('WARN retry scheduled')).toBe('warn');
    expect(cwSeverityFromMessage('warning: disk almost full')).toBe('warn');
  });

  it('レベル語が無ければ info', () => {
    expect(cwSeverityFromMessage('GET /healthz 200 2ms')).toBe('info');
    expect(cwSeverityFromMessage('')).toBe('info');
  });

  it('大文字小文字を無視する', () => {
    expect(cwSeverityFromMessage('error boom')).toBe('err');
  });

  it('先頭 200 文字より後のレベル語は拾わない', () => {
    const longInfo = `${'x'.repeat(250)} ERROR`;
    expect(cwSeverityFromMessage(longInfo)).toBe('info');
  });

  it('部分一致 (単語境界なし) は拾わない', () => {
    expect(cwSeverityFromMessage('terror is not an error level')).toBe('err');
    expect(cwSeverityFromMessage('errors_total=0 ok')).toBe('info');
  });
});
