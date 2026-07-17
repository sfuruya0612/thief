// GCP サービスの Raw → Row 変換関数
import type {
  CloudRunResourceRaw,
  CloudRunResourceRow,
  GcpProject,
  GcpProjectRaw,
  GcsBucketRaw,
  GcsBucketRow,
  GcsObjectRaw,
  GcsObjectRow,
  IAMBindingRaw,
  IAMBindingRow,
  IAMMemberRow,
  LogEntryRaw,
  LogEntryRow,
  ServiceAccountRaw,
  ServiceAccountRow,
} from '../types/gcp';

export function gcpProjectFromRaw(raw: GcpProjectRaw): GcpProject {
  return {
    id: raw.project_id,
    name: raw.name || raw.project_id,
    projectNumber: raw.project_number,
    state: raw.state,
    createTime: raw.create_time,
  };
}

export function cloudRunResourceFromRaw(raw: CloudRunResourceRaw): CloudRunResourceRow {
  return {
    id: `${raw.kind}/${raw.region}/${raw.name}`,
    name: raw.name,
    kind: raw.kind,
    region: raw.region,
    projectId: raw.project_id,
    uri: raw.uri,
    createTime: raw.create_time,
    updateTime: raw.update_time,
  };
}

export function gcsBucketFromRaw(raw: GcsBucketRaw): GcsBucketRow {
  return {
    id: raw.name,
    name: raw.name,
    region: raw.location,
    location: raw.location,
    storageClass: raw.storage_class,
    createTime: raw.create_time,
  };
}

export function gcsObjectFromRaw(raw: GcsObjectRaw, index: number): GcsObjectRow {
  return {
    id: `${raw.bucket}/${raw.name}#${index}`,
    name: raw.name,
    bucket: raw.bucket,
    size: raw.size,
    contentType: raw.content_type,
    updated: raw.updated,
    storageClass: raw.storage_class,
  };
}

export function iamBindingFromRaw(raw: IAMBindingRaw): IAMBindingRow {
  return {
    id: `${raw.member}/${raw.role}/${raw.condition_title}`,
    name: raw.member,
    member: raw.member,
    role: raw.role,
    projectId: raw.project_id,
    conditionTitle: raw.condition_title,
  };
}

// groupIAMBindingsByMember は member 単位で IAMBindingRow を集約する。
// GCP の IAM ポリシーは (role, members[]) のバインディング形式のため、同じメンバーが
// 複数のロールに紐づく場合バックエンドは行を分けて返す。一覧表示ではユーザーが
// 「このメンバーにどのロールが付いているか」を一目で見られるよう、メンバー単位の
// 1 行にロール一覧をまとめる。
export function groupIAMBindingsByMember(bindings: IAMBindingRow[]): IAMMemberRow[] {
  const order: string[] = [];
  const byMember = new Map<string, IAMMemberRow>();

  for (const b of bindings) {
    let row = byMember.get(b.member);
    if (!row) {
      row = { id: b.member, name: b.member, member: b.member, roles: [], projectId: b.projectId };
      byMember.set(b.member, row);
      order.push(b.member);
    }
    if (!row.roles.includes(b.role)) {
      row.roles.push(b.role);
    }
  }

  return order.map((member) => byMember.get(member)!);
}

export function serviceAccountFromRaw(raw: ServiceAccountRaw): ServiceAccountRow {
  return {
    id: raw.email,
    name: raw.display_name || raw.email,
    email: raw.email,
    displayName: raw.display_name,
    description: raw.description,
    projectId: raw.project_id,
    uniqueId: raw.unique_id,
    disabled: raw.disabled,
  };
}

// ============================================================
// Cloud Logging
// ============================================================
// insert_id はログエントリの一意な ID だが、同じ filter で複数ページ・複数回取得した
// 結果を突き合わせると理論上衝突しうるため、呼び出し側の連番 (index) も id に含めて一意性を担保する。
export function logEntryFromRaw(raw: LogEntryRaw, index: number): LogEntryRow {
  return {
    id: `${raw.insert_id || raw.timestamp}#${index}`,
    timestamp: raw.timestamp,
    severity: raw.severity,
    logName: raw.log_name,
    resourceType: raw.resource_type,
    resourceLabels: raw.resource_labels ?? {},
    labels: raw.labels ?? {},
    payload: raw.payload,
    insertId: raw.insert_id,
    trace: raw.trace ?? '',
  };
}

// logSeverityLevel は Cloud Logging の Severity 文字列 (Default/Debug/Info/Notice/
// Warning/Error/Critical/Alert/Emergency、backend の logging.Severity.String() 準拠) を
// app.css の .logbox 系クラス (.lvl-info/.lvl-warn/.lvl-err) に対応する 3 段階へ丸める。
export function logSeverityLevel(severity: string): 'info' | 'warn' | 'err' {
  switch (severity) {
    case 'Error':
    case 'Critical':
    case 'Alert':
    case 'Emergency':
      return 'err';
    case 'Warning':
    case 'Notice':
      return 'warn';
    default:
      return 'info';
  }
}
