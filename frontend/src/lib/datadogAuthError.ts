// Datadog の資格情報不備 (401 DATADOG_NO_CREDENTIALS) の判定を 1 箇所に集約する。
// backend の Datadog コスト取得ハンドラ (writeDatadogCostError) がこのコードを返すのは
// 「OAuth トークンが使えず静的キーも未設定」の場合だけであり、ブラウザからの再ログインで
// 解決できるケースと一致する。スコープ不足の 403 や静的キーの失効は 500 INTERNAL_ERROR の
// ままで、この判定には掛からない。
import { ApiError } from '../types/common';

// DATADOG_NO_CREDENTIALS_CODE は backend が資格情報不備を表すのに使うエラーコード。
// 判定式とテストのフィクスチャはこの定数を参照し、コード文字列の定義を 1 箇所に保つ。
export const DATADOG_NO_CREDENTIALS_CODE = 'DATADOG_NO_CREDENTIALS';

// isDatadogAuthError は err が Datadog の資格情報不備の ApiError かを返す。
export function isDatadogAuthError(err: unknown): boolean {
  return err instanceof ApiError && err.code === DATADOG_NO_CREDENTIALS_CODE;
}
