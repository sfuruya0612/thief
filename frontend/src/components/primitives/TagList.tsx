export interface TagListProps {
  tags: Record<string, string> | null | undefined;
  max?: number;
}

export function TagList({ tags, max = 3 }: TagListProps) {
  const entries = Object.entries(tags ?? {});
  const shown = entries.slice(0, max);
  const rest = entries.length - shown.length;
  return (
    <span className="tag-list">
      {shown.map(([k, v]) => {
        const envCls = k === 'Env' ? ` env-${v}` : '';
        return (
          <span key={k} className={`tag${envCls}`}>
            <span className="k">{k}:</span>
            <span className="v">{v}</span>
          </span>
        );
      })}
      {rest > 0 && (
        <span className="tag">
          <span className="v">+{rest}</span>
        </span>
      )}
    </span>
  );
}
