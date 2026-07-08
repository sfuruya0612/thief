// app.jsx StatsRow の移植
// $/mo (行単位コスト) は確定方針により削除済みのため totalCost 合計ロジックは使わない。
// Monthly cost は useCost(profile, region) の結果からサービス別合計を抽出して表示する。
import type { ReactNode } from 'react';
import type { CostRow } from '../types/aws';
import { Spark } from './icons/Spark';
import { makeSpark } from '../lib/spark';
import { formatMoney } from '../lib/format';

interface Stat {
  label: string;
  value: ReactNode;
  tone?: 'pos' | 'neg';
  spark?: number[] | null;
}

export interface StatsRowProps {
  resources: Array<{
    state?: string;
    kind?: string;
    activeServices?: number;
    runningTasks?: number;
    pendingTasks?: number;
  }>;
  service: string;
  showCharts: boolean;
  cost: CostRow[];
}

// running 系の状態一覧 (モックの running/available/active/deployed に対応)
const RUNNING_STATES = new Set(['running', 'available', 'active', 'deployed']);

// cost レコードの service (AWS Cost Explorer の製品表示名) からこのサービスに該当するものを合算する
const SERVICE_COST_MATCH: Record<string, string[]> = {
  ec2: ['Amazon Elastic Compute Cloud - Compute'],
  rds: ['Amazon Relational Database Service'],
  dynamo: ['Amazon DynamoDB'],
  cache: ['Amazon ElastiCache'],
  lambda: ['AWS Lambda'],
  ecs: ['Amazon Elastic Container Service'],
  s3: ['Amazon Simple Storage Service'],
  elb: ['Amazon Elastic Load Balancing'],
  cloudfront: ['Amazon CloudFront'],
  apigw: ['Amazon API Gateway'],
  natgw: ['Amazon Virtual Private Cloud', 'EC2 - Other'],
  sqs: ['Amazon Simple Queue Service'],
  kinesis: ['Amazon Kinesis'],
  waf: ['AWS WAF'],
  iam: ['AWS Identity and Access Management'],
};

function monthlyCostFor(
  service: string,
  cost: CostRow[],
): { unblended: number; netAmortized: number } | undefined {
  const names = SERVICE_COST_MATCH[service];
  if (!names) return undefined;
  const matched = cost.filter((c) => names.includes(c.service));
  if (matched.length === 0) return undefined;
  return {
    unblended: matched.reduce((sum, c) => sum + c.unblendedAmount, 0),
    netAmortized: matched.reduce((sum, c) => sum + c.netAmortizedAmount, 0),
  };
}

function costStats(service: string, cost: CostRow[], showCharts: boolean): Stat[] {
  const amounts = monthlyCostFor(service, cost);
  return [
    {
      label: 'Monthly cost (Unblended)',
      value: amounts === undefined ? '—' : formatMoney(amounts.unblended),
      spark: showCharts ? makeSpark(`cost-unblended-${service}`, 20) : null,
    },
    {
      label: 'Monthly cost (Net Amortized)',
      value: amounts === undefined ? '—' : formatMoney(amounts.netAmortized),
      spark: showCharts ? makeSpark(`cost-net-amortized-${service}`, 20) : null,
    },
  ];
}

const SIMPLE_COST_ONLY = new Set(['apigw', 'natgw', 'sqs', 'kinesis', 'waf', 'dynamo']);

export function StatsRow({ resources, service, showCharts, cost }: StatsRowProps) {
  const running = resources.filter((r) => RUNNING_STATES.has(r.state ?? '')).length;
  const stopped = resources.filter((r) => r.state === 'stopped').length;
  const other = resources.length - running - stopped;

  let stats: Stat[];
  if (service === 's3') {
    stats = [
      { label: 'Resources', value: resources.length },
      ...costStats(service, cost, showCharts),
    ];
  } else if (service === 'elb') {
    const otherElb = resources.length - running;
    stats = [
      { label: 'Resources', value: resources.length },
      { label: 'Active', value: running, tone: 'pos' },
      { label: 'Other', value: otherElb, tone: otherElb > 0 ? 'neg' : undefined },
      ...costStats(service, cost, showCharts),
    ];
  } else if (service === 'cloudfront') {
    const deployed = resources.filter((r) => r.state === 'deployed').length;
    const inProgress = resources.filter((r) => r.state === 'in-progress').length;
    const otherCf = resources.length - deployed - inProgress;
    stats = [
      { label: 'Resources', value: resources.length },
      { label: 'Deployed', value: deployed, tone: 'pos' },
      { label: 'In Progress', value: inProgress },
      { label: 'Other', value: otherCf, tone: otherCf > 0 ? 'neg' : undefined },
      ...costStats(service, cost, showCharts),
    ];
  } else if (SIMPLE_COST_ONLY.has(service)) {
    stats = [
      { label: 'Resources', value: resources.length },
      ...costStats(service, cost, showCharts),
    ];
  } else if (service === 'iam') {
    stats = [
      { label: 'Users', value: resources.filter((r) => r.kind === 'user').length },
      { label: 'Roles', value: resources.filter((r) => r.kind === 'role').length },
    ];
  } else if (service === 'ecs') {
    // ECSRow はクラスタ単位の集計値のためタスク単位の Desired/Running/Pending/Stopped を近似する
    const desired = resources.reduce((sum, r) => sum + (r.activeServices ?? 0), 0);
    const runningTasks = resources.reduce((sum, r) => sum + (r.runningTasks ?? 0), 0);
    const pendingTasks = resources.reduce((sum, r) => sum + (r.pendingTasks ?? 0), 0);
    const stoppedClusters = resources.filter((r) => r.state === 'stopped').length;
    stats = [
      { label: 'Desired', value: desired },
      { label: 'Running', value: runningTasks, tone: 'pos' },
      { label: 'Pending', value: pendingTasks },
      { label: 'Stopped', value: stoppedClusters },
      ...costStats(service, cost, showCharts),
    ];
  } else {
    stats = [
      { label: 'Resources', value: resources.length },
      { label: 'Running', value: running, tone: 'pos' },
      { label: 'Stopped', value: stopped },
      { label: 'Other', value: other, tone: other > 0 ? 'neg' : undefined },
      ...costStats(service, cost, showCharts),
    ];
  }

  return (
    <div className="stats" style={{ gridTemplateColumns: `repeat(${stats.length}, 1fr)` }}>
      {stats.map((s, i) => (
        <div key={i} className="stat">
          <div className="label">{s.label}</div>
          <div className="value">{s.value}</div>
          {s.spark ? (
            <div className="spark">
              <Spark values={s.spark} w={120} h={20} stroke="var(--accent)" fill="var(--accent)" />
            </div>
          ) : (
            <div className={`delta ${s.tone ?? ''}`}>{s.tone ? '' : ' '}</div>
          )}
        </div>
      ))}
    </div>
  );
}
