// GCP ビュー用サイドバー。Sidebar.tsx を GCP 向けに焼き直したもの。
// profile-card 相当は「アクティブセッション」カードのみ (region は Cloud Run/GCS が
// 個別に location を持つため、AWS のような全体 region 切替は持たない)。
// プロジェクトの切替はセッションタブ (GcpSessionTabs) が担い、一覧の refresh
// ボタンもタブ追加ピッカーへ移設した。
// 件数バッジは選択中サービスのみ即時取得され、他はキャッシュを読むだけの観測用クエリで表示する
// (Sidebar.tsx と同じ方針)。
import { useQuery } from '@tanstack/react-query';
import { GCP_SERVICES } from '../lib/serviceMeta';
import { startSidebarResize } from '../lib/sidebarResize';
import type { GcpProject } from '../types/gcp';
import { Icons } from '../components/icons/Icons';
import { GcpIcons } from '../components/icons/GcpIcons';
import { GcpActiveSessionCard } from '../components/session/GcpActiveSessionCard';

interface SidebarSection {
  label: string;
  services: string[];
}

const SECTIONS: SidebarSection[] = [
  { label: 'Compute', services: ['cloudrun'] },
  { label: 'Data', services: ['bigquery', 'gcs'] },
  { label: 'Security', services: ['gcpiam', 'gcpserviceaccounts'] },
];

export interface GcpSidebarProps {
  project: string;
  projects: GcpProject[];
  activeService: string;
  onService: (svc: string) => void;
  onWidthChange?: (width: number) => void;
}

export function GcpSidebar({
  project,
  projects,
  activeService,
  onService,
  onWidthChange,
}: GcpSidebarProps) {
  return (
    <aside className="sidebar">
      <div className="profile-card">
        <div className="profile-card-field">
          <span className="label">アクティブセッション</span>
          <GcpActiveSessionCard project={project} projects={projects} />
        </div>
      </div>

      {SECTIONS.map((section) => (
        <div key={section.label}>
          <div className="section-label">{section.label}</div>
          {section.services.map((svc) => (
            <SvcItem
              key={svc}
              svc={svc}
              project={project}
              active={activeService}
              onService={onService}
            />
          ))}
        </div>
      ))}

      <div
        className="sidebar-resizer"
        onPointerDown={startSidebarResize(onWidthChange)}
        title="Drag to resize"
      />
    </aside>
  );
}

interface SvcItemProps {
  svc: string;
  project: string;
  active: string;
  onService: (svc: string) => void;
}

function SvcItem({ svc, project, active, onService }: SvcItemProps) {
  const meta = GCP_SERVICES.find((s) => s.key === svc);
  // bigquery は GCP_SERVICE_TO_PATH にパスを持たず useGcpResources 経由で fetch されないため、
  // 件数バッジは他所からのキャッシュ観測が期待できない。ハイフンのみ表示にする。
  const isObservable = svc !== 'bigquery';
  const { data } = useQuery<unknown[]>({
    queryKey: ['gcp', svc, project],
    enabled: false,
  });
  const count = isObservable && data ? data.length : '-';
  const IconEl = GcpIcons[svc] ?? Icons[svc];

  return (
    <div className={`nav-item ${active === svc ? 'active' : ''}`} onClick={() => onService(svc)}>
      <span className="svc-icon">{IconEl ? <IconEl size={16} /> : null}</span>
      <span>{meta?.name}</span>
      <span className="count">{count}</span>
    </div>
  );
}
