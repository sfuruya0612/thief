// ServiceCard 内の 1 グループ (On-Demand / Reserved Instance / *** Savings Plans) の表示。
// RI/SP は条件 (期間/クラス/支払オプション) の組み合わせが多く、行チェックの羅列だと行数が
// 爆発するため、まずセレクタで 1 条件に絞ってから該当行だけを表に出す。選択中の条件は
// このコンポーネントのローカル state で持つ (チェック/数量のみ永続化対象であり、
// 条件セレクタ自体は永続化しない)。
import { useMemo, useState } from 'react';
import { formatPricingUnit, formatUnitPrice } from '../../lib/format';
import type { PriceRateRow } from '../../types/aws';
import { Icons } from '../icons/Icons';

export interface RateGroupSectionProps {
  group: string;
  rates: PriceRateRow[];
  selection: Record<string, { checked: boolean; qty: number }>;
  onToggleRate: (rateId: string) => void;
  instanceFilter: string;
  // On-Demand / Reserved Instance の行にだけ使う。値が入っている instance_type の集合で、
  // ここに無い instance_type は「この構成には Savings Plans の設定がありません」と注記する
  // (旧世代インスタンスの静的なリストを持たず、実データの有無で判定する)。
  spInstanceTypes?: Set<string>;
}

const LEASE_ORDER = ['1yr', '3yr'];
const OFFERING_CLASS_ORDER = ['standard', 'convertible'];
const PAYMENT_ORDER = ['No Upfront', 'Partial Upfront', 'All Upfront'];

function sortByPreferredOrder(values: string[], order: string[]): string[] {
  return [...values].sort((a, b) => {
    const ia = order.indexOf(a);
    const ib = order.indexOf(b);
    if (ia === -1 && ib === -1) return a.localeCompare(b);
    if (ia === -1) return 1;
    if (ib === -1) return -1;
    return ia - ib;
  });
}

function distinctValues(values: (string | null)[], order: string[]): string[] {
  const set = new Set(values.filter((v): v is string => !!v));
  return sortByPreferredOrder([...set], order);
}

function matchesInstanceFilter(rate: PriceRateRow, filter: string): boolean {
  if (!filter) return true;
  const needle = filter.trim().toLowerCase();
  if (!needle) return true;
  return (
    rate.label.toLowerCase().includes(needle) ||
    Object.values(rate.attributes).some((v) => v.toLowerCase().includes(needle))
  );
}

export function RateGroupSection({
  group,
  rates,
  selection,
  onToggleRate,
  instanceFilter,
  spInstanceTypes,
}: RateGroupSectionProps) {
  const model = rates[0]?.model ?? 'on_demand';

  const leaseOptions = useMemo(
    () =>
      distinctValues(
        rates.map((r) => r.term.lease),
        LEASE_ORDER,
      ),
    [rates],
  );
  const offeringClassOptions = useMemo(
    () =>
      distinctValues(
        rates.map((r) => r.term.offeringClass),
        OFFERING_CLASS_ORDER,
      ),
    [rates],
  );
  const paymentOptions = useMemo(
    () =>
      distinctValues(
        rates.map((r) => r.term.payment),
        PAYMENT_ORDER,
      ),
    [rates],
  );

  const [lease, setLease] = useState(() => leaseOptions[0] ?? '');
  const [offeringClass, setOfferingClass] = useState(() => offeringClassOptions[0] ?? '');
  const [payment, setPayment] = useState(() => paymentOptions[0] ?? '');
  const [collapsed, setCollapsed] = useState(false);

  const hasConditions = model !== 'on_demand';

  const filteredRates = useMemo(() => {
    return rates
      .filter((r) => !hasConditions || leaseOptions.length === 0 || r.term.lease === lease)
      .filter(
        (r) =>
          !hasConditions ||
          offeringClassOptions.length <= 1 ||
          r.term.offeringClass === offeringClass,
      )
      .filter((r) => !hasConditions || paymentOptions.length === 0 || r.term.payment === payment)
      .filter((r) => matchesInstanceFilter(r, instanceFilter));
  }, [
    rates,
    hasConditions,
    lease,
    offeringClass,
    payment,
    leaseOptions.length,
    offeringClassOptions.length,
    paymentOptions.length,
    instanceFilter,
  ]);

  return (
    <div className="pr-group">
      <div className="pr-group-head">
        <button
          type="button"
          className="pr-group-collapse"
          onClick={() => setCollapsed((c) => !c)}
          aria-expanded={!collapsed}
          title={collapsed ? '展開' : '折りたたむ'}
        >
          <Icons.chevron size={12} style={{ transform: collapsed ? 'none' : 'rotate(90deg)' }} />
        </button>
        <span className="pr-group-title">{group}</span>
        {hasConditions && !collapsed && (
          <div className="pr-group-conditions">
            {leaseOptions.length > 1 && (
              <select className="btn sm" value={lease} onChange={(e) => setLease(e.target.value)}>
                {leaseOptions.map((v) => (
                  <option key={v} value={v}>
                    {v}
                  </option>
                ))}
              </select>
            )}
            {offeringClassOptions.length > 1 && (
              <select
                className="btn sm"
                value={offeringClass}
                onChange={(e) => setOfferingClass(e.target.value)}
              >
                {offeringClassOptions.map((v) => (
                  <option key={v} value={v}>
                    {v}
                  </option>
                ))}
              </select>
            )}
            {paymentOptions.length > 1 && (
              <select
                className="btn sm"
                value={payment}
                onChange={(e) => setPayment(e.target.value)}
              >
                {paymentOptions.map((v) => (
                  <option key={v} value={v}>
                    {v}
                  </option>
                ))}
              </select>
            )}
          </div>
        )}
      </div>

      {!collapsed &&
        (filteredRates.length === 0 ? (
          <div className="pr-group-empty">この条件に一致する単価がありません</div>
        ) : (
          <table className="pr-rate-table">
            <tbody>
              {filteredRates.map((rate) => {
                const checked = selection[rate.rateId]?.checked ?? false;
                const instanceType = rate.attributes.instance_type;
                const noSp =
                  model !== 'savings_plan' &&
                  !!instanceType &&
                  !!spInstanceTypes &&
                  !spInstanceTypes.has(instanceType);
                return (
                  <tr key={rate.rateId} className={checked ? 'checked' : ''}>
                    <td className="pr-rate-check">
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={() => onToggleRate(rate.rateId)}
                        aria-label={rate.label}
                      />
                    </td>
                    <td className="pr-rate-label">{rate.label}</td>
                    <td className="pr-rate-price">
                      {formatUnitPrice(rate.priceUSD)}
                      {formatPricingUnit(rate.unit)}
                      {rate.upfrontUSD > 0 && (
                        <span className="pr-rate-upfront">
                          {' '}
                          + {formatUnitPrice(rate.upfrontUSD)} 前払い
                        </span>
                      )}
                    </td>
                    <td className="pr-rate-note">
                      {noSp && (
                        <span
                          className="pr-note-badge"
                          title="この構成には Savings Plans がありません"
                        >
                          SP対象外
                        </span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        ))}
    </div>
  );
}
