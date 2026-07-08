// drawer.jsx Drawer の移植: 右/下ドッキング可能な詳細パネル
import { useEffect, useState } from 'react';
import type { BaseRow, DrawerPos } from '../../types/common';
import { AwsIcons } from '../icons/AwsIcons';
import { Icons } from '../icons/Icons';
import { SERVICES } from '../../lib/serviceMeta';
import { StatusBadge } from '../primitives';
import { DrawerECRImages } from './DrawerECRImages';
import { DrawerECSServices } from './DrawerECSServices';
import { DrawerECSTasks } from './DrawerECSTasks';
import { DrawerS3Objects } from './DrawerS3Objects';
import { DrawerTags } from './DrawerTags';
import { DrawerTerminal } from './DrawerTerminal';
import type { OverviewEntry } from './overviewRows';

const DRAWER_TABS: Record<string, string[]> = {
  ec2: ['Overview', 'Terminal', 'Tags'],
  ecr: ['Overview', 'Images'],
  rds: ['Overview', 'Tags'],
  cache: ['Overview', 'Tags'],
  lambda: ['Overview', 'Tags'],
  ecs: ['Overview', 'Services', 'Tasks', 'Terminal', 'Tags'],
  s3: ['Overview', 'Objects', 'Tags'],
  iam: ['Overview', 'Tags'],
  elb: ['Overview', 'Tags'],
  cloudfront: ['Overview', 'Tags'],
  apigw: ['Overview', 'Tags'],
  natgw: ['Overview', 'Tags'],
  sqs: ['Overview', 'Tags'],
  kinesis: ['Overview', 'Tags'],
  waf: ['Overview', 'Tags'],
  dynamo: ['Overview', 'Tags'],
  ssm: ['Overview', 'Tags'],
  secrets: ['Overview', 'Tags'],
};

const DRAWER_SIZE_KEY = 'cloudlens:drawerSize';

interface DrawerSize {
  width?: number;
  height?: number;
}

function loadDrawerSize(): DrawerSize {
  try {
    return JSON.parse(localStorage.getItem(DRAWER_SIZE_KEY) || '{}') as DrawerSize;
  } catch {
    return {};
  }
}

function saveDrawerSize(size: DrawerSize): void {
  try {
    localStorage.setItem(DRAWER_SIZE_KEY, JSON.stringify(size));
  } catch {
    // quota / serialization エラーは無視
  }
}

