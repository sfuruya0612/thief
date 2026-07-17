// ステータスバー (StatusBar) の CLI コマンド表示を直近のクエリ操作で上書きする共有ストア。
// useTweaks と同じくモジュールレベルのストア + useSyncExternalStore で全購読者へ伝播する。
import { useSyncExternalStore } from 'react';

interface CliHint {
  service: string;
  command: string;
}

let current: CliHint | null = null;
const listeners = new Set<() => void>();

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function getSnapshot(): CliHint | null {
  return current;
}

// setCliHint はクエリ実行等の操作時に「等価な CLI コマンド」を記録する。
export function setCliHint(service: string, command: string): void {
  current = { service, command };
  listeners.forEach((listener) => listener());
}

// resetCliHintForTest はテスト間の分離のためにストアを初期化する。テスト専用。
export function resetCliHintForTest(): void {
  current = null;
  listeners.clear();
}

// useCliHint は現在表示中のサービスに対応する上書きコマンドを返す (無ければ null)。
export function useCliHint(service: string): string | null {
  const hint = useSyncExternalStore(subscribe, getSnapshot);
  return hint && hint.service === service ? hint.command : null;
}
