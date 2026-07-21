// 右レールの見積もり。チェック済みの行を数量入力付きの明細として並べ、月額継続/前払い一括/
// 実効月額の 3 系統を常に分けて表示する (性質の異なる金額を 1 つの合計に混ぜない)。
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Money } from '../primitives/Money';
import {
  estimate,
  subtotal,
  type PriceSelectionByService,
  type PriceTablesByService,
} from '../../lib/pricingEstimate';
import { PRICING_SERVICE_LABELS, type PricingService } from '../../lib/pricingSelection';
import { Icons } from '../icons/Icons';
import type { PriceRateRow } from '../../types/aws';

// RI/SP の行にのみ期間・オファリングクラス (EC2 RI のみ)・購入タイプを表示し、行を区別できる
// ようにする (同一インスタンスタイプで条件違いの行を複数チェックした場合に必要)。
// On-Demand はこれらの条件を持たない (term.* が null) ため表示しない。
// 表現は単価表のセレクタ (RateGroupSection) の値をそのまま使い、見た目を揃える。
function termLabel(rate: PriceRateRow): string | null {
  if (rate.model === 'on_demand') return null;
  const parts = [rate.term.lease, rate.term.offeringClass, rate.term.payment].filter(
    (v): v is string => !!v,
  );
  return parts.length > 0 ? parts.join(' / ') : null;
}

export interface EstimatorProps {
  selection: PriceSelectionByService;
  rates: PriceTablesByService;
  onSetQty: (service: string, rateId: string, qty: number) => void;
  onToggleRate: (service: string, rateId: string) => void;
  onClearAll: () => void;
}

export function Estimator({
  selection,
  rates,
  onSetQty,
  onToggleRate,
  onClearAll,
}: EstimatorProps) {
  const { t } = useTranslation('pricing');
  const result = useMemo(() => estimate(selection, rates), [selection, rates]);
  const hasEntries = result.byService.length > 0;

  // サービスごとの rateId -> PriceRateRow の対応表 (issue 0058)。以前はこれをレンダー
  // 本体の byService.map ループ内で毎回作り直しており、qty 入力の 1 打鍵ごとに
  // アクティブな全サービスの全行を rates が変わっていなくても再構築していた。計測では
  // 8 サービス・合計約 2450 行のとき 500 回の再レンダーで約 42ms (0.084ms/レンダー) の
  // 無駄になり、rates が変わったときだけ作る形にすると実質 0 になった。rates が変わった
  // ときだけ作り直すよう useMemo でメモ化する。
  const rateByIdByService = useMemo(() => {
    const out: Record<string, Map<string, PriceRateRow>> = {};
    for (const [service, table] of Object.entries(rates)) {
      out[service] = new Map(table.rates.map((r) => [r.rateId, r]));
    }
    return out;
  }, [rates]);

  const handleClearAll = () => {
    if (window.confirm(t('estimator.clearAllConfirm'))) {
      onClearAll();
    }
  };

  return (
    <aside className="pr-estimator">
      <div className="pr-estimator-head">
        <div className="pr-estimator-title-row">
          <h2>{t('estimator.title')}</h2>
          <button
            type="button"
            className="btn sm ghost"
            onClick={handleClearAll}
            disabled={!hasEntries}
            title={t('estimator.clearAllTitle')}
          >
            {t('estimator.clearAll')}
          </button>
        </div>
        <div className="pr-estimator-totals">
          <div className="pr-estimator-total">
            <span className="label">{t('estimator.totalRecurringMonthly')}</span>
            <Money value={result.totalRecurringMonthly} />
          </div>
          <div className="pr-estimator-total">
            <span className="label">{t('estimator.totalUpfrontOnce')}</span>
            <Money value={result.totalUpfrontOnce} />
          </div>
          <div className="pr-estimator-total">
            <span className="label">{t('estimator.totalEffectiveMonthly')}</span>
            <Money value={result.totalEffectiveMonthly} />
          </div>
        </div>
        <p className="pr-estimator-note">{t('estimator.note')}</p>
      </div>

      <div className="pr-estimator-body">
        {result.byService.length === 0 ? (
          <div className="pr-estimator-empty">{t('estimator.empty')}</div>
        ) : (
          result.byService.map((b) => {
            const rateById = rateByIdByService[b.service] ?? new Map<string, PriceRateRow>();
            const entries = Object.entries(selection[b.service] ?? {}).filter(([, e]) => e.checked);
            return (
              <div key={b.service} className="pr-estimator-group">
                <div className="pr-estimator-group-head">
                  <span>{PRICING_SERVICE_LABELS[b.service as PricingService] ?? b.service}</span>
                  <Money value={b.effectiveMonthly} />
                </div>
                {b.service === 'ec2-spot' && (
                  <p className="pr-estimator-note">{t('estimator.spotNote')}</p>
                )}
                {entries.map(([rateId, entry]) => {
                  const rate = rateById.get(rateId);
                  if (!rate) return null;
                  const sub = subtotal(entry.qty, rate);
                  const term = termLabel(rate);
                  return (
                    <div key={rateId} className="pr-estimator-line">
                      <div className="pr-estimator-line-head">
                        <span className="pr-estimator-line-label" title={rate.label}>
                          {rate.label}
                        </span>
                        <button
                          type="button"
                          className="pr-estimator-line-remove"
                          onClick={() => onToggleRate(b.service, rateId)}
                          title={t('estimator.removeTitle')}
                        >
                          <Icons.x size={11} />
                        </button>
                      </div>
                      {term && <div className="pr-estimator-line-term">{term}</div>}
                      <label className="pr-estimator-line-qty">
                        {t('estimator.qty')}
                        <input
                          type="number"
                          min={0}
                          step={1}
                          value={entry.qty}
                          onChange={(e) => {
                            const v = Number(e.target.value);
                            const qty = Number.isFinite(v) ? Math.max(0, Math.floor(v)) : 0;
                            onSetQty(b.service, rateId, qty);
                          }}
                        />
                      </label>
                      <div className="pr-estimator-line-amounts">
                        <span>
                          {t('estimator.lineRecurring')} <Money value={sub.recurringMonthly} />
                        </span>
                        <span>
                          {t('estimator.lineUpfront')} <Money value={sub.upfrontOnce} />
                        </span>
                        <span>
                          {t('estimator.lineEffective')} <Money value={sub.effectiveMonthly} />
                        </span>
                      </div>
                    </div>
                  );
                })}
              </div>
            );
          })
        )}
      </div>
    </aside>
  );
}
