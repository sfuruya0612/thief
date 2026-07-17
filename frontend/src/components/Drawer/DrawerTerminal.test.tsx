import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerTerminal } from './DrawerTerminal';

function renderWithQC(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('DrawerTerminal (ECS)', () => {
  const originalFetch = globalThis.fetch;
  const originalMatchMedia = globalThis.matchMedia;

  beforeEach(() => {
    globalThis.fetch = vi.fn();
    // xterm.js の CoreBrowserService が DPR 更新のために matchMedia を呼ぶ。jsdom は未実装のため
    // テスト用の no-op スタブを与える。
    globalThis.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
    }) as unknown as typeof globalThis.matchMedia;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    globalThis.matchMedia = originalMatchMedia;
    vi.restoreAllMocks();
  });

  it('事前選択された execTarget がある場合はドロップダウンを出さず直接ターミナルを描画する', () => {
    const { container } = renderWithQC(
      <DrawerTerminal
        service="ecs"
        profile="test"
        region="ap-northeast-1"
        resource={{ id: 'my-cluster', name: 'my-cluster' }}
        execTarget={{
          taskArn: 'arn:aws:ecs:ap-northeast-1:123:task/my-cluster/abc',
          container: 'app',
        }}
      />,
    );

    // タスク一覧 API を叩かず (fetch 未呼び出し)、ドロップダウンも出さずに直接ターミナルへ進む
    expect(globalThis.fetch).not.toHaveBeenCalled();
    expect(container.querySelectorAll('select')).toHaveLength(0);
    expect(container.querySelector('.terminal-container')).not.toBeNull();
  });

  it('execTarget が無い場合は従来どおりタスク一覧を取得してドロップダウンで選択させる', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: async () => [
        {
          arn: 'arn:aws:ecs:ap-northeast-1:123:task/my-cluster/abc',
          group: 'service:my-svc',
          last_status: 'RUNNING',
          desired_status: 'RUNNING',
          launch_type: 'FARGATE',
          enable_execute_command: true,
          container_names: ['app'],
          cpu: '',
          memory: '',
          started_at: '2026-07-08T00:00:00Z',
          stopped_at: '',
          stopped_reason: '',
          containers: [],
        },
        {
          arn: 'arn:aws:ecs:ap-northeast-1:123:task/my-cluster/def',
          group: 'service:my-svc',
          last_status: 'RUNNING',
          desired_status: 'RUNNING',
          launch_type: 'FARGATE',
          enable_execute_command: true,
          container_names: ['app'],
          cpu: '',
          memory: '',
          started_at: '2026-07-08T00:00:10Z',
          stopped_at: '',
          stopped_reason: '',
          containers: [],
        },
      ],
    } as Response);

    const { container } = renderWithQC(
      <DrawerTerminal
        service="ecs"
        profile="test"
        region="ap-northeast-1"
        resource={{ id: 'my-cluster', name: 'my-cluster' }}
      />,
    );

    await waitFor(() => {
      expect(container.querySelectorAll('select').length).toBeGreaterThan(0);
    });

    const taskSelect = container.querySelectorAll('select')[0];
    const optionTexts = Array.from(taskSelect.querySelectorAll('option')).map((o) => o.textContent);
    expect(optionTexts).toContain('service:my-svc / abc (2026-07-08T00:00:00Z)');
    expect(optionTexts).toContain('service:my-svc / def (2026-07-08T00:00:10Z)');
  });
});
