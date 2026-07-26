// CloudFormation スタックの Tags タブ。一覧 API (ListCFNStacks) はタグを返さないため
// スタック詳細 API (DescribeStacks) から取得したタグを表示する。
import { useCFNStackDetail } from '../../api/queries';
import { DrawerLoading } from './DrawerLoading';
import { DrawerError } from './drawerError';
import { DrawerTags } from './DrawerTags';

export interface DrawerCFNTagsProps {
  profile: string;
  region: string;
  stack: string;
}

export function DrawerCFNTags({ profile, region, stack }: DrawerCFNTagsProps) {
  const { data, isLoading, error } = useCFNStackDetail(profile, region, stack);

  if (isLoading) return <DrawerLoading />;
  // data が無いときはエラー表示のみ、data があるときは既存表示の上部にエラーを出す
  // (issues/0075 で確定した表示規則)。
  return (
    <>
      {error != null && <DrawerError error={error} />}
      {data !== undefined && <DrawerTags tags={data.tags} />}
    </>
  );
}
