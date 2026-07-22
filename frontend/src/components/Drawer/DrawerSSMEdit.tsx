// SSM Parameter Store の Drawer「Edit」タブ。一覧クエリ (useResources と同じ queryKey) から
// 現在値を取得し、DrawerValueEditor へ渡す。保存は useSSMUpdate ミューテーション経由。
import { useResources, useSSMUpdate } from '../../api/queries';
import { ssmFromRaw } from '../../lib/normalize';
import type { SSMParamRaw, SSMParamRow } from '../../types/aws';
import { DrawerValueEditor } from './DrawerValueEditor';

interface DrawerSSMEditProps {
  profile: string;
  region: string;
  name: string;
}

export function DrawerSSMEdit({ profile, region, name }: DrawerSSMEditProps) {
  const { data, isLoading, error } = useResources<SSMParamRaw, SSMParamRow>(
    'ssm',
    profile,
    region,
    ssmFromRaw,
  );
  const current = data?.find((r) => r.name === name);
  const update = useSSMUpdate(profile, region);

  return (
    <DrawerValueEditor
      infoRows={[
        ['Name', name],
        ['Type', current?.type ?? '—'],
        ['Tier', current?.tier ?? '—'],
        ['Version', current?.version ?? '—'],
      ]}
      value={current?.value}
      isLoading={isLoading}
      error={error}
      confirmName={name}
      onSave={(value) => update.mutateAsync({ name, value }).then(() => undefined)}
    />
  );
}
