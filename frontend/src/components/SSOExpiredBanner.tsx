// SSO トークン期限切れ (401 SSO_TOKEN_EXPIRED) 用の再ログイン導線
import { Trans, useTranslation } from 'react-i18next';
import { useSSOLogin } from '../api/queries';
import { Icons } from './icons/Icons';

export interface SSOExpiredBannerProps {
  profile: string;
}

export function SSOExpiredBanner({ profile }: SSOExpiredBannerProps) {
  const { t } = useTranslation('errors');
  const login = useSSOLogin(profile);

  return (
    <div className="sso-banner">
      <Icons.bell size={16} />
      <div className="sso-banner-text">
        <Trans
          i18nKey="sso.expired"
          ns="errors"
          values={{ profile }}
          components={{ strong: <strong /> }}
        />
        {login.isPending && <span className="sso-banner-hint"> {t('sso.pending')}</span>}
        {login.isError && <span className="sso-banner-error"> {t('sso.failed')}</span>}
      </div>
      <button className="btn sm primary" onClick={() => login.mutate()} disabled={login.isPending}>
        {login.isPending ? t('sso.loginButtonPending') : t('sso.loginButton')}
      </button>
    </div>
  );
}
