// Datadog の資格情報不備 (401 DATADOG_NO_CREDENTIALS) 用の再ログイン導線。
// 構造は SSOExpiredBanner と同じ (ログインボタン、pending / error 表示、ポップアップ
// ブロック時のフォールバックリンク) だが、Datadog には profile の概念が無く (単一サイト
// 運用)、完了の待ち方も login/status のポーリングであるため別コンポーネントとする。
// バナーのスタイルは再ログイン導線に共通のもの (.sso-banner 系) を再利用する。
import { useState } from 'react';
import { Trans, useTranslation } from 'react-i18next';
import {
  DATADOG_LOGIN_POLL_TIMEOUT,
  useDatadogLoginStart,
  useDatadogLoginStatus,
  type DatadogLoginSession,
} from '../hooks/useDatadogLogin';
import { ApiError } from '../types/common';
import type { DatadogLoginStartRow } from '../types/nonaws';
import { Icons } from './icons/Icons';

// backend が login/start で返す、redirect_uri 未登録 (OAuthRedirectBase の変更後に
// クライアントの再登録が要る状態) のエラーコード。再ログインでは解決しないため、
// 通常のログイン失敗とは別の文言を出す。
const REDIRECT_URI_NOT_REGISTERED_CODE = 'DATADOG_REDIRECT_URI_NOT_REGISTERED';

export function DatadogAuthBanner() {
  const { t } = useTranslation('errors');
  const start = useDatadogLoginStart();
  // 進行中のログイン。login/start の成功で作り、useDatadogLoginStatus のポーリング対象になる。
  const [session, setSession] = useState<DatadogLoginSession | null>(null);
  // start 応答の認可 URL。フォールバック表示 (下記 showFallback) にだけ使う。
  const [started, setStarted] = useState<DatadogLoginStartRow | null>(null);
  // 認可タブを自動制御できない状態。ポップアップブロックで開けなかった場合と、開いた空タブを
  // start 応答前にユーザが手動で閉じていた場合 (onTabUnavailable) の 2 経路で true になる。
  const [tabUnavailable, setTabUnavailable] = useState(false);
  const status = useDatadogLoginStatus(session);

  const onLogin = () => {
    // 認可タブは start の応答を待たずに click ハンドラの同期処理で開く。await を挟むと
    // transient activation が失効してポップアップブロックにかかりうるため。
    const authWindow = window.open('', '_blank');
    setTabUnavailable(authWindow === null);
    setStarted(null);
    setSession(null);
    start.mutate(
      { authWindow, onTabUnavailable: () => setTabUnavailable(true) },
      {
        onSuccess: (row) => {
          setStarted(row);
          setSession({
            state: row.state,
            deadline: Date.now() + DATADOG_LOGIN_POLL_TIMEOUT,
            authWindow,
          });
        },
      },
    );
  };

  // ポーリングは succeeded かエラー (failed / 打ち切り / セッション消失) で止まる。
  const polling = session !== null && !status.isError && status.data?.status !== 'succeeded';
  const loggingIn = start.isPending || polling;
  const redirectNotRegistered =
    start.error instanceof ApiError && start.error.code === REDIRECT_URI_NOT_REGISTERED_CODE;
  const failed = start.isError || status.isError;

  // 認可タブを自動制御できない場合は、ユーザが自分で認可ページを開くためのリンクを出す。
  const showFallback = polling && started !== null && tabUnavailable;

  return (
    <div className="sso-banner">
      <Icons.bell size={16} />
      <div className="sso-banner-text">
        <span>{t('datadog.credentialsMissing')}</span>
        {loggingIn && <span className="sso-banner-hint"> {t('datadog.pending')}</span>}
        {showFallback && (
          <span className="sso-banner-hint">
            {' '}
            <Trans
              i18nKey="datadog.fallbackLink"
              ns="errors"
              components={{
                // "link" は HTML の void 要素と衝突し Trans が子テキストを注入できない
                // ため、プレースホルダ名は authLink とする (SSOExpiredBanner と同じ)。
                authLink: <a href={started.authorizationUrl} target="_blank" rel="noreferrer" />,
              }}
            />
          </span>
        )}
        {failed && (
          <span className="sso-banner-error">
            {' '}
            {redirectNotRegistered ? t('datadog.redirectNotRegistered') : t('datadog.failed')}
          </span>
        )}
      </div>
      <button className="btn sm primary" onClick={onLogin} disabled={loggingIn}>
        {loggingIn ? t('datadog.loginButtonPending') : t('datadog.loginButton')}
      </button>
    </div>
  );
}
