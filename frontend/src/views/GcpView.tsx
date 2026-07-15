// GCP 統合ビュー: GcpSidebar + activeService に応じた ServicePanel / BigQueryView 埋め込み。
// AccountView のパターンを踏襲するが、リージョン切替や Cost Explorer 相当はサービスごとに
// 挙動が違うため、サービス単位で個別の分岐を書く。
import { useEffect, useMemo, useState } from 'react';
import { useGcpResources } from '../api/queries';
import {
  cloudRunResourceFromRaw,
  gcsBucketFromRaw,
  groupIAMBindingsByMember,
  iamBindingFromRaw,
  serviceAccountFromRaw,
} from '../lib/normalizeGcp';
import {
  cloudRunColumns,
  gcsBucketColumns,
  iamMemberColumns,
  serviceAccountColumns,
} from '../components/tables/gcpColumns';
import {
  cloudRunOverviewRows,
  gcsBucketOverviewRows,
  iamMemberOverviewRows,
  serviceAccountOverviewRows,
  type OverviewEntry,
} from '../components/Drawer/overviewRows';
import type { ColumnDef } from '../components/tables/columns';
import { GCP_SERVICES } from '../lib/serviceMeta';
import type { BaseRow, DrawerPos } from '../types/common';
import type {
  CloudRunResourceRaw,
  CloudRunResourceRow,
  GcpProject,
  GcsBucketRaw,
  GcsBucketRow,
  IAMBindingRaw,
  IAMBindingRow,
  IAMMemberRow,
  ServiceAccountRaw,
  ServiceAccountRow,
} from '../types/gcp';
import { GcpSidebar } from './GcpSidebar';
import { FacetBar, type Filters } from '../components/FacetBar';
import { DataTable } from '../components/DataTable';
import { Drawer } from '../components/Drawer/Drawer';
import { ErrorBanner } from '../components/ErrorBanner';
import { BigQueryView } from './nonaws/BigQueryView';

interface GcpRowsPanelProps<TRow extends BaseRow> {
  service: string;
  projectId: string;
  rows: TRow[];
  isLoading: boolean;
  error?: unknown;
  columns: ColumnDef<TRow>[];
  overviewRows: (row: TRow) => OverviewEntry[];
  drawerPos: DrawerPos;
  selectedId: string | null;
  onSelectId: (id: string | null) => void;
}

// GCP サービスパネルの表示部分 (Facet/Table/Drawer)。データ取得は呼び出し側の責務とし、
// 取得後に加工 (IAM のメンバー単位集約等) が必要なサービスでも再利用できるようにする。
function GcpRowsPanel<TRow extends BaseRow>({
  service,
  projectId,
  rows,
  isLoading,
  error,
  columns,
  overviewRows,
  drawerPos,
  selectedId,
  onSelectId,
}: GcpRowsPanelProps<TRow>) {
  const [filters, setFilters] = useState<Filters>({});

  const selected = rows.find((r) => r.id === selectedId) ?? null;

  const filtered = useMemo(() => {
    return rows.filter((r) => {
      if (filters.region?.length && !filters.region.includes(r.region ?? '')) return false;
      if (filters.state?.length && !filters.state.includes(r.state ?? '')) return false;
      return true;
    });
  }, [rows, filters]);

  const svcMeta = GCP_SERVICES.find((s) => s.key === service);

  return (
    <div className="main">
      <div className="toolbar">
        <div className="title">
          <h1>{svcMeta?.name}</h1>
          <span className="subtitle">{svcMeta?.sub.toLowerCase()}</span>
        </div>
      </div>

      {Boolean(error) && <ErrorBanner error={error} />}

      <FacetBar rows={rows} filters={filters} setFilters={setFilters} />

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
        profile={projectId}
        region={selected?.region ?? ''}
        position={drawerPos}
        overviewRows={selected ? overviewRows(selected) : []}
        onClose={() => onSelectId(null)}
      />
    </div>
  );
}

interface GcpServicePanelProps<TRaw, TRow extends BaseRow> {
  service: string;
  projectId: string;
  normalizer: (raw: TRaw) => TRow;
  columns: ColumnDef<TRow>[];
  overviewRows: (row: TRow) => OverviewEntry[];
  drawerPos: DrawerPos;
  selectedId: string | null;
  onSelectId: (id: string | null) => void;
}

