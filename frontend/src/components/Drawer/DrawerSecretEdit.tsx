// Secrets Manager の Drawer「Value」タブ。一覧クエリからメタデータ (Description) を、
// 値は useSecretValue でオンデマンド取得し、DrawerValueEditor へ渡す。
// 保存は useSecretUpdate ミューテーション経由。
import { useResources, useSecretUpdate, useSecretValue } from '../../api/queries';
import { secretFromRaw } from '../../lib/normalize';
import type { SecretRaw, SecretRow } from '../../types/aws';
import { DrawerValueEditor } from './DrawerValueEditor';

interface DrawerSecretEditProps {
  profile: string;
  region: string;
  name: string;
  onClose: () => void;
}

export function DrawerSecretEdit({ profile, region, name, onClose }: DrawerSecretEditProps) {
  const { data } = useResources<SecretRaw, SecretRow>('secrets', profile, region, secretFromRaw);
  const current = data?.find((r) => r.name === name);
  const { data: value, isLoading, error } = useSecretValue(profile, region, name);
  const update = useSecretUpdate(profile, region);

  return (
    <DrawerValueEditor
      infoRows={[
        ['Name', name],
        ['Description', current?.description || '—'],
      ]}
      value={value}
      isLoading={isLoading}
      error={error}
      confirmName={name}
      onSave={(v) => update.mutateAsync({ name, value: v }).then(() => undefined)}
      onClose={onClose}
    />
  );
}
