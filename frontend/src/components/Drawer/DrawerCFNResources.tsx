// CloudFormation スタックが管理するリソース一覧を表示する Drawer タブ
import { useMemo } from 'react';
import { useCFNStackResources } from '../../api/queries';
import { cfnResourceColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';

export interface DrawerCFNResourcesProps {
  profile: string;
  region: string;
  stack: string;
}

export function DrawerCFNResources({ profile, region, stack }: DrawerCFNResourcesProps) {
  const { data, isLoading } = useCFNStackResources(profile, region, stack);
  const resources = useMemo(() => data ?? [], [data]);

  return (
    <div className="section">
      <h3>Resources ({resources.length})</h3>
      {isLoading ? (
        <DrawerLoading />
      ) : (
        <DataTable
          rows={resources}
          columns={cfnResourceColumns}
          onSelect={() => {}}
          selectedId={null}
        />
      )}
    </div>
  );
}
