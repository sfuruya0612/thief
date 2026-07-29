import { describe, expect, it } from 'vitest';
import {
  cacheFromRaw,
  cacheParameterFromRaw,
  callerIdentityFromRaw,
  cfnFromRaw,
  cfnStackDetailFromRaw,
  cfnStackEventFromRaw,
  cfnStackResourceFromRaw,
  cloudfrontFromRaw,
  cwLogEventFromRaw,
  cwLogGroupFromRaw,
  dynamoTableSchemaFromRaw,
  ecsServiceFromRaw,
  ecsTaskFromRaw,
  kinesisFromRaw,
  objectPreviewFromRaw,
  profileFromRaw,
  rdsFromRaw,
  rdsClusterParameterGroupFromRaw,
  rdsParameterFromRaw,
  s3ObjectFromRaw,
  wafFromRaw,
  wafRuleFromRaw,
} from './normalize';
import type { CloudFrontRaw, WAFRaw } from '../types/aws';

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

describe('rdsFromRaw', () => {
  it('cluster_id を clusterId に変換する', () => {
    const row = rdsFromRaw(
      {
        id: 'db-1',
        name: 'db-1',
        state: 'available',
        engine: 'aurora-mysql',
        engine_version: '8.0.mysql_aurora.3.04.0',
        class: 'db.r6g.large',
        multi_az: false,
        endpoint: 'db-1.abc.ap-northeast-1.rds.amazonaws.com',
        port: 3306,
        vpc_id: 'vpc-1',
        parameter_groups: ['default.aurora-mysql8.0'],
        cluster_id: 'aurora-cluster-1',
        tags: {},
        cost_monthly: 0,
        launch_time: '2026-01-01T00:00:00Z',
      },
      'ap-northeast-1',
    );
    expect(row.clusterId).toBe('aurora-cluster-1');
  });

  it('cluster_id が空文字のとき clusterId も空文字になる', () => {
    const row = rdsFromRaw(
      {
        id: 'db-2',
        name: 'db-2',
        state: 'available',
        engine: 'mysql',
        engine_version: '8.0.35',
        class: 'db.t3.micro',
        multi_az: false,
        endpoint: 'db-2.abc.ap-northeast-1.rds.amazonaws.com',
        port: 3306,
        vpc_id: 'vpc-1',
        parameter_groups: ['default.mysql8.0'],
        cluster_id: '',
        tags: {},
        cost_monthly: 0,
        launch_time: '2026-01-01T00:00:00Z',
      },
      'ap-northeast-1',
    );
    expect(row.clusterId).toBe('');
  });
});

describe('rdsParameterFromRaw', () => {
  it('snake_case を camelCase に変換し name から id を導出する', () => {
    const row = rdsParameterFromRaw({
      name: 'max_connections',
      value: '100',
      allowed_values: '1-16384',
      apply_type: 'dynamic',
      data_type: 'integer',
      source: 'user',
      is_modifiable: true,
      description: 'maximum number of connections',
    });
    expect(row).toEqual({
      id: 'max_connections',
      name: 'max_connections',
      value: '100',
      allowedValues: '1-16384',
      applyType: 'dynamic',
      dataType: 'integer',
      source: 'user',
      isModifiable: true,
      description: 'maximum number of connections',
    });
  });
});

describe('rdsClusterParameterGroupFromRaw', () => {
  it('group_name を groupName に変換し parameters を Row に正規化する', () => {
    const group = rdsClusterParameterGroupFromRaw({
      group_name: 'default.aurora-mysql8.0',
      parameters: [
        {
          name: 'binlog_format',
          value: 'ROW',
          allowed_values: '',
          apply_type: 'static',
          data_type: 'string',
          source: 'user',
          is_modifiable: true,
          description: '',
        },
      ],
    });
    expect(group.groupName).toBe('default.aurora-mysql8.0');
    expect(group.parameters).toHaveLength(1);
    expect(group.parameters[0]).toMatchObject({ id: 'binlog_format', applyType: 'static' });
  });

  it('parameters が null のときは空配列に正規化する', () => {
    const group = rdsClusterParameterGroupFromRaw({ group_name: 'pg', parameters: null });
    expect(group.parameters).toEqual([]);
  });
});

