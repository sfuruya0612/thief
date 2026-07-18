// ログビューアの Live Tail (WebSocket) 共通フック。CloudWatch Logs / Cloud Logging で共有する。
// url が変わるたびに再接続し、接続時に行をクリアする。サーバの {"type":"end"} 制御メッセージで
// 終了扱いにする。それ以外の TEXT フレームは JSON としてパースし parse で行へ変換する。
//
// parse は依存配列に入るため、呼び出し側で useCallback により安定させること。
import { useCallback, useEffect, useRef, useState } from 'react';
import { appendLogLines, MAX_LOG_LINES } from '../../lib/logLines';

export type LiveStatus = 'idle' | 'connecting' | 'connected' | 'closed' | 'error';

export interface UseLiveTailOptions<T> {
  enabled: boolean;
  url: string;
  parse: (raw: Record<string, unknown>, seq: number) => T;
  maxLines?: number;
}

export function useLiveTail<T>({
  enabled,
  url,
  parse,
  maxLines = MAX_LOG_LINES,
}: UseLiveTailOptions<T>) {
  const [status, setStatus] = useState<LiveStatus>('idle');
  const [message, setMessage] = useState<string | null>(null);
  const [lines, setLines] = useState<T[]>([]);
  const seqRef = useRef(0);

  const clear = useCallback(() => {
    setLines([]);
    setMessage(null);
  }, []);

  useEffect(() => {
    if (!enabled) {
      setStatus('idle');
      return;
    }
    setStatus('connecting');
    setMessage(null);
    setLines([]);
    seqRef.current = 0;

    // Terminal.tsx と同じ「disposed フラグ + cleanup で close」の StrictMode 二重実行対策。
    let disposed = false;
    const ws = new WebSocket(url);

    ws.onopen = () => {
      if (!disposed) setStatus('connected');
    };
    ws.onmessage = (ev) => {
      if (disposed || typeof ev.data !== 'string') return;
      let msg: Record<string, unknown>;
      try {
        msg = JSON.parse(ev.data);
      } catch {
        return;
      }
      if (msg.type === 'end') {
        setStatus('closed');
        if (typeof msg.reason === 'string' && msg.reason) setMessage(msg.reason);
        return;
      }
      const seq = seqRef.current;
      seqRef.current += 1;
      const row = parse(msg, seq);
      setLines((prev) => appendLogLines(prev, [row], maxLines));
    };
    ws.onerror = () => {
      if (!disposed) setStatus('error');
    };
    ws.onclose = () => {
      if (!disposed) setStatus((prev) => (prev === 'error' ? prev : 'closed'));
    };

    return () => {
      disposed = true;
      ws.close();
    };
  }, [enabled, url, parse, maxLines]);

  return { status, message, lines, clear };
}
