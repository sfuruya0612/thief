import { describe, expect, it } from 'vitest';
import type { GcpProject } from '../types/gcp';
import type { Profile } from '../types/common';
import {
  awsPickerItems,
  formatSsoExpiry,
  gcpPickerItems,
  isExpiringSoon,
  profileAuthLabel,
  profileBadge,
  projectEnv,
} from './sessionMeta';

describe('projectEnv', () => {
  it.each([
    ['gumi-green-dev', 'dev'],
    ['foo_dev', 'dev'],
    ['gumi-green-stg', 'stg'],
    ['gumi-green-staging', 'stg'],
    ['gumi-green-prod', 'prod'],
    ['gumi-production', 'prod'],
    ['foodev', 'default'],
    ['devfoo', 'default'],
    ['gumi-sandbox', 'default'],
    ['prod', 'prod'],
  ] as const)('%s → %s', (id, want) => {
    expect(projectEnv(id)).toBe(want);
  });
});

describe('profileBadge', () => {
  const base: Profile = { name: 'p' };

  it('sso + valid は「SSO 有効」(ok)', () => {
    expect(profileBadge({ ...base, authType: 'sso', ssoStatus: 'valid' })).toEqual({
      label: 'SSO 有効',
      tone: 'ok',
    });
  });

  it('sso + expired は「期限切れ」(warn)', () => {
    expect(profileBadge({ ...base, authType: 'sso', ssoStatus: 'expired' })).toEqual({
      label: '期限切れ',
      tone: 'warn',
    });
  });

  it('sso + not_logged_in は「未ログイン」(warn)', () => {
    expect(profileBadge({ ...base, authType: 'sso', ssoStatus: 'not_logged_in' })).toEqual({
      label: '未ログイン',
      tone: 'warn',
    });
  });

  it('sso でも status 未定義 (設定不備等) ならバッジなし', () => {
    expect(profileBadge({ ...base, authType: 'sso' })).toBeNull();
  });

  it('access_key はバッジなし (未検証情報を「有効」と出さない)', () => {
    expect(profileBadge({ ...base, authType: 'access_key' })).toBeNull();
  });

  it('authType 未取得 (旧 backend) はバッジなし', () => {
    expect(profileBadge(base)).toBeNull();
  });
});

describe('profileAuthLabel', () => {
  it.each([
    ['sso', 'SSO'],
    ['access_key', 'アクセスキー'],
    ['assume_role', 'AssumeRole'],
    ['credential_process', 'credential_process'],
    ['unknown', ''],
    [undefined, ''],
  ] as const)('%s → %s', (authType, want) => {
    expect(profileAuthLabel({ name: 'p', authType })).toBe(want);
  });
});

describe('isExpiringSoon', () => {
  const now = new Date('2026-07-17T12:00:00Z');

  it('30 分以上先は false', () => {
    expect(isExpiringSoon('2026-07-17T12:31:00Z', now)).toBe(false);
  });

  it('30 分未満は true', () => {
    expect(isExpiringSoon('2026-07-17T12:29:00Z', now)).toBe(true);
  });

  it('過去も true', () => {
    expect(isExpiringSoon('2026-07-17T11:00:00Z', now)).toBe(true);
  });

  it('パース不能は false', () => {
    expect(isExpiringSoon('oops', now)).toBe(false);
  });
});

describe('formatSsoExpiry', () => {
  const now = new Date('2026-07-17T12:00:00Z');

  it('時間と分を表示する', () => {
    expect(formatSsoExpiry('2026-07-17T14:05:00Z', now)).toBe('残り 2 時間 5 分');
  });

  it('1 時間未満は分のみ', () => {
    expect(formatSsoExpiry('2026-07-17T12:45:00Z', now)).toBe('残り 45 分');
  });

  it('過去は「期限切れ」', () => {
    expect(formatSsoExpiry('2026-07-17T11:59:00Z', now)).toBe('期限切れ');
  });

  it('パース不能は空文字', () => {
    expect(formatSsoExpiry('oops', now)).toBe('');
  });
});

describe('awsPickerItems', () => {
  const profiles: Profile[] = [
    {
      name: 'sso-prof',
      accountId: '111111111111',
      ssoRoleName: 'AdministratorAccess',
      region: 'ap-northeast-1',
      authType: 'sso',
      ssoStatus: 'valid',
    },
    { name: 'static', authType: 'access_key' },
    { name: 'plain' },
  ];

  it('開設済みプロファイルは disabled になる', () => {
    const items = awsPickerItems(profiles, ['static']);
    expect(items.map((i) => i.disabled)).toEqual([false, true, false]);
  });

  it('meta は region と認証方式を連結する', () => {
    const items = awsPickerItems(profiles, []);
    expect(items[0].meta).toBe('ap-northeast-1 · SSO');
    expect(items[1].meta).toBe('アクセスキー');
    expect(items[2].meta).toBeUndefined();
  });

  it('searchText は name / accountId / role を含む小文字文字列', () => {
    const items = awsPickerItems(profiles, []);
    expect(items[0].searchText).toContain('sso-prof');
    expect(items[0].searchText).toContain('111111111111');
    expect(items[0].searchText).toContain('administratoraccess');
  });

  it('SSO バッジが載る', () => {
    const items = awsPickerItems(profiles, []);
    expect(items[0].badge).toEqual({ label: 'SSO 有効', tone: 'ok' });
    expect(items[1].badge).toBeNull();
  });
});

describe('gcpPickerItems', () => {
  const projects: GcpProject[] = [
    { id: 'gumi-dev', name: 'Gumi Dev', projectNumber: '1', state: 'ACTIVE', createTime: '' },
    { id: 'gumi-prod', name: 'gumi-prod', projectNumber: '2', state: 'ACTIVE', createTime: '' },
  ];

  it('開設済みプロジェクトは disabled になる', () => {
    const items = gcpPickerItems(projects, ['gumi-dev']);
    expect(items.map((i) => i.disabled)).toEqual([true, false]);
  });

  it('表示名が ID と同じなら meta を出さない', () => {
    const items = gcpPickerItems(projects, []);
    expect(items[0].meta).toBe('Gumi Dev');
    expect(items[1].meta).toBeUndefined();
  });

  it('バッジは常に null', () => {
    const items = gcpPickerItems(projects, []);
    expect(items.every((i) => i.badge === null)).toBe(true);
  });
});
