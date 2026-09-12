// 組織タブの表示と、未ログインの組織を開いたときの自動ログイン開始の検証 (issue 0168)。
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { DatadogOrgSessions } from '../../hooks/useDatadogOrgs';
import { DatadogOrgSessionTabs } from './DatadogOrgSessionTabs';

vi.mock('../../api/endpoints', () => ({
  getDatadogOrgs: vi.fn(),
  postDatadogLoginStart: vi.fn(),
  getDatadogLoginStatus: vi.fn(),
}));

import { getDatadogLoginStatus, postDatadogLoginStart } from '../../api/endpoints';

const mockedStart = vi.mocked(postDatadogLoginStart);
const mockedStatus = vi.mocked(getDatadogLoginStatus);

const startBody = {
  state: 'state-1',
  authorization_url: 'https://app.datadoghq.com/oauth2/v1/authorize?state=state-1',
};

function newSessions(overrides: Partial<DatadogOrgSessions> = {}): DatadogOrgSessions {
  return {
    orgs: [
      { id: 'abc123', name: 'Parent Org', loggedIn: true },
      { id: 'sub456', name: 'Sub Org', loggedIn: false },
    ],
    isLoading: false,
    isError: false,
    error: null,
    openOrgs: ['abc123', 'sub456'],
    activeOrg: 'abc123',
    activateOrg: vi.fn(),
    openOrg: vi.fn(),
    closeOrg: vi.fn(),
    moveOrg: vi.fn(),
    swapOrgToVisible: vi.fn(),
    ...overrides,
  };
}

function renderTabs(sessions: DatadogOrgSessions) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <DatadogOrgSessionTabs sessions={sessions} />
    </QueryClientProvider>,
  );
  return sessions;
}

// 認可タブの WindowProxy のモック。
function mockAuthWindow() {
  const authWindow = { closed: false, close: vi.fn(), location: { replace: vi.fn() } };
  vi.spyOn(window, 'open').mockReturnValue(authWindow as unknown as Window);
  return authWindow;
}

describe('DatadogOrgSessionTabs', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    mockedStart.mockReset();
    mockedStatus.mockReset();
    mockedStart.mockResolvedValue(startBody);
    mockedStatus.mockResolvedValue({ status: 'pending' });
  });

  it('組織の表示名をタブのラベルにする', () => {
    renderTabs(newSessions());
    const tabs = screen.getAllByRole('tab');
    expect(tabs.map((tab) => tab.textContent)).toEqual([
      expect.stringContaining('Parent Org'),
      expect.stringContaining('Sub Org'),
    ]);
    expect(tabs[0]).toHaveAttribute('aria-selected', 'true');
  });

  it('未ログインの組織のタブをクリックするとログインが自動的に始まる', async () => {
    const authWindow = mockAuthWindow();
    const sessions = renderTabs(newSessions());

    fireEvent.click(screen.getByText('Sub Org'));

    expect(sessions.activateOrg).toHaveBeenCalledWith('sub456');
    await waitFor(() => expect(mockedStart).toHaveBeenCalledWith('sub456'));
    expect(authWindow.location.replace).toHaveBeenCalledWith(startBody.authorization_url);
  });

  it('ログイン済みの組織のタブをクリックしてもログインを始めない', () => {
    mockAuthWindow();
    const sessions = renderTabs(newSessions({ activeOrg: 'sub456' }));

    fireEvent.click(screen.getByText('Parent Org'));

    expect(sessions.activateOrg).toHaveBeenCalledWith('abc123');
    expect(mockedStart).not.toHaveBeenCalled();
    expect(window.open).not.toHaveBeenCalled();
  });

  it('一覧に無い組織のタブをクリックしてもログインを始めない', () => {
    // ログイン状態が分からない組織で認可を始めると、ユーザが意図しない組織の
    // 認可画面を開くことになる。
    mockAuthWindow();
    const sessions = renderTabs(
      newSessions({ orgs: [{ id: 'abc123', name: 'Parent Org', loggedIn: true }] }),
    );

    fireEvent.click(screen.getByText('sub456'));

    expect(sessions.activateOrg).toHaveBeenCalledWith('sub456');
    expect(mockedStart).not.toHaveBeenCalled();
  });

  it('ピッカーから未ログインの組織を選ぶとタブを開いてログインを始める', async () => {
    mockAuthWindow();
    const sessions = renderTabs(newSessions({ openOrgs: ['abc123'], activeOrg: 'abc123' }));

    fireEvent.click(screen.getByRole('button', { name: '＋ 組織を追加' }));
    fireEvent.click(screen.getByText('Sub Org'));

    expect(sessions.openOrg).toHaveBeenCalledWith('sub456');
    await waitFor(() => expect(mockedStart).toHaveBeenCalledWith('sub456'));
  });

  it('ピッカーで未ログインの組織に未ログインバッジを出す', () => {
    renderTabs(newSessions({ openOrgs: ['abc123'], activeOrg: 'abc123' }));

    fireEvent.click(screen.getByRole('button', { name: '＋ 組織を追加' }));

    expect(screen.getByText('未ログイン')).toBeInTheDocument();
  });

  it('ログインの進行中に同じ組織を再度クリックしてもログインを重ねて始めない', async () => {
    mockAuthWindow();
    renderTabs(newSessions());

    fireEvent.click(screen.getByText('Sub Org'));
    await waitFor(() => expect(mockedStart).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByText('Sub Org'));
    expect(mockedStart).toHaveBeenCalledTimes(1);
  });
});
