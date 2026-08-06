// components/tables/columns.tsx の列定義のテスト。
import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import {
  apigwColumns,
  cacheColumns,
  cacheParameterColumns,
  cfnColumns,
  cfnEventColumns,
  cfnResourceColumns,
  cloudfrontBehaviorColumns,
  cloudfrontColumns,
  dynamoColumns,
  ec2Columns,
  ecrColumns,
  ecrImageColumns,
  ecsColumns,
  ecsServiceColumns,
  ecsTaskColumns,
  elbColumns,
  elbListenerColumns,
  elbRuleColumns,
  elbTargetGroupColumns,
  elbTargetHealthColumns,
  iamColumns,
  kinesisColumns,
  lambdaColumns,
  natgwColumns,
  rdsColumns,
  rdsParameterColumns,
  s3Columns,
  s3ObjectColumns,
  secretColumns,
  sqsColumns,
  ssmColumns,
  wafColumns,
  wafRuleColumns,
} from './columns';
import type {
  CacheRow,
  CloudFrontBehaviorRow,
  CloudFrontRow,
  IAMRow,
  WAFRow,
} from '../../types/aws';

const baseRow: WAFRow = {
  region: 'ap-northeast-1',
  id: 'acl-1',
  name: 'edge-acl',
  state: 'active',
  scope: 'REGIONAL',
  description: '',
  ruleCount: 3,
  associatedCount: 1,
  associatedCountFetchFailed: false,
  tags: {},
  tagsFetchFailed: false,
};

describe('wafColumns', () => {
  it('description 列が存在する', () => {
    expect(wafColumns.some((c) => c.key === 'description')).toBe(true);
  });

  it('列幅の合計が 100% になる', () => {
    const total = wafColumns.reduce((sum, c) => sum + Number.parseFloat(c.width), 0);
    expect(total).toBe(100);
  });

  it('description が空文字の行のセルはダッシュ表示になる', () => {
    const col = wafColumns.find((c) => c.key === 'description');
    if (!col) throw new Error('description column not found');
    const { container } = render(<>{col.cell(baseRow)}</>);
    expect(container).toHaveTextContent('—');
  });

  it('description が設定されていればその値を表示する', () => {
    const col = wafColumns.find((c) => c.key === 'description');
    if (!col) throw new Error('description column not found');
    const { container } = render(
      <>{col.cell({ ...baseRow, description: 'Protects the public API' })}</>,
    );
    expect(container).toHaveTextContent('Protects the public API');
  });

  it('列の並びが Web ACL, Description, State, Scope, Region, Rules, Associated の順になる', () => {
    expect(wafColumns.map((c) => c.key)).toEqual([
      'name',
      'description',
      'state',
      'scope',
      'region',
      'ruleCount',
      'associatedCount',
    ]);
    expect(wafColumns.map((c) => c.header)).toEqual([
      'Web ACL',
      'Description',
      'State',
      'Scope',
      'Region',
      'Rules',
      'Associated',
    ]);
  });

  it('各列の幅が並べ替え前と同じ対応を保つ', () => {
    const widthByKey = Object.fromEntries(wafColumns.map((c) => [c.key, c.width]));
    expect(widthByKey).toEqual({
      name: '20%',
      description: '17%',
      state: '8%',
      scope: '13%',
      region: '20%',
      ruleCount: '10%',
      associatedCount: '12%',
    });
  });

  it('ruleCount と associatedCount は右寄せのまま保たれる', () => {
    const ruleCount = wafColumns.find((c) => c.key === 'ruleCount');
    const associatedCount = wafColumns.find((c) => c.key === 'associatedCount');
    expect(ruleCount?.align).toBe('right');
    expect(associatedCount?.align).toBe('right');
  });

  it('associatedCountFetchFailed が true の行は警告アイコンを併記する', () => {
    const col = wafColumns.find((c) => c.key === 'associatedCount');
    if (!col) throw new Error('associatedCount column not found');
    const { container } = render(<>{col.cell({ ...baseRow, associatedCountFetchFailed: true })}</>);
    expect(container.querySelector('.fetch-failed-warning')).not.toBeNull();
    expect(container).toHaveTextContent('1');
  });

  it('associatedCountFetchFailed が false の行は警告アイコンを表示しない', () => {
    const col = wafColumns.find((c) => c.key === 'associatedCount');
    if (!col) throw new Error('associatedCount column not found');
    const { container } = render(<>{col.cell(baseRow)}</>);
    expect(container.querySelector('.fetch-failed-warning')).toBeNull();
  });
});

