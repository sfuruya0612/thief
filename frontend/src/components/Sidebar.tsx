// sidebar.jsx の移植
// 件数バッジは「選択中のサービスのみ即時取得、他はクリックするまで取得しない」方針を守るため、
// enabled: false の読み取り専用オブザーバとして useQuery を使う。
// 本体の useResources (ServicePanel 側) が同じ queryKey で fetch した際、
// react-query のキャッシュ共有によりここにも値が反映される。
import { useQuery } from '@tanstack/react-query';
import { AwsIcons } from './icons/AwsIcons';
import { Icons } from './icons/Icons';
import { SERVICES } from '../lib/serviceMeta';
import type { Profile } from '../types/common';

interface SidebarSection {
  label: string;
  services: string[];
}

const SECTIONS: SidebarSection[] = [
  { label: 'Compute', services: ['ec2', 'ecr', 'lambda', 'ecs'] },
  { label: 'Data', services: ['rds', 'dynamo', 'cache', 's3'] },
  { label: 'Network', services: ['elb', 'cloudfront', 'apigw', 'natgw'] },
  { label: 'Messaging', services: ['sqs', 'kinesis'] },
  { label: 'Security', services: ['waf', 'iam', 'ssm', 'secrets'] },
];

// よく使う AWS リージョン一覧 (プレーンな select で十分)
const REGIONS = [
  'us-east-1',
  'us-west-2',
  'ap-northeast-1',
  'ap-northeast-3',
  'ap-southeast-1',
  'ap-southeast-2',
  'eu-west-1',
  'eu-central-1',
];

const SIDEBAR_MIN_WIDTH = 160;
const SIDEBAR_MAX_WIDTH = 480;

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
  const startResize = (e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    const move = (ev: PointerEvent) => {
      const width = Math.min(Math.max(ev.clientX, SIDEBAR_MIN_WIDTH), SIDEBAR_MAX_WIDTH);
      document.documentElement.style.setProperty('--sidebar-w', `${width}px`);
      onWidthChange?.(width);
    };
    const up = () => {
      document.removeEventListener('pointermove', move);
      document.removeEventListener('pointerup', up);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };
    document.addEventListener('pointermove', move);
    document.addEventListener('pointerup', up);
    document.body.style.cursor = 'ew-resize';
    document.body.style.userSelect = 'none';
  };

  return (
    <aside className="sidebar">
      <div className="profile-card">
        <span className="label">AWS_PROFILE</span>
        <select
          className="btn sm"
          value={profile}
          onChange={(e) => onProfileChange(e.target.value)}
          title="AWS Profile"
        >
          {profiles.map((p) => (
            <option key={p.name} value={p.name}>
              {p.name}
            </option>
          ))}
        </select>
        <span className="label">AWS_REGION</span>
        <select
          className="btn sm"
          value={region}
          onChange={(e) => onRegionChange(e.target.value)}
          title="Region"
        >
          {REGIONS.map((r) => (
            <option key={r} value={r}>
              {r}
            </option>
          ))}
        </select>
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

      <div className="sidebar-resizer" onPointerDown={startResize} title="Drag to resize" />
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
