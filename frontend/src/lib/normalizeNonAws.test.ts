import { describe, expect, it } from 'vitest';
import { datadogLoginStartFromRaw, datadogLoginStatusFromRaw } from './normalizeNonAws';

describe('datadogLoginStartFromRaw', () => {
  it('state / authorization_url を camelCase に変換する', () => {
    const row = datadogLoginStartFromRaw({
      state: 'abc123',
      authorization_url: 'https://app.datadoghq.com/oauth2/v1/authorize?state=abc123',
    });
    expect(row).toEqual({
      state: 'abc123',
      authorizationUrl: 'https://app.datadoghq.com/oauth2/v1/authorize?state=abc123',
    });
  });
});

describe('datadogLoginStatusFromRaw', () => {
  it('status: pending をそのまま通す', () => {
    const row = datadogLoginStatusFromRaw({ status: 'pending' });
    expect(row).toEqual({ status: 'pending', errorMessage: '' });
  });

  it('status: succeeded をそのまま通す', () => {
    const row = datadogLoginStatusFromRaw({ status: 'succeeded' });
    expect(row).toEqual({ status: 'succeeded', errorMessage: '' });
  });

  it('status: failed で error_message をそのまま通す', () => {
    const row = datadogLoginStatusFromRaw({ status: 'failed', error_message: 'access_denied' });
    expect(row).toEqual({ status: 'failed', errorMessage: 'access_denied' });
  });

  it('error_message が未指定なら空文字にする', () => {
    const row = datadogLoginStatusFromRaw({ status: 'failed' });
    expect(row.errorMessage).toBe('');
  });

  it('backend が返す未知の status 値は failed として扱う (ポーリングを止め続けるため)', () => {
    const row = datadogLoginStatusFromRaw({ status: 'unknown_future_status' });
    expect(row.status).toBe('failed');
  });
});
