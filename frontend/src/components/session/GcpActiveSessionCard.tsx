// サイドバーの「アクティブセッション」カード (GCP)。
// ADC の identity / 有効期限を返す backend API が無いため、モック 4a の
// 「ADC · email / 有効期限 / 再認証」は出さず、プロジェクト情報 + 認証方式
// ラベルのみの縮退表示にしている (意図的なデザイン逸脱)。
import { projectEnv } from '../../lib/sessionMeta';
import type { GcpProject } from '../../types/gcp';

export interface GcpActiveSessionCardProps {
  project: string;
  projects: GcpProject[];
}

export function GcpActiveSessionCard({ project, projects }: GcpActiveSessionCardProps) {
  const meta = projects.find((p) => p.id === project);
  const displayName = meta?.name ?? project;

  return (
    <div>
      <div className="session-card-head">
        <span className={`session-tab-dot env-${projectEnv(project)}`} />
        <span className="session-card-name" title={displayName}>
          {displayName}
        </span>
      </div>
      <div className="session-card-meta">
        <div className="account-id">{project || '-'}</div>
        <div>ADC</div>
      </div>
    </div>
  );
}