const iamBaseRow: IAMRow = {
  region: 'global',
  id: 'user-1',
  name: 'deploy-bot',
  state: 'active',
  arn: 'arn:aws:iam::123456789012:user/deploy-bot',
  kind: 'user',
  mfaEnabled: true,
  mfaEnabledFetchFailed: false,
  lastActivity: '2026-01-01T00:00:00Z',
  groups: ['admins'],
  groupsFetchFailed: false,
  policies: ['AdministratorAccess'],
  policiesFetchFailed: false,
};

describe('iamColumns', () => {
  it('mfaEnabledFetchFailed が true の行は赤バツの代わりに警告アイコンを表示する', () => {
    const col = iamColumns.find((c) => c.key === 'mfaEnabled');
    if (!col) throw new Error('mfaEnabled column not found');
    const { container } = render(
      <>{col.cell({ ...iamBaseRow, mfaEnabled: false, mfaEnabledFetchFailed: true })}</>,
    );
    expect(container.querySelector('.fetch-failed-warning')).not.toBeNull();
    expect(container).not.toHaveTextContent('✕');
  });

  it('mfaEnabledFetchFailed が false の行は mfaEnabled の値どおり表示する', () => {
    const col = iamColumns.find((c) => c.key === 'mfaEnabled');
    if (!col) throw new Error('mfaEnabled column not found');
    const { container } = render(<>{col.cell({ ...iamBaseRow, mfaEnabled: false })}</>);
    expect(container.querySelector('.fetch-failed-warning')).toBeNull();
    expect(container).toHaveTextContent('✕');
  });

  it('policiesFetchFailed が true の行は件数に警告アイコンを併記する', () => {
    const col = iamColumns.find((c) => c.key === 'policies');
    if (!col) throw new Error('policies column not found');
    const { container } = render(<>{col.cell({ ...iamBaseRow, policiesFetchFailed: true })}</>);
    expect(container.querySelector('.fetch-failed-warning')).not.toBeNull();
    expect(container).toHaveTextContent('1');
  });

  it('policiesFetchFailed が false の行は警告アイコンを表示しない', () => {
    const col = iamColumns.find((c) => c.key === 'policies');
    if (!col) throw new Error('policies column not found');
    const { container } = render(<>{col.cell({ ...iamBaseRow, policiesFetchFailed: false })}</>);
    expect(container.querySelector('.fetch-failed-warning')).toBeNull();
  });

  it('groupsFetchFailed が true の行は件数に警告アイコンを併記する', () => {
    const col = iamColumns.find((c) => c.key === 'groups');
    if (!col) throw new Error('groups column not found');
    const { container } = render(<>{col.cell({ ...iamBaseRow, groupsFetchFailed: true })}</>);
    expect(container.querySelector('.fetch-failed-warning')).not.toBeNull();
    expect(container).toHaveTextContent('1');
  });

  it('groupsFetchFailed が false の行は警告アイコンを表示しない', () => {
    const col = iamColumns.find((c) => c.key === 'groups');
    if (!col) throw new Error('groups column not found');
    const { container } = render(<>{col.cell({ ...iamBaseRow, groupsFetchFailed: false })}</>);
    expect(container.querySelector('.fetch-failed-warning')).toBeNull();
  });
});

const cloudfrontBaseRow: CloudFrontRow = {
  region: 'global',
  id: 'E123',
  name: 'test',
  state: 'deployed',
  domainName: 'd123.cloudfront.net',
  aliases: [],
  origins: ['origin.example.com'],
  behaviors: [],
  enabled: true,
  priceClass: 'PriceClass_All',
};

describe('cloudfrontColumns', () => {
  it('列の並びが Distribution, State, Domain, Alternate domains, Origins の順になる', () => {
    expect(cloudfrontColumns.map((c) => c.key)).toEqual([
      'id',
      'state',
      'domainName',
      'aliases',
      'origins',
    ]);
    expect(cloudfrontColumns.map((c) => c.header)).toEqual([
      'Distribution',
      'State',
      'Domain',
      'Alternate domains',
      'Origins',
    ]);
  });

  it('列幅の合計が 100% になる', () => {
    const total = cloudfrontColumns.reduce((sum, c) => sum + Number.parseFloat(c.width), 0);
    expect(total).toBe(100);
  });

  it('aliases が複数件のとき、カンマ + 半角スペースで結合して表示する', () => {
    const col = cloudfrontColumns.find((c) => c.key === 'aliases');
    if (!col) throw new Error('aliases column not found');
    const { container } = render(
      <>{col.cell({ ...cloudfrontBaseRow, aliases: ['example.com', 'www.example.com'] })}</>,
    );
    expect(container).toHaveTextContent('example.com, www.example.com');
  });

  it('aliases が空配列のときダッシュ表示になる', () => {
    const col = cloudfrontColumns.find((c) => c.key === 'aliases');
    if (!col) throw new Error('aliases column not found');
    const { container } = render(<>{col.cell(cloudfrontBaseRow)}</>);
    expect(container).toHaveTextContent('—');
  });
});

