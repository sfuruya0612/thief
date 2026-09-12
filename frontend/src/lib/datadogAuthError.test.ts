import { describe, expect, it } from 'vitest';
import { ApiError } from '../types/common';
import { DATADOG_NO_CREDENTIALS_CODE, isDatadogAuthError } from './datadogAuthError';

describe('isDatadogAuthError', () => {
  it('DATADOG_NO_CREDENTIALS コードの ApiError なら true', () => {
    expect(
      isDatadogAuthError(
        new ApiError(401, DATADOG_NO_CREDENTIALS_CODE, 'no usable Datadog credentials'),
      ),
    ).toBe(true);
  });

  it('コード未設定の ApiError なら false', () => {
    expect(isDatadogAuthError(new ApiError(401, undefined, 'Unauthorized'))).toBe(false);
  });

  it('他コードの ApiError なら false', () => {
    // スコープ不足の 403 経路は再ログインで解決しないため、判定に掛けない。
    expect(isDatadogAuthError(new ApiError(500, 'INTERNAL_ERROR', '403 Forbidden'))).toBe(false);
    expect(isDatadogAuthError(new ApiError(401, 'SSO_TOKEN_EXPIRED', 'sso token expired'))).toBe(
      false,
    );
  });

  it('ApiError 以外の Error なら false', () => {
    expect(isDatadogAuthError(new Error(DATADOG_NO_CREDENTIALS_CODE))).toBe(false);
  });

  it('null なら false', () => {
    expect(isDatadogAuthError(null)).toBe(false);
  });
});
