import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { App } from './App';
import { cleanupStaleObjectQueryFiles } from './lib/opfs';
import './i18n';
import './app.css';

const rootElement = document.getElementById('root');
if (!rootElement) {
  throw new Error('root element not found');
}

// 前回のセッションが残した OPFS の作業ファイル (thief-query/) を削除する。他タブが使用中の
// ファイルはロックを保持しているため残す。描画は待たせず、失敗は cleanup 側で警告に留める
// (非対応ブラウザでは何もしない)。
void cleanupStaleObjectQueryFiles();

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      // backend のリソースキャッシュ TTL に合わせた既定値 (AGENTS.md の frontend 節を参照)。
      // 60 秒以外にしたいクエリだけが各 useQuery で staleTime を個別指定する。
      staleTime: 60_000,
    },
  },
});

createRoot(rootElement).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </StrictMode>,
);
