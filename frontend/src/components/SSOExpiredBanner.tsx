// SSO トークン期限切れ (401 SSO_TOKEN_EXPIRED) 用の再ログイン導線
import { useSSOLogin } from '../api/queries';
import { Icons } from './icons/Icons';

export interface SSOExpiredBannerProps {
  profile: string;
}

export function SSOExpiredBanner({ profile }: SSOExpiredBannerProps) {
  const login = useSSOLogin(profile);

  return (
    <div className="sso-banner">
      <Icons.bell size={16} />
      <div className="sso-banner-text">
        <strong>{profile}</strong> の SSO セッションが期限切れです。再ログインしてください。
        {login.isSuccess && (
          <span className="sso-banner-hint">
            {' '}
            ブラウザでログインを完了させたのち、再取得してください。
          </span>
        )}
        {login.isError && (
          <span className="sso-banner-error"> 再ログインの起動に失敗しました。</span>
        )}
      </div>
      <button className="btn sm primary" onClick={() => login.mutate()} disabled={login.isPending}>
        {login.isPending ? 'ログイン中…' : 'SSO 再ログイン'}
      </button>
    </div>
  );
}
