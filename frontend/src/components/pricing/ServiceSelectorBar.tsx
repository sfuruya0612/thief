// 対象 4 サービスの複数選択トグルバー。選択集合の順序 (追加された順) が中央スタックの表示順になる。
import {
  PRICING_SERVICE_ICON_KEY,
  PRICING_SERVICE_LABELS,
  PRICING_SERVICES,
} from '../../lib/pricingSelection';
import { AwsIcons } from '../icons/AwsIcons';
import { Icons } from '../icons/Icons';

export interface ServiceSelectorBarProps {
  activeServices: string[];
  onToggle: (service: string) => void;
}

export function ServiceSelectorBar({ activeServices, onToggle }: ServiceSelectorBarProps) {
  const active = new Set(activeServices);
  return (
    <div className="pr-service-selector">
      {PRICING_SERVICES.map((service) => {
        const isActive = active.has(service);
        const iconKey = PRICING_SERVICE_ICON_KEY[service];
        const IconEl = AwsIcons[iconKey] ?? Icons[iconKey];
        return (
          <button
            key={service}
            type="button"
            className={`pr-service-toggle ${isActive ? 'active' : ''}`}
            aria-pressed={isActive}
            onClick={() => onToggle(service)}
          >
            {IconEl && <IconEl size={14} />}
            {PRICING_SERVICE_LABELS[service]}
          </button>
        );
      })}
    </div>
  );
}
