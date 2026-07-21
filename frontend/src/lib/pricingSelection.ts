// Pricing 画面の選択状態 (アクティブサービス / 折りたたみ / チェック行+数量) を管理する reducer。
// region -> service -> rate_id の 3 段ネスト更新を PricingPanel から追い出すため、
// useReducer + 本ファイルの純関数に集約する (lib/sessionTabsState.ts と同じ方針)。
import type { PricingPersistedState } from './storage';

// issue 0055: Savings Plans を compute-sp/ec2-instance-sp/database-sp の独立した
// サービスに分離し、ec2/rds/elasticache/ecs のカードは On-Demand/Reserved Instance
// (ecs は On-Demand) のみを表示するようにした。
// issue 0056: EC2 Spot を独立サービス ec2-spot として追加した。ec2-spot はバックエンド
// のディスクキャッシュを経由しないライブ取得サービスで、staleTime も他サービスと異なる
// (usePricing の呼び出し側 (PricingPanel) で有限値を渡す)。
// 表示順は issue 0063 で決定した固定順 (EC2, EC2 Spot, ECS, RDS, ElastiCache,
// Compute SP, EC2 Instance SP, Database SP)。ServiceSelectorBar と PricingPanel の
// カード一覧はいずれもこの配列順で描画する (state.activeServices の永続化順ではなく)。
export const PRICING_SERVICES = [
  'ec2',
  'ec2-spot',
  'ecs',
  'rds',
  'elasticache',
  'compute-sp',
  'ec2-instance-sp',
  'database-sp',
] as const;
export type PricingService = (typeof PRICING_SERVICES)[number];

// Pricing のサービススラッグは backend/internal/aws/pricing.go の resourceServiceSpecs /
// savingsPlanServiceSpecs と対称の名前 ("elasticache") を使うが、既存の SERVICES
// (lib/serviceMeta.ts) はリソース一覧サイドバー向けの別名前空間 (ElastiCache は
// key: 'cache') のため、混同を避けて Pricing 専用の表示名/アイコンキー対応をここに持つ。
// SP 3 種のラベルは backend の spGroup が返す group 名 ("Compute Savings Plans" 等) と
// 意図的に一致させる。SP カードは group が 1 種類しかないため、カードタイトルと group
// 見出しが同一文字列であれば見出し側を抑制できる (components/pricing/ServiceCard.tsx 参照)。
export const PRICING_SERVICE_LABELS: Record<PricingService, string> = {
  ec2: 'EC2',
  rds: 'RDS',
  elasticache: 'ElastiCache',
  ecs: 'ECS (Fargate)',
  'compute-sp': 'Compute Savings Plans',
  'ec2-instance-sp': 'EC2 Instance Savings Plans',
  'database-sp': 'Database Savings Plans',
  'ec2-spot': 'EC2 Spot',
};

// components/icons/Icons.tsx・AwsIcons.tsx のキーへのマッピング (ElastiCache のみ 'cache' に読み替える)。
// Savings Plans には対応する AWS 公式アイコンが無いため、3 種とも Icons.tsx のインライン
// SVG (savingsPlan) を使う。EC2 Spot も対応する AWS 公式アイコンが無いため、専用の
// インライン SVG (spot) を使う (issue 0056)。
export const PRICING_SERVICE_ICON_KEY: Record<PricingService, string> = {
  ec2: 'ec2',
  rds: 'rds',
  elasticache: 'cache',
  ecs: 'ecs',
  'compute-sp': 'savingsPlan',
  'ec2-instance-sp': 'savingsPlan',
  'database-sp': 'savingsPlan',
  'ec2-spot': 'spot',
};

// PricingPersistedState.pricingSchemaVersion の現在値。既定 active なメンバーを
// PRICING_SERVICES に追加するリリースごとに 1 つずつ版を上げ、PRICING_SCHEMA_MIGRATIONS
// にその版で追加されたメンバーを追記する (既存のエントリは変更しない)。
export const PRICING_SCHEMA_VERSION = 2;

// 版ごとに新規追加された既定 active サービス。migratePricingState はこの版番号を
// 単調に辿り、各版で追加されたメンバーだけを補完する (PRICING_SERVICES 全体を無条件に
// 足し込むと、ユーザーが既に OFF にしていた既存サービスまで復活してしまうため)。
const PRICING_SCHEMA_MIGRATIONS: Record<number, PricingService[]> = {
  1: ['compute-sp', 'ec2-instance-sp', 'database-sp'],
  // issue 0056: EC2 Spot を追加。
  2: ['ec2-spot'],
};

