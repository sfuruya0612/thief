// GCP プロジェクト選択用の検索可能なカスタムドロップダウン。
// ProfileSelect のパターンを踏襲。プロジェクト名 / project_id で絞り込める。
import { useEffect, useMemo, useRef, useState } from 'react';
import type { GcpProject } from '../types/gcp';
import { Icons } from './icons/Icons';

export interface GcpProjectSelectProps {
  project: string;
  projects: GcpProject[];
  onProjectChange: (id: string) => void;
}

function matches(p: GcpProject, query: string): boolean {
  const q = query.toLowerCase();
  if (!q) return true;
  return p.name.toLowerCase().includes(q) || p.id.toLowerCase().includes(q);
}

export function GcpProjectSelect({ project, projects, onProjectChange }: GcpProjectSelectProps) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [activeIndex, setActiveIndex] = useState(0);
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const selected = projects.find((p) => p.id === project);

  const filtered = useMemo(() => projects.filter((p) => matches(p, search)), [projects, search]);

  useEffect(() => {
    setActiveIndex(0);
  }, [search]);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: PointerEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('pointerdown', onPointerDown);
    return () => document.removeEventListener('pointerdown', onPointerDown);
  }, [open]);

  const openMenu = () => {
    setOpen(true);
    setSearch('');
    setActiveIndex(
      Math.max(
        0,
        projects.findIndex((p) => p.id === project),
      ),
    );
    requestAnimationFrame(() => inputRef.current?.focus());
  };

  const choose = (id: string) => {
    onProjectChange(id);
    setOpen(false);
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActiveIndex((i) => Math.min(i + 1, filtered.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActiveIndex((i) => Math.max(i - 1, 0));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const target = filtered[activeIndex];
      if (target) choose(target.id);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      setOpen(false);
    }
  };

  const toggleMenu = () => (open ? setOpen(false) : openMenu());

  const displayLabel = selected?.name || project || '(no project)';
  const displayId = selected?.id;

  return (
    <div className="profile-select" ref={rootRef}>
      <div
        role="button"
        tabIndex={0}
        className="btn sm profile-select-trigger"
        onClick={toggleMenu}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            toggleMenu();
          }
        }}
        aria-haspopup="listbox"
        aria-expanded={open}
        title="Google Cloud Project"
      >
        <span className="profile-select-name">{displayLabel}</span>
        {displayId && displayId !== displayLabel && (
          <span className="profile-select-meta" onClick={(e) => e.stopPropagation()}>
            <span className="account-id">{displayId}</span>
          </span>
        )}
      </div>

      {open && (
        <div className="profile-select-menu">
          <span className="chip-search">
            <Icons.search size={12} />
            <input
              ref={inputRef}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onKeyDown={onKeyDown}
              placeholder="filter by name, project id…"
            />
          </span>
          <ul className="profile-select-list" role="listbox">
            {filtered.length === 0 && <li className="profile-select-empty">No projects match</li>}
            {filtered.map((p, i) => (
              <li
                key={p.id}
                role="option"
                aria-selected={p.id === project}
                className={`profile-select-option ${i === activeIndex ? 'active' : ''} ${p.id === project ? 'selected' : ''}`}
                onMouseEnter={() => setActiveIndex(i)}
                onClick={() => choose(p.id)}
              >
                <span className="profile-select-option-name">{p.name}</span>
                <span className="profile-select-option-meta">
                  <span className="account-id">{p.id}</span>
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
