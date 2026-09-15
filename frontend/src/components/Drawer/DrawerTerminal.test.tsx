// DrawerTerminal は Terminal を描画せず、常駐ターミナルドックへセッションを開く
// ランチャーだけを担う (issue 0174 の設計判断 4)。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerTerminal } from './DrawerTerminal';
import {
  resetTerminalSessionsForTest,
  useTerminalSessions,
  type TerminalSessionsState,
} from '../../hooks/useTerminalSessions';

function renderWithQC(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

function storeState(): TerminalSessionsState {
  return renderHook(() => useTerminalSessions()).result.current;
}

function connectButton(container: HTMLElement): HTMLButtonElement {
  const button = Array.from(container.querySelectorAll('button')).find((b) =>
    (b.textContent ?? '').includes('接続'),
  );
  expect(button).not.toBeUndefined();
  return button as HTMLButtonElement;
}

const TASKS_RESPONSE = [
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
];

const CONTAINERS_RESPONSE = [
  { name: 'app', image: '', last_status: 'RUNNING', health_status: '', exec_enabled: true },
];

describe('DrawerTerminal (ランチャー)', () => {
  const originalFetch = globalThis.fetch;
  const originalMatchMedia = globalThis.matchMedia;

  beforeEach(() => {
    resetTerminalSessionsForTest();
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

  it('EC2 では Connect ボタンの押下でストアに kind: ec2 のセッションが 1 つ追加される', () => {
    const { container } = renderWithQC(
      <DrawerTerminal
        service="ec2"
        profile="test-profile"
        region="ap-northeast-1"
        resource={{ id: 'i-0123456789abcdef0', name: 'web-01' }}
      />,
    );

    expect(container.querySelector('.terminal-container')).toBeNull();

    fireEvent.click(connectButton(container));

    const state = storeState();
    expect(state.tabs.open).toHaveLength(1);
    const session = state.sessions[state.tabs.active];
    expect(session.kind).toBe('ec2');
    expect(session.label).toBe('web-01');
    expect(session.wsUrl).toContain('/ec2/i-0123456789abcdef0/session');
    // ターミナル本体はドック側にしかマウントされない
    expect(container.querySelector('.terminal-container')).toBeNull();
  });

  it('ECS ではタスクとコンテナが確定するまで Connect が disabled で、押下で kind: ecs のセッションが追加される', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockImplementation(
      (input: RequestInfo | URL) => {
        const url = String(input);
        const body = url.includes('/containers') ? CONTAINERS_RESPONSE : TASKS_RESPONSE;
        return Promise.resolve({
          ok: true,
          status: 200,
          statusText: 'OK',
          json: async () => body,
        } as Response);
      },
    );

    const { container } = renderWithQC(
      <DrawerTerminal
        service="ecs"
        profile="test-profile"
        region="ap-northeast-1"
        resource={{ id: 'my-cluster', name: 'my-cluster' }}
      />,
    );

    // タスクが複数あるため未選択のまま。Connect は押せない
    await waitFor(() => {
      expect(container.querySelectorAll('select').length).toBeGreaterThan(0);
    });
    expect(connectButton(container).disabled).toBe(true);

    const taskSelect = container.querySelectorAll('select')[0];
    const optionTexts = Array.from(taskSelect.querySelectorAll('option')).map((o) => o.textContent);
    expect(optionTexts).toContain('service:my-svc / abc (2026-07-08T00:00:00Z)');
    expect(optionTexts).toContain('service:my-svc / def (2026-07-08T00:00:10Z)');

    fireEvent.change(taskSelect, {
      target: { value: 'arn:aws:ecs:ap-northeast-1:123:task/my-cluster/abc' },
    });

    // Exec 可能なコンテナが 1 つだけなら自動選択され、Connect が有効になる
    await waitFor(() => {
      expect(connectButton(container).disabled).toBe(false);
    });

    fireEvent.click(connectButton(container));

    const state = storeState();
    expect(state.tabs.open).toHaveLength(1);
    const session = state.sessions[state.tabs.active];
    expect(session.kind).toBe('ecs');
    expect(session.label).toBe('my-cluster / abc / app');
    expect(session.wsUrl).toContain('/ecs/my-cluster/tasks/abc/exec');
    expect(container.querySelector('.terminal-container')).toBeNull();
  });

  it('タスクが 0 件のときはヒントだけを出しターミナルを描画しない', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: async () => [],
    } as Response);

    const { container } = renderWithQC(
      <DrawerTerminal
        service="ecs"
        profile="test-profile"
        region="ap-northeast-1"
        resource={{ id: 'my-cluster', name: 'my-cluster' }}
      />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('実行中のタスクがありません');
    });
    expect(container.querySelector('.terminal-container')).toBeNull();
    expect(storeState().tabs.open).toHaveLength(0);
  });

  it('対象外のサービスでは何も描画しない', () => {
    const { container } = renderWithQC(
      <DrawerTerminal
        service="s3"
        profile="test-profile"
        region="ap-northeast-1"
        resource={{ id: 'bucket', name: 'bucket' }}
      />,
    );

    expect(container.querySelector('.terminal-container')).toBeNull();
    expect(container.textContent).toBe('');
  });
});
