import { describe, expect, it } from 'vitest';
import { ecsServiceFromRaw, ecsTaskFromRaw, s3ObjectFromRaw } from './normalize';

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

describe('ecsTaskFromRaw', () => {
  it('snake_case を camelCase に変換し container_names を保持する', () => {
    const row = ecsTaskFromRaw({
      arn: 'arn:aws:ecs:ap-northeast-1:123:task/my-cluster/abc',
      group: 'service:my-svc',
      last_status: 'running',
      desired_status: 'running',
      launch_type: 'FARGATE',
      enable_execute_command: true,
      container_names: ['app', 'sidecar'],
      cpu: '256',
      memory: '512',
      started_at: '2026-07-08T00:00:00Z',
      stopped_at: '',
      stopped_reason: '',
      containers: [
        {
          name: 'app',
          image: 'app:latest',
          last_status: 'running',
          health_status: 'healthy',
          exit_code: undefined,
          reason: '',
        },
      ],
    });
    expect(row).toEqual({
      arn: 'arn:aws:ecs:ap-northeast-1:123:task/my-cluster/abc',
      group: 'service:my-svc',
      lastStatus: 'running',
      desiredStatus: 'running',
      launchType: 'FARGATE',
      enableExecuteCommand: true,
      containerNames: ['app', 'sidecar'],
      cpu: '256',
      memory: '512',
      startedAt: '2026-07-08T00:00:00Z',
      stoppedAt: '',
      stoppedReason: '',
      containers: [
        {
          name: 'app',
          image: 'app:latest',
          lastStatus: 'running',
          healthStatus: 'healthy',
          exitCode: undefined,
          reason: '',
        },
      ],
    });
  });

  it('container_names が未指定の場合は空配列にフォールバックする', () => {
    const row = ecsTaskFromRaw({
      arn: 'arn:task/b',
      group: '',
      last_status: 'stopped',
      desired_status: 'stopped',
      launch_type: '',
      enable_execute_command: false,
      container_names: undefined as unknown as string[],
      cpu: '',
      memory: '',
      started_at: '',
      stopped_at: '',
      stopped_reason: '',
      containers: undefined as unknown as never[],
    });
    expect(row.containerNames).toEqual([]);
    expect(row.containers).toEqual([]);
  });
});

describe('s3ObjectFromRaw', () => {
  it('snake_case を camelCase に変換する', () => {
    const row = s3ObjectFromRaw({
      key: 'path/to/file.txt',
      size: 1024,
      last_modified: '2026-07-08T00:00:00Z',
      storage_class: 'STANDARD',
      etag: 'abc123',
    });
    expect(row).toEqual({
      key: 'path/to/file.txt',
      size: 1024,
      lastModified: '2026-07-08T00:00:00Z',
      storageClass: 'STANDARD',
      etag: 'abc123',
    });
  });

  it('空文字フィールドをそのまま保持する', () => {
    const row = s3ObjectFromRaw({
      key: '',
      size: 0,
      last_modified: '',
      storage_class: '',
      etag: '',
    });
    expect(row.key).toBe('');
    expect(row.size).toBe(0);
    expect(row.storageClass).toBe('');
  });
});
