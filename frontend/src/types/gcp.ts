// GCP サービスの Raw (JSON) / Row (UI 用) 型定義
// Raw は backend/internal/gcp/*.go の JSON タグをミラーする想定

// ============================================================
// Project
// ============================================================
export interface GcpProjectRaw {
  project_id: string;
  name: string;
  project_number: string;
  state: string;
  create_time: string;
}

export interface GcpProject {
  id: string;
  name: string;
  projectNumber: string;
  state: string;
  createTime: string;
}

// ============================================================
// Cloud Run (Service / Job 共通の一覧行)
// ============================================================
export interface CloudRunResourceRaw {
  name: string;
  kind: 'service' | 'job';
  region: string;
  project_id: string;
  uri: string;
  create_time: string;
  update_time: string;
}

export interface CloudRunResourceRow {
  id: string;
  name: string;
  kind: 'service' | 'job';
  region: string;
  projectId: string;
  uri: string;
  createTime: string;
  updateTime: string;
}

// ============================================================
// Cloud Storage (GCS)
// ============================================================
export interface GcsBucketRaw {
  name: string;
  location: string;
  storage_class: string;
  create_time: string;
}

export interface GcsBucketRow {
  id: string;
  name: string;
  region: string;
  location: string;
  storageClass: string;
  createTime: string;
}

export interface GcsObjectRaw {
  name: string;
  bucket: string;
  size: number;
  content_type: string;
  updated: string;
  storage_class: string;
}

export interface GcsObjectRow {
  id: string;
  name: string;
  bucket: string;
  size: number;
  contentType: string;
  updated: string;
  storageClass: string;
}

// ============================================================
// IAM (プロジェクトの IAM ポリシーをメンバー単位に展開した 1 行)
// ============================================================
export interface IAMBindingRaw {
  member: string;
  role: string;
  project_id: string;
  condition_title: string;
}

export interface IAMBindingRow {
  id: string;
  name: string;
  member: string;
  role: string;
  projectId: string;
  conditionTitle: string;
}

// メンバー単位に IAMBindingRow を集約した表示行。1 メンバーが複数ロールを持つ場合、
// role には全ロールが含まれる (一覧では 1 メンバー = 1 行として表示する)。
export interface IAMMemberRow {
  id: string;
  name: string;
  member: string;
  roles: string[];
  projectId: string;
}

// ============================================================
// Service Account
// ============================================================
export interface ServiceAccountRaw {
  email: string;
  display_name: string;
  description: string;
  project_id: string;
  unique_id: string;
  disabled: boolean;
}

export interface ServiceAccountRow {
  id: string;
  name: string;
  email: string;
  displayName: string;
  description: string;
  projectId: string;
  uniqueId: string;
  disabled: boolean;
}
