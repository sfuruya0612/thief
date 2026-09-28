// 詳細パネル (Drawer) の中身: サービスごとのタブ、見出し、タブごとの本文。
// 配置 (右 / 下 / 分割中の内包、backdrop、ESC、リサイズと寸法の永続化) は DrawerFrame が持つ
// (docs/issues/closed/0204)。
import { useEffect, useState } from 'react';
import type { BaseRow, DrawerPos } from '../../types/common';
import { DrawerFrame } from './DrawerFrame';
import { useTweaks } from '../../hooks/useTweaks';
import { AwsIcons } from '../icons/AwsIcons';
import { GcpIcons } from '../icons/GcpIcons';
import { Icons } from '../icons/Icons';
import { GCP_SERVICES, SERVICES } from '../../lib/serviceMeta';
import { Button, StatusBadge } from '../primitives';
import { DrawerCacheParameters } from './DrawerCacheParameters';
import { DrawerCFNEvents } from './DrawerCFNEvents';
import { DrawerCFNOverviewExtra } from './DrawerCFNOverviewExtra';
import { DrawerCFNResources } from './DrawerCFNResources';
import { DrawerCFNTags } from './DrawerCFNTags';
import { DrawerCloudFrontBehaviors } from './DrawerCloudFrontBehaviors';
import { DrawerDynamoItems } from './DrawerDynamoItems';
import { DrawerECRImages } from './DrawerECRImages';
import { DrawerECSContainerInstances } from './DrawerECSContainerInstances';
import { DrawerECSServices } from './DrawerECSServices';
import { DrawerECSTasks } from './DrawerECSTasks';
import { DrawerELBListeners } from './DrawerELBListeners';
import { DrawerELBTargets } from './DrawerELBTargets';
import { DrawerGCSObjects } from './DrawerGCSObjects';
import { DrawerRDSClusterParameters } from './DrawerRDSClusterParameters';
import { DrawerRDSInstanceParameters } from './DrawerRDSInstanceParameters';
import { DrawerS3Objects } from './DrawerS3Objects';
import { DrawerSecretEdit } from './DrawerSecretEdit';
import { DrawerSSMEdit } from './DrawerSSMEdit';
import { DrawerTags } from './DrawerTags';
import { DrawerWAFRules } from './DrawerWAFRules';
import { DrawerTerminal } from './DrawerTerminal';
import { openECSTerminalSession } from '../../lib/terminalLaunchers';
import type { OverviewEntry } from './overviewRows';

const DRAWER_TABS: Record<string, string[]> = {
  ec2: ['Overview', 'Terminal', 'Tags'],
  ecr: ['Overview', 'Images'],
  rds: ['Overview', 'Instance Parameters', 'Cluster Parameters', 'Tags'],
  cache: ['Overview', 'Parameters', 'Tags'],
  lambda: ['Overview', 'Tags'],
  ecs: ['Overview', 'Services', 'Tasks', 'Instances', 'Terminal', 'Tags'],
  s3: ['Overview', 'Objects', 'Tags'],
  iam: ['Overview', 'Tags'],
  elb: ['Overview', 'Listeners', 'Targets', 'Tags'],
  cloudfront: ['Overview', 'Behaviors', 'Tags'],
  apigw: ['Overview', 'Tags'],
  natgw: ['Overview', 'Tags'],
  sqs: ['Overview', 'Tags'],
  kinesis: ['Overview', 'Tags'],
  waf: ['Overview', 'Rules', 'Tags'],
  dynamo: ['Overview', 'Items', 'Tags'],
  ssm: ['Overview', 'Value', 'Tags'],
  secrets: ['Overview', 'Value', 'Tags'],
  cfn: ['Overview', 'Events', 'Resources', 'Tags'],
  gcs: ['Overview', 'Objects'],
};

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
  // 分割表示中はペインの中に収める (position: absolute + ペイン基準の % 上限)。
  // 既定値は現状と同じ position: fixed。
  contained?: boolean;
  // 分割表示中はフォーカス中のペインの Drawer だけが ESC を受け取る。既定値は true
  // (1 ペインでは分割前と同じく ESC で閉じる)。
  closeOnEscape?: boolean;
  onClose: () => void;
}

