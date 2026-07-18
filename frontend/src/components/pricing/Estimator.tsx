// 右レールの見積もり。チェック済みの行を数量入力付きの明細として並べ、月額継続/前払い一括/
// 実効月額の 3 系統を常に分けて表示する (性質の異なる金額を 1 つの合計に混ぜない)。
import { useMemo } from 'react';
import { Money } from '../primitives/Money';
import {
  estimate,
  subtotal,
  type PriceSelectionByService,
  type PriceTablesByService,
} from '../../lib/pricingEstimate';
import { PRICING_SERVICE_LABELS, type PricingService } from '../../lib/pricingSelection';
import { Icons } from '../icons/Icons';

export interface EstimatorProps {
  selection: PriceSelectionByService;
  rates: PriceTablesByService;
  onSetQty: (service: string, rateId: string, qty: number) => void;
  onToggleRate: (service: string, rateId: string) => void;
}

export function Estimator({ selection, rates, onSetQty, onToggleRate }: EstimatorProps) {
  const result = useMemo(() => estimate(selection, rates), [selection, rates]);

  return (
    <aside className="pr-estimator">
      <div className="pr-estimator-head">
        <h2>見積もり</h2>
        <div className="pr-estimator-totals">
          <div className="pr-estimator-total">
            <span className="label">月額継続</span>
            <Money value={result.totalRecurringMonthly} />
          </div>
          <div className="pr-estimator-total">
            <span className="label">前払い一括</span>
            <Money value={result.totalUpfrontOnce} />
          </div>
          <div className="pr-estimator-total">
            <span className="label">実効月額</span>
            <Money value={result.totalEffectiveMonthly} />
          </div>
        </div>
        <p className="pr-estimator-note">
          730 時間/月 (365×24/12 の近似) で計算しています。実月の時間数とは異なります。 Savings
          Plans の前払いは API 仕様上 $0 として扱われます (購入時のコミット額は含みません)。
        </p>
      </div>

      <div className="pr-estimator-body">
        {result.byService.length === 0 ? (
          <div className="pr-estimator-empty">単価表の行をチェックすると見積もりに追加されます</div>
        ) : (
          result.byService.map((b) => {
            const table = rates[b.service];
            const rateById = new Map(table?.rates.map((r) => [r.rateId, r]) ?? []);
            const entries = Object.entries(selection[b.service] ?? {}).filter(([, e]) => e.checked);
            return (
              <div key={b.service} className="pr-estimator-group">
                <div className="pr-estimator-group-head">
                  <span>{PRICING_SERVICE_LABELS[b.service as PricingService] ?? b.service}</span>
                  <Money value={b.effectiveMonthly} />
                </div>
                {entries.map(([rateId, entry]) => {
                  const rate = rateById.get(rateId);
                  if (!rate) return null;
                  const sub = subtotal(entry.qty, rate);
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
                          title="見積もりから外す"
                        >
                          <Icons.x size={11} />
                        </button>
                      </div>
                      <label className="pr-estimator-line-qty">
                        数量
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
                          継続 <Money value={sub.recurringMonthly} />
                        </span>
                        <span>
                          前払い <Money value={sub.upfrontOnce} />
                        </span>
                        <span>
                          実効 <Money value={sub.effectiveMonthly} />
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
