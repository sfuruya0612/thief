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
            cpu: '',
            memory: '',
            memory_reservation: '',
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
            cpu: '',
            memory: '',
            memory_reservation: '',
            exec_enabled: false,
          },
        ],
      },
    ],
  } as Response);
}

// コンテナ 1 つのタスクを返し、コンテナ単位の cpu / memory / memory_reservation だけを差し替える
function mockSingleContainerResponse(resource: {
  cpu: string;
  memory: string;
  memory_reservation: string;
}) {
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
        launch_type: 'EC2',
        enable_execute_command: false,
        container_names: ['app'],
        cpu: '',
        memory: '',
        started_at: '',
        stopped_at: '',
        stopped_reason: '',
        containers: [
          {
            name: 'app',
            image: 'app:latest',
            last_status: 'RUNNING',
            health_status: '',
            reason: '',
            runtime_id: '',
            exec_enabled: false,
            ...resource,
          },
        ],
      },
    ],
  } as Response);
}

// 詳細ペインを開き、Containers テーブル (h3 の直後の table) を返す
async function openContainersTable(container: HTMLElement): Promise<HTMLTableElement> {
  await waitFor(() => {
    expect(container.querySelector('td .primary.truncate')).not.toBeNull();
  });
  fireEvent.click(container.querySelector('td .primary.truncate')!);
  await waitFor(() => {
    expect(container.textContent).toContain('Containers (1)');
  });
  const h3 = Array.from(container.querySelectorAll('h3')).find((h) =>
    h.textContent?.startsWith('Containers'),
  )!;
  const table = h3.nextElementSibling;
  expect(table?.tagName).toBe('TABLE');
  return table as HTMLTableElement;
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
  describe('コンテナ単位の CPU / Memory 列', () => {
    // 見出しの順序: Name / Image / Last status / Health / CPU / Memory / Exit code / Reason / Exec
    const CPU_COL = 4;
    const MEMORY_COL = 5;

    it.each([
      {
        name: '値あり',
        resource: { cpu: '128', memory: '256', memory_reservation: '64' },
        cpu: '128',
        memory: '256 / 64',
      },
      {
        name: 'cpu が 0 (コンテナ定義で未指定)',
        resource: { cpu: '0', memory: '256', memory_reservation: '64' },
        cpu: '-',
        memory: '256 / 64',
      },
      {
        name: 'cpu が空文字列',
        resource: { cpu: '', memory: '256', memory_reservation: '64' },
        cpu: '-',
        memory: '256 / 64',
      },
      {
        name: 'ハードリミットのみ (ソフトが空文字列)',
        resource: { cpu: '128', memory: '256', memory_reservation: '' },
        cpu: '128',
        memory: '256',
      },
      {
        name: 'ハードリミットのみ (ソフトが 0)',
        resource: { cpu: '128', memory: '256', memory_reservation: '0' },
        cpu: '128',
        memory: '256',
      },
      {
        name: 'ソフトリミットのみ (ハードが空文字列)',
        resource: { cpu: '128', memory: '', memory_reservation: '64' },
        cpu: '128',
        memory: '64',
      },
      {
        name: 'ソフトリミットのみ (ハードが 0)',
        resource: { cpu: '128', memory: '0', memory_reservation: '64' },
        cpu: '128',
        memory: '64',
      },
      {
        name: 'Memory が両方未設定 (空と空)',
        resource: { cpu: '128', memory: '', memory_reservation: '' },
        cpu: '128',
        memory: '-',
      },
      {
        name: 'Memory が両方未設定 (0 と空)',
        resource: { cpu: '128', memory: '0', memory_reservation: '' },
        cpu: '128',
        memory: '-',
      },
    ])('$name', async ({ resource, cpu, memory }) => {
      mockSingleContainerResponse(resource);
      const { container } = renderWithQC(
        <DrawerECSTasks profile="test" region="ap-northeast-1" cluster="my-cluster" />,
      );
      const table = await openContainersTable(container);

      const headers = Array.from(table.querySelectorAll('thead th')).map((th) => th.textContent);
      expect(headers[CPU_COL]).toBe('CPU');
      expect(headers[MEMORY_COL]).toBe('Memory');

      const cells = table.querySelectorAll('tbody tr td');
      expect(cells[CPU_COL].textContent).toBe(cpu);
      expect(cells[MEMORY_COL].textContent).toBe(memory);
    });

    it('colgroup は 9 列で width の合計が変更前の 7 列と同じ 100% である', async () => {
      mockSingleContainerResponse({ cpu: '', memory: '', memory_reservation: '' });
      const { container } = renderWithQC(
        <DrawerECSTasks profile="test" region="ap-northeast-1" cluster="my-cluster" />,
      );
      const table = await openContainersTable(container);

      const cols = Array.from(table.querySelectorAll('colgroup col'));
      expect(cols).toHaveLength(9);
      expect(table.querySelectorAll('thead th')).toHaveLength(9);
      // 変更前は 18 + 26 + 12 + 12 + 7 + 13 + 12 = 100
      const total = cols.reduce(
        (sum, col) => sum + parseFloat((col as HTMLElement).style.width),
        0,
      );
      expect(total).toBe(100);
    });
  });
});
