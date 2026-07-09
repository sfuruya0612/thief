// ECR リポジトリのイメージタグ一覧を表示する Drawer タブ
import { useMemo, useState } from 'react';
import { useECRImages } from '../../api/queries';
import { ecrImageColumns } from '../tables/columns';
import { DataTable } from '../DataTable';
import { FacetBar } from '../FacetBar';

export interface DrawerECRImagesProps {
  profile: string;
  region: string;
  repo: string;
}

export function DrawerECRImages({ profile, region, repo }: DrawerECRImagesProps) {
  const { data, isLoading } = useECRImages(profile, region, repo);
  const [search, setSearch] = useState('');
  const images = useMemo(() => data ?? [], [data]);

  const filtered = useMemo(() => {
    if (!search) return images;
    const q = search.toLowerCase();
    return images.filter((r) => `${r.imageTag} ${r.imageDigest}`.toLowerCase().includes(q));
  }, [images, search]);

  return (
    <div className="section">
      <h3>
        Images ({filtered.length}/{images.length})
      </h3>
      {isLoading ? (
        <div style={{ padding: 20, color: 'var(--text-3)' }}>Loading…</div>
      ) : (
        <>
          <FacetBar
            rows={[]}
            filters={{}}
            setFilters={() => {}}
            search={search}
            setSearch={setSearch}
          />
          <DataTable
            rows={filtered}
            columns={ecrImageColumns}
            onSelect={() => {}}
            selectedId={null}
          />
        </>
      )}
    </div>
  );
}
