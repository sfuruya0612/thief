// SSO トークン期限切れ (401 SSO_TOKEN_EXPIRED) 用の再ログイン導線
import { useState } from 'react';
import { Trans, useTranslation } from 'react-i18next';
import { useSSOLogin } from '../api/queries';
import type { SSOLoginStartRow } from '../types/aws';
import { Icons } from './icons/Icons';

export interface SSOExpiredBannerProps {
  profile: string;
}

export function SSOExpiredBanner({ profile }: SSOExpiredBannerProps) {
  const { t } = useTranslation('errors');
  const login = useSSOLogin(profile);
  // start 応答の認可 URL。フォールバック表示 (下記 showFallback) にだけ使う。
  const [started, setStarted] = useState<SSOLoginStartRow | null>(null);
  // 認可タブを自動制御できない状態。ポップアップブロックで開けなかった場合と、
  // 開いた空タブを start 応答前にユーザが手動で閉じていた場合 (useSSOLogin の
  // onTabUnavailable) の 2 経路で true になる。
  const [tabUnavailable, setTabUnavailable] = useState(false);

  const onLogin = () => {
    // 認可タブは start の応答を待たずに click ハンドラの同期処理で開く。await を挟むと
    // transient activation が失効してポップアップブロックにかかりうるため。開けた場合は
    // WindowProxy を保持し、認可完了後の close() でフォーカスを自タブへ戻す (useSSOLogin)。
    const authWindow = window.open('', '_blank');
    setTabUnavailable(authWindow === null);
    setStarted(null);
    login.mutate({
      authWindow,
      onStarted: setStarted,
      onTabUnavailable: () => setTabUnavailable(true),
    });
  };

  // 認可タブを自動制御できない場合は、ユーザが自分で認可ページを開くためのリンクを
  // バナーに出す。tabUnavailable の 2 経路と、verification_uri_complete が無く開いた
  // 空タブを閉じた場合 (このとき user_code の入力も必要) がある。
  const showFallback =
    login.isPending && started !== null && (tabUnavailable || !started.verificationUriComplete);
  const fallbackUri = started ? started.verificationUriComplete || started.verificationUri : '';

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
        {showFallback && (
          <span className="sso-banner-hint">
            {' '}
            <Trans
              i18nKey="sso.fallbackLink"
              ns="errors"
              components={{
                // "link" は HTML の void 要素と衝突し Trans が子テキストを注入できない
                // ため、プレースホルダ名は authLink とする。
                authLink: <a href={fallbackUri} target="_blank" rel="noreferrer" />,
              }}
            />
            {started !== null && !started.verificationUriComplete && (
              <> {t('sso.userCode', { code: started.userCode })}</>
            )}
          </span>
        )}
        {login.isError && <span className="sso-banner-error"> {t('sso.failed')}</span>}
      </div>
      <button className="btn sm primary" onClick={onLogin} disabled={login.isPending}>
        {login.isPending ? t('sso.loginButtonPending') : t('sso.loginButton')}
      </button>
    </div>
  );
}
