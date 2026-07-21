// AWS Pricing (単価確認・見積もり) 専用パネル。ServicePanel (汎用) や CostExplorerPanel と
// 同様に AccountView から activeService === 'pricing' で分岐する。
// 対象は固定 8 サービス (issue 0055 で Savings Plans 3 種、issue 0056 で ec2-spot が
// 独立サービスとして加わった) のため、rules-of-hooks を守るために usePricing/
// useRefreshPricing を可変長配列の map ではなく 8 個ずつ無条件に呼び、enabled で
// active/inactive をゲートする。ec2-spot はバックエンドのディスクキャッシュを経由
// しないライブ取得サービスのため、usePricing の staleTime のみ他と異なり有限値
// (EC2_SPOT_STALE_TIME) を渡す。
import { useEffect, useMemo, useReducer } from 'react';
import { usePricing, useRefreshPricing, useRegions } from '../api/queries';
import { Estimator } from '../components/pricing/Estimator';
import { PricingToolbar } from '../components/pricing/PricingToolbar';
import { ServiceCard } from '../components/pricing/ServiceCard';
import { ServiceSelectorBar } from '../components/pricing/ServiceSelectorBar';
import type { PriceTablesByService } from '../lib/pricingEstimate';
import {
  initialPricingState,
  pricingReducer,
  PRICING_SERVICES,
  type PricingService,
} from '../lib/pricingSelection';
import { loadPersisted, savePersisted } from '../lib/storage';
import { ApiError } from '../types/common';
import { SSOExpiredBanner } from '../components/SSOExpiredBanner';

export interface PricingPanelProps {
  profile: string;
  region: string;
  onRegionChange: (region: string) => void;
}

function isPricingService(s: string): s is PricingService {
  return (PRICING_SERVICES as readonly string[]).includes(s);
}

// ec2-spot はディスクキャッシュを経由しないライブ取得のため staleTime を有限値に
// する (他サービスは Infinity のまま)。refetchOnWindowFocus: false / refetchInterval
// なしの全体設定のもとでは、この staleTime によってもマウント時と手動更新以外の
// 自動再取得は起きない (パネルを開いたままにしても継続的には最新化されない)。
const EC2_SPOT_STALE_TIME = 60_000;

