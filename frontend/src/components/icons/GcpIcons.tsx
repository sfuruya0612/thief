// Google Cloud 公式アイコン (public/assets/gcp-icons/*.svg) を表示するアイコンコンポーネント
import { AssetIcon } from './AssetIcon';

export interface GcpIconProps {
  size?: number;
}

type GcpIconComponent = (p?: GcpIconProps) => JSX.Element;

// サービスキー → public/assets/gcp-icons/ 配下のファイル名
const GCP_ICON_FILES: Record<string, string> = {
  cloudrun: 'cloudrun.svg',
  bigquery: 'bigquery.svg',
  gcs: 'gcs.svg',
};

function GcpIcon(svc: string, { size = 16 }: GcpIconProps = {}) {
  return <AssetIcon src={`/assets/gcp-icons/${GCP_ICON_FILES[svc]}`} alt={svc} size={size} />;
}

export const GcpIcons: Record<string, GcpIconComponent> = Object.fromEntries(
  Object.keys(GCP_ICON_FILES).map((svc) => [svc, (p?: GcpIconProps) => GcpIcon(svc, p)]),
);
