// backend が生成したゴールデン JSON (src/types/__contract__/) と Raw 型の契約検査。
// 各 Raw 型とゴールデンのキー集合の双方向一致と、各フィールドの型の適合を型レベルで
// 検査する。不一致は tsc --noEmit (npm run lint に含まれる) のエラーとして現れる。
// ゴールデンの生成と再生成は backend 側 (backend/internal/contract/) が担う。
// 再生成: backend で UPDATE_GOLDEN=1 go test ./internal/contract/ を実行する。
//
// 検査から除外したフィールドの一覧 (キー集合の検査には含まれ、型適合の検査だけを除外する。
// 除外は当該フィールドの型を string に広げた検査用の型 (下の PriceRateRawWidened) で行う):
// | 型 | フィールド | 理由 |
// | --- | --- | --- |
// | PriceRateRaw | model | Raw 側が PriceModel リテラルユニオンで backend 側は string。フィラーの生成値がユニオン外になるため |
// | PriceTableRaw | rates[].model | 同上 (PriceRateRaw のネストとして現れる) |
//
// 検査の限界 (issues/closed/0099 の「決定した設計」のとおり受け入れる):
// - null 許容 (| null) の過不足は検出できない (ゴールデンは全フィールドに非 null の値を持つ)。
// - backend の omitempty を Raw 側が必須と宣言する食い違いは検出できない (ゴールデンには
//   キーが常に現れる)。omitempty と optional の対応付けはフィールドを追加する issue 側の
//   レビューで担保する。

import type {
  APIGWRaw,
  CFNStackRaw,
  CFNStackDetailRaw,
  CFNParameterRaw,
  CFNOutputRaw,
  CFNStackEventRaw,
  CFNStackResourceRaw,
  CloudFrontRaw,
  CloudFrontBehaviorRaw,
  CWLogGroupRaw,
  CWLogEventRaw,
  CWLogEventPageRaw,
  CostRaw,
  ForecastRaw,
  DynamoRaw,
  DynamoKeyAttributeRaw,
  DynamoIndexSchemaRaw,
  DynamoTableSchemaRaw,
  EC2Raw,
  ECRRepoRaw,
  ECRImageRaw,
  ECSRaw,
  ECSServiceRaw,
  ECSTaskRaw,
  ECSTaskContainerDetailRaw,
  ECSContainerRaw,
  CacheRaw,
  CacheParameterRaw,
  ELBRaw,
  ELBListenerRaw,
  ELBRuleRaw,
  ELBTargetGroupRaw,
  ELBTargetHealthRaw,
  IAMRaw,
  KinesisRaw,
  LambdaRaw,
  NATGWRaw,
  PriceTableRaw,
  PriceRateRaw,
  PriceTermRaw,
  RDSRaw,
  RDSParameterRaw,
  RDSClusterParameterGroupRaw,
  RegionRaw,
  S3Raw,
  S3ObjectRaw,
  SecretRaw,
  SQSRaw,
  SSMParamRaw,
  WAFRaw,
  WAFRuleRaw,
  ValueRaw,
} from './aws';
import type {
  AthenaCatalogRaw,
  AthenaDatabaseRaw,
  AthenaWorkgroupRaw,
  AthenaColumnRaw,
  AthenaTableRaw,
  AthenaExecutionRaw,
  AthenaResultColumnRaw,
  AthenaResultPageRaw,
} from './query';

