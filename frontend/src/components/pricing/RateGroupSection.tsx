// ServiceCard 内の 1 グループ (On-Demand / Reserved Instance / *** Savings Plans) の表示。
// RI/SP は条件 (期間/クラス/支払オプション) の組み合わせが多く、行チェックの羅列だと行数が
// 爆発するため、まずセレクタで 1 条件に絞ってから該当行だけを表に出す。選択中の条件は
// このコンポーネントのローカル state で持つ (チェック/数量のみ永続化対象であり、
// 条件セレクタ自体は永続化しない)。
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { formatPercent, formatPricingUnit, formatUnitPrice } from '../../lib/format';
import { effectiveHourlyRate, monthlyRecurring, savingsPercent } from '../../lib/pricingEstimate';
import { useWindowedRows } from '../../hooks/useWindowedRows';
import type { PriceRateRow } from '../../types/aws';
import { Icons } from '../icons/Icons';

export interface RateGroupSectionProps {
  group: string;
  rates: PriceRateRow[];
  selection: Record<string, { checked: boolean; qty: number }>;
  onToggleRate: (rateId: string) => void;
  instanceFilter: string;
  // SP カードは group が 1 種類しかなく、その名前がカードタイトルと同じ文字列になるため
  // (issue 0055)、group 見出しの重複表示を呼び出し側 (ServiceCard) の判断で抑制する。
  hideTitle?: boolean;
  // Reserved Instance 行の On-Demand 比節減率を出すための、同一 label の On-Demand
  // 時間単価の対応表 (issue 0057)。ServiceCard が table.rates 全体から作って渡す。
  // On-Demand/Savings Plans の行では参照しない。
  onDemandHourlyByLabel?: Map<string, number>;
}

const LEASE_ORDER = ['1yr', '3yr'];
const OFFERING_CLASS_ORDER = ['standard', 'convertible'];
const PAYMENT_ORDER = ['No Upfront', 'Partial Upfront', 'All Upfront'];

// この行数以上のグループだけ windowing を有効化する。少数行では DOM 生成コストより
// スペーサー管理・スクロール購読のオーバーヘッドが勝るため、そのまま全描画する。
const WINDOW_THRESHOLD = 60;
// 行高の初期推定値 (px)。実測が済むまでの初回描画用。Reserved 行は ReservedRatePrice が
// 2 段組みで背が高いため別値。実測後は useWindowedRows がこの値を測定値へ補正する。
const ROW_HEIGHT_ESTIMATE = 28;
const RESERVED_ROW_HEIGHT_ESTIMATE = 44;

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
  hideTitle,
  onDemandHourlyByLabel,
}: RateGroupSectionProps) {
  const { t } = useTranslation('pricing');
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

  const rowCount = filteredRates.length;
  const { range, rootRef, listRef, rowRef } = useWindowedRows({
    rowCount,
    enabled: !collapsed && rowCount >= WINDOW_THRESHOLD,
    estimateRowHeight: model === 'reserved' ? RESERVED_ROW_HEIGHT_ESTIMATE : ROW_HEIGHT_ESTIMATE,
  });

  return (
    <div className="pr-group" ref={rootRef}>
      <div className="pr-group-head">
        <button
          type="button"
          className="pr-group-collapse"
          onClick={() => setCollapsed((c) => !c)}
          aria-expanded={!collapsed}
          title={collapsed ? t('rateGroupSection.expand') : t('rateGroupSection.collapse')}
        >
          <Icons.chevron size={12} style={{ transform: collapsed ? 'none' : 'rotate(90deg)' }} />
        </button>
        {!hideTitle && <span className="pr-group-title">{group}</span>}
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
          <div className="pr-group-empty">{t('rateGroupSection.noMatch')}</div>
        ) : (
          <table className="pr-rate-table">
            <tbody ref={listRef}>
              {range.topPad > 0 && (
                <tr aria-hidden="true" className="pr-rate-spacer">
                  <td colSpan={3} style={{ height: range.topPad }} />
                </tr>
              )}
              {filteredRates.slice(range.start, range.end).map((rate, i) => {
                const checked = selection[rate.rateId]?.checked ?? false;
                return (
                  <tr
                    key={rate.rateId}
                    ref={i === 0 ? rowRef : undefined}
                    className={checked ? 'checked' : ''}
                  >
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
                      {rate.model === 'reserved' ? (
                        <ReservedRatePrice
                          rate={rate}
                          onDemandHourly={onDemandHourlyByLabel?.get(rate.label)}
                        />
                      ) : (
                        <>
                          {formatUnitPrice(rate.priceUSD)}
                          {formatPricingUnit(rate.unit)}
                          {rate.upfrontUSD > 0 && (
                            <span className="pr-rate-upfront">
                              {' '}
                              + {formatUnitPrice(rate.upfrontUSD)} {t('rateGroupSection.upfront')}
                            </span>
                          )}
                        </>
                      )}
                    </td>
                  </tr>
                );
              })}
              {range.bottomPad > 0 && (
                <tr aria-hidden="true" className="pr-rate-spacer">
                  <td colSpan={3} style={{ height: range.bottomPad }} />
                </tr>
              )}
            </tbody>
          </table>
        ))}
    </div>
  );
}

// Reserved Instance 行の価格表示 (issue 0057)。支払オプションをまたいで比較できる
// 実効時間単価 (前払い按分込み) を主表示にし、内訳 (前払い、継続月額) と On-Demand 比
// 節減率を添える。All Upfront (継続時間単価が 0 になる) でも実効時間単価は前払いの
// 按分だけで正しく算出される。
function ReservedRatePrice({
  rate,
  onDemandHourly,
}: {
  rate: PriceRateRow;
  onDemandHourly: number | undefined;
}) {
  const { t } = useTranslation('pricing');
  const effective = effectiveHourlyRate(rate);
  const monthly = monthlyRecurring(rate);
  const savings = savingsPercent(effective, onDemandHourly);
  return (
    <>
      <div className="pr-rate-effective-row">
        <span className="pr-rate-effective">
          {formatUnitPrice(effective)}
          {formatPricingUnit(rate.unit)}
        </span>
        <span className="pr-rate-effective-label">{t('rateGroupSection.effective')}</span>
      </div>
      <div className="pr-rate-ri-detail">
        <span>
          {t('rateGroupSection.monthly')} {formatUnitPrice(monthly)}
        </span>
        {rate.upfrontUSD > 0 && (
          <span>
            {t('rateGroupSection.upfront')} {formatUnitPrice(rate.upfrontUSD)}
          </span>
        )}
        <span className={savings !== null && savings < 0 ? 'pr-rate-savings-negative' : undefined}>
          {t('rateGroupSection.onDemandComparison')}{' '}
          {savings === null ? '—' : formatPercent(savings)}
        </span>
      </div>
    </>
  );
}
