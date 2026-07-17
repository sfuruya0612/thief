import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerECSTasks } from './DrawerECSTasks';

function renderWithQC(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

function mockTasksResponse(enableExecuteCommand: boolean) {
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
        enable_execute_command: enableExecuteCommand,
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
            last_status: 'RUNNING',
            health_status: 'HEALTHY',
            reason: '',
            runtime_id: 'runtime-app',
            // backend は enable_execute_command && runtimeId!=='' で判定するため、
            // enableExecuteCommand が false なら実データも false になる
            exec_enabled: enableExecuteCommand,
          },
          {
            name: 'sidecar',
            image: 'sidecar:latest',
            last_status: 'RUNNING',
            health_status: 'HEALTHY',
            reason: '',
            runtime_id: '',
            exec_enabled: false,
          },
        ],
      },
    ],
  } as Response);
}

describe('DrawerECSTasks', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('exec 可否でボタンの活性/非活性が分かれ、クリックで対象を通知する', async () => {
    mockTasksResponse(true);
    const onExec = vi.fn();

    const { container } = renderWithQC(
      <DrawerECSTasks
        profile="test"
        region="ap-northeast-1"
        cluster="my-cluster"
        onExec={onExec}
      />,
    );

    await waitFor(() => {
      expect(container.querySelector('td .primary.truncate')).not.toBeNull();
    });

    // タスク行 (group セル) をクリックして詳細ペインを開く
    fireEvent.click(container.querySelector('td .primary.truncate')!);

    await waitFor(() => {
      expect(container.textContent).toContain('Containers (2)');
    });

    const execButtons = Array.from(container.querySelectorAll('button')).filter(
      (b) => b.textContent === 'Exec',
    );
    expect(execButtons).toHaveLength(2);

    const [appButton, sidecarButton] = execButtons;
    expect(appButton.disabled).toBe(false);
    expect(sidecarButton.disabled).toBe(true);

    fireEvent.click(appButton);
    expect(onExec).toHaveBeenCalledWith({
      taskArn: 'arn:aws:ecs:ap-northeast-1:123:task/my-cluster/abc',
      container: 'app',
    });
  });

  it('タスク全体で ECS Exec が無効な場合は理由をツールチップに示す', async () => {
    mockTasksResponse(false);

    const { container } = renderWithQC(
      <DrawerECSTasks profile="test" region="ap-northeast-1" cluster="my-cluster" />,
    );

    await waitFor(() => {
      expect(container.querySelector('td .primary.truncate')).not.toBeNull();
    });
    fireEvent.click(container.querySelector('td .primary.truncate')!);

    await waitFor(() => {
      expect(container.textContent).toContain('Containers (2)');
    });

    const execButtons = Array.from(container.querySelectorAll('button')).filter(
      (b) => b.textContent === 'Exec',
    );
    for (const b of execButtons) {
      expect(b.disabled).toBe(true);
      expect(b.title).toBe('このタスクは ECS Exec が有効になっていません');
    }
  });
});