export function PricingPanel({ profile, region, onRegionChange }: PricingPanelProps) {
  const [state, dispatch] = useReducer(pricingReducer, undefined, () =>
    initialPricingState(loadPersisted().pricing),
  );

  useEffect(() => {
    savePersisted({ ...loadPersisted(), pricing: state });
  }, [state]);

  const { data: regions } = useRegions(profile);
  const regionOptions = regions && regions.length > 0 ? regions : [{ code: region, name: region }];

  const activeSet = new Set(state.activeServices);
  const ec2Query = usePricing(profile, region, 'ec2', activeSet.has('ec2'));
  const rdsQuery = usePricing(profile, region, 'rds', activeSet.has('rds'));
  const elasticacheQuery = usePricing(profile, region, 'elasticache', activeSet.has('elasticache'));
  const ecsQuery = usePricing(profile, region, 'ecs', activeSet.has('ecs'));
  const computeSpQuery = usePricing(profile, region, 'compute-sp', activeSet.has('compute-sp'));
  const ec2InstanceSpQuery = usePricing(
    profile,
    region,
    'ec2-instance-sp',
    activeSet.has('ec2-instance-sp'),
  );
  const databaseSpQuery = usePricing(profile, region, 'database-sp', activeSet.has('database-sp'));
  const ec2SpotQuery = usePricing(
    profile,
    region,
    'ec2-spot',
    activeSet.has('ec2-spot'),
    EC2_SPOT_STALE_TIME,
  );

  const ec2Refresh = useRefreshPricing(profile, region, 'ec2');
  const rdsRefresh = useRefreshPricing(profile, region, 'rds');
  const elasticacheRefresh = useRefreshPricing(profile, region, 'elasticache');
  const ecsRefresh = useRefreshPricing(profile, region, 'ecs');
  const computeSpRefresh = useRefreshPricing(profile, region, 'compute-sp');
  const ec2InstanceSpRefresh = useRefreshPricing(profile, region, 'ec2-instance-sp');
  const databaseSpRefresh = useRefreshPricing(profile, region, 'database-sp');
  const ec2SpotRefresh = useRefreshPricing(profile, region, 'ec2-spot');

  const queries: Record<PricingService, typeof ec2Query> = {
    ec2: ec2Query,
    rds: rdsQuery,
    elasticache: elasticacheQuery,
    ecs: ecsQuery,
    'compute-sp': computeSpQuery,
    'ec2-instance-sp': ec2InstanceSpQuery,
    'database-sp': databaseSpQuery,
    'ec2-spot': ec2SpotQuery,
  };
  const refreshes: Record<PricingService, typeof ec2Refresh> = {
    ec2: ec2Refresh,
    rds: rdsRefresh,
    elasticache: elasticacheRefresh,
    ecs: ecsRefresh,
    'compute-sp': computeSpRefresh,
    'ec2-instance-sp': ec2InstanceSpRefresh,
    'database-sp': databaseSpRefresh,
    'ec2-spot': ec2SpotRefresh,
  };

  // 現テーブルに存在しない rate_id の選択は破棄する (リージョン切替、または単価改定で
  // rate_id が変わった場合)。テーブルが更新されるたびに、その region/service の選択を
  // 現在の rate_id 集合へ突き合わせる。
  useEffect(() => {
    for (const service of PRICING_SERVICES) {
      const table = queries[service].data;
      if (!table) continue;
      dispatch({
        type: 'pruneStaleRates',
        region,
        service,
        validRateIds: table.rates.map((r) => r.rateId),
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    region,
    ec2Query.data,
    rdsQuery.data,
    elasticacheQuery.data,
    ecsQuery.data,
    computeSpQuery.data,
    ec2InstanceSpQuery.data,
    databaseSpQuery.data,
    ec2SpotQuery.data,
  ]);

  const rates: PriceTablesByService = useMemo(() => {
    const out: PriceTablesByService = {};
    for (const service of PRICING_SERVICES) {
      const table = queries[service].data;
      if (table) out[service] = table;
    }
    return out;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    ec2Query.data,
    rdsQuery.data,
    elasticacheQuery.data,
    ecsQuery.data,
    computeSpQuery.data,
    ec2InstanceSpQuery.data,
    databaseSpQuery.data,
    ec2SpotQuery.data,
  ]);

  const ssoExpired = useMemo(
    () =>
      PRICING_SERVICES.some((s) => {
        const err = queries[s].error;
        return err instanceof ApiError && err.code === 'SSO_TOKEN_EXPIRED';
      }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [
      ec2Query.error,
      rdsQuery.error,
      elasticacheQuery.error,
      ecsQuery.error,
      computeSpQuery.error,
      ec2InstanceSpQuery.error,
      databaseSpQuery.error,
      ec2SpotQuery.error,
    ],
  );

  const lastFetchedAt = useMemo(() => {
    let min: string | null = null;
    for (const service of state.activeServices) {
      if (!isPricingService(service)) continue;
      const table = queries[service].data;
      if (!table) continue;
      if (min === null || table.fetchedAt < min) min = table.fetchedAt;
    }
    return min;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    state.activeServices,
    ec2Query.data,
    rdsQuery.data,
    elasticacheQuery.data,
    ecsQuery.data,
    computeSpQuery.data,
    ec2InstanceSpQuery.data,
    databaseSpQuery.data,
    ec2SpotQuery.data,
  ]);

  const anyRefreshing = PRICING_SERVICES.some((s) => refreshes[s].isPending);
  const refreshAll = () => {
    for (const service of state.activeServices) {
      if (isPricingService(service)) refreshes[service].mutate();
    }
  };

  const selectionForRegion = state.selection[region] ?? {};

  return (
    <div className="main pr-panel">
      <PricingToolbar
        region={region}
        regionOptions={regionOptions}
        onRegionChange={onRegionChange}
        lastFetchedAt={lastFetchedAt}
        onRefreshAll={refreshAll}
        refreshing={anyRefreshing}
      />

      {ssoExpired && <SSOExpiredBanner profile={profile} />}

      <ServiceSelectorBar
        activeServices={state.activeServices}
        onToggle={(service) => dispatch({ type: 'toggleService', service })}
      />

      <div className="pr-body">
        <div className="pr-stack">
          {state.activeServices.length === 0 ? (
            <div className="pr-stack-empty">
              上のサービス選択から表示するサービスを選んでください。
            </div>
          ) : (
            state.activeServices.filter(isPricingService).map((service) => {
              const q = queries[service];
              const r = refreshes[service];
              return (
                <ServiceCard
                  key={service}
                  service={service}
                  table={q.data}
                  isLoading={q.isLoading}
                  error={q.error}
                  onRetry={() => void q.refetch()}
                  collapsed={!!state.collapsed[service]}
                  onToggleCollapsed={() => dispatch({ type: 'toggleCollapsed', service })}
                  selection={selectionForRegion[service] ?? {}}
                  onToggleRate={(rateId) =>
                    dispatch({ type: 'toggleRate', region, service, rateId })
                  }
                  onRefresh={() => r.mutate()}
                  refreshing={r.isPending}
                />
              );
            })
          )}
        </div>

        <Estimator
          selection={selectionForRegion}
          rates={rates}
          onSetQty={(service, rateId, qty) =>
            dispatch({ type: 'setQty', region, service, rateId, qty })
          }
          onToggleRate={(service, rateId) =>
            dispatch({ type: 'toggleRate', region, service, rateId })
          }
          onClearAll={() => dispatch({ type: 'clearSelection', region })}
        />
      </div>
    </div>
  );
}
