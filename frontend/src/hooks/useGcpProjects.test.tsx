import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { GcpProjectRaw } from '../types/gcp';
import { STORAGE_KEY } from '../lib/storage';
import { useActiveGcpProject } from './useGcpProjects';

vi.mock('../api/endpoints', () => ({
  getGcpProjects: vi.fn(),
}));

import { getGcpProjects } from '../api/endpoints';

const mockedGetGcpProjects = vi.mocked(getGcpProjects);

const raw = (id: string): GcpProjectRaw => ({
  project_id: id,
  name: id,
  project_number: '1',
  state: 'ACTIVE',
  create_time: '',
});

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe('useActiveGcpProject', () => {
  beforeEach(() => {
    localStorage.clear();
    mockedGetGcpProjects.mockReset();
  });

  it('初回ロードで開いているタブが無ければ先頭を自動オープンする', async () => {
    mockedGetGcpProjects.mockResolvedValue([raw('proj-a'), raw('proj-b')]);
    const { result } = renderHook(() => useActiveGcpProject(), { wrapper });

    await waitFor(() => expect(result.current.activeProject).toBe('proj-a'));
    expect(result.current.openProjects).toEqual(['proj-a']);
  });

  it('一覧に無い active プロジェクトを先頭へフォールバックしない (回帰防止)', async () => {
    // 旧実装は「永続値が一覧に無ければ先頭を採用」しており、refresh で
    // プロジェクトが消えるとアクティブタブが勝手に飛んでいた。
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ gcpSessions: { open: ['gone-project'], active: 'gone-project' } }),
    );
    mockedGetGcpProjects.mockResolvedValue([raw('proj-a')]);
    const { result } = renderHook(() => useActiveGcpProject(), { wrapper });

    await waitFor(() => expect(result.current.projects.length).toBe(1));
    expect(result.current.activeProject).toBe('gone-project');
    expect(result.current.openProjects).toEqual(['gone-project']);
  });
});
