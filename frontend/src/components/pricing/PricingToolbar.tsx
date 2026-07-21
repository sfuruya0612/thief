// Pricing 画面のツールバー。リージョン選択はグローバル (Sidebar と同じ region/onRegionChange
// を共有する。切替は他パネルにも波及する) で、Pricing 専用の状態は持たない。
import { useTranslation } from 'react-i18next';
import { formatFetchedAt } from '../../lib/format';
import { Icons } from '../icons/Icons';

export interface PricingToolbarProps {
  region: string;
  regionOptions: { code: string; name: string }[];
  onRegionChange: (region: string) => void;
  // アクティブなサービスのうち取得済みのものの中で最も古い fetched_at。1 件も無ければ null。
  lastFetchedAt: string | null;
  onRefreshAll: () => void;
  refreshing: boolean;
}

export function PricingToolbar({
  region,
  regionOptions,
  onRegionChange,
  lastFetchedAt,
  onRefreshAll,
  refreshing,
}: PricingToolbarProps) {
  const { t } = useTranslation('pricing');
  return (
    <div className="toolbar">
      <div className="title">
        <h1>Pricing</h1>
        <span className="subtitle">rates &amp; estimate</span>
      </div>
      <div className="pr-toolbar-actions">
        <select
          className="btn sm"
          value={region}
          onChange={(e) => onRegionChange(e.target.value)}
          title="Region"
        >
          {regionOptions.map((r) => (
            <option key={r.code} value={r.code}>
              {r.name === r.code ? r.code : `${r.name} (${r.code})`}
            </option>
          ))}
        </select>
        <span className="pr-freshness">
          {lastFetchedAt
            ? t('pricingToolbar.freshnessUpdated', { at: formatFetchedAt(lastFetchedAt) })
            : t('pricingToolbar.freshnessNotFetched')}
        </span>
        <button className="btn sm" onClick={onRefreshAll} disabled={refreshing}>
          <Icons.refresh size={12} />
          {refreshing ? t('pricingToolbar.refreshing') : t('pricingToolbar.refresh')}
        </button>
        <a
          className="btn sm ghost"
          href="https://aws.amazon.com/pricing/"
          target="_blank"
          rel="noopener noreferrer"
        >
          {t('pricingToolbar.officialPricingPage')}
          <Icons.external size={11} />
        </a>
      </div>
    </div>
  );
}
