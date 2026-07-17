import { describe, expect, it } from 'vitest';
import {
  cloudRunResourceFromRaw,
  gcpProjectFromRaw,
  groupIAMBindingsByMember,
  gcsBucketFromRaw,
  gcsObjectFromRaw,
  logEntryFromRaw,
  logSeverityLevel,
} from './normalizeGcp';
import type { IAMBindingRow } from '../types/gcp';

describe('gcpProjectFromRaw', () => {
  it('project_id / project_number / create_time を camelCase に変換する', () => {
    const row = gcpProjectFromRaw({
      project_id: 'my-project',
      name: 'My Project',
      project_number: '123456789',
      state: 'ACTIVE',
      create_time: '2024-01-01T00:00:00Z',
    });
    expect(row).toEqual({
      id: 'my-project',
      name: 'My Project',
      projectNumber: '123456789',
      state: 'ACTIVE',
      createTime: '2024-01-01T00:00:00Z',
    });
  });

  it('name が空なら project_id を name に流用する', () => {
    const row = gcpProjectFromRaw({
      project_id: 'p-1',
      name: '',
      project_number: '1',
      state: 'ACTIVE',
      create_time: '',
    });
    expect(row.name).toBe('p-1');
  });
});

describe('cloudRunResourceFromRaw', () => {
  it('kind/region/name の組で id を生成し camelCase に整える', () => {
    const row = cloudRunResourceFromRaw({
      name: 'api',
      kind: 'service',
      region: 'asia-northeast1',
      project_id: 'p-1',
      uri: 'https://api-xxx.a.run.app',
      create_time: '2024-01-01T00:00:00Z',
      update_time: '2024-02-01T00:00:00Z',
    });
    expect(row).toEqual({
      id: 'service/asia-northeast1/api',
      name: 'api',
      kind: 'service',
      region: 'asia-northeast1',
      projectId: 'p-1',
      uri: 'https://api-xxx.a.run.app',
      createTime: '2024-01-01T00:00:00Z',
      updateTime: '2024-02-01T00:00:00Z',
    });
  });
});

describe('gcsBucketFromRaw', () => {
  it('location を region と location の両方に設定する (FacetBar の region facet 用)', () => {
    const row = gcsBucketFromRaw({
      name: 'my-bucket',
      location: 'ASIA-NORTHEAST1',
      storage_class: 'STANDARD',
      create_time: '2024-01-01T00:00:00Z',
    });
    expect(row.id).toBe('my-bucket');
    expect(row.name).toBe('my-bucket');
    expect(row.region).toBe('ASIA-NORTHEAST1');
    expect(row.location).toBe('ASIA-NORTHEAST1');
    expect(row.storageClass).toBe('STANDARD');
    expect(row.createTime).toBe('2024-01-01T00:00:00Z');
  });
});

describe('groupIAMBindingsByMember', () => {
  function binding(overrides: Partial<IAMBindingRow>): IAMBindingRow {
    return {
      id: `${overrides.member}/${overrides.role}`,
      name: overrides.member ?? '',
      member: '',
      role: '',
      projectId: 'p-1',
      conditionTitle: '',
      ...overrides,
    };
  }

  it('同じメンバーの複数バインディングを 1 行にまとめ、roles に全ロールを並べる', () => {
    const bindings = [
      binding({ member: 'user:alice@example.com', role: 'roles/storage.objectCreator' }),
      binding({ member: 'user:alice@example.com', role: 'roles/bigquery.dataViewer' }),
      binding({ member: 'user:bob@example.com', role: 'roles/viewer' }),
    ];

    const rows = groupIAMBindingsByMember(bindings);

    expect(rows).toEqual([
      {
        id: 'user:alice@example.com',
        name: 'user:alice@example.com',
        member: 'user:alice@example.com',
        roles: ['roles/storage.objectCreator', 'roles/bigquery.dataViewer'],
        projectId: 'p-1',
      },
      {
        id: 'user:bob@example.com',
        name: 'user:bob@example.com',
        member: 'user:bob@example.com',
        roles: ['roles/viewer'],
        projectId: 'p-1',
      },
    ]);
  });

  it('同一メンバー・同一ロールの重複バインディングは 1 つに統合する', () => {
    const bindings = [
      binding({ member: 'user:alice@example.com', role: 'roles/viewer' }),
      binding({
        member: 'user:alice@example.com',
        role: 'roles/viewer',
        conditionTitle: 'expires',
      }),
    ];

    const rows = groupIAMBindingsByMember(bindings);

    expect(rows).toHaveLength(1);
    expect(rows[0].roles).toEqual(['roles/viewer']);
  });

  it('入力が空なら空配列を返す', () => {
    expect(groupIAMBindingsByMember([])).toEqual([]);
  });
});

