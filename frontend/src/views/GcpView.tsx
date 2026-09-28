// GCP 統合ビュー: GcpSidebar + service に応じた ServicePanel / BigQueryView 埋め込み。
// AccountView のパターンを踏襲するが、リージョン切替や Cost Explorer 相当はサービスごとに
// 挙動が違うため、サービス単位で個別の分岐を書く。
// 分割表示 (issue 0175) ではペインごとに GcpServicePane を 1 つ描画し、選択リソース
// (Drawer で開く対象) もペインごとに持つ。
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
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
import { useTweaks } from '../hooks/useTweaks';
import type { SplitPanesState } from '../lib/splitPanes';
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
import { Icons } from '../components/icons/Icons';
import { ErrorBanner } from '../components/ErrorBanner';
import { BigQueryView } from './nonaws/BigQueryView';
import { CloudLoggingView } from './nonaws/CloudLoggingView';

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
  // 分割中は Drawer をペインの中に収め、ESC はフォーカス中のペインだけが受け取る
  // (未指定なら Drawer の既定値 = 現状の position: fixed と ESC で閉じる)。
  contained?: boolean;
  closeOnEscape?: boolean;
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
  contained,
  closeOnEscape,
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
  // Layout = workbench では上段 (toolbar / facets) を 1 行の .panel-bar にまとめる (issue 0212)
  const workbench = useTweaks().tweaks.layout === 'workbench';
  const title = (
    <div className="title">
      <h1>{svcMeta?.name}</h1>
      <span className="subtitle">{svcMeta?.sub.toLowerCase()}</span>
    </div>
  );
  const facets = <FacetBar rows={rows} filters={filters} setFilters={setFilters} />;

  return (
    <div className="main">
      {workbench ? (
        <div className="panel-bar">
          {title}
          {facets}
        </div>
      ) : (
        <div className="toolbar">{title}</div>
      )}

      {Boolean(error) && <ErrorBanner error={error} />}

      {!workbench && facets}

      {/* 表と Drawer を .main-row で包む (issue 0213)。standard では縦積み (overlay の Drawer は
          流れの外)、workbench では docked の Drawer が右 / 下に列として並ぶ。 */}
      <div className={`main-row${drawerPos === 'bottom' ? ' drawer-bottom' : ''}`}>
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
          contained={contained}
          closeOnEscape={closeOnEscape}
          onClose={() => onSelectId(null)}
        />
      </div>
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
  contained?: boolean;
  closeOnEscape?: boolean;
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
  contained,
  closeOnEscape,
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
      contained={contained}
      closeOnEscape={closeOnEscape}
    />
  );
}

interface GcpIAMPanelProps {
  projectId: string;
  drawerPos: DrawerPos;
  selectedId: string | null;
  onSelectId: (id: string | null) => void;
  contained?: boolean;
  closeOnEscape?: boolean;
}

// IAM パネル専用: バインディング (1 メンバー x 1 ロール) をメンバー単位に集約してから表示する。
// 同じメンバーに複数ロールが付いている場合、一覧では 1 行にまとめてロールを列挙する。
function GcpIAMPanel({
  projectId,
  drawerPos,
  selectedId,
  onSelectId,
  contained,
  closeOnEscape,
}: GcpIAMPanelProps) {
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
      contained={contained}
      closeOnEscape={closeOnEscape}
    />
  );
}

interface GcpServicePaneProps {
  service: string;
  projectId: string;
  drawerPos: DrawerPos;
  // 分割中だけ渡す (設計判断 5)。1 ペインでは未指定のまま Drawer の既定値に任せる。
  contained?: boolean;
  closeOnEscape?: boolean;
}

