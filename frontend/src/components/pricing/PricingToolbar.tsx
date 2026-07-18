// Pricing 画面のツールバー。リージョン選択はグローバル (Sidebar と同じ region/onRegionChange
// を共有する。切替は他パネルにも波及する) で、Pricing 専用の状態は持たない。
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
            ? `ローカルキャッシュ · 最終更新 ${formatFetchedAt(lastFetchedAt)}`
            : 'ローカルキャッシュ · 未取得'}
        </span>
        <button className="btn sm" onClick={onRefreshAll} disabled={refreshing}>
          <Icons.refresh size={12} />
          {refreshing ? '更新中…' : '更新'}
        </button>
        <a
          className="btn sm ghost"
          href="https://aws.amazon.com/pricing/"
          target="_blank"
          rel="noopener noreferrer"
        >
          AWS 公式料金ページ
          <Icons.external size={11} />
        </a>
      </div>
    </div>
  );
}
