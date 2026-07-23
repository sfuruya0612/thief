// SSM Parameter Store の Drawer「Value」タブ。一覧クエリからメタデータ (Type / Tier / Version)
// を、値は useSSMValue でオンデマンド取得し、DrawerValueEditor へ渡す。
// 保存は useSSMUpdate ミューテーション経由。
import { useResources, useSSMUpdate, useSSMValue } from '../../api/queries';
import { ssmFromRaw } from '../../lib/normalize';
import type { SSMParamRaw, SSMParamRow } from '../../types/aws';
import { DrawerValueEditor } from './DrawerValueEditor';

interface DrawerSSMEditProps {
  profile: string;
  region: string;
  name: string;
  onClose: () => void;
}

export function DrawerSSMEdit({ profile, region, name, onClose }: DrawerSSMEditProps) {
  const { data } = useResources<SSMParamRaw, SSMParamRow>('ssm', profile, region, ssmFromRaw);
  const current = data?.find((r) => r.name === name);
  const { data: value, isLoading, error } = useSSMValue(profile, region, name);
  const update = useSSMUpdate(profile, region);

  return (
    <DrawerValueEditor
      infoRows={[
        ['Name', name],
        ['Type', current?.type ?? '—'],
        ['Tier', current?.tier ?? '—'],
        ['Version', current?.version ?? '—'],
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
