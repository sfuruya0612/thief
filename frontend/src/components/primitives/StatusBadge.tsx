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
  creating: { cls: 'info', label: 'creating' },
  deleting: { cls: 'warn', label: 'deleting' },
  stopping: { cls: 'warn', label: 'stopping' },
  'shutting-down': { cls: 'warn', label: 'shutting-down' },
  inactive: { cls: 'muted', label: 'inactive' },
  failed: { cls: 'err', label: 'failed' },
  archived: { cls: 'muted', label: 'archived' },
  'active-impaired': { cls: 'warn', label: 'active-impaired' },
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
