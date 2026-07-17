// 共通型定義

// バックエンドが返す標準エラー DTO を表現するクラス
export class ApiError extends Error {
  readonly statusCode: number;
  readonly code?: string;
  readonly details?: unknown;

  constructor(statusCode: number, code: string | undefined, message: string, details?: unknown) {
    super(message);
    this.name = 'ApiError';
    this.statusCode = statusCode;
    this.code = code;
    this.details = details;
  }
}

export type ServiceGroup = 'compute' | 'data' | 'network' | 'messaging' | 'security' | 'cost';

export interface ServiceMeta {
  key: string;
  name: string;
  sub: string;
  color: string;
  group: ServiceGroup;
}

// トップレベルビュー切替 (AWS / GCP / 非 AWS 統合先)
export type AppView = 'aws' | 'gcp' | 'datadog' | 'tidb';

// tweaks.jsx / index.html 由来のフィールド
export type Theme = 'dark' | 'light';
export type Density = 'compact' | 'cozy' | 'comfortable';
export type Accent = 'indigo' | 'amber' | 'blue' | 'green' | 'purple' | 'pink';
export type DrawerPos = 'right' | 'bottom';
export type Layout = 'tabs-top';

export interface Tweaks {
  theme: Theme;
  density: Density;
  accent: Accent;
  layout: Layout;
  drawerPos: DrawerPos;
}

// GET /api/aws/profiles のバックエンド JSON 形状。
// AccountID / SSORoleName は ~/.aws/config の静的パース結果で、
// SSO プロファイル以外では欠落する。auth_type 以降は認証方式と
// SSO トークンキャッシュ由来のローカル状態 (best-effort 表示用)。
export interface ProfileRaw {
  name: string;
  account_id?: string;
  sso_role_name?: string;
  region?: string;
  auth_type?: string;
  sso_status?: string;
  sso_expires_at?: string;
}

export type ProfileAuthType =
  'sso' | 'access_key' | 'assume_role' | 'credential_process' | 'unknown';
export type ProfileSSOStatus = 'valid' | 'expired' | 'not_logged_in';

export interface Profile {
  name: string;
  accountId?: string;
  ssoRoleName?: string;
  region?: string;
  authType?: ProfileAuthType;
  ssoStatus?: ProfileSSOStatus;
  ssoExpiresAt?: string;
}

// GET /api/aws/profiles/{profile}/identity のバックエンド JSON 形状。
// 選択中プロファイルに対して STS GetCallerIdentity で解決した実際の Account ID。
export interface CallerIdentityRaw {
  account_id: string;
  arn: string;
  user_id: string;
}

export interface CallerIdentity {
  accountId: string;
  arn: string;
  userId: string;
}

// 全 15 サービスの XxxRow が共通して持つ形状 (ServicePanel / Drawer で共有する)
export interface BaseRow {
  id: string;
  name: string;
  state?: string;
  region?: string;
  tags?: Record<string, string>;
}
