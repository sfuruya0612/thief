// ServicePanel (AccountView) のエラーバナー出し分けの検証 (issue 0073)。
// SSO_TOKEN_EXPIRED のときだけ SSOExpiredBanner を出し、それ以外の ApiError
// (403 ACCESS_DENIED 等) は ErrorBanner に落ちることを確認する。
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AccountView } from './AccountView';
import { ApiError } from '../types/common';

// ServicePanel が使う useResources / useCost だけを差し替え、他のフック
// (Sidebar の useRegions など) は実装のまま使う。fetch は解決しない Promise に
// して副作用のリクエストを止める。
const mocks = vi.hoisted(() => ({
  useResources: vi.fn(),
  useCost: vi.fn(),
}));

vi.mock('../api/queries', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/queries')>();
  return {
    ...actual,
    useResources: mocks.useResources,
    useCost: mocks.useCost,
  };
});

function renderView() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={qc}>
      <AccountView
        profile="test-profile"
        region="ap-northeast-1"
        profiles={[]}
        onRegionChange={() => {}}
        activeService="ec2"
        onServiceChange={() => {}}
        drawerPos="right"
      />
    </QueryClientProvider>,
  );
}

describe('AccountView のエラーバナー出し分け', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    globalThis.fetch = vi.fn().mockImplementation(() => new Promise(() => {}));
    mocks.useCost.mockReturnValue({ data: [], isLoading: false, error: null });
  });

  it('403 ACCESS_DENIED の ApiError では SSOExpiredBanner を出さず ErrorBanner に内容を表示する', () => {
    const err = new ApiError(
      403,
      'ACCESS_DENIED',
      'User is not authorized to perform: ec2:DescribeInstances',
    );
    mocks.useResources.mockReturnValue({ data: undefined, isLoading: false, error: err });

    const { container } = renderView();

    expect(container.querySelector('.sso-banner')).not.toBeInTheDocument();
    const banner = container.querySelector('.error-banner');
    expect(banner).toBeInTheDocument();
    expect(banner).toHaveTextContent('403');
    expect(banner).toHaveTextContent('ACCESS_DENIED');
    expect(banner).toHaveTextContent('User is not authorized to perform: ec2:DescribeInstances');
  });

  it('401 SSO_TOKEN_EXPIRED の ApiError では SSOExpiredBanner を表示する', () => {
    const err = new ApiError(401, 'SSO_TOKEN_EXPIRED', 'SSO token expired');
    mocks.useResources.mockReturnValue({ data: undefined, isLoading: false, error: err });

    const { container } = renderView();

    expect(container.querySelector('.sso-banner')).toBeInTheDocument();
    expect(container.querySelector('.error-banner')).not.toBeInTheDocument();
  });

  it('エラーが無ければどちらのバナーも表示しない', () => {
    mocks.useResources.mockReturnValue({ data: [], isLoading: false, error: null });

    const { container } = renderView();

    expect(container.querySelector('.sso-banner')).not.toBeInTheDocument();
    expect(container.querySelector('.error-banner')).not.toBeInTheDocument();
    expect(screen.getByRole('table')).toBeInTheDocument();
  });
});
