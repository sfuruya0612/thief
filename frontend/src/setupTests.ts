import { afterEach } from 'vitest';
import { cleanup } from '@testing-library/react';
import '@testing-library/jest-dom/vitest';

// vite.config.ts の test.globals は false のため、@testing-library/react が
// afterEach グローバルの存在を検知して行う自動クリーンアップ登録が発火しない
// (afterEach はグローバルスコープに存在せず import が必要)。登録されないと
// render() したコンポーネントが document.body に蓄積し、同一ファイル内の
// 後続テストに前のテストの DOM が混入する。
afterEach(() => {
  cleanup();
});

// Node 22+ は experimental な localStorage グローバルを持ち、--localstorage-file 未指定時は
// undefined のまま jsdom の localStorage をシャドウしてしまう。テストで localStorage を使った
// 永続化を検証できるよう、undefined の場合は in-memory 実装で置き換える。
function createMemoryStorage(): Storage {
  let store = new Map<string, string>();
  return {
    get length() {
      return store.size;
    },
    clear: () => {
      store = new Map();
    },
    getItem: (key: string) => store.get(key) ?? null,
    key: (index: number) => [...store.keys()][index] ?? null,
    removeItem: (key: string) => {
      store.delete(key);
    },
    setItem: (key: string, value: string) => {
      store.set(key, String(value));
    },
  };
}

if (globalThis.localStorage === undefined) {
  Object.defineProperty(globalThis, 'localStorage', {
    value: createMemoryStorage(),
    writable: true,
    configurable: true,
  });
}

// jsdom は ResizeObserver を持たない。SessionTabs (タブバー) はコンストラクタ
// 参照で落ちないよう no-op 実装を与える (発火しないため、表示本数のテストは
// visibleCountOverride prop で注入する)。
if (globalThis.ResizeObserver === undefined) {
  class NoopResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  Object.defineProperty(globalThis, 'ResizeObserver', {
    value: NoopResizeObserver,
    writable: true,
    configurable: true,
  });
}
