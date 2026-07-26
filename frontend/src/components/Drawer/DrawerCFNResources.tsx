// CloudFormation スタックが管理するリソース一覧を表示する Drawer タブ
import { useMemo } from 'react';
import { useCFNStackResources } from '../../api/queries';
import { cfnResourceColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';
import { DrawerError } from './drawerError';

export interface DrawerCFNResourcesProps {
  profile: string;
  region: string;
  stack: string;
}

export function DrawerCFNResources({ profile, region, stack }: DrawerCFNResourcesProps) {
  const { data, isLoading, error } = useCFNStackResources(profile, region, stack);
  const resources = useMemo(() => data ?? [], [data]);

  // data が無いときはエラー表示のみ、data があるときは既存表示の上部にエラーを出す
  // (issues/0075 で確定した表示規則)。
  return (
    <div className="section">
      <h3>Resources{data !== undefined ? ` (${resources.length})` : ''}</h3>
      {isLoading ? (
        <DrawerLoading />
      ) : (
        <>
          {error != null && <DrawerError error={error} />}
          {data !== undefined && (
            <DataTable
              rows={resources}
              columns={cfnResourceColumns}
              onSelect={() => {}}
              selectedId={null}
            />
          )}
        </>
      )}
    </div>
  );
}
