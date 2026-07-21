// セッションタブ / ピッカー / アクティブセッションカードの表示ロジック。
// 環境色ドットの判定、SSO 状態バッジ、ピッカー項目の構築を純関数で提供する。
import i18n from '../i18n';
import type { GcpProject } from '../types/gcp';
import type { Profile } from '../types/common';

export type SessionEnv = 'dev' | 'stg' | 'prod' | 'default';

export interface SessionBadge {
  label: string;
  tone: 'ok' | 'warn';
}

// AddSessionPicker に渡す 1 行分の表示データ。
export interface SessionPickerItem {
  id: string;
  name: string;
  meta?: string;
  badge?: SessionBadge | null;
  // 小文字化済みの検索対象文字列 (name / accountId / role 等を連結)
  searchText: string;
  // 開設済み → グレーアウト + 「開いています」
  disabled?: boolean;
}

const ENV_DEV_RE = /(^|[-_])dev$/;
const ENV_STG_RE = /(^|[-_])(stg|staging)$/;
const ENV_PROD_RE = /(^|[-_])(prod|production)$/;

// GCP プロジェクト ID のサフィックスから環境色を判定する。
// AWS プロファイルには使わない (モック 4b: ドットは接続状態のみで環境の
// 特別扱いなし。常に default = 緑)。
export function projectEnv(projectId: string): SessionEnv {
  if (ENV_DEV_RE.test(projectId)) return 'dev';
  if (ENV_STG_RE.test(projectId)) return 'stg';
  if (ENV_PROD_RE.test(projectId)) return 'prod';
  return 'default';
}

// SSO 状態バッジ。sso 以外の認証方式では出さない (アクセスキーの「有効」緑
// バッジはモックにあるが、実際には検証していない情報のため意図的に省く)。
// ssoStatus 未定義 (設定不備やキャッシュ読み取り失敗) もバッジなし。
export function profileBadge(p: Profile): SessionBadge | null {
  if (p.authType !== 'sso') return null;
  switch (p.ssoStatus) {
    case 'valid':
      return { label: i18n.t('session:sessionMeta.badgeSsoValid'), tone: 'ok' };
    case 'expired':
      return { label: i18n.t('session:sessionMeta.badgeExpired'), tone: 'warn' };
    case 'not_logged_in':
      return { label: i18n.t('session:sessionMeta.badgeNotLoggedIn'), tone: 'warn' };
    default:
      return null;
  }
}

// 認証方式の表示ラベル。unknown / 未取得は空文字 (表示しない)。
export function profileAuthLabel(p: Profile): string {
  switch (p.authType) {
    case 'sso':
      return 'SSO';
    case 'access_key':
      return i18n.t('session:sessionMeta.authAccessKey');
    case 'assume_role':
      return 'AssumeRole';
    case 'credential_process':
      return 'credential_process';
    default:
      return '';
  }
}

// 有効期限の残りが短い (30 分未満 or 既に過ぎた) かどうか。
export const SSO_EXPIRY_SOON_MS = 30 * 60 * 1000;

export function isExpiringSoon(iso: string, now: Date): boolean {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return false;
  return t - now.getTime() < SSO_EXPIRY_SOON_MS;
}

// SSO 有効期限の表示用文字列。期限切れは「期限切れ」、それ以外は残り時間を
// 「残り 2 時間 5 分」形式で返す。パース不能は空文字。
export function formatSsoExpiry(iso: string, now: Date): string {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return '';
  const diffMs = t - now.getTime();
  if (diffMs <= 0) return i18n.t('session:sessionMeta.expiryExpired');
  const totalMin = Math.floor(diffMs / 60_000);
  const hours = Math.floor(totalMin / 60);
  const minutes = totalMin % 60;
  if (hours > 0) return i18n.t('session:sessionMeta.expiryHoursMinutes', { hours, minutes });
  return i18n.t('session:sessionMeta.expiryMinutes', { minutes });
}

// AWS プロファイルのピッカー項目を構築する。開設済み (open に含まれる) 行は
// disabled。期限切れ / 未ログインでも選択は可能なまま (開いた後は既存の
// SSOExpiredBanner が案内する)。
export function awsPickerItems(profiles: Profile[], open: string[]): SessionPickerItem[] {
  const opened = new Set(open);
  return profiles.map((p) => {
    const metaParts = [p.region, profileAuthLabel(p)].filter(Boolean);
    return {
      id: p.name,
      name: p.name,
      meta: metaParts.join(' · ') || undefined,
      badge: profileBadge(p),
      searchText: [p.name, p.accountId ?? '', p.ssoRoleName ?? ''].join(' ').toLowerCase(),
      disabled: opened.has(p.name),
    };
  });
}

// GCP プロジェクトのピッカー項目を構築する。認証状態バッジは出さない
// (ADC 単一認証で一覧に出る = アクセス可能のため。モック 6a からの意図的逸脱)。
export function gcpPickerItems(projects: GcpProject[], open: string[]): SessionPickerItem[] {
  const opened = new Set(open);
  return projects.map((p) => ({
    id: p.id,
    name: p.id,
    meta: p.name !== p.id ? p.name : undefined,
    badge: null,
    searchText: [p.id, p.name].join(' ').toLowerCase(),
    disabled: opened.has(p.id),
  }));
}
