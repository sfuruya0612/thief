// CloudFormation スタックのイベント一覧 (新しい順) を表示する Drawer タブ
import { useMemo } from 'react';
import { useCFNStackEvents } from '../../api/queries';
import { cfnEventColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';
import { DrawerError } from './drawerError';

export interface DrawerCFNEventsProps {
  profile: string;
  region: string;
  stack: string;
}

export function DrawerCFNEvents({ profile, region, stack }: DrawerCFNEventsProps) {
  const { data, isLoading, error } = useCFNStackEvents(profile, region, stack);
  const events = useMemo(() => data ?? [], [data]);

  // data が無いときはエラー表示のみ、data があるときは既存表示の上部にエラーを出す
  // (issues/0075 で確定した表示規則)。
  return (
    <div className="section">
      <h3>Events{data !== undefined ? ` (${events.length})` : ''}</h3>
      {isLoading ? (
        <DrawerLoading />
      ) : (
        <>
          {error != null && <DrawerError error={error} />}
          {data !== undefined && (
            <DataTable
              rows={events}
              columns={cfnEventColumns}
              onSelect={() => {}}
              selectedId={null}
            />
          )}
        </>
      )}
    </div>
  );
}
