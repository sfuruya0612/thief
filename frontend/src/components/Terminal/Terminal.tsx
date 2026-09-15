// EC2 Start Session / ECS Exec Command のブラウザ側ターミナル。
// xterm.js で疑似端末を表示し、WebSocket 経由でバックエンドのデータチャネルブリッジと通信する。
// フレーム規約 (backend/internal/session/bridge.go と対になる):
//   BINARY = 端末バイト列 (双方向)
//   TEXT   = JSON 制御。{"type":"resize","cols":N,"rows":N} (送信) / {"type":"exit"|"error","message":...} (受信)
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FitAddon } from '@xterm/addon-fit';
import { Terminal as XTerm } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';

export type ConnectionStatus = 'connecting' | 'connected' | 'closed' | 'error';

interface ControlMessage {
  type: 'resize' | 'exit' | 'error';
  message?: string;
}

export interface TerminalProps {
  // 接続先の WebSocket URL (api/terminal.ts の ec2SessionUrl/ecsExecUrl で組み立てる)
  wsUrl: string;
  // 表示中かどうか。ターミナルドックでは、非アクティブなタブと折りたたみ中に false になる。
  // false から true へ変わったときだけ再フィットと入力フォーカスの移動を行う。
  active?: boolean;
  // 接続状態の変化の通知 (ドックのタブに接続状態を表示するために使う)
  onStatusChange?: (status: ConnectionStatus) => void;
}

export function Terminal({ wsUrl, active = true, onStatusChange }: TerminalProps) {
  const { t } = useTranslation('drawerStorage');
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
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
    termRef.current = term;
    fitAddonRef.current = fitAddon;
    if (containerRef.current) {
      term.open(containerRef.current);
      fitAddon.fit();
      term.focus();
    }

    // StrictMode の effect 二重実行 (マウント→即クリーンアップ→再マウント) や、クリーンアップ後に
    // 配送される WebSocket/ResizeObserver の遅延コールバックから dispose 済みの term を守るためのフラグ。
    // dispose 済み term への write()/fit() 呼び出しは xterm 内部の _renderService が
    // undefined になっており例外になるため、すべてのコールバックの先頭でこれを確認する。
    let disposed = false;

    const ws = new WebSocket(wsUrl);
    ws.binaryType = 'arraybuffer';

    function sendResize() {
      if (disposed || ws.readyState !== WebSocket.OPEN) return;
      ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
    }

    ws.onopen = () => {
      if (disposed) return;
      setStatus('connected');
      sendResize();
    };

    ws.onmessage = (ev) => {
      if (disposed) return;
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
      if (disposed) return;
      setStatus('error');
    };

    ws.onclose = () => {
      if (disposed) return;
      setStatus((prev) => (prev === 'error' ? prev : 'closed'));
    };

    const dataSub = term.onData((data) => {
      if (disposed) return;
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data));
      }
    });
    const resizeSub = term.onResize(() => sendResize());

    const resizeObserver = new ResizeObserver(() => {
      if (disposed) return;
      // 非表示のコンテナ (ドックの非アクティブなタブ、折りたたみ中の本文) では寸法が 0 に
      // なりうる。@xterm/addon-fit の proposeDimensions はセル寸法が前回値を保持するため
      // 早期 return せず、ターミナルを 2 列 1 行へ縮めて term.onResize 経由でその寸法を
      // backend へ送り、リモートの端末を再レイアウトさせてしまう。CSS に依存しないよう
      // ここで寸法 0 をガードする。
      const el = containerRef.current;
      if (!el || el.clientWidth === 0 || el.clientHeight === 0) return;
      fitAddon.fit();
    });
    if (containerRef.current) resizeObserver.observe(containerRef.current);

    return () => {
      disposed = true;
      resizeObserver.disconnect();
      dataSub.dispose();
      resizeSub.dispose();
      ws.close();
      // xterm.js は Viewport 生成時に setTimeout(() => this.syncScrollArea()) を内部で登録しており、
      // これは公開 API からキャンセルできない。term.dispose() を同期的に呼ぶと、その内部タイマーが
      // 発火した時点で破棄済みの内部状態 (_renderService) にアクセスして例外になる
      // (StrictMode の mount→cleanup→再 mount のような同一タイミングで顕在化しやすい)。
      // dispose 自体を次のマクロタスクへ遅らせ、内部タイマーを先に消化させてから破棄する。
      setTimeout(() => term.dispose());
      termRef.current = null;
      fitAddonRef.current = null;
    };
  }, [wsUrl]);

  // 非表示から表示に戻ったとき (ドックのタブ切替、折りたたみの解除) に、ResizeObserver の
  // 発火を待たずに寸法を合わせ、入力フォーカスを移す。
  const prevActiveRef = useRef(active);
  useEffect(() => {
    const wasActive = prevActiveRef.current;
    prevActiveRef.current = active;
    if (!active || wasActive) return;
    fitAddonRef.current?.fit();
    termRef.current?.focus();
  }, [active]);

  // 接続状態の通知。onStatusChange の identity 変化で再通知しないよう ref 経由で呼ぶ。
  const onStatusChangeRef = useRef(onStatusChange);
  useEffect(() => {
    onStatusChangeRef.current = onStatusChange;
  }, [onStatusChange]);
  useEffect(() => {
    onStatusChangeRef.current?.(status);
  }, [status]);

  return (
    <div className="terminal-panel">
      {status !== 'connected' && (
        <div className={`terminal-status terminal-status-${status}`}>
          {status === 'connecting' && t('terminal.connecting')}
          {status === 'closed' &&
            (message
              ? t('terminal.sessionEndedWithMessage', { message })
              : t('terminal.sessionEnded'))}
          {status === 'error' &&
            (message ? t('terminal.errorWithMessage', { message }) : t('terminal.connectError'))}
        </div>
      )}
      <div ref={containerRef} className="terminal-container" />
    </div>
  );
}
