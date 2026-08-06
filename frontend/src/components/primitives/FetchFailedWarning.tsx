// フィールド単位の取得失敗を明示する警告アイコン。一覧列と Drawer で共用する。
import { useTranslation } from 'react-i18next';
import { Icons } from '../icons/Icons';

export function FetchFailedWarning() {
  const { t } = useTranslation('drawerAws');
  return (
    <span className="fetch-failed-warning" title={t('fetchFailedWarning.tooltip')}>
      <Icons.alertTriangle size={14} />
    </span>
  );
}
