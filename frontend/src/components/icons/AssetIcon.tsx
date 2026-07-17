// public/assets 配下のアイコンアセットを表示する img。
// アセット未展開 (404) 時はブラウザの壊れ画像アイコンではなく無地のプレースホルダを表示する
// (公式アイコンはライセンス上コミットされないため、fetch スクリプト未実行の環境で発生する)。
import { useState } from 'react';

export interface AssetIconProps {
  src: string;
  alt: string;
  size: number;
}

export function AssetIcon({ src, alt, size }: AssetIconProps) {
  const [failed, setFailed] = useState(false);
  if (failed) {
    return (
      <span
        role="img"
        aria-label={alt}
        title={alt}
        style={{
          display: 'block',
          width: size,
          height: size,
          borderRadius: 3.5,
          background: 'var(--bg-3)',
          border: '1px solid var(--line-2)',
          boxSizing: 'border-box',
        }}
      />
    );
  }
  return (
    <img
      src={src}
      width={size}
      height={size}
      alt={alt}
      style={{ display: 'block', borderRadius: 3.5 }}
      onError={() => setFailed(true)}
    />
  );
}
