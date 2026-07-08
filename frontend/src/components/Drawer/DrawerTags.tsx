// drawer.jsx DrawerTags の移植
import { Fragment } from 'react';

export interface DrawerTagsProps {
  tags: Record<string, string> | undefined;
}

export function DrawerTags({ tags }: DrawerTagsProps) {
  const entries = Object.entries(tags ?? {});
  return (
    <div className="section">
      <h3>Tags ({entries.length})</h3>
      <div className="kv">
        {entries.map(([k, v]) => (
          <Fragment key={k}>
            <div className="k">{k}</div>
            <div className="v">{v}</div>
          </Fragment>
        ))}
      </div>
    </div>
  );
}
