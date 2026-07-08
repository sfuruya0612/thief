// 非 AWS サービス (BigQuery / Datadog / TiDB) の Raw (JSON) / Row (UI 用) 型定義
// Raw は backend/internal/{bigquery,datadog,tidb}/*.go の JSON タグをミラーする

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

export interface BQQueryResult {
  columns: string[];
  rows: string[][];
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
