// app.jsx FacetBar の移植
import { useMemo } from 'react';
import { Icons } from './icons/Icons';

export type Filters = Record<string, string[]>;

export interface FacetRow {
  tags?: Record<string, string>;
  state?: string;
  region?: string;
}

export interface FacetBarProps {
  rows: FacetRow[];
  filters: Filters;
  setFilters: (f: Filters) => void;
}

const FACET_TYPES = ['Env', 'state', 'region', 'Team'] as const;

export function FacetBar({ rows, filters, setFilters }: FacetBarProps) {
  const available = useMemo(() => {
    const envs = new Set<string>();
    const states = new Set<string>();
    const teams = new Set<string>();
    const regions = new Set<string>();
    rows.forEach((r) => {
      if (r.tags?.Env) envs.add(r.tags.Env);
      if (r.state) states.add(r.state);
      if (r.tags?.Team) teams.add(r.tags.Team);
      if (r.tags?.Owner) teams.add(r.tags.Owner);
      if (r.region) regions.add(r.region);
    });
    return {
      Env: [...envs],
      state: [...states],
      region: [...regions],
      Team: [...teams],
    };
  }, [rows]);

  const toggleFacet = (type: string, val: string) => {
    const cur = new Set(filters[type] ?? []);
    if (cur.has(val)) cur.delete(val);
    else cur.add(val);
    setFilters({ ...filters, [type]: [...cur] });
  };
  const clearAll = () => {
    setFilters({});
  };

  const hasFilters = Object.values(filters).some((v) => v?.length);

  return (
    <div className="facets">
      {FACET_TYPES.map(
        (type) =>
          available[type]?.length > 0 && (
            <span key={type} style={{ display: 'contents' }}>
              {available[type].map((v) => {
                const active = filters[type]?.includes(v);
                return (
                  <span
                    key={type + v}
                    className={`facet ${active ? 'active' : ''}`}
                    onClick={() => toggleFacet(type, v)}
                  >
                    <span className="k">{type}:</span>
                    <span className="v">{v}</span>
                    {active && <Icons.x size={10} />}
                  </span>
                );
              })}
            </span>
          ),
      )}

      {hasFilters && (
        <button className="btn sm ghost clear-btn" onClick={clearAll}>
          Clear all
        </button>
      )}
    </div>
  );
}
