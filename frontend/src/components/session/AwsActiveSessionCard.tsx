// サイドバーの「アクティブセッション」カード (AWS)。旧 ProfileSelect の
// トリガーが担っていた STS identity の確定表示 (useProfileIdentity) は
// ここへ移設した。タブや一覧では呼ばず、アクティブセッション 1 箇所のみで
// 発火させる (プロファイル数ぶんの STS 呼び出しを避ける)。
import { useEffect, useState } from 'react';
import { useProfileIdentity } from '../../api/queries';
import {
  formatSsoExpiry,
  isExpiringSoon,
  profileAuthLabel,
  profileBadge,
} from '../../lib/sessionMeta';
import type { Profile } from '../../types/common';

export interface AwsActiveSessionCardProps {
  profile: string;
  profiles: Profile[];
}

export function AwsActiveSessionCard({ profile, profiles }: AwsActiveSessionCardProps) {
  const meta = profiles.find((p) => p.name === profile);
  // config 由来の accountId をまず表示し、STS で確定した値が来たら上書きする
  const identity = useProfileIdentity(profile);
  const displayAccountId = identity.data?.accountId || meta?.accountId;

  // 残り時間表示を 1 分ごとに更新する
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const t = setInterval(() => setNow(new Date()), 60_000);
    return () => clearInterval(t);
  }, []);

  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const t = setTimeout(() => setCopied(false), 2000);
    return () => clearTimeout(t);
  }, [copied]);

  const badge = meta ? profileBadge(meta) : null;
  const authLabel = meta ? profileAuthLabel(meta) : '';
  const expiry = meta?.ssoExpiresAt ? formatSsoExpiry(meta.ssoExpiresAt, now) : '';
  const expiring = meta?.ssoExpiresAt ? isExpiringSoon(meta.ssoExpiresAt, now) : false;
  const authLine = [authLabel, meta?.ssoRoleName].filter(Boolean).join(' · ');
  // 再認証コマンドは期限切れ / 未ログイン / 期限間近のときに案内する
  // (プロセス起動はせずコピー導線のみ。実行はユーザーのターミナルで行う)
  const needsReauth = badge?.tone === 'warn' || (expiry !== '' && expiring);
  const loginCmd = `aws sso login --profile ${profile}`;

  return (
    <div>
      <div className="session-card-head">
        <span className="session-tab-dot" />
        <span className="session-card-name" title={profile}>
          {profile}
        </span>
        {badge && <span className={`session-picker-badge ${badge.tone}`}>{badge.label}</span>}
      </div>
      <div className="session-card-meta">
        {authLine && <div>{authLine}</div>}
        <div className="account-id">{displayAccountId || '-'}</div>
        {expiry && (
          <div>
            有効期限{' '}
            <span className={`session-card-expiry ${expiring ? 'expiring' : ''}`}>{expiry}</span>
          </div>
        )}
      </div>
      {needsReauth && (
        <div className="session-card-reauth">
          <code title={loginCmd}>{loginCmd}</code>
          <button
            className="btn sm ghost"
            title="再認証コマンドをコピー"
            onClick={() => {
              void navigator.clipboard.writeText(loginCmd);
              setCopied(true);
            }}
          >
            {copied ? 'コピーしました' : 'コピー'}
          </button>
        </div>
      )}
    </div>
  );
}
