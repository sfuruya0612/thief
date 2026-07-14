// app.jsx AccountView の移植
// サービスごとに useResources<TRaw,TRow> の型引数が異なるため、汎用 ServicePanel を用意し
// activeService に応じて 15 分岐で呼び分ける (各分岐は normalizer/columns/overviewRows を渡すだけ)
import { useEffect, useMemo, useState } from 'react';
import {
  apigwFromRaw,
  cacheFromRaw,
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
import { Drawer } from '../components/Drawer/Drawer';
import { useCost, useResources } from '../api/queries';
import { SERVICES } from '../lib/serviceMeta';
import type { BaseRow, DrawerPos, Profile } from '../types/common';
import { ApiError } from '../types/common';
import { Sidebar } from '../components/Sidebar';
import { StatsRow } from '../components/StatsRow';
import { FacetBar, type Filters } from '../components/FacetBar';
import { DataTable } from '../components/DataTable';
import { SSOExpiredBanner } from '../components/SSOExpiredBanner';
import { ErrorBanner } from '../components/ErrorBanner';
import { CostExplorerPanel } from './CostExplorerPanel';

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
}

// 汎用サービスパネル: useResources 呼び出し + Stats/Facet/Table/Drawer 描画
// フッター (StatusBar) は全ビュー共通化のため App.tsx ルートに移管した (課題 4-2)
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
}: ServicePanelProps<TRaw, TRow>) {
  const { data, isLoading, error } = useResources<TRaw, TRow>(service, profile, region, normalizer);
  const { data: cost } = useCost(profile, region);
  const [filters, setFilters] = useState<Filters>({});
  const [search, setSearch] = useState('');

  const ssoExpired = error instanceof ApiError && error.code === 'SSO_TOKEN_EXPIRED';
  const allResources = data ?? [];
  const selected = allResources.find((r) => r.id === selectedId) ?? null;

  const filtered = useMemo(() => {
    return allResources.filter((r) => {
      if (search) {
        const q = search.toLowerCase();
        const hay = `${r.name} ${r.id}`.toLowerCase();
        if (!hay.includes(q)) return false;
      }
      if (filters.Env?.length && !filters.Env.includes(r.tags?.Env ?? '')) return false;
      if (filters.state?.length && !filters.state.includes(r.state ?? '')) return false;
      if (filters.region?.length && !filters.region.includes(r.region ?? '')) return false;
      if (filters.Team?.length) {
        const team = r.tags?.Team ?? r.tags?.Owner ?? '';
        if (!filters.Team.includes(team)) return false;
      }
      return true;
    });
  }, [allResources, filters, search]);

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

      <FacetBar
        rows={allResources}
        filters={filters}
        setFilters={setFilters}
        search={search}
        setSearch={setSearch}
      />

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
  onProfileChange: (name: string) => void;
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
  onProfileChange,
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
        onProfileChange={onProfileChange}
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
      {activeService === 'costexplorer' && <CostExplorerPanel profile={profile} region={region} />}
    </div>
  );
}