const cloudfrontBehaviorBaseRow: CloudFrontBehaviorRow = {
  id: '1',
  order: 1,
  pathPattern: '/api/*',
  targetOriginId: 'origin-1',
  viewerProtocolPolicy: 'https-only',
  allowedMethods: ['GET', 'HEAD'],
  compress: true,
  isDefault: false,
};

// 列順序と列幅の合計は後方の一覧網羅ブロック (issue 0104) が検証する。
describe('cloudfrontBehaviorColumns', () => {
  it('allowedMethods が複数件のとき、カンマ + 半角スペースで結合して表示する', () => {
    const col = cloudfrontBehaviorColumns.find((c) => c.key === 'allowedMethods');
    if (!col) throw new Error('allowedMethods column not found');
    const { container } = render(<>{col.cell(cloudfrontBehaviorBaseRow)}</>);
    expect(container).toHaveTextContent('GET, HEAD');
  });

  it('allowedMethods が空配列のときダッシュ表示になる', () => {
    const col = cloudfrontBehaviorColumns.find((c) => c.key === 'allowedMethods');
    if (!col) throw new Error('allowedMethods column not found');
    const { container } = render(
      <>{col.cell({ ...cloudfrontBehaviorBaseRow, allowedMethods: [] })}</>,
    );
    expect(container).toHaveTextContent('—');
  });

  it('compress が true のときチェックマークを表示する', () => {
    const col = cloudfrontBehaviorColumns.find((c) => c.key === 'compress');
    if (!col) throw new Error('compress column not found');
    const { container } = render(<>{col.cell(cloudfrontBehaviorBaseRow)}</>);
    expect(container).toHaveTextContent('✓');
  });

  it('compress が false のときダッシュ表示になる', () => {
    const col = cloudfrontBehaviorColumns.find((c) => c.key === 'compress');
    if (!col) throw new Error('compress column not found');
    const { container } = render(
      <>{col.cell({ ...cloudfrontBehaviorBaseRow, compress: false })}</>,
    );
    expect(container).toHaveTextContent('—');
  });

  it('pathPattern の filterValue は既定ビヘイビアで Default (*) を返す', () => {
    const col = cloudfrontBehaviorColumns.find((c) => c.key === 'pathPattern');
    if (!col || !col.filterValue) throw new Error('pathPattern column or filterValue not found');
    expect(col.filterValue({ ...cloudfrontBehaviorBaseRow, isDefault: true })).toBe('Default (*)');
  });

  it('pathPattern の filterValue は追加ビヘイビアで生の pathPattern を返す', () => {
    const col = cloudfrontBehaviorColumns.find((c) => c.key === 'pathPattern');
    if (!col || !col.filterValue) throw new Error('pathPattern column or filterValue not found');
    expect(col.filterValue(cloudfrontBehaviorBaseRow)).toBe('/api/*');
  });
});

describe('kinesisColumns', () => {
  it('列の並びが Stream, State, Mode, Shards, Retention, Encryption, Region の順になる', () => {
    expect(kinesisColumns.map((c) => c.key)).toEqual([
      'name',
      'state',
      'mode',
      'shardCount',
      'retentionHours',
      'encryptionType',
      'region',
    ]);
    expect(kinesisColumns.map((c) => c.header)).toEqual([
      'Stream',
      'State',
      'Mode',
      'Shards',
      'Retention',
      'Encryption',
      'Region',
    ]);
  });

  it('列幅の合計が 100% になる', () => {
    const total = kinesisColumns.reduce((sum, c) => sum + Number.parseFloat(c.width), 0);
    expect(total).toBe(100);
  });

  it('各列の幅が Mode 列追加後の再配分と同じ対応を保つ', () => {
    const widthByKey = Object.fromEntries(kinesisColumns.map((c) => [c.key, c.width]));
    expect(widthByKey).toEqual({
      name: '20%',
      state: '10%',
      mode: '15%',
      shardCount: '9%',
      retentionHours: '12%',
      encryptionType: '13%',
      region: '21%',
    });
  });
});

