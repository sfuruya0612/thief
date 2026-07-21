// backend 起動待ちの間、画面全体を覆って接続待ちであることを示す。
// 疎通が回復すると useHealthCheck が success になり、呼び出し側で非表示になる。
import { useTranslation } from 'react-i18next';

export function ConnectionWaiting() {
  const { t } = useTranslation('app');
  return (
    <div className="connection-waiting">
      <div className="connection-waiting-spinner" />
      <div className="connection-waiting-title">{t('connectionWaiting.title')}</div>
      <div className="connection-waiting-hint">{t('connectionWaiting.hint')}</div>
    </div>
  );
}
