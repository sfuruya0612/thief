// 非 AWS サービス (BigQuery / Datadog / TiDB) の Raw (JSON) / Row (UI 用) 型定義
// Raw は backend/internal/{bigquery,datadog,tidb}/*.go の JSON タグをミラーする
import type { TimeseriesPoint } from '../lib/timeseries';

// ============================================================
// BigQuery
// ============================================================
export interface BQDatasetRaw {
  dataset_id: string;
  location: string;
  creation_time: string;
  last_modified_time: string;
  description: string;
}

export interface BQDatasetRow {
  id: string;
  name: string;
  location: string;
  creationTime: string;
  lastModifiedTime: string;
  description: string;
}

export interface BQTableRaw {
  table_id: string;
  type: string;
  creation_time: string;
  last_modified_time: string;
  num_rows: number;
  num_bytes: number;
}

export interface BQTableRow {
  id: string;
  name: string;
  type: string;
  creationTime: string;
  lastModifiedTime: string;
  numRows: number;
  numBytes: number;
}

export interface BQFieldRaw {
  name: string;
  type: string;
  mode: string;
  description: string;
}

export interface BQFieldRow {
  id: string;
  name: string;
  type: string;
  mode: string;
  description: string;
}

// ============================================================
// Datadog
// ============================================================
export interface DatadogCostRaw {
  month: string;
  account_name: string;
  org_name: string;
  product_name: string;
  charge_type: string;
  cost: number;
}

export interface DatadogCostRow {
  id: string;
  month: string;
  accountName: string;
  orgName: string;
  productName: string;
  chargeType: string;
  cost: number;
}

// Datadog の組織 (親組織と Sub Organization)。id は backend が小文字へ正規化した
// public_id で、API 呼び出しとトークンの保存先の識別子を兼ねる。name は表示名。
// loggedIn はその組織向けの OAuth トークンが保存されているかどうか。
export interface DatadogOrgRaw {
  id: string;
  name: string;
  logged_in: boolean;
}

export interface DatadogOrgRow {
  id: string;
  name: string;
  loggedIn: boolean;
}

// Datadog OAuth ログイン (backend の datadogLoginStartResponse /
// datadogLoginStatusResponse をミラーする)。state は認可要求の state であり、
// login/status で進行状態を引くためのキーでもある。
export interface DatadogLoginStartRaw {
  state: string;
  authorization_url: string;
}

export interface DatadogLoginStartRow {
  state: string;
  authorizationUrl: string;
}

// status は backend の datadogLoginStatus ("pending" / "succeeded" / "failed")。
// error_message は failed のときだけ入る (成功時は省略される)。
export interface DatadogLoginStatusRaw {
  status: string;
  error_message?: string;
}

export type DatadogLoginStatus = 'pending' | 'succeeded' | 'failed';

export interface DatadogLoginStatusRow {
  status: DatadogLoginStatus;
  errorMessage: string;
}

// Datadog のダッシュボード。url は backend が app.<site> を補った絶対 URL で、
// 「Open in Datadog」リンクにそのまま使える。
export interface DatadogDashboardRaw {
  id: string;
  title: string;
  description: string;
  url: string;
}

export interface DatadogDashboardRow {
  id: string;
  title: string;
  description: string;
  url: string;
}

// kind は thief がウィジェットをどう描くか (backend の WidgetKind)。type は Datadog の
// 種別名で、kind が 'unsupported' のときに何のウィジェットだったかを利用者へ示す。
export type DatadogWidgetKind = 'timeseries' | 'query_value' | 'unsupported';

export interface DatadogWidgetRaw {
  id: number;
  kind: string;
  type: string;
  title: string;
  queries: string[] | null;
}

export interface DatadogWidgetRow {
  id: number;
  kind: DatadogWidgetKind;
  type: string;
  title: string;
  queries: string[];
}

export interface DatadogDashboardDetailRaw {
  id: string;
  title: string;
  description: string;
  url: string;
  widgets: DatadogWidgetRaw[] | null;
}

export interface DatadogDashboardDetailRow {
  id: string;
  title: string;
  description: string;
  url: string;
  widgets: DatadogWidgetRow[];
}

// ウィジェットのクエリを実行した結果の時系列。t はエポックミリ秒、v は欠測が null。
export interface DatadogMetricPointRaw {
  t: number;
  v: number | null;
}

export interface DatadogMetricSeriesRaw {
  name: string;
  scope: string;
  unit: string;
  points: DatadogMetricPointRaw[] | null;
}

export interface DatadogMetricQueryResultRaw {
  query: string;
  series: DatadogMetricSeriesRaw[] | null;
}

export interface DatadogMetricSeriesRow {
  name: string;
  scope: string;
  unit: string;
  points: TimeseriesPoint[];
}

export interface DatadogMetricQueryResultRow {
  query: string;
  series: DatadogMetricSeriesRow[];
}

// ============================================================
// TiDB
// ============================================================
export interface TiDBProjectRaw {
  id: string;
  name: string;
  org_id: string;
  cluster_count: number;
  user_count: number;
  created_at: string;
}

export interface TiDBProjectRow {
  id: string;
  name: string;
  orgId: string;
  clusterCount: number;
  userCount: number;
  createdAt: string;
}

export interface TiDBClusterRaw {
  id: string;
  name: string;
  status: string;
  region: string;
  cluster_type: string;
  cloud_provider: string;
  created_at: string;
}

export interface TiDBClusterRow {
  id: string;
  name: string;
  status: string;
  region: string;
  clusterType: string;
  cloudProvider: string;
  createdAt: string;
}

export interface TiDBCostRaw {
  billed_date: string;
  project_name: string;
  cluster_name: string;
  service_path_name: string;
  credits: number;
  discounts: number;
  running_total: number;
  total_cost: number;
}

export interface TiDBCostRow {
  id: string;
  billedDate: string;
  projectName: string;
  clusterName: string;
  servicePathName: string;
  credits: number;
  discounts: number;
  runningTotal: number;
  totalCost: number;
}
