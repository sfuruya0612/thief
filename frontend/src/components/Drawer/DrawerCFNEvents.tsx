// CloudFormation スタックのイベント一覧 (新しい順) を表示する Drawer タブ
import { useMemo } from 'react';
import { useCFNStackEvents } from '../../api/queries';
import { cfnEventColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';

export interface DrawerCFNEventsProps {
  profile: string;
  region: string;
  stack: string;
}

export function DrawerCFNEvents({ profile, region, stack }: DrawerCFNEventsProps) {
  const { data, isLoading } = useCFNStackEvents(profile, region, stack);
  const events = useMemo(() => data ?? [], [data]);

  return (
    <div className="section">
      <h3>Events ({events.length})</h3>
      {isLoading ? (
        <DrawerLoading />
      ) : (
        <DataTable rows={events} columns={cfnEventColumns} onSelect={() => {}} selectedId={null} />
      )}
    </div>
  );
}
