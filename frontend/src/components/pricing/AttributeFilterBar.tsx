// ServiceCard 内、サービス固有の curated attribute (OS/Engine/Deployment 等) による
// トグルチップ絞り込み。instance_type のように種類が多い属性はテキスト検索
// (instanceFilter) に任せ、値の種類が少なく一覧提示に向く属性だけをここで扱う
// (lib/pricingAttributeFilters.ts の定義に従う)。
import type { AttributeFilterSpec } from '../../lib/pricingAttributeFilters';

export interface AttributeFilterBarProps {
  specs: AttributeFilterSpec[];
  options: Record<string, string[]>;
  selected: Record<string, Set<string>>;
  onToggle: (key: string, value: string) => void;
}

export function AttributeFilterBar({
  specs,
  options,
  selected,
  onToggle,
}: AttributeFilterBarProps) {
  // 値が 1 種類以下の属性は絞り込む意味がないため表示しない
  // (RateGroupSection の offeringClassOptions.length <= 1 判定と同じ考え方)。
  const visible = specs.filter((spec) => (options[spec.key]?.length ?? 0) > 1);
  if (visible.length === 0) return null;

  return (
    <div className="pr-attr-filters">
      {visible.map((spec) => (
        <div key={spec.key} className="pr-attr-group">
          <span className="pr-attr-label">{spec.label}</span>
          {options[spec.key].map((value) => {
            const active = selected[spec.key]?.has(value) ?? false;
            return (
              <button
                key={value}
                type="button"
                className={`pr-attr-chip ${active ? 'active' : ''}`}
                aria-pressed={active}
                onClick={() => onToggle(spec.key, value)}
              >
                {spec.valueLabels?.[value] ?? value}
              </button>
            );
          })}
        </div>
      ))}
    </div>
  );
}
