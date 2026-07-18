// Pricing 画面の選択状態 (アクティブサービス / 折りたたみ / チェック行+数量) を管理する reducer。
// region -> service -> rate_id の 3 段ネスト更新を PricingPanel から追い出すため、
// useReducer + 本ファイルの純関数に集約する (lib/sessionTabsState.ts と同じ方針)。
import type { PricingPersistedState } from './storage';

export const PRICING_SERVICES = ['ec2', 'rds', 'elasticache', 'ecs'] as const;
export type PricingService = (typeof PRICING_SERVICES)[number];

// Pricing のサービススラッグは backend/internal/aws/pricing.go の pricingServiceSpecs と
// 対称の名前 ("elasticache") を使うが、既存の SERVICES (lib/serviceMeta.ts) はリソース
// 一覧サイドバー向けの別名前空間 (ElastiCache は key: 'cache') のため、混同を避けて
// Pricing 専用の表示名/アイコンキー対応をここに持つ。
export const PRICING_SERVICE_LABELS: Record<PricingService, string> = {
  ec2: 'EC2',
  rds: 'RDS',
  elasticache: 'ElastiCache',
  ecs: 'ECS (Fargate)',
};

// components/icons/Icons.tsx・AwsIcons.tsx のキーへのマッピング (ElastiCache のみ 'cache' に読み替える)。
export const PRICING_SERVICE_ICON_KEY: Record<PricingService, string> = {
  ec2: 'ec2',
  rds: 'rds',
  elasticache: 'cache',
  ecs: 'ecs',
};

// 新規ユーザ (永続化データなし) は全サービスを表示した状態で開始する。
export function initialPricingState(persisted?: PricingPersistedState): PricingPersistedState {
  if (persisted) return persisted;
  return {
    activeServices: [...PRICING_SERVICES],
    collapsed: {},
    selection: {},
  };
}

export type PricingAction =
  | { type: 'toggleService'; service: string }
  | { type: 'toggleCollapsed'; service: string }
  | { type: 'toggleRate'; region: string; service: string; rateId: string }
  | { type: 'setQty'; region: string; service: string; rateId: string; qty: number }
  | { type: 'pruneStaleRates'; region: string; service: string; validRateIds: string[] };

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
    default:
      return state;
  }
}
