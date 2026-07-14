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