// 1 ペイン分の GCP サービス表示。Drawer で開く選択リソースはペインごとに持つため、
// コンポーネントのインスタンスが分かれることでフィルタ (GcpRowsPanel の filters) と
// Drawer のタブ (Drawer の tab) も自然にペインごとになる。
function GcpServicePane({
  service,
  projectId,
  drawerPos,
  contained,
  closeOnEscape,
}: GcpServicePaneProps) {
  const [selectedId, setSelectedId] = useState<string | null>(null);

  // サービス切替時は選択状態をリセット
  useEffect(() => {
    setSelectedId(null);
  }, [service]);

  return (
    <>
      {service === 'cloudrun' && (
        <GcpServicePanel<CloudRunResourceRaw, CloudRunResourceRow>
          service="cloudrun"
          projectId={projectId}
          normalizer={cloudRunResourceFromRaw}
          columns={cloudRunColumns}
          overviewRows={cloudRunOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
          contained={contained}
          closeOnEscape={closeOnEscape}
        />
      )}
      {service === 'gcs' && (
        <GcpServicePanel<GcsBucketRaw, GcsBucketRow>
          service="gcs"
          projectId={projectId}
          normalizer={gcsBucketFromRaw}
          columns={gcsBucketColumns}
          overviewRows={gcsBucketOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
          contained={contained}
          closeOnEscape={closeOnEscape}
        />
      )}
      {service === 'gcpiam' && (
        <GcpIAMPanel
          projectId={projectId}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
          contained={contained}
          closeOnEscape={closeOnEscape}
        />
      )}
      {service === 'gcpserviceaccounts' && (
        <GcpServicePanel<ServiceAccountRaw, ServiceAccountRow>
          service="gcpserviceaccounts"
          projectId={projectId}
          normalizer={serviceAccountFromRaw}
          columns={serviceAccountColumns}
          overviewRows={serviceAccountOverviewRows}
          drawerPos={drawerPos}
          selectedId={selectedId}
          onSelectId={setSelectedId}
          contained={contained}
          closeOnEscape={closeOnEscape}
        />
      )}
      {service === 'bigquery' && <BigQueryView projectId={projectId} />}
      {service === 'cloudlogging' && <CloudLoggingView projectId={projectId} />}
    </>
  );
}

export interface GcpViewProps {
  activeProject: string;
  projects: GcpProject[];
  // 分割表示のペイン状態 (services の長さ 1 = 分割なし / 2 = 分割中)
  panes: SplitPanesState;
  onSelectService: (service: string) => void;
  onFocusPane: (index: number) => void;
  onClosePane: (index: number) => void;
  drawerPos: DrawerPos;
  onSidebarWidthChange?: (width: number) => void;
  // workbench レイアウトの rail (issue 0211)。standard では渡さない。
  sidebarCollapsed?: boolean;
  onToggleSidebar?: () => void;
}

export function GcpView({
  activeProject,
  projects,
  panes,
  onSelectService,
  onFocusPane,
  onClosePane,
  drawerPos,
  onSidebarWidthChange,
  sidebarCollapsed = false,
  onToggleSidebar,
}: GcpViewProps) {
  const { t } = useTranslation('app');
  const split = panes.services.length === 2;
  // サイドバーが強調するサービスはフォーカス中のペインのもの (サービス未選択なら null)
  const focusedService = panes.services[panes.focused] ?? null;

  return (
    <div className={`body${split ? ' split' : ''}${sidebarCollapsed ? ' rail' : ''}`}>
      <GcpSidebar
        project={activeProject}
        projects={projects}
        onWidthChange={onSidebarWidthChange}
        activeService={focusedService}
        onService={onSelectService}
        paneServices={split ? panes.services : undefined}
        collapsed={sidebarCollapsed}
        onToggleCollapsed={onToggleSidebar}
      />

      {panes.services.map((service, index) => {
        const focused = split && index === panes.focused;
        return (
          <div
            key={panes.ids[index]}
            // 分割していないときは display: contents のラッパーとして置き、レイアウトに
            // 影響させない (1 ペインでも包む理由は AccountView と同じ)。
            className={`pane${split ? '' : ' single'}${focused ? ' focused' : ''}`}
            // ペイン内のどこを操作してもフォーカスが移るようにする (1 ペインでは常に 0)
            onPointerDownCapture={split ? () => onFocusPane(index) : undefined}
          >
            {/* 分割中だけペイン番号と閉じるボタンを出す。分割していないときも位置を空けて
                後ろのペインの中身の位置を変えない。 */}
            {split && (
              <div className="pane-bar">
                <span className="pane-index" aria-label={t('panes.label', { index: index + 1 })}>
                  {index + 1}
                </span>
                <button
                  className="pane-close"
                  aria-label={t('panes.close', { index: index + 1 })}
                  onClick={() => onClosePane(index)}
                >
                  <Icons.x size={12} />
                </button>
              </div>
            )}
            {service ? (
              <GcpServicePane
                service={service}
                projectId={activeProject}
                drawerPos={drawerPos}
                contained={split || undefined}
                closeOnEscape={split ? focused : undefined}
              />
            ) : (
              <div className="main pane-empty">{t('panes.empty')}</div>
            )}
          </div>
        );
      })}
    </div>
  );
}
