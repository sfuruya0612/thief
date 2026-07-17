import { describe, expect, it } from 'vitest';
import {
  callerIdentityFromRaw,
  cfnFromRaw,
  cfnStackDetailFromRaw,
  cfnStackEventFromRaw,
  cfnStackResourceFromRaw,
  dynamoTableSchemaFromRaw,
  ecsServiceFromRaw,
  ecsTaskFromRaw,
  profileFromRaw,
  s3ObjectFromRaw,
} from './normalize';

describe('profileFromRaw', () => {
  it('sso_account_id / sso_role_name を camelCase に変換する', () => {
    const row = profileFromRaw({
      name: 'my-sso-profile',
      account_id: '111111111111',
      sso_role_name: 'AdministratorAccess',
    });
    expect(row).toEqual({
      name: 'my-sso-profile',
      accountId: '111111111111',
      ssoRoleName: 'AdministratorAccess',
    });
  });

  it('account_id / sso_role_name が欠落した非 SSO プロファイルでも変換できる', () => {
    const row = profileFromRaw({ name: 'plain-profile' });
    expect(row).toEqual({
      name: 'plain-profile',
      accountId: undefined,
      ssoRoleName: undefined,
    });
  });

  it('auth_type / sso_status / region / sso_expires_at を変換する', () => {
    const row = profileFromRaw({
      name: 'sso-prof',
      region: 'ap-northeast-1',
      auth_type: 'sso',
      sso_status: 'valid',
      sso_expires_at: '2026-07-17T20:00:00Z',
    });
    expect(row.region).toBe('ap-northeast-1');
    expect(row.authType).toBe('sso');
    expect(row.ssoStatus).toBe('valid');
    expect(row.ssoExpiresAt).toBe('2026-07-17T20:00:00Z');
  });

  it('未知の enum 文字列は undefined に落とす', () => {
    const row = profileFromRaw({
      name: 'future-prof',
      auth_type: 'quantum_auth',
      sso_status: 'maybe',
    });
    expect(row.authType).toBeUndefined();
    expect(row.ssoStatus).toBeUndefined();
  });
});

describe('callerIdentityFromRaw', () => {
  it('snake_case を camelCase に変換する', () => {
    const row = callerIdentityFromRaw({
      account_id: '222222222222',
      arn: 'arn:aws:iam::222222222222:user/me',
      user_id: 'AIDAEXAMPLE',
    });
    expect(row).toEqual({
      accountId: '222222222222',
      arn: 'arn:aws:iam::222222222222:user/me',
      userId: 'AIDAEXAMPLE',
    });
  });
});

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
          runtime_id: 'runtime-app',
          exec_enabled: true,
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
          runtimeId: 'runtime-app',
          execEnabled: true,
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

describe('dynamoTableSchemaFromRaw', () => {
  it('snake_case を camelCase に変換し sort_key を保持する', () => {
    const row = dynamoTableSchemaFromRaw({
      table_name: 'users',
      table: {
        name: 'users',
        partition_key: { name: 'pk', type: 'S' },
        sort_key: { name: 'sk', type: 'N' },
      },
      gsis: [
        {
          name: 'gsi1',
          partition_key: { name: 'gsi1pk', type: 'S' },
        },
      ],
    });
    expect(row).toEqual({
      tableName: 'users',
      table: {
        name: 'users',
        partitionKey: { name: 'pk', type: 'S' },
        sortKey: { name: 'sk', type: 'N' },
      },
      gsis: [
        {
          name: 'gsi1',
          partitionKey: { name: 'gsi1pk', type: 'S' },
          sortKey: undefined,
        },
      ],
    });
  });

  it('sort_key と gsis が null の場合を空配列/undefined として扱う', () => {
    const row = dynamoTableSchemaFromRaw({
      table_name: 'orders',
      table: {
        name: 'orders',
        partition_key: { name: 'pk', type: 'S' },
      },
      gsis: null,
    });
    expect(row.table.sortKey).toBeUndefined();
    expect(row.gsis).toEqual([]);
  });
});

