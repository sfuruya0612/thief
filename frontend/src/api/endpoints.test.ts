import { afterEach, describe, expect, it, vi } from 'vitest';
import { getCost } from './endpoints';

// fetch に渡された URL を取り出して検証する。buildUrl (client.ts の非公開関数) を
// 直接呼べないため、実際に組み立てられた URL を fetch の引数から観測する。
function stubFetch(): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn(async () => new Response('[]', { status: 200 }));
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function requestedUrl(fetchMock: ReturnType<typeof vi.fn>): URL {
  const first = fetchMock.mock.calls.at(0);
  if (first === undefined) throw new Error('fetch was not called');
  return new URL(String(first[0]));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('getCost', () => {
  it('service と account が空文字でもクエリパラメータとして送る', async () => {
    const fetchMock = stubFetch();

    await getCost('test-profile', 'ap-northeast-1', { service: '', account: '' });

    // 空文字は「絞り込み解除」を表す確定値であり、パラメータ自体を省略する挙動とは
    // 区別される。client.ts の buildUrl が undefined のみを省く実装であることに依存する。
    const url = requestedUrl(fetchMock);
    expect(url.searchParams.has('service')).toBe(true);
    expect(url.searchParams.has('account')).toBe(true);
    expect(url.searchParams.get('service')).toBe('');
    expect(url.searchParams.get('account')).toBe('');
    expect(url.search).toContain('service=');
    expect(url.search).toContain('account=');
  });

  it('service と account を渡さない場合はクエリパラメータを付けない', async () => {
    const fetchMock = stubFetch();

    await getCost('test-profile', 'ap-northeast-1');

    const url = requestedUrl(fetchMock);
    expect(url.searchParams.has('service')).toBe(false);
    expect(url.searchParams.has('account')).toBe(false);
  });

  it('service と account の値は URL エンコードして送る', async () => {
    const fetchMock = stubFetch();

    await getCost('test-profile', 'ap-northeast-1', {
      service: 'Amazon Elastic Compute Cloud - Compute',
      account: '123456789012',
    });

    const url = requestedUrl(fetchMock);
    expect(url.searchParams.get('service')).toBe('Amazon Elastic Compute Cloud - Compute');
    expect(url.searchParams.get('account')).toBe('123456789012');
    // 生の空白がクエリ文字列にそのまま現れないこと (URLSearchParams による符号化)
    expect(url.search).not.toContain('Cloud - Compute');
  });
});