describe('cacheFromRaw', () => {
  it('replication_group_id を replicationGroupId に変換する', () => {
    const row = cacheFromRaw(
      {
        id: 'cc-1',
        name: 'cc-1',
        state: 'available',
        engine: 'redis',
        engine_version: '7.1',
        node_type: 'cache.r6g.large',
        num_nodes: 1,
        endpoint: 'cc-1.abc.apne1.cache.amazonaws.com',
        port: 6379,
        parameter_group: 'default.redis7',
        replication_group_id: 'my-redis-rg',
        node_availability_zones: ['ap-northeast-1a'],
        cost_monthly: 0,
      },
      'ap-northeast-1',
    );
    expect(row.replicationGroupId).toBe('my-redis-rg');
  });

  it('replication_group_id が空文字のとき replicationGroupId も空文字になる', () => {
    const row = cacheFromRaw(
      {
        id: 'cc-2',
        name: 'cc-2',
        state: 'available',
        engine: 'memcached',
        engine_version: '1.6.22',
        node_type: 'cache.t4g.micro',
        num_nodes: 3,
        endpoint: 'cc-2.abc.apne1.cache.amazonaws.com',
        port: 11211,
        parameter_group: 'default.memcached1.6',
        replication_group_id: '',
        node_availability_zones: null,
        cost_monthly: 0,
      },
      'ap-northeast-1',
    );
    expect(row.replicationGroupId).toBe('');
  });

  it('node_availability_zones を nodeAvailabilityZones にそのまま変換する', () => {
    const row = cacheFromRaw(
      {
        id: 'cc-3',
        name: 'cc-3',
        state: 'available',
        engine: 'valkey',
        engine_version: '8.0',
        node_type: 'cache.t4g.micro',
        num_nodes: 3,
        endpoint: 'cc-3.abc.apne1.cache.amazonaws.com',
        port: 6379,
        parameter_group: 'default.valkey8',
        replication_group_id: 'my-valkey-rg',
        node_availability_zones: ['ap-northeast-1a', 'ap-northeast-1c', 'ap-northeast-1a'],
        cost_monthly: 0,
      },
      'ap-northeast-1',
    );
    expect(row.nodeAvailabilityZones).toEqual([
      'ap-northeast-1a',
      'ap-northeast-1c',
      'ap-northeast-1a',
    ]);
  });

  it('node_availability_zones が null のとき nodeAvailabilityZones は空配列になる', () => {
    const row = cacheFromRaw(
      {
        id: 'cc-4',
        name: 'cc-4',
        state: 'creating',
        engine: 'redis',
        engine_version: '7.1',
        node_type: 'cache.t4g.micro',
        num_nodes: 1,
        endpoint: '',
        port: 0,
        parameter_group: 'default.redis7',
        replication_group_id: '',
        node_availability_zones: null,
        cost_monthly: 0,
      },
      'ap-northeast-1',
    );
    expect(row.nodeAvailabilityZones).toEqual([]);
  });
});

describe('cacheParameterFromRaw', () => {
  it('snake_case を camelCase に変換し name から id を導出する', () => {
    const row = cacheParameterFromRaw({
      name: 'maxmemory-policy',
      value: 'noeviction',
      allowed_values: 'volatile-lru,allkeys-lru,noeviction',
      change_type: 'immediate',
      data_type: 'string',
      source: 'system',
      is_modifiable: true,
      minimum_engine_version: '2.8.6',
      description: 'max memory eviction policy',
    });
    expect(row).toEqual({
      id: 'maxmemory-policy',
      name: 'maxmemory-policy',
      value: 'noeviction',
      allowedValues: 'volatile-lru,allkeys-lru,noeviction',
      changeType: 'immediate',
      dataType: 'string',
      source: 'system',
      isModifiable: true,
      minimumEngineVersion: '2.8.6',
      description: 'max memory eviction policy',
    });
  });
});

