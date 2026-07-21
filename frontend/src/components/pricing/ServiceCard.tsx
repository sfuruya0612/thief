// Pricing 画面の 1 サービス分のカード。sticky ヘッダー (名前/選択数/小計/折りたたみ/更新) +
// 本体 (インスタンスタイプ絞り込み + On-Demand/RI/SP のグループ別セクション)。
// データ取得はサービスごとに独立するため、あるカードがローディング中でも他カードは表示済みになりうる
// (状態はカードごとに個別に持つ)。
import { useMemo, useState } from 'react';
import { estimate } from '../../lib/pricingEstimate';
import {
  attributeValueOptions,
  matchesAttributeSelection,
  PRICING_ATTRIBUTE_FILTERS,
} from '../../lib/pricingAttributeFilters';
import {
  PRICING_SERVICE_ICON_KEY,
  PRICING_SERVICE_LABELS,
  type PricingService,
} from '../../lib/pricingSelection';
import { formatMoney } from '../../lib/format';
import type { PriceRateRow, PriceTableRow } from '../../types/aws';
import { AwsIcons } from '../icons/AwsIcons';
import { Icons } from '../icons/Icons';
import { ErrorBanner } from '../ErrorBanner';
import { Loading } from '../Loading';
import { AttributeFilterBar } from './AttributeFilterBar';
import { RateGroupSection } from './RateGroupSection';

export interface ServiceCardProps {
  service: PricingService;
  table: PriceTableRow | undefined;
  isLoading: boolean;
  error: unknown;
  onRetry: () => void;
  collapsed: boolean;
  onToggleCollapsed: () => void;
  selection: Record<string, { checked: boolean; qty: number }>;
  onToggleRate: (rateId: string) => void;
  onRefresh: () => void;
  refreshing: boolean;
}

// issue 0055 の SP 分離後、リソースカード (ec2/rds/elasticache/ecs) は On-Demand /
// Reserved Instance の 2 group のみを持つ。SP カード (compute-sp 等) は常に 1 group
// (spGroup が返す "Compute Savings Plans" 等) のみのため、SP 種別ごとの順序は不要になった。
// issue 0056: ec2-spot カードも常に 1 group ("Spot") のみを持つ。
const GROUP_ORDER = ['On-Demand', 'Reserved Instance', 'Spot'];

function groupRates(rates: PriceRateRow[]): [string, PriceRateRow[]][] {
  const byGroup = new Map<string, PriceRateRow[]>();
  for (const r of rates) {
    const list = byGroup.get(r.group);
    if (list) list.push(r);
    else byGroup.set(r.group, [r]);
  }
  return [...byGroup.entries()].sort(([a], [b]) => {
    const ia = GROUP_ORDER.indexOf(a);
    const ib = GROUP_ORDER.indexOf(b);
    if (ia === -1 && ib === -1) return a.localeCompare(b);
    if (ia === -1) return 1;
    if (ib === -1) return -1;
    return ia - ib;
  });
}

