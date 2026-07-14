// バックエンドがエラーを返した際に DevTools を開かずとも内容を確認できるようにする汎用バナー。
// SSO 期限切れ (SSOExpiredBanner) 以外の API エラーはこちらで表示する。
import { ApiError } from '../types/common';
import { Icons } from './icons/Icons';

export interface ErrorBannerProps {
  error: unknown;
}

export function ErrorBanner({ error }: ErrorBannerProps) {
  if (!(error instanceof Error)) return null;

  const isApiError = error instanceof ApiError;

  return (
    <div className="error-banner">
      <Icons.alertTriangle size={16} />
      <div className="error-banner-text">
        {isApiError ? (
          <>
            <strong>
              {(error as ApiError).statusCode}
              {(error as ApiError).code ? ` ${(error as ApiError).code}` : ''}
            </strong>
            <span> {error.message}</span>
          </>
        ) : (
          <span>{error.message}</span>
        )}
      </div>
    </div>
  );
}
