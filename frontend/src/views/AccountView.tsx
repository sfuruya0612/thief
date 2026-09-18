// app.jsx AccountView の移植
// サービスごとに useResources<TRaw,TRow> の型引数が異なるため、汎用 ServicePanel を用意し
// activeService に応じて 15 分岐で呼び分ける (各分岐は normalizer/columns/overviewRows を渡すだけ)
import { useEffect, useMemo, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  apigwFromRaw,
  cacheFromRaw,
  cfnFromRaw,
  cloudfrontFromRaw,
  dynamoFromRaw,
  ec2FromRaw,
  ecrFromRaw,
  ecsFromRaw,
  elbFromRaw,
  iamFromRaw,
  kinesisFromRaw,
  lambdaFromRaw,
  natgwFromRaw,
  rdsFromRaw,
  s3FromRaw,
  secretFromRaw,
  sqsFromRaw,
  ssmFromRaw,
  wafFromRaw,
} from '../lib/normalize';
import {
  apigwColumns,
  cacheColumns,
  cfnColumns,
  cloudfrontColumns,
  dynamoColumns,
  ec2Columns,
  ecrColumns,
  ecsColumns,
  elbColumns,
  iamColumns,
  kinesisColumns,
  lambdaColumns,
  natgwColumns,
  rdsColumns,
  s3Columns,
  secretColumns,
  sqsColumns,
  ssmColumns,
  wafColumns,
  type ColumnDef,
} from '../components/tables/columns';
import {
  apigwOverviewRows,
  cacheOverviewRows,
  cfnOverviewRows,
  cloudfrontOverviewRows,
  dynamoOverviewRows,
  ec2OverviewRows,
  ecrOverviewRows,
  ecsOverviewRows,
  elbOverviewRows,
  iamOverviewRows,
  kinesisOverviewRows,
  lambdaOverviewRows,
  natgwOverviewRows,
  rdsOverviewRows,
  s3OverviewRows,
  secretOverviewRows,
  sqsOverviewRows,
  ssmOverviewRows,
  wafOverviewRows,
  type OverviewEntry,
} from '../components/Drawer/overviewRows';
import { ResourceCountChart } from '../components/charts/ResourceCountChart';
import { Drawer } from '../components/Drawer/Drawer';
import { useCost, useResources } from '../api/queries';
import { SERVICES } from '../lib/serviceMeta';
import type { BaseRow, DrawerPos, Profile } from '../types/common';
import { isSSOExpiredError } from '../lib/ssoError';
import { Sidebar } from '../components/Sidebar';
import { StatsRow } from '../components/StatsRow';
import { FacetBar, type Filters } from '../components/FacetBar';
import { DataTable } from '../components/DataTable';
import { SSOExpiredBanner } from '../components/SSOExpiredBanner';
import { ErrorBanner } from '../components/ErrorBanner';
import { AthenaView } from './AthenaView';
import { CloudWatchLogsView } from './CloudWatchLogsView';
import { CostExplorerPanel } from './CostExplorerPanel';
import { PricingPanel } from './PricingPanel';

interface ServicePanelProps<TRaw, TRow extends BaseRow> {
  service: string;
  profile: string;
  region: string;
  normalizer: (raw: TRaw, region: string) => TRow;
  columns: ColumnDef<TRow>[];
  overviewRows: (row: TRow) => OverviewEntry[];
  drawerPos: DrawerPos;
  selectedId: string | null;
  onSelectId: (id: string | null) => void;
  // 台数の推移グラフの見出し。渡したサービスだけグラフを表示する
  // (時系列エンドポイントを持つ ec2 と ecs のみ)。
  countChartTitle?: string;
  // 台数の推移グラフの右下に出す注記。値の対象範囲の補足に使う。
  countChartCaption?: string;
}

