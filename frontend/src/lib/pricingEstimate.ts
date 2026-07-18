// Pricing 画面の見積もり計算。月額継続 / 前払い一括 / 実効月額の 3 系統を常に分けて扱い、
// 性質の異なる金額 (継続課金と一括前払い) を 1 つの合計に混ぜない。
import type { PriceRateRow, PriceTableRow } from '../types/aws';

// 365 日 × 24 時間 / 12 か月の近似値。実月の時間数 (28〜31 日) とは一致しない。
export const HOURS_PER_MONTH = 730;

export interface PriceSubtotal {
  recurringMonthly: number;
  upfrontOnce: number;
  effectiveMonthly: number;
}

// term.lease から前払い分の月割に使う契約月数を引く。on-demand (lease=null) や
// 未知の値は 0 を返し、呼び出し側で「月割なし」として扱わせる (0 除算を避ける)。
function contractMonthsFromLease(lease: string | null): number {
  switch (lease) {
    case '1yr':
      return 12;
    case '3yr':
      return 36;
    default:
      return 0;
  }
}

// qty 件のレートの小計を 3 系統で返す。
// - recurringMonthly: 時間単価 × 730h × qty (RI/SP でも継続して発生する分)
// - upfrontOnce: 前払い額 × qty (一括、月額ではない)
// - effectiveMonthly: recurringMonthly に前払いの月割 (upfrontOnce / 契約月数) を加えた実効値
export function subtotal(qty: number, rate: PriceRateRow): PriceSubtotal {
  const recurringMonthly = rate.priceUSD * HOURS_PER_MONTH * qty;
  const upfrontOnce = rate.upfrontUSD * qty;
  const months = contractMonthsFromLease(rate.term.lease);
  const amortizedUpfront = months > 0 ? upfrontOnce / months : 0;
  return {
    recurringMonthly,
    upfrontOnce,
    effectiveMonthly: recurringMonthly + amortizedUpfront,
  };
}

export interface PriceSelectionEntry {
  checked: boolean;
  qty: number;
}

// service -> rate_id -> 選択状態 (現在表示中のリージョンに対するもの)。
export type PriceSelectionByService = Record<string, Record<string, PriceSelectionEntry>>;

// service -> 現在表示中のリージョンで取得済みのレート表。
export type PriceTablesByService = Record<string, PriceTableRow>;

export interface PriceServiceBreakdown {
  service: string;
  recurringMonthly: number;
  upfrontOnce: number;
  effectiveMonthly: number;
}

export interface PriceEstimate {
  byService: PriceServiceBreakdown[];
  totalRecurringMonthly: number;
  totalUpfrontOnce: number;
  totalEffectiveMonthly: number;
}

// selection と rates (現在のリージョンのもの) からサービス別の内訳と 3 系統の合計を算出する。
// rates に対応テーブルがないサービス、テーブルに存在しない rate_id、チェック外の行は
// 安全にスキップする (リージョン切替直後の未解決な選択を想定)。
export function estimate(
  selection: PriceSelectionByService,
  rates: PriceTablesByService,
): PriceEstimate {
  const byService: PriceServiceBreakdown[] = [];
  let totalRecurringMonthly = 0;
  let totalUpfrontOnce = 0;
  let totalEffectiveMonthly = 0;

  for (const [service, byRateId] of Object.entries(selection)) {
    const table = rates[service];
    if (!table) continue;
    const rateById = new Map(table.rates.map((r) => [r.rateId, r]));

    let recurringMonthly = 0;
    let upfrontOnce = 0;
    let effectiveMonthly = 0;
    let countedRows = 0;

    for (const [rateId, entry] of Object.entries(byRateId)) {
      if (!entry.checked) continue;
      const rate = rateById.get(rateId);
      if (!rate) continue;
      const sub = subtotal(entry.qty, rate);
      recurringMonthly += sub.recurringMonthly;
      upfrontOnce += sub.upfrontOnce;
      effectiveMonthly += sub.effectiveMonthly;
      countedRows += 1;
    }

    if (countedRows === 0) continue;
    byService.push({ service, recurringMonthly, upfrontOnce, effectiveMonthly });
    totalRecurringMonthly += recurringMonthly;
    totalUpfrontOnce += upfrontOnce;
    totalEffectiveMonthly += effectiveMonthly;
  }

  return { byService, totalRecurringMonthly, totalUpfrontOnce, totalEffectiveMonthly };
}
