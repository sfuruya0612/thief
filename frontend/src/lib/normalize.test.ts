import { describe, expect, it } from 'vitest';
import { ecsServiceFromRaw } from './normalize';

describe('ecsServiceFromRaw', () => {
  it('snake_case を camelCase に変換する', () => {
    const row = ecsServiceFromRaw({
      arn: 'arn:aws:ecs:ap-northeast-1:123:service/my-cluster/my-svc',
      name: 'my-svc',
      status: 'active',
      desired_count: 3,
      running_count: 2,
      pending_count: 1,
      task_definition: 'my-td:12',
      launch_type: 'FARGATE',
    });
    expect(row).toEqual({
      arn: 'arn:aws:ecs:ap-northeast-1:123:service/my-cluster/my-svc',
      name: 'my-svc',
      status: 'active',
      desiredCount: 3,
      runningCount: 2,
      pendingCount: 1,
      taskDefinition: 'my-td:12',
      launchType: 'FARGATE',
    });
  });
});
