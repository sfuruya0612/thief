// Secrets Manager の Drawer「Edit」タブ。一覧クエリ (useResources と同じ queryKey) から
// 現在値を取得し、DrawerValueEditor へ渡す。保存は useSecretUpdate ミューテーション経由。
import { useResources, useSecretUpdate } from '../../api/queries';
import { secretFromRaw } from '../../lib/normalize';
import type { SecretRaw, SecretRow } from '../../types/aws';
import { DrawerValueEditor } from './DrawerValueEditor';

interface DrawerSecretEditProps {
  profile: string;
  region: string;
  name: string;
}

export function DrawerSecretEdit({ profile, region, name }: DrawerSecretEditProps) {
  const { data, isLoading, error } = useResources<SecretRaw, SecretRow>(
    'secrets',
    profile,
    region,
    secretFromRaw,
  );
  const current = data?.find((r) => r.name === name);
  const update = useSecretUpdate(profile, region);

  return (
    <DrawerValueEditor
      infoRows={[
        ['Name', name],
        ['Description', current?.description || '—'],
      ]}
      value={current?.value}
      isLoading={isLoading}
      error={error}
      confirmName={name}
      onSave={(value) => update.mutateAsync({ name, value }).then(() => undefined)}
    />
  );
}
