// 非 AWS サービス (BigQuery / Datadog / TiDB) の Raw → Row 変換関数
import type {
  BQDatasetRaw,
  BQDatasetRow,
  BQFieldRaw,
  BQFieldRow,
  BQTableRaw,
  BQTableRow,
  DatadogCostRaw,
  DatadogCostRow,
  DatadogLoginStartRaw,
  DatadogLoginStartRow,
  DatadogLoginStatus,
  DatadogLoginStatusRaw,
  DatadogLoginStatusRow,
  TiDBClusterRaw,
  TiDBClusterRow,
  TiDBCostRaw,
  TiDBCostRow,
  TiDBProjectRaw,
  TiDBProjectRow,
} from '../types/nonaws';

export function bqDatasetFromRaw(raw: BQDatasetRaw): BQDatasetRow {
  return {
    id: raw.dataset_id,
    name: raw.dataset_id,
    location: raw.location,
    creationTime: raw.creation_time,
    lastModifiedTime: raw.last_modified_time,
    description: raw.description,
  };
}

export function bqTableFromRaw(raw: BQTableRaw): BQTableRow {
  return {
    id: raw.table_id,
    name: raw.table_id,
    type: raw.type,
    creationTime: raw.creation_time,
    lastModifiedTime: raw.last_modified_time,
    numRows: raw.num_rows,
    numBytes: raw.num_bytes,
  };
}

export function bqFieldFromRaw(raw: BQFieldRaw): BQFieldRow {
  return {
    id: raw.name,
    name: raw.name,
    type: raw.type,
    mode: raw.mode,
    description: raw.description,
  };
}

export function datadogCostFromRaw(raw: DatadogCostRaw, index: number): DatadogCostRow {
  return {
    id: `${raw.month}-${raw.product_name}-${raw.charge_type}-${index}`,
    month: raw.month,
    accountName: raw.account_name,
    orgName: raw.org_name,
    productName: raw.product_name,
    chargeType: raw.charge_type,
    cost: raw.cost,
  };
}

export function datadogLoginStartFromRaw(raw: DatadogLoginStartRaw): DatadogLoginStartRow {
  return {
    state: raw.state,
    authorizationUrl: raw.authorization_url,
  };
}

// backend が返す進行状態を union 型へ縮小する。未知の値は判定できないため failed として
// 扱い、ポーリングが止まらなくなる (pending とみなし続ける) 事態を避ける。
export function datadogLoginStatusFromRaw(raw: DatadogLoginStatusRaw): DatadogLoginStatusRow {
  const status: DatadogLoginStatus =
    raw.status === 'pending' || raw.status === 'succeeded' ? raw.status : 'failed';
  return {
    status,
    errorMessage: raw.error_message ?? '',
  };
}

export function tidbProjectFromRaw(raw: TiDBProjectRaw): TiDBProjectRow {
  return {
    id: raw.id,
    name: raw.name,
    orgId: raw.org_id,
    clusterCount: raw.cluster_count,
    userCount: raw.user_count,
    createdAt: raw.created_at,
  };
}

export function tidbClusterFromRaw(raw: TiDBClusterRaw): TiDBClusterRow {
  return {
    id: raw.id,
    name: raw.name,
    status: raw.status,
    region: raw.region,
    clusterType: raw.cluster_type,
    cloudProvider: raw.cloud_provider,
    createdAt: raw.created_at,
  };
}

export function tidbCostFromRaw(raw: TiDBCostRaw, index: number): TiDBCostRow {
  return {
    id: `${raw.billed_date}-${raw.project_name}-${raw.cluster_name}-${index}`,
    billedDate: raw.billed_date,
    projectName: raw.project_name,
    clusterName: raw.cluster_name,
    servicePathName: raw.service_path_name,
    credits: raw.credits,
    discounts: raw.discounts,
    runningTotal: raw.running_total,
    totalCost: raw.total_cost,
  };
}
