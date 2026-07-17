// CloudFormation スタックの Tags タブ。一覧 API (ListCFNStacks) はタグを返さないため
// スタック詳細 API (DescribeStacks) から取得したタグを表示する。
import { useCFNStackDetail } from '../../api/queries';
import { DrawerLoading } from './DrawerLoading';
import { DrawerTags } from './DrawerTags';

export interface DrawerCFNTagsProps {
  profile: string;
  region: string;
  stack: string;
}

export function DrawerCFNTags({ profile, region, stack }: DrawerCFNTagsProps) {
  const { data, isLoading } = useCFNStackDetail(profile, region, stack);

  if (isLoading) return <DrawerLoading />;
  return <DrawerTags tags={data?.tags} />;
}
