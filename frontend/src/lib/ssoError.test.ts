import { describe, expect, it } from 'vitest';
import { ApiError } from '../types/common';
import { isSSOExpiredError } from './ssoError';

describe('isSSOExpiredError', () => {
  it('SSO_TOKEN_EXPIRED コードの ApiError なら true', () => {
    expect(isSSOExpiredError(new ApiError(401, 'SSO_TOKEN_EXPIRED', 'sso token expired'))).toBe(
      true,
    );
  });

  it('他コードの ApiError なら false', () => {
    expect(isSSOExpiredError(new ApiError(403, 'ACCESS_DENIED', 'access denied'))).toBe(false);
  });

  it('ApiError 以外の Error なら false', () => {
    expect(isSSOExpiredError(new Error('SSO_TOKEN_EXPIRED'))).toBe(false);
  });

  it('null なら false', () => {
    expect(isSSOExpiredError(null)).toBe(false);
  });
});
