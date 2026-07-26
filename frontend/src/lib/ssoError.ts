// SSO トークン期限切れ (401 SSO_TOKEN_EXPIRED) の判定を 1 箇所に集約する。
// backend の全 AWS リソースハンドラがこのコードでエラーを返すため、
// 各ビューはこの純関数で error を判定して SSOExpiredBanner を出す。
import { ApiError } from '../types/common';

// SSO_TOKEN_EXPIRED_CODE は backend が SSO トークン期限切れを表すのに使うエラーコード。
// 判定式とテストのフィクスチャはこの定数を参照し、コード文字列の定義を 1 箇所に保つ。
export const SSO_TOKEN_EXPIRED_CODE = 'SSO_TOKEN_EXPIRED';

// isSSOExpiredError は err が SSO トークン期限切れの ApiError かを返す。
export function isSSOExpiredError(err: unknown): boolean {
  return err instanceof ApiError && err.code === SSO_TOKEN_EXPIRED_CODE;
}