describe('gcsObjectFromRaw', () => {
  it('bucket + name + index で一意な id を組み立てる', () => {
    const row = gcsObjectFromRaw(
      {
        name: 'path/to/file.txt',
        bucket: 'my-bucket',
        size: 1024,
        content_type: 'text/plain',
        updated: '2024-02-01T00:00:00Z',
        storage_class: 'STANDARD',
      },
      3,
    );
    expect(row).toEqual({
      id: 'my-bucket/path/to/file.txt#3',
      name: 'path/to/file.txt',
      bucket: 'my-bucket',
      size: 1024,
      contentType: 'text/plain',
      updated: '2024-02-01T00:00:00Z',
      storageClass: 'STANDARD',
    });
  });
});

describe('logEntryFromRaw', () => {
  it('snake_case を camelCase に変換し、insert_id + index で id を組み立てる', () => {
    const row = logEntryFromRaw(
      {
        timestamp: '2026-07-18T00:00:00Z',
        severity: 'Error',
        log_name: 'projects/p-1/logs/run.googleapis.com%2Fstderr',
        resource_type: 'cloud_run_revision',
        resource_labels: { service_name: 'api' },
        labels: { foo: 'bar' },
        payload: 'boom',
        insert_id: 'abc123',
        trace: 'projects/p-1/traces/xyz',
      },
      0,
    );
    expect(row).toEqual({
      id: 'abc123#0',
      timestamp: '2026-07-18T00:00:00Z',
      severity: 'Error',
      logName: 'projects/p-1/logs/run.googleapis.com%2Fstderr',
      resourceType: 'cloud_run_revision',
      resourceLabels: { service_name: 'api' },
      labels: { foo: 'bar' },
      payload: 'boom',
      insertId: 'abc123',
      trace: 'projects/p-1/traces/xyz',
    });
  });

  it('insert_id が空なら timestamp + index で id を組み立てる', () => {
    const row = logEntryFromRaw(
      {
        timestamp: '2026-07-18T00:00:00Z',
        severity: 'Info',
        log_name: 'projects/p-1/logs/l',
        resource_type: 'global',
        payload: 'hello',
        insert_id: '',
      },
      2,
    );
    expect(row.id).toBe('2026-07-18T00:00:00Z#2');
  });

  it('resource_labels/labels/trace 省略時は空オブジェクト/空文字にフォールバックする', () => {
    const row = logEntryFromRaw(
      {
        timestamp: '2026-07-18T00:00:00Z',
        severity: 'Info',
        log_name: 'projects/p-1/logs/l',
        resource_type: 'global',
        payload: 'hello',
        insert_id: 'i-1',
      },
      0,
    );
    expect(row.resourceLabels).toEqual({});
    expect(row.labels).toEqual({});
    expect(row.trace).toBe('');
  });
});

describe('logSeverityLevel', () => {
  it.each([
    ['Error', 'err'],
    ['Critical', 'err'],
    ['Alert', 'err'],
    ['Emergency', 'err'],
    ['Warning', 'warn'],
    ['Notice', 'warn'],
    ['Default', 'info'],
    ['Debug', 'info'],
    ['Info', 'info'],
    ['', 'info'],
  ] as const)('%s は %s に丸められる', (severity, want) => {
    expect(logSeverityLevel(severity)).toBe(want);
  });
});