export function Drawer({
  resource,
  service,
  profile,
  region,
  position = 'right',
  overviewRows,
  contained = false,
  closeOnEscape = true,
  onClose,
}: DrawerProps) {
  const [tab, setTab] = useState('Overview');
  const open = !!resource;
  // Layout = workbench では docked (表の右 / 下に列として並ぶ)。issue 0213
  const mode = useTweaks().tweaks.layout === 'workbench' ? 'docked' : 'overlay';

  useEffect(() => {
    if (resource) {
      setTab('Overview');
    }
  }, [resource?.id]);

  const tabs = DRAWER_TABS[service] ?? ['Overview'];
  const svcMeta =
    SERVICES.find((s) => s.key === service) ?? GCP_SERVICES.find((s) => s.key === service);
  const IconEl = AwsIcons[service] ?? GcpIcons[service];

  return (
    <DrawerFrame
      open={open}
      position={position}
      mode={mode}
      contained={contained}
      closeOnEscape={closeOnEscape}
      onClose={onClose}
    >
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
                <Button size="sm" onClick={() => setTab('Terminal')}>
                  <Icons.terminal size={12} /> Open CLI
                </Button>
              )}
              <Button size="sm" variant="ghost" style={{ marginLeft: 'auto' }}>
                <Icons.more size={14} />
              </Button>
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
            {tab === 'Overview' && (
              <>
                <DrawerOverview rows={overviewRows} />
                {service === 'cfn' && (
                  <DrawerCFNOverviewExtra profile={profile} region={region} stack={resource.name} />
                )}
              </>
            )}
            {tab === 'Tags' && service !== 'cfn' && (
              <DrawerTags tags={resource.tags} fetchFailed={resource.tagsFetchFailed} />
            )}
            {tab === 'Tags' && service === 'cfn' && (
              <DrawerCFNTags profile={profile} region={region} stack={resource.name} />
            )}
            {tab === 'Events' && service === 'cfn' && (
              <DrawerCFNEvents profile={profile} region={region} stack={resource.name} />
            )}
            {tab === 'Resources' && service === 'cfn' && (
              <DrawerCFNResources profile={profile} region={region} stack={resource.name} />
            )}
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
              <DrawerECSTasks
                profile={profile}
                region={region}
                cluster={resource.name}
                // Terminal タブへ切り替えず、常駐ドックへ直接セッションを開く
                onExec={(target) => {
                  openECSTerminalSession(
                    profile,
                    region,
                    resource.name,
                    target.taskArn,
                    target.container,
                  );
                }}
              />
            )}
            {tab === 'Instances' && service === 'ecs' && (
              <DrawerECSContainerInstances
                profile={profile}
                region={region}
                cluster={resource.name}
              />
            )}
            {tab === 'Objects' && service === 's3' && (
              <DrawerS3Objects profile={profile} region={region} bucket={resource.name} />
            )}
            {tab === 'Objects' && service === 'gcs' && (
              <DrawerGCSObjects projectId={profile} bucket={resource.name} />
            )}
            {tab === 'Listeners' && service === 'elb' && (
              <DrawerELBListeners profile={profile} region={region} lbArn={resource.id} />
            )}
            {tab === 'Targets' && service === 'elb' && (
              <DrawerELBTargets profile={profile} region={region} lbArn={resource.id} />
            )}
            {tab === 'Instance Parameters' && service === 'rds' && (
              <DrawerRDSInstanceParameters
                profile={profile}
                region={region}
                instance={resource.name}
              />
            )}
            {tab === 'Cluster Parameters' && service === 'rds' && (
              <DrawerRDSClusterParameters
                profile={profile}
                region={region}
                instance={resource.name}
              />
            )}
            {tab === 'Parameters' && service === 'cache' && (
              <DrawerCacheParameters profile={profile} region={region} cluster={resource.name} />
            )}
            {tab === 'Behaviors' && service === 'cloudfront' && (
              <DrawerCloudFrontBehaviors profile={profile} region={region} id={resource.id} />
            )}
            {tab === 'Rules' && service === 'waf' && (
              <DrawerWAFRules
                profile={profile}
                region={region}
                id={resource.id}
                name={resource.name}
              />
            )}
            {tab === 'Items' && service === 'dynamo' && (
              <DrawerDynamoItems profile={profile} region={region} table={resource.name} />
            )}
            {tab === 'Value' && service === 'ssm' && (
              <DrawerSSMEdit
                profile={profile}
                region={region}
                name={resource.name}
                onClose={onClose}
              />
            )}
            {tab === 'Value' && service === 'secrets' && (
              <DrawerSecretEdit
                profile={profile}
                region={region}
                name={resource.name}
                onClose={onClose}
              />
            )}
          </div>
        </>
      )}
    </DrawerFrame>
  );
}