export function ServiceCard({
  service,
  table,
  isLoading,
  error,
  onRetry,
  collapsed,
  onToggleCollapsed,
  selection,
  onToggleRate,
  onRefresh,
  refreshing,
}: ServiceCardProps) {
  const [instanceFilter, setInstanceFilter] = useState('');
  const [attrSelection, setAttrSelection] = useState<Record<string, Set<string>>>({});

  const attributeSpecs = PRICING_ATTRIBUTE_FILTERS[service];
  const attributeOptions = useMemo(() => {
    const out: Record<string, string[]> = {};
    for (const spec of attributeSpecs) {
      out[spec.key] = attributeValueOptions(table?.rates ?? [], spec.key);
    }
    return out;
  }, [attributeSpecs, table]);

  const toggleAttrValue = (key: string, value: string) => {
    setAttrSelection((prev) => {
      const next = new Set(prev[key] ?? []);
      if (next.has(value)) next.delete(value);
      else next.add(value);
      return { ...prev, [key]: next };
    });
  };

  // グルーピング (On-Demand/RI の区分。SP カードは常に単一 group) は属性フィルタ
  // 適用後の行に対して行う。
  const attrFilteredRates = useMemo(
    () => (table ? table.rates.filter((r) => matchesAttributeSelection(r, attrSelection)) : []),
    [table, attrSelection],
  );
  const groups = useMemo(() => groupRates(attrFilteredRates), [attrFilteredRates]);

  // SP カードは group が 1 種類のみで、その名前 (spGroup が返す "Compute Savings Plans"
  // 等) はカードタイトル (PRICING_SERVICE_LABELS) と意図的に一致させてある。両者が同じ
  // 文字列を並べて表示すると冗長なため、単一 group の見出しをカード側で抑制する。
  const hideSingleGroupTitle =
    groups.length === 1 && groups[0][0] === PRICING_SERVICE_LABELS[service];

  const subtotal = useMemo(() => {
    if (!table) return null;
    return estimate({ [service]: selection }, { [service]: table });
  }, [service, selection, table]);

  const iconKey = PRICING_SERVICE_ICON_KEY[service];
  const IconEl = AwsIcons[iconKey] ?? Icons[iconKey];
  const selectedCount = Object.values(selection).filter((e) => e.checked).length;
  const totalCount = table?.rates.length ?? 0;

  return (
    <section className="pr-card">
      <header className="pr-card-header">
        <button
          type="button"
          className="pr-card-collapse"
          onClick={onToggleCollapsed}
          aria-expanded={!collapsed}
          title={collapsed ? '展開' : '折りたたむ'}
        >
          <Icons.chevron size={14} style={{ transform: collapsed ? 'none' : 'rotate(90deg)' }} />
        </button>
        {IconEl && <IconEl size={16} />}
        <span className="pr-card-title">{PRICING_SERVICE_LABELS[service]}</span>
        <span className="pr-card-count">
          {selectedCount}/{totalCount} 選択中
        </span>
        {subtotal && subtotal.byService.length > 0 && (
          <span className="pr-card-subtotal">
            実効月額 {formatMoney(subtotal.totalEffectiveMonthly)}
          </span>
        )}
        <span className="pr-card-spacer" />
        {refreshing && <span className="pr-card-refreshing">更新中…</span>}
        {service === 'ec2-spot' && (
          <span
            className="pr-card-live-note"
            title="ディスクキャッシュを使わないライブ取得です。開いた時と更新ボタン押下時にのみ最新化されます"
          >
            ライブ取得 (自動更新なし)
          </span>
        )}
        {table?.licenseUnresolved && (
          <span className="pr-card-fetched">ライセンス区別 未解決 (縮退表示)</span>
        )}
        <button
          type="button"
          className="btn sm ghost"
          onClick={onRefresh}
          disabled={refreshing}
          title="このサービスの単価を再取得する"
        >
          <Icons.refresh size={12} />
        </button>
      </header>

      {!collapsed && (
        <div className="pr-card-body">
          {isLoading && <Loading />}

          {!isLoading && Boolean(error) && !table && (
            <div className="pr-card-error">
              <ErrorBanner error={error} />
              <button className="btn sm" onClick={onRetry}>
                再試行
              </button>
            </div>
          )}

          {!isLoading && table && (
            <>
              {Boolean(error) && (
                <div className="pr-card-inline-error">
                  更新に失敗しました (表示中はキャッシュされた前回取得分です)
                </div>
              )}
              {table.licenseUnresolved && (
                <div className="pr-card-partial-note">
                  ライセンスモデルの区別ができなかったため、Savings Plans
                  の一部の行でライセンス条件を区別せずに表示しています。
                </div>
              )}

              {service === 'ecs' && (
                <div className="pr-group pr-group-disabled">
                  <div className="pr-group-head">
                    <span className="pr-group-title">Reserved Instance</span>
                  </div>
                  <div className="pr-group-empty">
                    ECS (Fargate) はリザーブドインスタンスに対応していません。
                  </div>
                </div>
              )}

              {table.rates.length === 0 ? (
                <div className="pr-card-empty">該当する単価がありません。</div>
              ) : (
                <>
                  <div className="pr-card-filter">
                    <span className="chip-search">
                      <Icons.search size={12} />
                      <input
                        value={instanceFilter}
                        onChange={(e) => setInstanceFilter(e.target.value)}
                        placeholder="インスタンスタイプ等で絞り込み…"
                      />
                    </span>
                  </div>
                  <AttributeFilterBar
                    specs={attributeSpecs}
                    options={attributeOptions}
                    selected={attrSelection}
                    onToggle={toggleAttrValue}
                  />
                  {groups.length === 0 ? (
                    <div className="pr-card-empty">この条件に一致する単価がありません。</div>
                  ) : (
                    groups.map(([group, rates]) => (
                      <RateGroupSection
                        key={group}
                        group={group}
                        rates={rates}
                        selection={selection}
                        onToggleRate={onToggleRate}
                        instanceFilter={instanceFilter}
                        hideTitle={hideSingleGroupTitle}
                      />
                    ))
                  )}
                </>
              )}
            </>
          )}
        </div>
      )}
    </section>
  );
}
