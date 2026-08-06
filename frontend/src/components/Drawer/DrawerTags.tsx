// drawer.jsx DrawerTags の移植
import { Fragment } from 'react';
import { useTranslation } from 'react-i18next';
import { FetchFailedWarning } from '../primitives';

export interface DrawerTagsProps {
  tags: Record<string, string> | undefined;
  fetchFailed?: boolean;
}

export function DrawerTags({ tags, fetchFailed }: DrawerTagsProps) {
  const { t } = useTranslation('drawerAws');
  const entries = Object.entries(tags ?? {});
  return (
    <div className="section">
      <h3>
        {fetchFailed ? (
          <>
            {t('drawerTags.fetchFailedHeading')}
            <FetchFailedWarning />
          </>
        ) : (
          `Tags (${entries.length})`
        )}
      </h3>
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