// 汎用サービスパネル: useResources 呼び出し + Stats/Facet/Table/Drawer 描画
function ServicePanel<TRaw, TRow extends BaseRow>({
  service,
  profile,
  region,
  normalizer,
  columns,
  overviewRows,
  drawerPos,
  selectedId,
  onSelectId,
  countChartTitle,
  countChartCaption,
}: ServicePanelProps<TRaw, TRow>) {
  const { data, isLoading, error, dataUpdatedAt } = useResources<TRaw, TRow>(
    service,
    profile,
    region,
    normalizer,
  );
  const { data: cost } = useCost(profile, region);
  const [filters, setFilters] = useState<Filters>({});
  const queryClient = useQueryClient();

  const ssoExpired = isSSOExpiredError(error);

  // SSO 期限切れを検知したらプロファイル一覧も再取得し、セッションタブの
  // ピッカーやアクティブセッションカードのバッジを「期限切れ」へ追随させる
  // (一覧の staleTime 5 分を待たない)。
  useEffect(() => {
    if (!ssoExpired) return;
    void queryClient.invalidateQueries({ queryKey: ['aws', 'profiles'] });
  }, [ssoExpired, queryClient]);

  // 一覧の取得が成功したら、同じ profile と region の時系列を取り直す。
  // backend が台数を記録するのは一覧を実際に取得したときだけであり、Refresh では
  // 時系列の再取得が一覧の取得完了より先に終わるため、追随させないとその回の記録が
  // グラフに入らない (期間ごとのキーをまとめて無効化するので range は指定しない)。
  useEffect(() => {
    if (!dataUpdatedAt) return;
    void queryClient.invalidateQueries({
      queryKey: ['aws', service, profile, region, 'timeseries'],
    });
  }, [dataUpdatedAt, queryClient, service, profile, region]);

  const allResources = data ?? [];
  const selected = allResources.find((r) => r.id === selectedId) ?? null;

  const filtered = useMemo(() => {
    return allResources.filter((r) => {
      if (filters.Env?.length && !filters.Env.includes(r.tags?.Env ?? '')) return false;
      if (filters.state?.length && !filters.state.includes(r.state ?? '')) return false;
      if (filters.region?.length && !filters.region.includes(r.region ?? '')) return false;
      if (filters.Team?.length) {
        const team = r.tags?.Team ?? r.tags?.Owner ?? '';
        if (!filters.Team.includes(team)) return false;
      }
      return true;
    });
  }, [allResources, filters]);

  const svcMeta = SERVICES.find((s) => s.key === service);

  return (
    <div className="main">
      <div className="toolbar">
        <div className="title">
          <h1>{svcMeta?.name}</h1>
          <span className="subtitle">{svcMeta?.sub.toLowerCase()}</span>
        </div>
      </div>

      {ssoExpired && <SSOExpiredBanner profile={profile} />}
      {!ssoExpired && error && <ErrorBanner error={error} />}

      <StatsRow resources={allResources} service={service} cost={cost ?? []} />

      {countChartTitle && (
        <ResourceCountChart
          service={service}
          profile={profile}
          region={region}
          title={countChartTitle}
          caption={countChartCaption}
        />
      )}

      <FacetBar rows={allResources} filters={filters} setFilters={setFilters} />

      <DataTable
        rows={filtered}
        columns={columns}
        onSelect={(r) => onSelectId(r.id)}
        selectedId={selectedId}
        isLoading={isLoading}
      />

      <Drawer
        resource={selected}
        service={service}
        profile={profile}
        region={region}
        position={drawerPos}
        overviewRows={selected ? overviewRows(selected) : []}
        onClose={() => onSelectId(null)}
      />
    </div>
  );
}

export interface AccountViewProps {
  profile: string;
  region: string;
  profiles: Profile[];
  onRegionChange: (region: string) => void;
  activeService: string;
  onServiceChange: (service: string) => void;
  drawerPos: DrawerPos;
  onSidebarWidthChange?: (width: number) => void;
}

