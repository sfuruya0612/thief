// data.jsx makeSpark の移植: シードから決定論的な値列を返す

function hashSeed(seed: string): number {
  let h = 0;
  for (let i = 0; i < seed.length; i++) {
    h = (h * 31 + seed.charCodeAt(i)) | 0;
  }
  // 正の整数に正規化
  return Math.abs(h) || 1;
}

function makeRng(seed: number): () => number {
  let s = seed;
  return () => {
    s = (s * 9301 + 49297) % 233280;
    return s / 233280;
  };
}

export function makeSpark(seed: string, n: number = 24): number[] {
  const r = makeRng(hashSeed(seed));
  const out: number[] = [];
  let v = 0.4 + r() * 0.3;
  for (let i = 0; i < n; i++) {
    v = Math.max(0.05, Math.min(0.98, v + (r() - 0.5) * 0.18));
    out.push(v);
  }
  return out;
}
