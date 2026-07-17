import { describe, expect, it } from 'vitest';
import { AWS_SERVICE_GROUPS, GCP_SERVICE_GROUPS, GCP_SERVICES, SERVICES } from './serviceMeta';

// カテゴリ定義 (AWS_SERVICE_GROUPS / GCP_SERVICE_GROUPS) からサービスの group を導出したときに
// 期待どおりのセクション構成 (key の配列) になることを確認する。サイドバー側の描画ロジックと
// 同じ導出方法 (filter + map) を使い、二重管理に戻る変更を検知する。
function sectionsOf(groups: { key: string }[], services: { key: string; group: string }[]) {
  return groups.map((g) => ({
    key: g.key,
    services: services.filter((s) => s.group === g.key).map((s) => s.key),
  }));
}

describe('AWS_SERVICE_GROUPS / SERVICES', () => {
  it('全サービスがいずれかの定義済みカテゴリに属する', () => {
    const groupKeys = new Set(AWS_SERVICE_GROUPS.map((g) => g.key));
    for (const s of SERVICES) {
      expect(groupKeys.has(s.group)).toBe(true);
    }
  });

  it('公式プロダクトカテゴリどおりのセクション構成になる', () => {
    expect(sectionsOf(AWS_SERVICE_GROUPS, SERVICES)).toEqual([
      { key: 'compute', services: ['ec2', 'lambda'] },
      { key: 'containers', services: ['ecr', 'ecs'] },
      { key: 'storage', services: ['s3'] },
      { key: 'database', services: ['rds', 'dynamo', 'cache'] },
      { key: 'networking', services: ['elb', 'cloudfront', 'apigw', 'natgw'] },
      { key: 'analytics', services: ['athena', 'kinesis'] },
      { key: 'integration', services: ['sqs'] },
      { key: 'security', services: ['iam', 'waf', 'secrets'] },
      { key: 'management', services: ['ssm', 'cfn'] },
      { key: 'cost', services: ['costexplorer'] },
    ]);
  });
});

describe('GCP_SERVICE_GROUPS / GCP_SERVICES', () => {
  it('全サービスがいずれかの定義済みカテゴリに属する', () => {
    const groupKeys = new Set(GCP_SERVICE_GROUPS.map((g) => g.key));
    for (const s of GCP_SERVICES) {
      expect(groupKeys.has(s.group)).toBe(true);
    }
  });

  it('公式プロダクトカテゴリどおりのセクション構成になる', () => {
    expect(sectionsOf(GCP_SERVICE_GROUPS, GCP_SERVICES)).toEqual([
      { key: 'compute', services: ['cloudrun'] },
      { key: 'analytics', services: ['bigquery'] },
      { key: 'storage', services: ['gcs'] },
      { key: 'security', services: ['gcpiam', 'gcpserviceaccounts'] },
      { key: 'observability', services: ['cloudlogging'] },
    ]);
  });
});
