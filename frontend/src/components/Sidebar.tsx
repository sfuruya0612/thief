// sidebar.jsx の移植
// 件数バッジは「選択中のサービスのみ即時取得、他はクリックするまで取得しない」方針を守るため、
// useCachedQueryData でクエリキャッシュを読み取るだけにする。useQuery に queryFn: skipToken を
// 渡すと同一 queryKey のクエリの options を skipToken で上書きし、invalidateQueries の再取得が
// Missing queryFn になるため (issue 0187)、QueryObserver を作らない。
// 本体の useResources (ServicePanel 側) が同じ queryKey で fetch した際、
// react-query のキャッシュ共有によりここにも値が反映される。
import { useTranslation } from 'react-i18next';
import { useCachedQueryData } from '../hooks/useCachedQueryData';
import { AwsIcons } from './icons/AwsIcons';
import { Icons } from './icons/Icons';
import { AWS_SERVICE_GROUPS, SERVICES } from '../lib/serviceMeta';
import { useRegions } from '../api/queries';
import { startSidebarResize } from '../lib/sidebarResize';
import type { Profile } from '../types/common';
import { AwsActiveSessionCard } from './session/AwsActiveSessionCard';
import { SidebarToggle } from './SidebarToggle';

// カテゴリ定義 (AWS_SERVICE_GROUPS) の表示順に、各サービスの group から所属サービスを導出する。
// 該当サービスが 1 つもないカテゴリは表示しない。
const SECTIONS = AWS_SERVICE_GROUPS.map((g) => ({
  label: g.label,
  services: SERVICES.filter((s) => s.group === g.key).map((s) => s.key),
})).filter((section) => section.services.length > 0);

export interface SidebarProps {
  profile: string;
  region: string;
  profiles: Profile[];
  onRegionChange: (region: string) => void;
  // フォーカス中のペインが表示中のサービス。分割中にサービス未選択のペインが
  // フォーカス中なら null (強調する項目が無い)。
  activeService: string | null;
  onService: (svc: string) => void;
  // 分割中に各ペインが表示中のサービスへペイン番号の印を出す。長さが 2 のときだけ
  // 印を出し、未指定・1 ペインでは現状と同じ描画にする。
  paneServices?: (string | null)[];
  onWidthChange?: (width: number) => void;
  // workbench レイアウトの rail (issue 0211): true でアイコンと件数だけの 44px に畳む。
  // onToggleCollapsed を渡したときだけ畳むボタンを出す (standard では渡さない)。
  collapsed?: boolean;
  onToggleCollapsed?: () => void;
}

export function Sidebar({
  profile,
  region,
  profiles,
  onRegionChange,
  activeService,
  onService,
  paneServices,
  onWidthChange,
  collapsed = false,
  onToggleCollapsed,
}: SidebarProps) {
  const { t } = useTranslation('sidebar');
  // リージョン一覧は DescribeRegions から動的に取得する
  // 取得前は現在選択中の region のみを単一オプションとして表示するフォールバックにする
  const { data: regions } = useRegions(profile);
  const regionOptions = regions && regions.length > 0 ? regions : [{ code: region, name: region }];
  // 分割中だけ、サービスが表示中のペインの添字を返す (どちらのペインにも無ければ -1)
  const paneOf = (svc: string) => (paneServices?.length === 2 ? paneServices.indexOf(svc) : -1);

  return (
    <aside className={`sidebar${collapsed ? ' rail' : ''}`}>
      <div className="profile-card">
        <div className="profile-card-field">
          <span className="label">{t('sidebar.activeSession')}</span>
          <AwsActiveSessionCard profile={profile} profiles={profiles} />
        </div>
        <div className="profile-card-field">
          <span className="label">AWS_REGION</span>
          <select
            className="btn sm"
            value={region}
            onChange={(e) => onRegionChange(e.target.value)}
            title="Region"
          >
            {regionOptions.map((r) => (
              <option key={r.code} value={r.code}>
                {r.name === r.code ? r.code : `${r.name} (${r.code})`}
              </option>
            ))}
          </select>
        </div>
      </div>

      {SECTIONS.map((section) => (
        <div key={section.label}>
          <div className="section-label">{section.label}</div>
          {section.services.map((svc) => (
            <SvcItem
              key={svc}
              svc={svc}
              profile={profile}
              region={region}
              active={activeService}
              paneIndex={paneOf(svc)}
              onService={onService}
            />
          ))}
        </div>
      ))}

      {!collapsed && (
        <div
          className="sidebar-resizer"
          onPointerDown={startSidebarResize(onWidthChange)}
          title="Drag to resize"
        />
      )}
      {onToggleCollapsed && <SidebarToggle collapsed={collapsed} onToggle={onToggleCollapsed} />}
    </aside>
  );
}

interface SvcItemProps {
  svc: string;
  profile: string;
  region: string;
  active: string | null;
  // 分割中にこのサービスを表示しているペインの添字 (-1 はどのペインにも無い)
  paneIndex: number;
  onService: (svc: string) => void;
}

function SvcItem({ svc, profile, region, active, paneIndex, onService }: SvcItemProps) {
  const meta = SERVICES.find((s) => s.key === svc);
  // fetch は発生させず、他所で埋まったキャッシュを読み取るだけの読み取り専用フック。
  const data = useCachedQueryData<unknown[]>(['aws', svc, profile, region]);
  const count = data ? data.length : '-';
  const IconEl = AwsIcons[svc] ?? Icons[svc];

  return (
    <div
      className={`nav-item ${active === svc ? 'active' : ''}`}
      onClick={() => onService(svc)}
      title={meta?.name}
    >
      <span className="svc-icon">{IconEl ? <IconEl size={16} /> : null}</span>
      <span className="nav-label">{meta?.name}</span>
      {paneIndex >= 0 && <span className="pane-mark">{paneIndex + 1}</span>}
      <span className="count">{count}</span>
    </div>
  );
}
