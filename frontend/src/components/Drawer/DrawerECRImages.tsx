// ECR リポジトリのイメージタグ一覧を表示する Drawer タブ
import { useMemo } from 'react';
import { useECRImages } from '../../api/queries';
import { ecrImageColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';
import { DrawerError } from './drawerError';

export interface DrawerECRImagesProps {
  profile: string;
  region: string;
  repo: string;
}

export function DrawerECRImages({ profile, region, repo }: DrawerECRImagesProps) {
  const { data, isLoading, error } = useECRImages(profile, region, repo);
  const images = useMemo(() => data ?? [], [data]);

  // data が無いときはエラー表示のみ、data があるときは既存表示の上部にエラーを出す
  // (issues/0075 で確定した表示規則)。
  return (
    <div className="section">
      <h3>Images{data !== undefined ? ` (${images.length})` : ''}</h3>
      {isLoading ? (
        <DrawerLoading />
      ) : (
        <>
          {error != null && <DrawerError error={error} />}
          {data !== undefined && (
            <DataTable
              rows={images}
              columns={ecrImageColumns}
              onSelect={() => {}}
              selectedId={null}
            />
          )}
        </>
      )}
    </div>
  );
}