// 汎用 GCP サービスパネル: useGcpResources 呼び出し + GcpRowsPanel 描画
function GcpServicePanel<TRaw, TRow extends BaseRow>({
  service,
  projectId,
  normalizer,
  columns,
  overviewRows,
  drawerPos,
  selectedId,
  onSelectId,
}: GcpServicePanelProps<TRaw, TRow>) {
  const { data, isLoading, error } = useGcpResources<TRaw, TRow>(service, projectId, normalizer);

  return (
    <GcpRowsPanel<TRow>
      service={service}
      projectId={projectId}
      rows={data ?? []}
      isLoading={isLoading}
      error={error}
      columns={columns}
      overviewRows={overviewRows}
      drawerPos={drawerPos}
      selectedId={selectedId}
      onSelectId={onSelectId}
    />
  );
}

interface GcpIAMPanelProps {
  projectId: string;
  drawerPos: DrawerPos;
  selectedId: string | null;
  onSelectId: (id: string | null) => void;
}

// IAM パネル専用: バインディング (1 メンバー x 1 ロール) をメンバー単位に集約してから表示する。
// 同じメンバーに複数ロールが付いている場合、一覧では 1 行にまとめてロールを列挙する。
function GcpIAMPanel({ projectId, drawerPos, selectedId, onSelectId }: GcpIAMPanelProps) {
  const { data, isLoading, error } = useGcpResources<IAMBindingRaw, IAMBindingRow>(
    'gcpiam',
    projectId,
    iamBindingFromRaw,
  );

  const memberRows = useMemo(() => groupIAMBindingsByMember(data ?? []), [data]);

  return (
    <GcpRowsPanel<IAMMemberRow>
      service="gcpiam"
      projectId={projectId}
      rows={memberRows}
      isLoading={isLoading}
      error={error}
      columns={iamMemberColumns}
      overviewRows={iamMemberOverviewRows}
      drawerPos={drawerPos}
      selectedId={selectedId}
      onSelectId={onSelectId}
    />
  );
}

export interface GcpViewProps {
  activeProject: string;
  projects: GcpProject[];
  onProjectChange: (id: string) => void;
  activeService: string;
  onServiceChange: (service: string) => void;
  drawerPos: DrawerPos;
  onSidebarWidthChange?: (width: number) => void;
}

export function GcpView({
  activeProject,
  projects,
  onProjectChange,
  activeService,
  onServiceChange,
  drawerPos,
  onSidebarWidthChange,
}: GcpViewProps) {
  const [selectedId, setSelectedId] = useState<string | null>(null);

  // サービス切替時は選択状態をリセット
  useEffect(() => {
    setSelectedId(null);
  }, [activeService]);

  return (
    <div className="body">
      <GcpSidebar
        project={activeProject}
        projects={projects}
        onProjectChange={onProjectChange}
        onWidthChange={onSidebarWidthChange}
        activeService={activeService}
        onService={onServiceChange}
      />

      {activeService === 'cloudrun' && (
        <GcpServicePanel<CloudRunResourceRaw, CloudRunResourceRow>
          service="cloudrun"
          projectId={activeProject}
          normalizer={cloudRunResourceFromRaw}
          columns={cloudRunColumns}
          overviewRows={cloudRunOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'gcs' && (
        <GcpServicePanel<GcsBucketRaw, GcsBucketRow>
          service="gcs"
          projectId={activeProject}
          normalizer={gcsBucketFromRaw}
          columns={gcsBucketColumns}
          overviewRows={gcsBucketOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'gcpiam' && (
        <GcpIAMPanel
          projectId={activeProject}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'gcpserviceaccounts' && (
        <GcpServicePanel<ServiceAccountRaw, ServiceAccountRow>
          service="gcpserviceaccounts"
          projectId={activeProject}
          normalizer={serviceAccountFromRaw}
          columns={serviceAccountColumns}
          overviewRows={serviceAccountOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
        />
      )}
      {activeService === 'bigquery' && <BigQueryView projectId={activeProject} />}
    </div>
  );
}
