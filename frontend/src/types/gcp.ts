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
