// AWS_PROFILE 選択用の検索可能なカスタムドロップダウン。
// プロファイル数が多い環境で名前 / Account ID / 権限名から絞り込めるようにする。
import { useEffect, useMemo, useRef, useState } from 'react';
import { useProfileIdentity } from '../api/queries';
import type { Profile } from '../types/common';
import { Icons } from './icons/Icons';

export interface ProfileSelectProps {
  profile: string;
  profiles: Profile[];
  onProfileChange: (name: string) => void;
}

function matches(p: Profile, query: string): boolean {
  const q = query.toLowerCase();
  if (!q) return true;
  return (
    p.name.toLowerCase().includes(q) ||
    (p.accountId?.toLowerCase().includes(q) ?? false) ||
    (p.ssoRoleName?.toLowerCase().includes(q) ?? false)
  );
}

export function ProfileSelect({ profile, profiles, onProfileChange }: ProfileSelectProps) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [activeIndex, setActiveIndex] = useState(0);
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const selected = profiles.find((p) => p.name === profile);
  // config 由来の accountId をまず表示し、選択時に STS で確定した値が来たら上書きする
  const identity = useProfileIdentity(profile);
  const displayAccountId = identity.data?.accountId || selected?.accountId;

  const filtered = useMemo(() => profiles.filter((p) => matches(p, search)), [profiles, search]);

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
        profiles.findIndex((p) => p.name === profile),
      ),
    );
    // input へのフォーカスは open 後の再描画を待つ必要がある
    requestAnimationFrame(() => inputRef.current?.focus());
  };

  const choose = (name: string) => {
    onProfileChange(name);
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
      if (target) choose(target.name);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      setOpen(false);
    }
  };

  const toggleMenu = () => (open ? setOpen(false) : openMenu());

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
        title="AWS Profile"
      >
        <span className="profile-select-name">{profile || '(no profile)'}</span>
        {(displayAccountId || selected?.ssoRoleName) && (
          // Account ID / Role Name はダブルクリック/ダブルタップでテキスト選択したい。
          // トリガー全体の click に伝播すると開閉がトグルしてしまうため止める。
          <span className="profile-select-meta" onClick={(e) => e.stopPropagation()}>
            <span className="account-id">{displayAccountId || '-'}</span>
            {selected?.ssoRoleName && <span className="role-name">{selected.ssoRoleName}</span>}
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
              placeholder="filter by name, account id, role…"
            />
          </span>
          <ul className="profile-select-list" role="listbox">
            {filtered.length === 0 && <li className="profile-select-empty">No profiles match</li>}
            {filtered.map((p, i) => (
              <li
                key={p.name}
                role="option"
                aria-selected={p.name === profile}
                className={`profile-select-option ${i === activeIndex ? 'active' : ''} ${p.name === profile ? 'selected' : ''}`}
                onMouseEnter={() => setActiveIndex(i)}
                onClick={() => choose(p.name)}
              >
                <span className="profile-select-option-name">{p.name}</span>
                <span className="profile-select-option-meta">
                  <span className="account-id">{p.accountId || '-'}</span>
                  {p.ssoRoleName && <span className="role-name">{p.ssoRoleName}</span>}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
