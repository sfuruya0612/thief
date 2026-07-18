// クリップボードコピーと「コピー済み」の一時表示を扱う小さなフック。
// ログビューアのエクスポート (表示中の行の CSV / JSON コピー) で使う。
import { useCallback, useEffect, useRef, useState } from 'react';

export function useCopy(): { copied: boolean; copy: (text: string) => void } {
  const [copied, setCopied] = useState(false);
  const timer = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (timer.current) window.clearTimeout(timer.current);
    };
  }, []);

  const copy = useCallback((text: string) => {
    void navigator.clipboard?.writeText(text);
    setCopied(true);
    if (timer.current) window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setCopied(false), 1200);
  }, []);

  return { copied, copy };
}
