// AWS 公式 Architecture Icons (public/assets/aws-icons/*.svg) を表示するアイコンコンポーネント
import { AssetIcon } from './AssetIcon';

export interface AwsIconProps {
  size?: number;
}

type AwsIconComponent = (p?: AwsIconProps) => JSX.Element;

// サービスキー → public/assets/aws-icons/ 配下のファイル名
const AWS_ICON_FILES: Record<string, string> = {
  ec2: 'ec2.svg',
  ecr: 'ecr.svg',
  athena: 'athena.svg',
  lambda: 'lambda.svg',
  ecs: 'ecs.svg',
  rds: 'rds.svg',
  dynamo: 'dynamo.svg',
  cache: 'cache.svg',
  s3: 's3.svg',
  iam: 'iam.svg',
  elb: 'elb.svg',
  cloudfront: 'cloudfront.svg',
  apigw: 'apigw.svg',
  natgw: 'natgw.svg',
  sqs: 'sqs.svg',
  kinesis: 'kinesis.svg',
  waf: 'waf.svg',
  ssm: 'ssm.svg',
  secrets: 'secrets.svg',
  costexplorer: 'costexplorer.svg',
  cfn: 'cfn.svg',
  cloudwatchlogs: 'cloudwatchlogs.svg',
};

function AwsIcon(svc: string, { size = 16 }: AwsIconProps = {}) {
  return <AssetIcon src={`/assets/aws-icons/${AWS_ICON_FILES[svc]}`} alt={svc} size={size} />;
}

export const AwsIcons: Record<string, AwsIconComponent> = Object.fromEntries(
  Object.keys(AWS_ICON_FILES).map((svc) => [svc, (p?: AwsIconProps) => AwsIcon(svc, p)]),
);
