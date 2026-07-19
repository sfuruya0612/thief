// sidebar.jsx の移植
// 件数バッジは「選択中のサービスのみ即時取得、他はクリックするまで取得しない」方針を守るため、
// queryFn: skipToken の読み取り専用オブザーバとして useQuery を使う。
// 本体の useResources (ServicePanel 側) が同じ queryKey で fetch した際、
// react-query のキャッシュ共有によりここにも値が反映される。
import { skipToken, useQuery } from '@tanstack/react-query';
import { AwsIcons } from './icons/AwsIcons';
import { Icons } from './icons/Icons';
import { AWS_SERVICE_GROUPS, SERVICES } from '../lib/serviceMeta';
import { useRegions } from '../api/queries';
import { startSidebarResize } from '../lib/sidebarResize';
import type { Profile } from '../types/common';
import { AwsActiveSessionCard } from './session/AwsActiveSessionCard';

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
  activeService: string;
  onService: (svc: string) => void;
  onWidthChange?: (width: number) => void;
}

export function Sidebar({
  profile,
  region,
  profiles,
  onRegionChange,
  activeService,
  onService,
  onWidthChange,
}: SidebarProps) {
  // リージョン一覧は DescribeRegions から動的に取得する
  // 取得前は現在選択中の region のみを単一オプションとして表示するフォールバックにする
  const { data: regions } = useRegions(profile);
  const regionOptions = regions && regions.length > 0 ? regions : [{ code: region, name: region }];

  return (
    <aside className="sidebar">
      <div className="profile-card">
        <div className="profile-card-field">
          <span className="label">アクティブセッション</span>
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
  profile: string;
  region: string;
  active: string;
  onService: (svc: string) => void;
}

function SvcItem({ svc, profile, region, active, onService }: SvcItemProps) {
  const meta = SERVICES.find((s) => s.key === svc);
  // fetch は発生させず、他所で埋まったキャッシュを読み取るだけの観測用クエリ。
  // queryFn に skipToken を渡すことで「フェッチしない」動作は維持しつつ、
  // queryFn 省略による dev 専用の console.error (No queryFn was passed) を回避する。
  const { data } = useQuery<unknown[]>({
    queryKey: ['aws', svc, profile, region],
    queryFn: skipToken,
  });
  const count = data ? data.length : '-';
  const IconEl = AwsIcons[svc] ?? Icons[svc];

  return (
    <div className={`nav-item ${active === svc ? 'active' : ''}`} onClick={() => onService(svc)}>
      <span className="svc-icon">{IconEl ? <IconEl size={16} /> : null}</span>
      <span>{meta?.name}</span>
      <span className="count">{count}</span>
    </div>
  );
}
