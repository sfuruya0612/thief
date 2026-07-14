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