// 永続化された Pricing 状態を、単調増加のスキーマ版に基づいて一度だけ移行する。
// version が現行版に達していれば何もしない (ユーザーが OFF にした新サービスを毎ロードで
// 復活させる非対称な退行を避けるため、無条件の集合差分補完はしない)。version が古い
// 場合のみ、その版までに追加された新メンバーを activeServices へ補完し、version を
// 現行版まで進める。0055 と 0056 のように新サービスを追加するリリースが複数回に
// 分かれても、各版の補完が一度ずつ確実に実行される。
export function migratePricingState(persisted: PricingPersistedState): PricingPersistedState {
  let version = persisted.pricingSchemaVersion ?? 0;
  if (version >= PRICING_SCHEMA_VERSION) return persisted;
  const active = new Set(persisted.activeServices);
  while (version < PRICING_SCHEMA_VERSION) {
    version += 1;
    for (const service of PRICING_SCHEMA_MIGRATIONS[version] ?? []) {
      active.add(service);
    }
  }
  return {
    ...persisted,
    activeServices: [...active],
    pricingSchemaVersion: PRICING_SCHEMA_VERSION,
  };
}

// 新規ユーザ (永続化データなし) は全サービスを表示した状態で開始する。
// 既存ユーザ (永続化データあり) はスキーマ版の移行を経由する。
export function initialPricingState(persisted?: PricingPersistedState): PricingPersistedState {
  if (!persisted) {
    return {
      activeServices: [...PRICING_SERVICES],
      collapsed: {},
      selection: {},
      pricingSchemaVersion: PRICING_SCHEMA_VERSION,
    };
  }
  return migratePricingState(persisted);
}

export type PricingAction =
  | { type: 'toggleService'; service: string }
  | { type: 'toggleCollapsed'; service: string }
  | { type: 'toggleRate'; region: string; service: string; rateId: string }
  | { type: 'setQty'; region: string; service: string; rateId: string; qty: number }
  | { type: 'pruneStaleRates'; region: string; service: string; validRateIds: string[] }
  | { type: 'clearSelection'; region: string };

type RateEntry = { checked: boolean; qty: number };

function withRateEntry(
  state: PricingPersistedState,
  region: string,
  service: string,
  rateId: string,
  update: (prev: RateEntry) => RateEntry,
): PricingPersistedState {
  const prevRegion = state.selection[region] ?? {};
  const prevService = prevRegion[service] ?? {};
  const prevEntry = prevService[rateId] ?? { checked: false, qty: 1 };
  return {
    ...state,
    selection: {
      ...state.selection,
      [region]: {
        ...prevRegion,
        [service]: {
          ...prevService,
          [rateId]: update(prevEntry),
        },
      },
    },
  };
}

export function pricingReducer(
  state: PricingPersistedState,
  action: PricingAction,
): PricingPersistedState {
  switch (action.type) {
    case 'toggleService': {
      const active = new Set(state.activeServices);
      if (active.has(action.service)) {
        active.delete(action.service);
      } else {
        active.add(action.service);
      }
      return { ...state, activeServices: [...active] };
    }
    case 'toggleCollapsed':
      return {
        ...state,
        collapsed: { ...state.collapsed, [action.service]: !state.collapsed[action.service] },
      };
    case 'toggleRate':
      return withRateEntry(state, action.region, action.service, action.rateId, (prev) => ({
        ...prev,
        checked: !prev.checked,
      }));
    case 'setQty':
      return withRateEntry(state, action.region, action.service, action.rateId, (prev) => ({
        ...prev,
        qty: action.qty,
      }));
    case 'pruneStaleRates': {
      const regionSel = state.selection[action.region];
      const serviceSel = regionSel?.[action.service];
      if (!serviceSel) return state;
      const validSet = new Set(action.validRateIds);
      const next: Record<string, RateEntry> = {};
      let changed = false;
      for (const [rateId, entry] of Object.entries(serviceSel)) {
        if (validSet.has(rateId)) {
          next[rateId] = entry;
        } else {
          changed = true;
        }
      }
      if (!changed) return state;
      return {
        ...state,
        selection: {
          ...state.selection,
          [action.region]: { ...regionSel, [action.service]: next },
        },
      };
    }
    case 'clearSelection':
      return {
        ...state,
        selection: {
          ...state.selection,
          [action.region]: {},
        },
      };
    default:
      return state;
  }
}