import apigw from './__contract__/APIGatewayResource.json';
import athenaCatalog from './__contract__/AthenaCatalog.json';
import athenaDatabase from './__contract__/AthenaDatabase.json';
import athenaWorkgroup from './__contract__/AthenaWorkgroup.json';
import athenaColumn from './__contract__/AthenaColumn.json';
import athenaTable from './__contract__/AthenaTable.json';
import athenaExecution from './__contract__/AthenaQueryExecution.json';
import athenaResultColumn from './__contract__/AthenaResultColumn.json';
import athenaResultPage from './__contract__/AthenaResultPage.json';
import cfnStack from './__contract__/CFNStackResource.json';
import cfnStackDetail from './__contract__/CFNStackDetail.json';
import cfnParameter from './__contract__/CFNParameter.json';
import cfnOutput from './__contract__/CFNOutput.json';
import cfnStackEvent from './__contract__/CFNStackEvent.json';
import cfnStackResource from './__contract__/CFNStackResourceSummary.json';
import cloudfront from './__contract__/CloudFrontResource.json';
import cloudfrontBehavior from './__contract__/CloudFrontBehavior.json';
import cwLogGroup from './__contract__/LogGroupInfo.json';
import cwLogEvent from './__contract__/LogEventInfo.json';
import cwLogEventPage from './__contract__/LogEventPage.json';
import cost from './__contract__/CostResource.json';
import forecast from './__contract__/ForecastResource.json';
import dynamo from './__contract__/DynamoResource.json';
import dynamoKeyAttribute from './__contract__/DynamoKeyAttribute.json';
import dynamoIndexSchema from './__contract__/DynamoIndexSchema.json';
import dynamoTableSchema from './__contract__/DynamoTableSchema.json';
import ec2 from './__contract__/EC2Resource.json';
import ecrRepo from './__contract__/ECRRepoResource.json';
import ecrImage from './__contract__/ECRImageResource.json';
import ecs from './__contract__/ECSResource.json';
import ecsService from './__contract__/ECSServiceResource.json';
import ecsTask from './__contract__/ECSTaskResource.json';
import ecsTaskContainerDetail from './__contract__/ECSTaskContainerDetail.json';
import ecsContainer from './__contract__/ECSContainerResource.json';
import cache from './__contract__/ElastiCacheResource.json';
import cacheParameter from './__contract__/ElastiCacheParameter.json';
import elb from './__contract__/ELBResource.json';
import elbListener from './__contract__/ELBListenerResource.json';
import elbRule from './__contract__/ELBRuleResource.json';
import elbTargetGroup from './__contract__/ELBTargetGroupResource.json';
import elbTargetHealth from './__contract__/ELBTargetHealthResource.json';
import iam from './__contract__/IAMResource.json';
import kinesis from './__contract__/KinesisResource.json';
import lambda from './__contract__/LambdaResource.json';
import natgw from './__contract__/NATGatewayResource.json';
import priceTable from './__contract__/PriceTable.json';
import priceRate from './__contract__/PriceRate.json';
import priceTerm from './__contract__/PriceTerm.json';
import rds from './__contract__/RDSResource.json';
import rdsParameter from './__contract__/RDSParameter.json';
import rdsClusterParameterGroup from './__contract__/RDSClusterParameterGroup.json';
import region from './__contract__/RegionResource.json';
import s3 from './__contract__/S3Resource.json';
import s3Object from './__contract__/S3ObjectResource.json';
import secret from './__contract__/SecretResource.json';
import sqs from './__contract__/SQSResource.json';
import ssmParam from './__contract__/SSMParameterResource.json';
import waf from './__contract__/WAFResource.json';
import wafRule from './__contract__/WAFRule.json';
import valueResponse from './__contract__/ValueResponse.json';

// NonUndef は optional フィールドの型から undefined を除く (keyof は optional のキーも
// 含むため、キー集合の検査は optional の宣言に影響されない)。
type NonUndef<T> = Exclude<T, undefined>;

// Check はゴールデンの値の型 G が Raw のフィールド型 R に適合するかを再帰的に検査する。
// 一致なら true、不一致なら原因を示すオブジェクト型に解決される。Raw 側の | null は
// 検査前に取り除く (null 許容の過不足は本検査の対象外)。
type Check<G, R> = G extends readonly (infer GE)[]
  ? Exclude<R, null> extends readonly (infer RE)[]
    ? Check<GE, RE>
    : { error: 'raw side is not an array'; raw: R }
  : G extends object
    ? Exclude<R, null> extends object
      ? ObjectCheck<G, Exclude<R, null>>
      : { error: 'raw side is not an object'; raw: R }
    : [G] extends [R]
      ? true
      : { error: 'field type mismatch'; golden: G; raw: R };

// ObjectCheck はキー集合の双方向一致を検査し、一致した場合に各フィールドへ Check を適用する。
// Raw 側がインデックスシグネチャ (Record<string, T> 等) の場合はキー集合が固定されないため、
// 値の型の適合だけを検査する。この分岐は純粋な辞書型 (tags 等) だけを想定しており、名前付き
// フィールドとインデックスシグネチャが同居する Raw 型が契約対象に加わると、その階層は
// キー集合もフィールドごとの型も検査されなくなる。そのような型を追加する場合は分岐の拡張が
// 必要である。
type ObjectCheck<G, R> = string extends keyof R
  ? Collapse<{ [K in keyof G]: Check<G[K], NonUndef<R[keyof R]>> }>
  : [Exclude<keyof G, keyof R>] extends [never]
    ? [Exclude<keyof R, keyof G>] extends [never]
      ? Collapse<{
          [K in keyof G & keyof R]: Check<G[K], NonUndef<R[K]>>;
        }>
      : {
          error: 'keys only in raw (backend side lacks these fields)';
          keys: Exclude<keyof R, keyof G>;
        }
    : {
        error: 'keys only in golden (raw side lacks these fields)';
        keys: Exclude<keyof G, keyof R>;
      };

// Collapse はフィールドごとの検査結果を集約し、全フィールドが true なら true、
// そうでなければ不一致のフィールドだけを残したオブジェクト型に解決される。
type Collapse<M> = M extends { [K in keyof M]: true }
  ? true
  : { [K in keyof M as M[K] extends true ? never : K]: M[K] };

// Contract は 1 つのゴールデンと Raw 型の対の検査の入口。
type Contract<G, R> = ObjectCheck<G, R>;

// Expect は T が true (契約成立) でなければ型エラーになる。
type Expect<T extends true> = T;

// PriceRateRawWidened は除外一覧の model の型を string に広げた検査用の型。Omit と交差の
// 合成なのでキー集合は PriceRateRaw と同一に保たれ、キー集合の検査は除外の影響を受けない。
type PriceRateRawWidened = Omit<PriceRateRaw, 'model'> & { model: string };
type PriceTableRawWidened = Omit<PriceTableRaw, 'rates'> & { rates: PriceRateRawWidened[] };