function DrawerOverview({ rows }: { rows: OverviewEntry[] }) {
  return (
    <div className="section">
      <h3>Resource details</h3>
      <div className="kv">
        {rows.map(([k, v]) => (
          <div key={k} style={{ display: 'contents' }}>
            <div className="k">{k}</div>
            <div className="v">
              {v}
              <span className="copy" title="copy">
                ⎘
              </span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

export interface DrawerProps {
  resource: BaseRow | null;
  service: string;
  profile: string;
  region: string;
  position?: DrawerPos;
  overviewRows: OverviewEntry[];
  onClose: () => void;
}

export function Drawer({
  resource,
  service,
  profile,
  region,
  position = 'right',
  overviewRows,
  onClose,
}: DrawerProps) {
  const [tab, setTab] = useState('Overview');
  const [size, setSize] = useState<DrawerSize>(loadDrawerSize);
  const open = !!resource;

  useEffect(() => {
    if (resource) setTab('Overview');
  }, [resource?.id]);

  const startResize = (e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    const move = (ev: PointerEvent) => {
      setSize((prev) => {
        let next: DrawerSize;
        if (position === 'bottom') {
          const h = Math.min(
            Math.max(window.innerHeight - ev.clientY - 8, 220),
            window.innerHeight * 0.85,
          );
          next = { ...prev, height: Math.round(h) };
        } else {
          const w = Math.min(
            Math.max(window.innerWidth - ev.clientX - 8, 380),
            window.innerWidth * 0.85,
          );
          next = { ...prev, width: Math.round(w) };
        }
        saveDrawerSize(next);
        return next;
      });
    };
    const up = () => {
      document.removeEventListener('pointermove', move);
      document.removeEventListener('pointerup', up);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };
    document.addEventListener('pointermove', move);
    document.addEventListener('pointerup', up);
    document.body.style.cursor = position === 'bottom' ? 'ns-resize' : 'ew-resize';
    document.body.style.userSelect = 'none';
  };

  const tabs = DRAWER_TABS[service] ?? ['Overview'];
  const svcMeta = SERVICES.find((s) => s.key === service);
  const IconEl = AwsIcons[service];

  const sizeStyle: React.CSSProperties =
    position === 'bottom'
      ? size.height
        ? { height: size.height }
        : {}
      : size.width
        ? { width: Math.min(size.width, window.innerWidth * 0.85) }
        : {};

  return (
    <>
      <div className={`drawer-backdrop ${open ? 'open' : ''}`} onClick={onClose} />
      <div
        className={`drawer ${position === 'bottom' ? 'pos-bottom' : ''} ${open ? 'open' : ''}`}
        style={{
          ...sizeStyle,
          transform: open
            ? 'translate(0, 0)'
            : position === 'bottom'
              ? 'translateY(calc(100% + 16px))'
              : 'translateX(calc(100% + 16px))',
        }}
      >
        <div
          className={`resize-handle ${position === 'bottom' ? 'rh-top' : 'rh-left'}`}
          onPointerDown={startResize}
          title="Drag to resize"
        />
        {resource && (
          <>
            <div className="dh">
              <div className="top">
                <span className="svc-pill" style={{ gap: 6 }}>
                  {IconEl ? (
                    <IconEl size={13} />
                  ) : (
                    <span className="dot" style={{ background: svcMeta?.color }} />
                  )}
                  {svcMeta?.name}
                </span>
                <span style={{ color: 'var(--text-4)' }}>/</span>
                <span className="mono" style={{ color: 'var(--text-2)' }}>
                  {profile}
                </span>
                <span style={{ color: 'var(--text-4)' }}>/</span>
                <span className="mono" style={{ color: 'var(--text-3)' }}>
                  {region}
                </span>
                <button className="x" onClick={onClose}>
                  <Icons.x />
                </button>
              </div>
              <h2>
                {resource.name}
                <StatusBadge state={resource.state ?? ''} />
              </h2>
              <div className="id">{resource.id}</div>
              <div className="actions">
                {tabs.includes('Terminal') && (
                  <button className="btn sm" onClick={() => setTab('Terminal')}>
                    <Icons.terminal size={12} /> Open CLI
                  </button>
                )}
                <button className="btn sm">
                  <Icons.external size={12} /> Console
                </button>
                <button className="btn sm ghost" style={{ marginLeft: 'auto' }}>
                  <Icons.more size={14} />
                </button>
              </div>
            </div>

            <div className="dtabs">
              {tabs.map((t) => (
                <div
                  key={t}
                  className={`dtab ${tab === t ? 'active' : ''}`}
                  onClick={() => setTab(t)}
                >
                  {t}
                </div>
              ))}
            </div>

            <div className="dbody">
              {tab === 'Overview' && <DrawerOverview rows={overviewRows} />}
              {tab === 'Tags' && <DrawerTags tags={resource.tags} />}
              {tab === 'Terminal' && (
                <DrawerTerminal
                  service={service}
                  profile={profile}
                  region={region}
                  resource={resource}
                />
              )}
              {tab === 'Images' && (
                <DrawerECRImages profile={profile} region={region} repo={resource.name} />
              )}
              {tab === 'Services' && service === 'ecs' && (
                <DrawerECSServices profile={profile} region={region} cluster={resource.name} />
              )}
              {tab === 'Tasks' && service === 'ecs' && (
                <DrawerECSTasks profile={profile} region={region} cluster={resource.name} />
              )}
              {tab === 'Objects' && service === 's3' && (
                <DrawerS3Objects profile={profile} region={region} bucket={resource.name} />
              )}
            </div>
          </>
        )}
      </div>
    </>
  );
}
