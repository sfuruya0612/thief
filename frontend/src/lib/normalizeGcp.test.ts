import { describe, expect, it } from 'vitest';
import {
  cloudRunResourceFromRaw,
  gcpProjectFromRaw,
  gcsBucketFromRaw,
  gcsObjectFromRaw,
} from './normalizeGcp';

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