// 契約対象 60 対の検査。並びは backend/internal/contract/contract.go の Registry と同じ。
export type ContractChecks = [
  Expect<Contract<typeof apigw, APIGWRaw>>,
  Expect<Contract<typeof athenaCatalog, AthenaCatalogRaw>>,
  Expect<Contract<typeof athenaDatabase, AthenaDatabaseRaw>>,
  Expect<Contract<typeof athenaWorkgroup, AthenaWorkgroupRaw>>,
  Expect<Contract<typeof athenaColumn, AthenaColumnRaw>>,
  Expect<Contract<typeof athenaTable, AthenaTableRaw>>,
  Expect<Contract<typeof athenaExecution, AthenaExecutionRaw>>,
  Expect<Contract<typeof athenaResultColumn, AthenaResultColumnRaw>>,
  Expect<Contract<typeof athenaResultPage, AthenaResultPageRaw>>,
  Expect<Contract<typeof cfnStack, CFNStackRaw>>,
  Expect<Contract<typeof cfnStackDetail, CFNStackDetailRaw>>,
  Expect<Contract<typeof cfnParameter, CFNParameterRaw>>,
  Expect<Contract<typeof cfnOutput, CFNOutputRaw>>,
  Expect<Contract<typeof cfnStackEvent, CFNStackEventRaw>>,
  Expect<Contract<typeof cfnStackResource, CFNStackResourceRaw>>,
  Expect<Contract<typeof cloudfront, CloudFrontRaw>>,
  Expect<Contract<typeof cloudfrontBehavior, CloudFrontBehaviorRaw>>,
  Expect<Contract<typeof cwLogGroup, CWLogGroupRaw>>,
  Expect<Contract<typeof cwLogEvent, CWLogEventRaw>>,
  Expect<Contract<typeof cwLogEventPage, CWLogEventPageRaw>>,
  Expect<Contract<typeof cost, CostRaw>>,
  Expect<Contract<typeof forecast, ForecastRaw>>,
  Expect<Contract<typeof dynamo, DynamoRaw>>,
  Expect<Contract<typeof dynamoKeyAttribute, DynamoKeyAttributeRaw>>,
  Expect<Contract<typeof dynamoIndexSchema, DynamoIndexSchemaRaw>>,
  Expect<Contract<typeof dynamoTableSchema, DynamoTableSchemaRaw>>,
  Expect<Contract<typeof ec2, EC2Raw>>,
  Expect<Contract<typeof ecrRepo, ECRRepoRaw>>,
  Expect<Contract<typeof ecrImage, ECRImageRaw>>,
  Expect<Contract<typeof ecs, ECSRaw>>,
  Expect<Contract<typeof ecsService, ECSServiceRaw>>,
  Expect<Contract<typeof ecsTask, ECSTaskRaw>>,
  Expect<Contract<typeof ecsTaskContainerDetail, ECSTaskContainerDetailRaw>>,
  Expect<Contract<typeof ecsContainer, ECSContainerRaw>>,
  Expect<Contract<typeof cache, CacheRaw>>,
  Expect<Contract<typeof cacheParameter, CacheParameterRaw>>,
  Expect<Contract<typeof elb, ELBRaw>>,
  Expect<Contract<typeof elbListener, ELBListenerRaw>>,
  Expect<Contract<typeof elbRule, ELBRuleRaw>>,
  Expect<Contract<typeof elbTargetGroup, ELBTargetGroupRaw>>,
  Expect<Contract<typeof elbTargetHealth, ELBTargetHealthRaw>>,
  Expect<Contract<typeof iam, IAMRaw>>,
  Expect<Contract<typeof kinesis, KinesisRaw>>,
  Expect<Contract<typeof lambda, LambdaRaw>>,
  Expect<Contract<typeof natgw, NATGWRaw>>,
  Expect<Contract<typeof priceTable, PriceTableRawWidened>>,
  Expect<Contract<typeof priceRate, PriceRateRawWidened>>,
  Expect<Contract<typeof priceTerm, PriceTermRaw>>,
  Expect<Contract<typeof rds, RDSRaw>>,
  Expect<Contract<typeof rdsParameter, RDSParameterRaw>>,
  Expect<Contract<typeof rdsClusterParameterGroup, RDSClusterParameterGroupRaw>>,
  Expect<Contract<typeof region, RegionRaw>>,
  Expect<Contract<typeof s3, S3Raw>>,
  Expect<Contract<typeof s3Object, S3ObjectRaw>>,
  Expect<Contract<typeof secret, SecretRaw>>,
  Expect<Contract<typeof sqs, SQSRaw>>,
  Expect<Contract<typeof ssmParam, SSMParamRaw>>,
  Expect<Contract<typeof waf, WAFRaw>>,
  Expect<Contract<typeof wafRule, WAFRuleRaw>>,
  Expect<Contract<typeof valueResponse, ValueRaw>>,
];
