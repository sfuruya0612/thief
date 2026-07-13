import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ProfileSelect } from './ProfileSelect';
import type { Profile } from '../types/common';

// テスト間で QueryClient を独立させるためのラッパー
function renderWithQC(ui: React.ReactElement) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

const PROFILES: Profile[] = [
  { name: 'prod-admin', accountId: '111111111111', ssoRoleName: 'AdministratorAccess' },
  { name: 'stg-readonly', accountId: '222222222222', ssoRoleName: 'ReadOnlyAccess' },
  { name: 'dev-poweruser', accountId: '333333333333', ssoRoleName: 'PowerUserAccess' },
];

describe('ProfileSelect', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    // useProfileIdentity (STS 補完) は選択中プロファイルに対して発火するが、
    // このテストでは検索/選択の挙動のみを見たいので解決させないままにする。
    globalThis.fetch = vi.fn(() => new Promise(() => {})) as unknown as typeof fetch;
  });

  afterEach(() => {
    cleanup();
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('トリガーに選択中プロファイルの名前と Account ID / 権限名を表示する', () => {
    renderWithQC(
      <ProfileSelect profile="prod-admin" profiles={PROFILES} onProfileChange={() => {}} />,
    );
    expect(screen.getByText('prod-admin')).toBeInTheDocument();
    expect(screen.getByText('111111111111')).toBeInTheDocument();
    expect(screen.getByText('AdministratorAccess')).toBeInTheDocument();
  });

  it('トリガークリックで検索ボックスと候補一覧が開く', () => {
    renderWithQC(
      <ProfileSelect profile="prod-admin" profiles={PROFILES} onProfileChange={() => {}} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /prod-admin/ }));
    expect(screen.getByPlaceholderText('filter by name, account id, role…')).toBeInTheDocument();
    expect(screen.getAllByRole('option')).toHaveLength(3);
  });

  it('name で絞り込める', () => {
    renderWithQC(
      <ProfileSelect profile="prod-admin" profiles={PROFILES} onProfileChange={() => {}} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /prod-admin/ }));
    fireEvent.change(screen.getByPlaceholderText('filter by name, account id, role…'), {
      target: { value: 'stg' },
    });
    const options = screen.getAllByRole('option');
    expect(options).toHaveLength(1);
    expect(options[0]).toHaveTextContent('stg-readonly');
  });

  it('accountId で絞り込める', () => {
    renderWithQC(
      <ProfileSelect profile="prod-admin" profiles={PROFILES} onProfileChange={() => {}} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /prod-admin/ }));
    fireEvent.change(screen.getByPlaceholderText('filter by name, account id, role…'), {
      target: { value: '333333333333' },
    });
    const options = screen.getAllByRole('option');
    expect(options).toHaveLength(1);
    expect(options[0]).toHaveTextContent('dev-poweruser');
  });

  it('ssoRoleName で絞り込める', () => {
    renderWithQC(
      <ProfileSelect profile="prod-admin" profiles={PROFILES} onProfileChange={() => {}} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /prod-admin/ }));
    fireEvent.change(screen.getByPlaceholderText('filter by name, account id, role…'), {
      target: { value: 'ReadOnly' },
    });
    const options = screen.getAllByRole('option');
    expect(options).toHaveLength(1);
    expect(options[0]).toHaveTextContent('stg-readonly');
  });

  it('候補をクリックすると onProfileChange が呼ばれメニューが閉じる', () => {
    const onProfileChange = vi.fn();
    renderWithQC(
      <ProfileSelect profile="prod-admin" profiles={PROFILES} onProfileChange={onProfileChange} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /prod-admin/ }));
    fireEvent.click(screen.getByText('stg-readonly'));
    expect(onProfileChange).toHaveBeenCalledWith('stg-readonly');
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
  });

  it('Enter キーで active な候補を確定する', () => {
    const onProfileChange = vi.fn();
    renderWithQC(
      <ProfileSelect profile="prod-admin" profiles={PROFILES} onProfileChange={onProfileChange} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /prod-admin/ }));
    const input = screen.getByPlaceholderText('filter by name, account id, role…');
    fireEvent.keyDown(input, { key: 'ArrowDown' });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onProfileChange).toHaveBeenCalledWith('stg-readonly');
  });

  it('Escape キーでメニューを閉じる', () => {
    renderWithQC(
      <ProfileSelect profile="prod-admin" profiles={PROFILES} onProfileChange={() => {}} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /prod-admin/ }));
    const input = screen.getByPlaceholderText('filter by name, account id, role…');
    fireEvent.keyDown(input, { key: 'Escape' });
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
  });

  it('外側クリックでメニューを閉じる', async () => {
    renderWithQC(
      <div>
        <div data-testid="outside" />
        <ProfileSelect profile="prod-admin" profiles={PROFILES} onProfileChange={() => {}} />
      </div>,
    );
    fireEvent.click(screen.getByRole('button', { name: /prod-admin/ }));
    expect(screen.getByRole('listbox')).toBeInTheDocument();
    fireEvent.pointerDown(screen.getByTestId('outside'));
    await waitFor(() => {
      expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    });
  });

  it('一致する候補がない場合は空状態を表示する', () => {
    renderWithQC(
      <ProfileSelect profile="prod-admin" profiles={PROFILES} onProfileChange={() => {}} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /prod-admin/ }));
    fireEvent.change(screen.getByPlaceholderText('filter by name, account id, role…'), {
      target: { value: 'no-such-profile' },
    });
    expect(screen.getByText('No profiles match')).toBeInTheDocument();
  });
});
