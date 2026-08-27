import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerECSContainerInstances } from './DrawerECSContainerInstances';
import { DrawerECSTasks } from './DrawerECSTasks';

const CI_ARN_1 = 'arn:aws:ecs:ap-northeast-1:123:container-instance/my-cluster/ci-1';
const CI_ARN_2 = 'arn:aws:ecs:ap-northeast-1:123:container-instance/my-cluster/ci-2';
const CI_ARN_GONE = 'arn:aws:ecs:ap-northeast-1:123:container-instance/my-cluster/ci-gone';

function newQC() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

function renderWithQC(ui: React.ReactElement, qc = newQC()) {
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

function instance(over: Partial<Record<string, unknown>> = {}) {
  return {
    arn: CI_ARN_1,
    ec2_instance_id: 'i-0000000000000001',
    status: 'active',
    agent_connected: true,
    running_tasks_count: 2,
    pending_tasks_count: 0,
    registered_cpu: 2048,
    registered_memory: 3900,
    remaining_cpu: 1024,
    remaining_memory: 1500,
    ...over,
  };
}

function task(over: Partial<Record<string, unknown>> = {}) {
  return {
    arn: `arn:aws:ecs:ap-northeast-1:123:task/my-cluster/${String(over.group ?? 'x')}`,
    group: 'service:svc-a',
    last_status: 'running',
    desired_status: 'running',
    launch_type: 'EC2',
    enable_execute_command: false,
    container_names: ['app'],
    cpu: '256',
    memory: '512',
    started_at: '2026-08-01T00:00:00Z',
    stopped_at: '',
    stopped_reason: '',
    container_instance_arn: CI_ARN_1,
    containers: [],
    ...over,
  };
}

type Resp = { status: number; body: unknown };
const ok = (body: unknown): Resp => ({ status: 200, body });
const forbidden: Resp = {
  status: 403,
  body: { error: 'access denied', code: 'ACCESS_DENIED' },
};
const serverError: Resp = {
  status: 500,
  body: { error: 'internal error', code: 'INTERNAL' },
};

// URL に応じて container-instances と tasks のレスポンスを返す fetch モック。
// pendingTasks を true にすると tasks の応答を返さず loading のままにする。
function mockFetch(instances: Resp | null, tasks: Resp | null) {
  return vi.fn((input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString();
    const r = url.includes('/container-instances') ? instances : tasks;
    if (r === null) return new Promise<Response>(() => {});
    return Promise.resolve({
      ok: r.status < 400,
      status: r.status,
      statusText: r.status === 200 ? 'OK' : 'Error',
      json: async () => r.body,
    } as Response);
  });
}

function headings(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll('h3')).map((h) => h.textContent ?? '');
}

function sections(container: HTMLElement) {
  return Array.from(container.querySelectorAll('[data-testid="ecs-container-instance"]'));
}

const originalFetch = globalThis.fetch;

