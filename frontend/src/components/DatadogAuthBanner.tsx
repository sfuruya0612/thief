// Datadog の資格情報不備 (401 DATADOG_NO_CREDENTIALS) 用の再ログイン導線。
// 構造は SSOExpiredBanner と同じ (ログインボタン、pending / error 表示、ポップアップ
// ブロック時のフォールバックリンク) だが、完了の待ち方が login/status のポーリングで
// あるため別コンポーネントとする。org は再ログイン対象の組織 (空文字は親組織) で、
// ログインするのは表示中のタブの組織でなければならない。
// バナーのスタイルは再ログイン導線に共通のもの (.sso-banner 系) を再利用する。
import { Trans, useTranslation } from 'react-i18next';
import { useDatadogLoginFlow } from '../hooks/useDatadogLogin';
import { Icons } from './icons/Icons';

export interface DatadogAuthBannerProps {
  org: string;
}

export function DatadogAuthBanner({ org }: DatadogAuthBannerProps) {
  const { t } = useTranslation('errors');
  const flow = useDatadogLoginFlow();

  return (
    <div className="sso-banner">
      <Icons.bell size={16} />
      <div className="sso-banner-text">
        <span>{t('datadog.credentialsMissing')}</span>
        {flow.loggingIn && <span className="sso-banner-hint"> {t('datadog.pending')}</span>}
        {flow.fallbackUrl !== '' && (
          <span className="sso-banner-hint">
            {' '}
            <Trans
              i18nKey="datadog.fallbackLink"
              ns="errors"
              components={{
                // "link" は HTML の void 要素と衝突し Trans が子テキストを注入できない
                // ため、プレースホルダ名は authLink とする (SSOExpiredBanner と同じ)。
                authLink: <a href={flow.fallbackUrl} target="_blank" rel="noreferrer" />,
              }}
            />
          </span>
        )}
        {flow.failed && (
          <span className="sso-banner-error">
            {' '}
            {flow.redirectNotRegistered ? t('datadog.redirectNotRegistered') : t('datadog.failed')}
          </span>
        )}
      </div>
      <button className="btn sm primary" onClick={() => flow.begin(org)} disabled={flow.loggingIn}>
        {flow.loggingIn ? t('datadog.loginButtonPending') : t('datadog.loginButton')}
      </button>
    </div>
  );
}
