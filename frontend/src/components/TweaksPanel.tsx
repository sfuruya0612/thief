// tweaks.jsx を TSX に移植し useTweaks と接続する
import type { Accent, DrawerPos, Theme, Tweaks } from '../types/common';
import { useTweaks } from '../hooks/useTweaks';
import { Icons } from './icons/Icons';

interface TweaksPanelInnerProps {
  tweaks: Tweaks;
  update: (patch: Partial<Tweaks>) => void;
  onClose?: () => void;
}

const ACCENTS: Array<[Accent, string]> = [
  ['indigo', '#5e6ad2'],
  ['amber', '#d9a514'],
  ['blue', '#2f80ed'],
  ['green', '#12a66c'],
  ['purple', '#8b5cf6'],
  ['pink', '#de5d9c'],
];

function TweaksPanelInner({ tweaks, update, onClose }: TweaksPanelInnerProps) {
  const drawerPos: DrawerPos = tweaks.drawerPos ?? 'right';
  return (
    <div className="tweaks-panel open">
      <div className="th">
        <span>Tweaks</span>
        <button className="btn sm ghost" onClick={onClose} style={{ padding: '0 6px' }}>
          <Icons.x />
        </button>
      </div>
      <div className="tb">
        <div className="trow">
          <span className="lbl">Theme</span>
          <div className="seg">
            {(['dark', 'light'] as Theme[]).map((t) => (
              <button
                key={t}
                className={tweaks.theme === t ? 'active' : ''}
                onClick={() => update({ theme: t })}
              >
                {t === 'dark' ? 'Dark' : 'Light'}
              </button>
            ))}
          </div>
        </div>
        <div className="trow">
          <span className="lbl">Detail panel</span>
          <div className="seg">
            {(['right', 'bottom'] as DrawerPos[]).map((p) => (
              <button
                key={p}
                className={drawerPos === p ? 'active' : ''}
                onClick={() => update({ drawerPos: p })}
              >
                {p === 'right' ? 'Right' : 'Bottom'}
              </button>
            ))}
          </div>
        </div>
        <div className="trow">
          <span className="lbl">Accent</span>
          <div className="swatches">
            {ACCENTS.map(([name, color]) => (
              <button
                key={name}
                className={tweaks.accent === name ? 'active' : ''}
                onClick={() => update({ accent: name })}
                style={{ background: color }}
                title={name}
              />
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

export interface TweaksPanelProps {
  open?: boolean;
  onClose?: () => void;
}

// open 未指定時は常時表示 (親コンポーネントで表示制御を行う想定)
export function TweaksPanel({ open, onClose }: TweaksPanelProps) {
  const { tweaks, update } = useTweaks();
  if (open === false) return null;
  return <TweaksPanelInner tweaks={tweaks} update={update} onClose={onClose} />;
}