describe('DrawerECSContainerInstances', () => {
  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });
  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it('2 インスタンス 3 タスクをコンテナインスタンスごとにグルーピングして表示する', async () => {
    globalThis.fetch = mockFetch(
      ok([
        instance(),
        instance({ arn: CI_ARN_2, ec2_instance_id: 'i-0000000000000002', running_tasks_count: 1 }),
      ]),
      ok([
        task({ group: 'service:svc-a' }),
        task({ group: 'service:svc-b' }),
        task({ group: 'service:svc-c', container_instance_arn: CI_ARN_2 }),
      ]),
    );
    const { container } = renderWithQC(
      <DrawerECSContainerInstances profile="p" region="ap-northeast-1" cluster="my-cluster" />,
    );
    await waitFor(() => {
      expect(sections(container)).toHaveLength(2);
    });
    const [s1, s2] = sections(container);
    expect(s1.querySelector('h3')?.textContent).toContain('i-0000000000000001');
    expect(s1.textContent).toContain('タスク (2)');
    expect(s1.textContent).toContain('service:svc-a');
    expect(s1.textContent).toContain('service:svc-b');
    expect(s1.textContent).not.toContain('service:svc-c');
    expect(s2.querySelector('h3')?.textContent).toContain('i-0000000000000002');
    expect(s2.textContent).toContain('タスク (1)');
    expect(s2.textContent).toContain('service:svc-c');
    // ECS が報告する件数と実際に並べた件数は別のラベルで出す
    expect(s1.textContent).toContain('ECS reported');
    expect(s1.textContent).toContain('実行中 2 / 保留 0');
    expect(s1.textContent).toContain('接続中');
    // CPU / Memory は 残り / 登録
    expect(s1.textContent).toContain('1024 / 2048');
    expect(s1.textContent).toContain('1500 / 3900');
    // タスク行: group / lastStatus / cpu / memory / startedAt
    const row = s1.querySelector('tbody tr')!;
    expect(row.textContent).toContain('2026-08-01T00:00:00Z');
    expect(row.textContent).toContain('256');
    expect(row.textContent).toContain('512');
    expect(row.querySelector('span.status')?.className).toBe('status ok');
    expect(container.querySelector('h3')?.textContent).toBe('コンテナインスタンス (2)');
    expect(container.textContent).not.toContain('不明なコンテナインスタンス');
  });

  it('draining は warn、registration-failed は err のバッジで表示する', async () => {
    globalThis.fetch = mockFetch(
      ok([
        instance({ status: 'draining' }),
        instance({ arn: CI_ARN_2, ec2_instance_id: 'i-2', status: 'registration-failed' }),
      ]),
      ok([]),
    );
    const { container } = renderWithQC(
      <DrawerECSContainerInstances profile="p" region="r" cluster="c" />,
    );
    await waitFor(() => {
      expect(sections(container)).toHaveLength(2);
    });
    const [s1, s2] = sections(container);
    expect(s1.querySelector('h3 span.status')?.className).toBe('status warn');
    expect(s1.querySelector('h3 span.status')?.textContent).toBe('draining');
    expect(s2.querySelector('h3 span.status')?.className).toBe('status err');
    expect(s2.querySelector('h3 span.status')?.textContent).toBe('registration-failed');
    // タスクが無いインスタンスは空行を出す
    expect(s1.textContent).toContain('タスク (0)');
    expect(s1.textContent).toContain('タスクがありません');
  });

  it('registered_cpu が null のときは - を表示し、agent 切断も表示する', async () => {
    globalThis.fetch = mockFetch(
      ok([
        instance({
          registered_cpu: null,
          remaining_cpu: null,
          remaining_memory: null,
          agent_connected: false,
        }),
      ]),
      ok([]),
    );
    const { container } = renderWithQC(
      <DrawerECSContainerInstances profile="p" region="r" cluster="c" />,
    );
    await waitFor(() => {
      expect(sections(container)).toHaveLength(1);
    });
    const s1 = sections(container)[0];
    // CPU は残り / 登録の両方が null、Memory は残りだけ null
    expect(s1.textContent).toContain('- / -');
    expect(s1.textContent).toContain('- / 3900');
    expect(s1.textContent).toContain('切断');
  });

  it('コンテナインスタンスが 0 件のときは空表示を出す', async () => {
    globalThis.fetch = mockFetch(ok([]), ok([task({ container_instance_arn: '' })]));
    const { container } = renderWithQC(
      <DrawerECSContainerInstances profile="p" region="r" cluster="c" />,
    );
    await waitFor(() => {
      expect(container.textContent).toContain('コンテナインスタンスがありません');
    });
    expect(container.textContent).toContain('コンテナインスタンス (0)');
    // Unknown タスクがあるケースと矛盾するため Fargate のみと断定する文言は出さない
    expect(container.textContent).not.toContain('Fargate');
    expect(sections(container)).toHaveLength(0);
    expect(container.textContent).not.toContain('不明なコンテナインスタンス');
  });

  it('コンテナインスタンスが 0 件で一覧に無い ARN のタスクがあるときは空表示と Unknown を両方出す', async () => {
    globalThis.fetch = mockFetch(
      ok([]),
      ok([task({ group: 'service:gone', container_instance_arn: CI_ARN_GONE })]),
    );
    const { container } = renderWithQC(
      <DrawerECSContainerInstances profile="p" region="r" cluster="c" />,
    );
    await waitFor(() => {
      expect(container.textContent).toContain('不明なコンテナインスタンス');
    });
    expect(container.textContent).toContain('コンテナインスタンスがありません');
    expect(sections(container)).toHaveLength(0);
    const unknown = container.querySelector('[data-testid="ecs-container-instance-unknown"]')!;
    expect(unknown).not.toBeNull();
    expect(unknown.textContent).toContain('service:gone');
    expect(unknown.textContent).toContain('タスク (1)');
  });

  it('一覧に無い ARN を持つタスクは末尾の Unknown container instance に出す', async () => {
    globalThis.fetch = mockFetch(
      ok([instance()]),
      ok([
        task({ group: 'service:svc-a' }),
        task({ group: 'service:gone', container_instance_arn: CI_ARN_GONE }),
      ]),
    );
    const { container } = renderWithQC(
      <DrawerECSContainerInstances profile="p" region="r" cluster="c" />,
    );
    await waitFor(() => {
      expect(container.textContent).toContain('不明なコンテナインスタンス');
    });
    const all = headings(container);
    expect(all[all.length - 1]).toBe('不明なコンテナインスタンス');
    const unknown = container.querySelector('[data-testid="ecs-container-instance-unknown"]')!;
    expect(unknown.textContent).toContain('service:gone');
    expect(unknown.textContent).toContain('タスク (1)');
    expect(sections(container)[0].textContent).not.toContain('service:gone');
  });

  it('Fargate のタスク (containerInstanceArn が空) はどこにも表示しない', async () => {
    globalThis.fetch = mockFetch(
      ok([instance()]),
      ok([
        task({ group: 'service:ec2-task-1' }),
        task({ group: 'service:ec2-task-2' }),
        task({ group: 'service:fargate-task', launch_type: 'FARGATE', container_instance_arn: '' }),
      ]),
    );
    const { container } = renderWithQC(
      <DrawerECSContainerInstances profile="p" region="r" cluster="c" />,
    );
    await waitFor(() => {
      expect(sections(container)).toHaveLength(1);
    });
    expect(container.textContent).toContain('service:ec2-task-1');
    expect(container.textContent).toContain('service:ec2-task-2');
    expect(container.textContent).not.toContain('service:fargate-task');
    expect(container.textContent).not.toContain('不明なコンテナインスタンス');
    expect(sections(container)[0].textContent).toContain('タスク (2)');
  });

  it('instances 成功 + tasks 403 のときは DrawerError のみを出し見出しを出さない', async () => {
    globalThis.fetch = mockFetch(ok([instance()]), forbidden);
    const { container } = renderWithQC(
      <DrawerECSContainerInstances profile="p" region="r" cluster="c" />,
    );
    await waitFor(() => {
      expect(container.textContent).toContain('Error 403 (ACCESS_DENIED): access denied');
    });
    expect(container.querySelectorAll('h3')).toHaveLength(0);
    expect(sections(container)).toHaveLength(0);
    expect(container.textContent).not.toContain('i-0000000000000001');
  });

  it('tasks 成功 + instances 403 のときは DrawerError のみを出す', async () => {
    globalThis.fetch = mockFetch(forbidden, ok([task()]));
    const { container } = renderWithQC(
      <DrawerECSContainerInstances profile="p" region="r" cluster="c" />,
    );
    await waitFor(() => {
      expect(container.textContent).toContain('Error 403 (ACCESS_DENIED): access denied');
    });
    expect(container.querySelectorAll('h3')).toHaveLength(0);
    expect(container.textContent).not.toContain('service:svc-a');
  });

  it('両方エラー (instances 403、tasks 500) のときは instances 側のエラーを出す', async () => {
    globalThis.fetch = mockFetch(forbidden, serverError);
    const { container } = renderWithQC(
      <DrawerECSContainerInstances profile="p" region="r" cluster="c" />,
    );
    await waitFor(() => {
      expect(container.textContent).toContain('Error 403 (ACCESS_DENIED): access denied');
    });
    expect(container.textContent).not.toContain('Error 500');
  });

  it('片方が loading の間は DrawerLoading のみを出す', async () => {
    globalThis.fetch = mockFetch(ok([instance()]), null);
    const qc = newQC();
    const { container } = renderWithQC(
      <DrawerECSContainerInstances profile="p" region="r" cluster="c" />,
      qc,
    );
    // instances の query が success になった後も tasks が未解決なので Loading のまま
    await waitFor(() => {
      expect(qc.getQueryState(['aws', 'ecs-container-instances', 'p', 'r', 'c'])?.status).toBe(
        'success',
      );
    });
    expect(container.textContent).toBe('Loading…');
    expect(container.querySelectorAll('h3')).toHaveLength(0);
  });

  it('同じ QueryClient で Tasks タブと同時に表示しても tasks の取得は 1 回', async () => {
    globalThis.fetch = mockFetch(ok([instance()]), ok([task()]));
    const qc = newQC();
    const { container } = renderWithQC(
      <>
        <DrawerECSTasks profile="p" region="r" cluster="c" />
        <DrawerECSContainerInstances profile="p" region="r" cluster="c" />
      </>,
      qc,
    );
    await waitFor(() => {
      expect(sections(container)).toHaveLength(1);
    });
    const urls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.map((c) =>
      typeof c[0] === 'string' ? c[0] : String(c[0]),
    );
    // /tasks/{task}/containers と区別するため pathname の末尾で判定する
    const paths = urls.map((u) => new URL(u).pathname);
    expect(paths.filter((p) => p.endsWith('/tasks'))).toHaveLength(1);
    expect(paths.filter((p) => p.endsWith('/container-instances'))).toHaveLength(1);
  });
});