describe('objectPreviewFromRaw', () => {
  it('snake_case を camelCase に変換する', () => {
    const row = objectPreviewFromRaw({
      content: 'a,b\n1,2\n',
      content_type: 'text/csv',
      size: 8,
    });
    expect(row).toEqual({
      content: 'a,b\n1,2\n',
      contentType: 'text/csv',
      size: 8,
    });
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

describe('cwLogGroupFromRaw', () => {
  it('snake_case を camelCase に変換する', () => {
    expect(
      cwLogGroupFromRaw({
        name: '/aws/lambda/api',
        arn: 'arn:aws:logs:...:log-group:/aws/lambda/api',
        stored_bytes: 4096,
        retention_days: 30,
        creation_time: '2026-07-18T00:00:00Z',
      }),
    ).toEqual({
      name: '/aws/lambda/api',
      arn: 'arn:aws:logs:...:log-group:/aws/lambda/api',
      storedBytes: 4096,
      retentionDays: 30,
      creationTime: '2026-07-18T00:00:00Z',
    });
  });
});

describe('cwLogEventFromRaw', () => {
  it('snake_case を camelCase に変換し index 付きの id を作る', () => {
    const row = cwLogEventFromRaw(
      {
        timestamp: '2026-07-18T03:04:05Z',
        ingestion_time: '2026-07-18T03:04:06Z',
        message: 'ERROR boom',
        log_group: '/aws/lambda/api',
        log_stream: 'stream-1',
        event_id: 'evt-1',
      },
      3,
    );
    expect(row).toEqual({
      id: 'evt-1#3',
      timestamp: '2026-07-18T03:04:05Z',
      ingestionTime: '2026-07-18T03:04:06Z',
      message: 'ERROR boom',
      logGroup: '/aws/lambda/api',
      logStream: 'stream-1',
      eventId: 'evt-1',
    });
  });

  it('event_id が空なら timestamp を id のベースにする', () => {
    const row = cwLogEventFromRaw(
      {
        timestamp: '2026-07-18T03:04:05Z',
        ingestion_time: '',
        message: 'hi',
        log_group: '/g',
        log_stream: 's',
        event_id: '',
      },
      0,
    );
    expect(row.id).toBe('2026-07-18T03:04:05Z#0');
  });
});

describe('wafFromRaw', () => {
  const base: WAFRaw = {
    id: 'acl-1',
    name: 'edge-acl',
    state: 'active',
    scope: 'REGIONAL',
    description: 'Protects the public API',
    rule_count: 3,
    associated_count: 1,
    tags: { Env: 'prod' },
    cost_monthly: 0,
  };

  it('snake_case を camelCase に変換し description を写す', () => {
    const row = wafFromRaw(base, 'ap-northeast-1');
    expect(row).toEqual({
      region: 'ap-northeast-1',
      id: 'acl-1',
      name: 'edge-acl',
      state: 'active',
      scope: 'REGIONAL',
      description: 'Protects the public API',
      ruleCount: 3,
      associatedCount: 1,
      tags: { Env: 'prod' },
    });
  });

  it('description が欠落したレスポンスでは空文字に既定する', () => {
    const raw = { ...base, description: undefined as unknown as string };
    const row = wafFromRaw(raw, 'ap-northeast-1');
    expect(row.description).toBe('');
  });
});

describe('wafRuleFromRaw', () => {
  it('フィールドを写し、ルール名を DataTable の行キー (id) に使う', () => {
    const row = wafRuleFromRaw({
      name: 'rate-limit',
      priority: 1,
      action: 'Block',
      statement: 'RateBased',
      rule_json: '{"Name":"rate-limit"}',
    });
    expect(row).toEqual({
      id: 'rate-limit',
      name: 'rate-limit',
      priority: 1,
      action: 'Block',
      statement: 'RateBased',
      ruleJson: '{"Name":"rate-limit"}',
    });
  });

  it('rule_json 欠落時は空文字に既定する', () => {
    const row = wafRuleFromRaw({
      name: 'rate-limit',
      priority: 1,
      action: 'Block',
      statement: 'RateBased',
      rule_json: undefined as unknown as string,
    });
    expect(row.ruleJson).toBe('');
  });
});

describe('cloudfrontFromRaw', () => {
  const base: CloudFrontRaw = {
    id: 'E123',
    name: 'test',
    state: 'deployed',
    domain_name: 'd123.cloudfront.net',
    aliases: null,
    origins: null,
    behaviors: null,
    enabled: true,
    price_class: 'PriceClass_All',
    cost_monthly: 0,
  };

  it('aliases が null のとき空配列になる', () => {
    const row = cloudfrontFromRaw(base, 'global');
    expect(row.aliases).toEqual([]);
  });

  it('aliases に値があれば写る', () => {
    const row = cloudfrontFromRaw(
      { ...base, aliases: ['example.com', 'www.example.com'] },
      'global',
    );
    expect(row.aliases).toEqual(['example.com', 'www.example.com']);
  });

  it('behaviors が null のとき空配列になる', () => {
    const row = cloudfrontFromRaw(base, 'global');
    expect(row.behaviors).toEqual([]);
  });

  it('behaviors の snake_case が camelCase に変換される', () => {
    const row = cloudfrontFromRaw(
      {
        ...base,
        behaviors: [
          {
            path_pattern: '/api/*',
            target_origin_id: 'origin-1',
            viewer_protocol_policy: 'https-only',
            allowed_methods: ['GET', 'HEAD'],
            compress: true,
            is_default: false,
          },
        ],
      },
      'global',
    );
    expect(row.behaviors[0]).toMatchObject({
      pathPattern: '/api/*',
      targetOriginId: 'origin-1',
      viewerProtocolPolicy: 'https-only',
      allowedMethods: ['GET', 'HEAD'],
      compress: true,
      isDefault: false,
    });
  });

  it('behaviors の order は 1 始まりの配列インデックスから導出され、id はその文字列になる', () => {
    const row = cloudfrontFromRaw(
      {
        ...base,
        behaviors: [
          {
            path_pattern: '/api/*',
            target_origin_id: 'origin-1',
            viewer_protocol_policy: 'https-only',
            allowed_methods: [],
            compress: false,
            is_default: false,
          },
          {
            path_pattern: '',
            target_origin_id: 'origin-default',
            viewer_protocol_policy: 'allow-all',
            allowed_methods: [],
            compress: false,
            is_default: true,
          },
        ],
      },
      'global',
    );
    expect(row.behaviors[0].order).toBe(1);
    expect(row.behaviors[0].id).toBe('1');
    expect(row.behaviors[1].order).toBe(2);
    expect(row.behaviors[1].id).toBe('2');
  });
});

describe('kinesisFromRaw', () => {
  it('mode を含む全フィールドを Row に変換する', () => {
    const row = kinesisFromRaw(
      {
        id: 'arn:aws:kinesis:ap-northeast-1:123:stream/foo',
        name: 'foo',
        state: 'active',
        mode: 'on-demand',
        shard_count: 4,
        retention_hours: 24,
        encryption_type: 'KMS',
        tags: {},
        cost_monthly: 0,
      },
      'ap-northeast-1',
    );
    expect(row).toEqual({
      region: 'ap-northeast-1',
      id: 'arn:aws:kinesis:ap-northeast-1:123:stream/foo',
      name: 'foo',
      state: 'active',
      mode: 'on-demand',
      shardCount: 4,
      retentionHours: 24,
      encryptionType: 'KMS',
      tags: {},
    });
  });
});