describe('cfnFromRaw', () => {
  it('snake_case を camelCase に変換する', () => {
    const row = cfnFromRaw(
      {
        id: 'arn:aws:cloudformation:ap-northeast-1:111111111111:stack/my-stack/abc',
        name: 'my-stack',
        state: 'CREATE_COMPLETE',
        creation_time: '2026-07-01T00:00:00Z',
        last_updated_time: '2026-07-02T00:00:00Z',
        drift_status: 'IN_SYNC',
        tags: { env: 'prod' },
      },
      'ap-northeast-1',
    );
    expect(row).toEqual({
      region: 'ap-northeast-1',
      id: 'arn:aws:cloudformation:ap-northeast-1:111111111111:stack/my-stack/abc',
      name: 'my-stack',
      state: 'CREATE_COMPLETE',
      createdAt: '2026-07-01T00:00:00Z',
      updatedAt: '2026-07-02T00:00:00Z',
      driftStatus: 'IN_SYNC',
      tags: { env: 'prod' },
    });
  });
});

describe('cfnStackDetailFromRaw', () => {
  it('parameters / outputs を含めて camelCase に変換する', () => {
    const row = cfnStackDetailFromRaw({
      stack_name: 'my-stack',
      status: 'UPDATE_COMPLETE',
      drift_status: 'NOT_CHECKED',
      created_time: '2026-07-01T00:00:00Z',
      updated_time: '2026-07-02T00:00:00Z',
      description: 'test stack',
      parameters: [{ key: 'Env', value: 'prod', resolved_value: 'prod' }],
      outputs: [
        { key: 'BucketName', value: 'my-bucket', export_name: 'my-export', description: 'desc' },
      ],
      tags: { owner: 'team-a' },
    });
    expect(row).toEqual({
      stackName: 'my-stack',
      status: 'UPDATE_COMPLETE',
      driftStatus: 'NOT_CHECKED',
      createdAt: '2026-07-01T00:00:00Z',
      updatedAt: '2026-07-02T00:00:00Z',
      description: 'test stack',
      parameters: [{ key: 'Env', value: 'prod', resolvedValue: 'prod' }],
      outputs: [
        { key: 'BucketName', value: 'my-bucket', exportName: 'my-export', description: 'desc' },
      ],
      tags: { owner: 'team-a' },
    });
  });
});

describe('cfnStackEventFromRaw', () => {
  it('idx を含む一意な id を生成し camelCase に変換する', () => {
    const row = cfnStackEventFromRaw(
      {
        timestamp: '2026-07-01T00:00:00Z',
        logical_resource_id: 'MyBucket',
        resource_type: 'AWS::S3::Bucket',
        resource_status: 'CREATE_FAILED',
        resource_status_reason: 'Bucket already exists',
      },
      2,
    );
    expect(row).toEqual({
      id: '2026-07-01T00:00:00Z-MyBucket-2',
      timestamp: '2026-07-01T00:00:00Z',
      logicalResourceId: 'MyBucket',
      resourceType: 'AWS::S3::Bucket',
      resourceStatus: 'CREATE_FAILED',
      resourceStatusReason: 'Bucket already exists',
    });
  });
});

describe('cfnStackResourceFromRaw', () => {
  it('logical_resource_id を id として camelCase に変換する', () => {
    const row = cfnStackResourceFromRaw({
      logical_resource_id: 'MyBucket',
      physical_resource_id: 'my-bucket-abc123',
      resource_type: 'AWS::S3::Bucket',
      resource_status: 'UPDATE_COMPLETE',
      last_updated_time: '2026-07-02T00:00:00Z',
    });
    expect(row).toEqual({
      id: 'MyBucket',
      logicalResourceId: 'MyBucket',
      physicalResourceId: 'my-bucket-abc123',
      resourceType: 'AWS::S3::Bucket',
      resourceStatus: 'UPDATE_COMPLETE',
      lastUpdatedTime: '2026-07-02T00:00:00Z',
    });
  });
});
