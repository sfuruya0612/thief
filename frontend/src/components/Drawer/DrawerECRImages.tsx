// ECR リポジトリのイメージタグ一覧を表示する Drawer タブ
import { useMemo } from 'react';
import { useECRImages } from '../../api/queries';
import { ecrImageColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { DrawerLoading } from './DrawerLoading';

export interface DrawerECRImagesProps {
  profile: string;
  region: string;
  repo: string;
}

export function DrawerECRImages({ profile, region, repo }: DrawerECRImagesProps) {
  const { data, isLoading } = useECRImages(profile, region, repo);
  const images = useMemo(() => data ?? [], [data]);

  return (
    <div className="section">
      <h3>Images ({images.length})</h3>
      {isLoading ? (
        <DrawerLoading />
      ) : (
        <DataTable rows={images} columns={ecrImageColumns} onSelect={() => {}} selectedId={null} />
      )}
    </div>
  );
}