export function AccountView({
  profile,
  region,
  profiles,
  onRegionChange,
  activeService,
  onServiceChange,
  drawerPos,
  onSidebarWidthChange,
}: AccountViewProps) {
  const [selectedId, setSelectedId] = useState<string | null>(null);

  // サービス切替時は選択状態をリセットする (mock の setService 相当)
  useEffect(() => {
    setSelectedId(null);
  }, [activeService]);

  return (
    <div className="body">
      <Sidebar
        profile={profile}
        region={region}
        profiles={profiles}
        onRegionChange={onRegionChange}
        onWidthChange={onSidebarWidthChange}
        activeService={activeService}
        onService={onServiceChange}
      />

      {activeService === 'ec2' && (
        <ServicePanel
          service="ec2"
          profile={profile}
          region={region}
          normalizer={ec2FromRaw}
          columns={ec2Columns}
          overviewRows={ec2OverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
          countChartTitle="In-service instances"
          countChartCaption="Only instances in an Auto Scaling group"
        />
      )}
      {activeService === 'ecr' && (
        <ServicePanel
          service="ecr"
          profile={profile}
          region={region}
          normalizer={ecrFromRaw}
          columns={ecrColumns}
          overviewRows={ecrOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'cfn' && (
        <ServicePanel
          service="cfn"
          profile={profile}
          region={region}
          normalizer={cfnFromRaw}
          columns={cfnColumns}
          overviewRows={cfnOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'rds' && (
        <ServicePanel
          service="rds"
          profile={profile}
          region={region}
          normalizer={rdsFromRaw}
          columns={rdsColumns}
          overviewRows={rdsOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'dynamo' && (
        <ServicePanel
          service="dynamo"
          profile={profile}
          region={region}
          normalizer={dynamoFromRaw}
          columns={dynamoColumns}
          overviewRows={dynamoOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'cache' && (
        <ServicePanel
          service="cache"
          profile={profile}
          region={region}
          normalizer={cacheFromRaw}
          columns={cacheColumns}
          overviewRows={cacheOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'lambda' && (
        <ServicePanel
          service="lambda"
          profile={profile}
          region={region}
          normalizer={lambdaFromRaw}
          columns={lambdaColumns}
          overviewRows={lambdaOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'ecs' && (
        <ServicePanel
          service="ecs"
          profile={profile}
          region={region}
          normalizer={ecsFromRaw}
          columns={ecsColumns}
          overviewRows={ecsOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
          countChartTitle="Tasks per cluster"
        />
      )}
      {activeService === 's3' && (
        <ServicePanel
          service="s3"
          profile={profile}
          region={region}
          normalizer={s3FromRaw}
          columns={s3Columns}
          overviewRows={s3OverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'iam' && (
        <ServicePanel
          service="iam"
          profile={profile}
          region={region}
          normalizer={iamFromRaw}
          columns={iamColumns}
          overviewRows={iamOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'elb' && (
        <ServicePanel
          service="elb"
          profile={profile}
          region={region}
          normalizer={elbFromRaw}
          columns={elbColumns}
          overviewRows={elbOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'cloudfront' && (
        <ServicePanel
          service="cloudfront"
          profile={profile}
          region={region}
          normalizer={cloudfrontFromRaw}
          columns={cloudfrontColumns}
          overviewRows={cloudfrontOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'apigw' && (
        <ServicePanel
          service="apigw"
          profile={profile}
          region={region}
          normalizer={apigwFromRaw}
          columns={apigwColumns}
          overviewRows={apigwOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'natgw' && (
        <ServicePanel
          service="natgw"
          profile={profile}
          region={region}
          normalizer={natgwFromRaw}
          columns={natgwColumns}
          overviewRows={natgwOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'sqs' && (
        <ServicePanel
          service="sqs"
          profile={profile}
          region={region}
          normalizer={sqsFromRaw}
          columns={sqsColumns}
          overviewRows={sqsOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'kinesis' && (
        <ServicePanel
          service="kinesis"
          profile={profile}
          region={region}
          normalizer={kinesisFromRaw}
          columns={kinesisColumns}
          overviewRows={kinesisOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'waf' && (
        <ServicePanel
          service="waf"
          profile={profile}
          region={region}
          normalizer={wafFromRaw}
          columns={wafColumns}
          overviewRows={wafOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'ssm' && (
        <ServicePanel
          service="ssm"
          profile={profile}
          region={region}
          normalizer={ssmFromRaw}
          columns={ssmColumns}
          overviewRows={ssmOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'secrets' && (
        <ServicePanel
          service="secrets"
          profile={profile}
          region={region}
          normalizer={secretFromRaw}
          columns={secretColumns}
          overviewRows={secretOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'athena' && <AthenaView profile={profile} region={region} />}
      {activeService === 'cloudwatchlogs' && (
        <CloudWatchLogsView profile={profile} region={region} />
      )}
      {/* key= でリージョン切り替え時に再マウントし、絞り込み state (フィルタ / 日付レンジ /
          granularity 等) を初期値へ戻す。プロファイル切り替えは App.tsx の key={activeProfile}
          による AccountView ごとの再マウントで既に初期化される。 */}
      {activeService === 'costexplorer' && (
        <CostExplorerPanel key={region} profile={profile} region={region} />
      )}
      {activeService === 'pricing' && (
        <PricingPanel profile={profile} region={region} onRegionChange={onRegionChange} />
      )}
    </div>
  );
}
