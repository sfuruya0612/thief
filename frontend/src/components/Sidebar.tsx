// sidebar.jsx の移植
// 件数バッジは「選択中のサービスのみ即時取得、他はクリックするまで取得しない」方針を守るため、
// enabled: false の読み取り専用オブザーバとして useQuery を使う。
// 本体の useResources (ServicePanel 側) が同じ queryKey で fetch した際、
// react-query のキャッシュ共有によりここにも値が反映される。
import { useQuery } from '@tanstack/react-query';
import { AwsIcons } from './icons/AwsIcons';
import { Icons } from './icons/Icons';
import { SERVICES } from '../lib/serviceMeta';
import { useRegions } from '../api/queries';
import { startSidebarResize } from '../lib/sidebarResize';
import type { Profile } from '../types/common';
import { ProfileSelect } from './ProfileSelect';

interface SidebarSection {
  label: string;
  services: string[];
}

const SECTIONS: SidebarSection[] = [
  { label: 'Compute', services: ['ec2', 'ecr', 'lambda', 'ecs'] },
  { label: 'Data', services: ['athena', 'rds', 'dynamo', 'cache', 's3'] },
  { label: 'Network', services: ['elb', 'cloudfront', 'apigw', 'natgw'] },
  { label: 'Messaging', services: ['sqs', 'kinesis'] },
  { label: 'Security', services: ['waf', 'iam', 'ssm', 'secrets'] },
  { label: 'Cost', services: ['costexplorer'] },
];

export interface SidebarProps {
  profile: string;
  region: string;
  profiles: Profile[];
  onProfileChange: (name: string) => void;
  onRegionChange: (region: string) => void;
  activeService: string;
  onService: (svc: string) => void;
  onWidthChange?: (width: number) => void;
}

export function Sidebar({
  profile,
  region,
  profiles,
  onProfileChange,
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
          <span className="label">AWS_PROFILE</span>
          <ProfileSelect profile={profile} profiles={profiles} onProfileChange={onProfileChange} />
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
  // fetch は発生させず、他所で埋まったキャッシュを読み取るだけの観測用クエリ
  const { data } = useQuery<unknown[]>({
    queryKey: ['aws', svc, profile, region],
    enabled: false,
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
