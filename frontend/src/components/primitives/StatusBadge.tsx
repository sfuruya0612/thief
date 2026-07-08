// primitives.jsx StatusBadge の TSX 移植

type Cls = 'ok' | 'muted' | 'info' | 'warn' | 'err';

interface Entry {
  cls: Cls;
  label: string;
}

const MAP: Record<string, Entry> = {
  running: { cls: 'ok', label: 'running' },
  available: { cls: 'ok', label: 'available' },
  active: { cls: 'ok', label: 'active' },
  stopped: { cls: 'muted', label: 'stopped' },
  pending: { cls: 'info', label: 'pending' },
  provisioning: { cls: 'warn', label: 'provisioning' },
  deployed: { cls: 'ok', label: 'deployed' },
  'in-progress': { cls: 'info', label: 'in-progress' },
  modifying: { cls: 'warn', label: 'modifying' },
  updating: { cls: 'warn', label: 'updating' },
  'backing-up': { cls: 'info', label: 'backing-up' },
  errors: { cls: 'err', label: 'errors' },
  'policy-warning': { cls: 'warn', label: 'policy' },
  stale: { cls: 'warn', label: 'stale' },
};

export interface StatusBadgeProps {
  state: string;
}

export function StatusBadge({ state }: StatusBadgeProps) {
  const m: Entry = MAP[state] ?? { cls: 'muted', label: state };
  return (
    <span className={`status ${m.cls}`}>
      <span className="dot" />
      {m.label}
    </span>
  );
}
