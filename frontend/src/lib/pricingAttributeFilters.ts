// ServiceCard の curated attribute (os/engine/deployment_option 等) によるマルチセレクト
// チップ絞り込み。instance_type は種類が多すぎるためテキスト検索 (instanceFilter) に任せ、
// 値の種類が少なく一覧提示に向く属性だけをここで扱う。
import type { PriceRateRow } from '../types/aws';
import type { PricingService } from './pricingSelection';

export interface AttributeFilterSpec {
  key: string;
  label: string;
  // 属性の生値 (例: "standard") をチップの表示ラベル (例: "Standard") に変換する。
  // 未指定ならそのまま表示する。os/engine 等は AWS 側の値をそのまま出すため不要だが、
  // storage_type は本プロジェクトが正規化した内部値のため表示用の変換が要る。
  valueLabels?: Record<string, string>;
}

// backend/internal/aws/pricing.go の curatedInstanceAttributes が rate.attributes に
// 詰める curated キーのうち、値の種類が少なくチップ絞り込みに向くものだけを対象にする。
// instance_type は対象外 (種類が多すぎる)、EC2 の tenancy は対象外 ("Shared" 固定で
// 絞り込む意味がない)。ECS は curated attribute が os/architecture のみで、Fargate は
// インスタンスタイプという概念自体がないためチップ絞り込みの対象にしない。
//
// RDS の storage_type (Standard/IO-Optimized) は Aurora 専用の軸で、Savings Plans の
// 行には存在しない (attributes.storage_type が未設定になる) ため、matchesAttributeSelection
// は値を持たない行をこのフィルタの対象外として扱う (SP 行が誤って弾かれないようにするため)。
//
// license_model (RDS の Oracle 等の BYOL/License Included、EC2 の Windows BYOL 等) は
// On-Demand/Reserved だけでなく Savings Plans の行にも付与される (issue 0053: Operation
// コード経由で On-Demand 側から逆引きする)。ただし対応する Operation が見つからない行では
// 値を持たない (storage_type と同様、matchesAttributeSelection がその行を対象外として扱う)。
export const PRICING_ATTRIBUTE_FILTERS: Record<PricingService, AttributeFilterSpec[]> = {
  ec2: [
    { key: 'os', label: 'OS' },
    { key: 'license_model', label: 'License' },
  ],
  rds: [
    { key: 'engine', label: 'Engine' },
    { key: 'deployment_option', label: 'Deployment' },
    {
      key: 'storage_type',
      label: 'Storage',
      valueLabels: { standard: 'Standard', io_optimized: 'IO-Optimized' },
    },
    { key: 'license_model', label: 'License' },
  ],
  elasticache: [{ key: 'engine', label: 'Engine' }],
  ecs: [],
};

// rates 全体から、指定した attribute key に実在する値の集合を昇順で返す。
export function attributeValueOptions(rates: PriceRateRow[], key: string): string[] {
  const set = new Set<string>();
  for (const r of rates) {
    const v = r.attributes[key];
    if (v) set.add(v);
  }
  return [...set].sort();
}

// selected の各キーについて、値集合が空なら絞り込みなし (常に一致)。1 つ以上選択されて
// いれば、rate.attributes[key] がその集合に含まれるものだけを残す。複数キーは AND 条件。
// rate がそのキー自体を持たない (例: Savings Plans 行の storage_type) 場合は、その軸に
// 関して絞り込みの対象外として常に一致させる (フィルタ非対応のモデルが誤って隠れないため)。
export function matchesAttributeSelection(
  rate: PriceRateRow,
  selected: Record<string, Set<string>>,
): boolean {
  return Object.entries(selected).every(([key, values]) => {
    if (values.size === 0) return true;
    const value = rate.attributes[key];
    if (!value) return true;
    return values.has(value);
  });
}
