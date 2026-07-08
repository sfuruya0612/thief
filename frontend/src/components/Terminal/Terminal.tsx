// EC2 Start Session / ECS Exec Command のブラウザ側ターミナル。
// xterm.js で疑似端末を表示し、WebSocket 経由でバックエンドのデータチャネルブリッジと通信する。
// フレーム規約 (backend/internal/session/bridge.go と対になる):
//   BINARY = 端末バイト列 (双方向)
//   TEXT   = JSON 制御。{"type":"resize","cols":N,"rows":N} (送信) / {"type":"exit"|"error","message":...} (受信)
import { useEffect, useRef, useState } from 'react';
import { FitAddon } from '@xterm/addon-fit';
import { Terminal as XTerm } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';

type ConnectionStatus = 'connecting' | 'connected' | 'closed' | 'error';

interface ControlMessage {
  type: 'resize' | 'exit' | 'error';
  message?: string;
}

export interface TerminalProps {
  // 接続先の WebSocket URL (api/terminal.ts の ec2SessionUrl/ecsExecUrl で組み立てる)
  wsUrl: string;
}

export function Terminal({ wsUrl }: TerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [status, setStatus] = useState<ConnectionStatus>('connecting');
  const [message, setMessage] = useState<string | null>(null);

  useEffect(() => {
    setStatus('connecting');
    setMessage(null);

    const term = new XTerm({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
      theme: { background: '#0a0a0a' },
    });
    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    if (containerRef.current) {
      term.open(containerRef.current);
      fitAddon.fit();
      term.focus();
    }

    const ws = new WebSocket(wsUrl);
    ws.binaryType = 'arraybuffer';

    function sendResize() {
      if (ws.readyState !== WebSocket.OPEN) return;
      ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
    }

    ws.onopen = () => {
      setStatus('connected');
      sendResize();
    };

    ws.onmessage = (ev) => {
      if (typeof ev.data === 'string') {
        let msg: ControlMessage;
        try {
          msg = JSON.parse(ev.data) as ControlMessage;
        } catch {
          return;
        }
        if (msg.type === 'error') {
          setStatus('error');
          setMessage(msg.message ?? 'unknown error');
        } else if (msg.type === 'exit') {
          setStatus('closed');
          if (msg.message) setMessage(msg.message);
        }
        return;
      }
      term.write(new Uint8Array(ev.data as ArrayBuffer));
    };

    ws.onerror = () => {
      setStatus('error');
    };

    ws.onclose = () => {
      setStatus((prev) => (prev === 'error' ? prev : 'closed'));
    };

    const dataSub = term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data));
      }
    });
    const resizeSub = term.onResize(() => sendResize());

    const resizeObserver = new ResizeObserver(() => fitAddon.fit());
    if (containerRef.current) resizeObserver.observe(containerRef.current);

    return () => {
      resizeObserver.disconnect();
      dataSub.dispose();
      resizeSub.dispose();
      ws.close();
      term.dispose();
    };
  }, [wsUrl]);

  return (
    <div className="terminal-panel">
      {status !== 'connected' && (
        <div className={`terminal-status terminal-status-${status}`}>
          {status === 'connecting' && '接続中...'}
          {status === 'closed' && (message ? `セッション終了: ${message}` : 'セッション終了')}
          {status === 'error' && (message ? `エラー: ${message}` : '接続エラー')}
        </div>
      )}
      <div ref={containerRef} className="terminal-container" />
    </div>
  );
}