const cacheBaseRow: CacheRow = {
  region: 'ap-northeast-1',
  id: 'cc-1',
  name: 'cc-1',
  state: 'available',
  engine: 'redis',
  engineVersion: '7.1.0',
  nodeType: 'cache.t4g.micro',
  numNodes: 2,
  endpoint: 'cc-1.example.cache.amazonaws.com',
  port: 6379,
  parameterGroup: 'default.redis7',
  replicationGroupId: 'rg-1',
  nodeAvailabilityZones: [],
};

// 列順序と列幅の合計は後方の一覧網羅ブロック (issue 0104) が検証する。
describe('cacheColumns', () => {
  it('nodeAvailabilityZones が複数件のとき、カンマ + 半角スペースで結合して表示する', () => {
    const col = cacheColumns.find((c) => c.key === 'nodeAvailabilityZones');
    if (!col) throw new Error('nodeAvailabilityZones column not found');
    const { container } = render(
      <>
        {col.cell({
          ...cacheBaseRow,
          nodeAvailabilityZones: ['ap-northeast-1a', 'ap-northeast-1c'],
        })}
      </>,
    );
    expect(container).toHaveTextContent('ap-northeast-1a, ap-northeast-1c');
  });

  it('nodeAvailabilityZones が空配列のときダッシュ表示になる', () => {
    const col = cacheColumns.find((c) => c.key === 'nodeAvailabilityZones');
    if (!col) throw new Error('nodeAvailabilityZones column not found');
    const { container } = render(<>{col.cell(cacheBaseRow)}</>);
    expect(container).toHaveTextContent('—');
  });
});

// 全列定義の列順序 (key / header) と列幅の合計を一覧で検証する (issue 0104)。
// wafColumns / cloudfrontColumns / kinesisColumns は前方の個別 describe が
// 順序と幅の合計を検証済みのため、このブロックには含めない。
// 個別の width 値は検証しない (issues/closed/0089 の「幅は調整の自由度として残す」方針)。
interface ColumnOrderCase {
  name: string;
  columns: readonly { key: string; header: string; width: string }[];
  keys: string[];
  headers: string[];
  // 列幅の合計の期待値 (%)。100 にならない 3 定義は現状の実測値を期待値とする。
  widthSum: number;
}

