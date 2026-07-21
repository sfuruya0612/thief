// Pricing (AWS Price List / Savings Plans の正規化レート表) の Raw → Row 変換
import type {
  PriceRateRaw,
  PriceRateRow,
  PriceTableRaw,
  PriceTableRow,
  PriceTermRaw,
  PriceTermRow,
} from '../types/aws';

export function priceTermFromRaw(raw: PriceTermRaw): PriceTermRow {
  return {
    lease: raw.lease,
    offeringClass: raw.offering_class,
    payment: raw.payment,
  };
}

export function priceRateFromRaw(raw: PriceRateRaw): PriceRateRow {
  return {
    rateId: raw.rate_id,
    model: raw.model,
    group: raw.group,
    label: raw.label,
    attributes: raw.attributes,
    term: priceTermFromRaw(raw.term),
    unit: raw.unit,
    priceUSD: raw.price_usd,
    upfrontUSD: raw.upfront_usd,
    currency: raw.currency,
  };
}

export function priceTableFromRaw(raw: PriceTableRaw): PriceTableRow {
  return {
    service: raw.service,
    region: raw.region,
    fetchedAt: raw.fetched_at,
    licenseUnresolved: raw.license_unresolved,
    rates: raw.rates.map(priceRateFromRaw),
  };
}
