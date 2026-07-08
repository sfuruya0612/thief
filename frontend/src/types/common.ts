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

export type ServiceGroup = 'compute' | 'data' | 'network' | 'messaging' | 'security';

export interface ServiceMeta {
  key: string;
  name: string;
  sub: string;
  color: string;
  group: ServiceGroup;
}

// トップレベルビュー切替 (AWS / 非 AWS 統合先)
export type AppView = 'aws' | 'bigquery' | 'datadog' | 'tidb';

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
  showMiniCharts: boolean;
}

export interface Profile {
  name: string;
}

// 全 15 サービスの XxxRow が共通して持つ形状 (ServicePanel / Drawer で共有する)
export interface BaseRow {
  id: string;
  name: string;
  state?: string;
  region?: string;
  tags?: Record<string, string>;
}