const columnOrderCases: ColumnOrderCase[] = [
  {
    name: 'ec2Columns',
    columns: ec2Columns,
    keys: ['name', 'state', 'instanceType', 'region', 'az', 'privateIp', 'publicIp', 'uptime'],
    headers: ['Name', 'State', 'Type', 'Region', 'AZ', 'Private IP', 'Public IP', 'Uptime'],
    widthSum: 100,
  },
  {
    name: 'rdsColumns',
    columns: rdsColumns,
    keys: [
      'name',
      'state',
      'engine',
      'engineVersion',
      'clusterId',
      'class',
      'region',
      'multiAz',
      'endpoint',
      'uptime',
    ],
    headers: [
      'Identifier',
      'State',
      'Engine',
      'Engine Version',
      'Cluster',
      'Class',
      'Region',
      'MultiAZ',
      'Endpoint',
      'Uptime',
    ],
    widthSum: 100,
  },
  {
    name: 'rdsParameterColumns',
    columns: rdsParameterColumns,
    keys: ['name', 'value', 'applyType', 'dataType', 'isModifiable', 'source'],
    headers: ['Name', 'Value', 'Apply type', 'Data type', 'Modifiable', 'Source'],
    widthSum: 96,
  },
  {
    name: 'cacheParameterColumns',
    columns: cacheParameterColumns,
    keys: ['name', 'value', 'changeType', 'dataType', 'isModifiable', 'source'],
    headers: ['Name', 'Value', 'Change type', 'Data type', 'Modifiable', 'Source'],
    widthSum: 96,
  },
  {
    name: 'cacheColumns',
    columns: cacheColumns,
    keys: [
      'name',
      'state',
      'engine',
      'replicationGroupId',
      'nodeType',
      'region',
      'nodeAvailabilityZones',
      'numNodes',
      'endpoint',
    ],
    headers: [
      'Cluster',
      'State',
      'Engine',
      'Replication Group',
      'Node type',
      'Region',
      'AZs',
      'Nodes',
      'Endpoint',
    ],
    widthSum: 100,
  },
  {
    name: 'lambdaColumns',
    columns: lambdaColumns,
    keys: ['name', 'state', 'runtime', 'region', 'memoryMb', 'timeoutSec'],
    headers: ['Function', 'State', 'Runtime', 'Region', 'Memory', 'Timeout'],
    widthSum: 100,
  },
  {
    name: 'ecsColumns',
    columns: ecsColumns,
    keys: [
      'name',
      'state',
      'activeServices',
      'runningTasks',
      'pendingTasks',
      'registeredEc2',
      'region',
    ],
    headers: ['Cluster', 'State', 'Active svc', 'Running', 'Pending', 'Registered EC2', 'Region'],
    widthSum: 100,
  },
  {
    name: 'ecsServiceColumns',
    columns: ecsServiceColumns,
    keys: [
      'name',
      'status',
      'desiredCount',
      'runningCount',
      'pendingCount',
      'launchType',
      'taskDefinition',
    ],
    headers: [
      'Service',
      'State',
      'Desired',
      'Running',
      'Pending',
      'Launch type',
      'Task definition',
    ],
    widthSum: 100,
  },
  {
    name: 'ecsTaskColumns',
    columns: ecsTaskColumns,
    keys: [
      'group',
      'containerNames',
      'lastStatus',
      'desiredStatus',
      'launchType',
      'enableExecuteCommand',
      'arn',
    ],
    headers: [
      'Group',
      'Containers',
      'Last status',
      'Desired status',
      'Launch type',
      'Exec enabled',
      'ARN',
    ],
    widthSum: 100,
  },
  {
    name: 'ecrColumns',
    columns: ecrColumns,
    keys: ['name', 'uri', 'imageTagMutability', 'scanOnPush', 'createdAt'],
    headers: ['Repository', 'URI', 'Tag mutability', 'Scan on push', 'Created'],
    widthSum: 100,
  },
  {
    name: 'ecrImageColumns',
    columns: ecrImageColumns,
    keys: ['imageTag', 'imageDigest', 'imageSizeBytes', 'pushedAt', 'lastPulledAt'],
    headers: ['Tag', 'Digest', 'Size', 'Pushed', 'Pulled'],
    widthSum: 100,
  },
  {
    name: 'ssmColumns',
    columns: ssmColumns,
    keys: ['name', 'type', 'version', 'lastModified'],
    headers: ['Name', 'Type', 'Version', 'Last modified'],
    widthSum: 100,
  },
  {
    name: 'secretColumns',
    columns: secretColumns,
    keys: ['name', 'description', 'lastChanged'],
    headers: ['Name', 'Description', 'Last changed'],
    widthSum: 100,
  },
  {
    name: 's3Columns',
    columns: s3Columns,
    keys: ['name', 'state', 'region', 'createdAt', 'public', 'encryption'],
    headers: ['Bucket', 'State', 'Region', 'Created', 'Public', 'Encryption'],
    widthSum: 100,
  },
  {
    name: 's3ObjectColumns',
    columns: s3ObjectColumns,
    keys: ['key', 'size', 'lastModified', 'storageClass'],
    headers: ['Key', 'Size', 'Last modified', 'Storage class'],
    widthSum: 90,
  },
  {
    name: 'elbColumns',
    columns: elbColumns,
    keys: ['name', 'state', 'type', 'scheme', 'region', 'dnsName', 'azs'],
    headers: ['Name', 'State', 'Type', 'Scheme', 'Region', 'DNS name', 'AZs'],
    widthSum: 100,
  },
  {
    name: 'elbListenerColumns',
    columns: elbListenerColumns,
    keys: ['protocol', 'port', 'defaultActionType', 'defaultTargetGroupArn', 'arn'],
    headers: ['Protocol', 'Port', 'Default action', 'Default target group', 'ARN'],
    widthSum: 100,
  },
  {
    name: 'elbRuleColumns',
    columns: elbRuleColumns,
    keys: ['priority', 'conditions', 'actionType', 'targetGroupArn', 'arn'],
    headers: ['Priority', 'Conditions', 'Action', 'Target group', 'ARN'],
    widthSum: 100,
  },
  {
    name: 'elbTargetGroupColumns',
    columns: elbTargetGroupColumns,
    keys: ['name', 'protocol', 'port', 'targetType', 'healthCheckPath', 'vpcId', 'arn'],
    headers: ['Name', 'Protocol', 'Port', 'Target type', 'Health check', 'VPC', 'ARN'],
    widthSum: 100,
  },
  {
    name: 'elbTargetHealthColumns',
    columns: elbTargetHealthColumns,
    keys: ['targetId', 'port', 'state', 'availabilityZone', 'reason', 'description'],
    headers: ['Target', 'Port', 'State', 'AZ', 'Reason', 'Description'],
    widthSum: 100,
  },
  {
    name: 'cloudfrontBehaviorColumns',
    columns: cloudfrontBehaviorColumns,
    keys: [
      'order',
      'pathPattern',
      'targetOriginId',
      'viewerProtocolPolicy',
      'allowedMethods',
      'compress',
    ],
    headers: [
      'Precedence',
      'Path pattern',
      'Target origin',
      'Viewer protocol',
      'Allowed methods',
      'Compress',
    ],
    widthSum: 100,
  },
  {
    name: 'apigwColumns',
    columns: apigwColumns,
    keys: ['name', 'state', 'type', 'stage', 'endpoint', 'region'],
    headers: ['API', 'State', 'Type', 'Stage', 'Endpoint', 'Region'],
    widthSum: 100,
  },
  {
    name: 'natgwColumns',
    columns: natgwColumns,
    keys: ['name', 'state', 'id', 'vpcId', 'elasticIp', 'region', 'uptime'],
    headers: ['Name', 'State', 'Gateway ID', 'VPC', 'Elastic IP', 'Region', 'Uptime'],
    widthSum: 100,
  },
  {
    name: 'dynamoColumns',
    columns: dynamoColumns,
    keys: ['name', 'state', 'mode', 'itemCount', 'sizeBytes', 'gsiCount', 'region'],
    headers: ['Table', 'State', 'Mode', 'Items', 'Size', 'GSI', 'Region'],
    widthSum: 100,
  },
  {
    name: 'sqsColumns',
    columns: sqsColumns,
    keys: ['name', 'state', 'type', 'availableMessages', 'inFlight', 'retentionDays', 'region'],
    headers: ['Queue', 'State', 'Type', 'Available', 'In flight', 'Retention', 'Region'],
    widthSum: 100,
  },
  {
    name: 'wafRuleColumns',
    columns: wafRuleColumns,
    keys: ['name', 'priority', 'action', 'statement'],
    headers: ['Name', 'Priority', 'Action', 'Statement'],
    widthSum: 100,
  },
  {
    name: 'iamColumns',
    columns: iamColumns,
    keys: ['name', 'kind', 'mfaEnabled', 'state', 'lastActivity', 'policies', 'groups'],
    headers: ['Name', 'Kind', 'MFA', 'Activity', 'Last active', 'Policies', 'Groups'],
    widthSum: 100,
  },
  {
    name: 'cfnColumns',
    columns: cfnColumns,
    keys: ['name', 'state', 'driftStatus', 'createdAt', 'updatedAt'],
    headers: ['Stack', 'State', 'Drift', 'Created', 'Updated'],
    widthSum: 100,
  },
  {
    name: 'cfnEventColumns',
    columns: cfnEventColumns,
    keys: [
      'timestamp',
      'logicalResourceId',
      'resourceType',
      'resourceStatus',
      'resourceStatusReason',
    ],
    headers: ['Time', 'Logical ID', 'Type', 'Status', 'Reason'],
    widthSum: 100,
  },
  {
    name: 'cfnResourceColumns',
    columns: cfnResourceColumns,
    keys: [
      'logicalResourceId',
      'physicalResourceId',
      'resourceType',
      'resourceStatus',
      'lastUpdatedTime',
    ],
    headers: ['Logical ID', 'Physical ID', 'Type', 'Status', 'Updated'],
    widthSum: 100,
  },
];

describe.each(columnOrderCases)('$name', ({ columns, keys, headers, widthSum }) => {
  it('列の並び (key / header) が定義順のまま保たれる', () => {
    expect(columns.map((c) => c.key)).toEqual(keys);
    expect(columns.map((c) => c.header)).toEqual(headers);
  });

  it(`列幅の合計が ${widthSum}% になる`, () => {
    const total = columns.reduce((sum, c) => sum + Number.parseFloat(c.width), 0);
    expect(total).toBe(widthSum);
  });
});
