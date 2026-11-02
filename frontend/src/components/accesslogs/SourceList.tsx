import { useTranslation } from 'react-i18next';
import type { AccessLogSource, AccessLogSourceType } from '../../types/accesslogs';
import { Button } from '../primitives';

interface SourceListProps {
  sources: AccessLogSource[];
  selected: ReadonlySet<string>;
  onToggle: (id: string) => void;
  onSelectAll: () => void;
  onRecheck: () => void;
  isRefreshing?: boolean;
}

const TYPE_ORDER: AccessLogSourceType[] = ['alb', 'cloudfront', 'waf'];

function sourceTypeLabel(type: AccessLogSourceType): string {
  switch (type) {
    case 'alb':
      return 'ALB';
    case 'cloudfront':
      return 'CloudFront';
    case 'waf':
      return 'WAF';
    default:
      return type;
  }
}

function destinationLabel(source: AccessLogSource): string {
  const destination = source.destination;
  switch (destination.type) {
    case 's3':
      return `s3://${destination.bucket ?? ''}${destination.prefix ? `/${destination.prefix}` : ''}`;
    case 'cloudwatch_logs':
      return `CloudWatch Logs: ${destination.logGroup ?? ''}`;
    case 'firehose':
      return 'Firehose';
    default:
      return 'No destination';
  }
}

function sourceBadges(source: AccessLogSource) {
  const badges: { label: string; tone: string }[] = [];
  if (!source.status.loggingEnabled) {
    badges.push({ label: 'Logging disabled', tone: 'muted' });
  } else if (source.engine === '') {
    badges.push({ label: 'Unsupported', tone: 'warn' });
  }
  if (source.global) badges.push({ label: 'Global', tone: 'info' });
  if (source.status.sharedBy.length > 0) badges.push({ label: 'Shared', tone: 'info' });
  if (source.originOf) badges.push({ label: 'CloudFront origin', tone: 'muted' });
  if (source.status.dataPresent === false) badges.push({ label: 'No data yet', tone: 'muted' });
  if (!source.status.attribution) badges.push({ label: 'No attribution', tone: 'warn' });
  return badges;
}

function canSelect(source: AccessLogSource): boolean {
  return source.status.loggingEnabled && source.engine !== '';
}

export function SourceList({
  sources,
  selected,
  onToggle,
  onSelectAll,
  onRecheck,
  isRefreshing,
}: SourceListProps) {
  const { t } = useTranslation('accesslogs');
  const grouped = new Map<AccessLogSourceType, AccessLogSource[]>();
  for (const type of TYPE_ORDER) grouped.set(type, []);
  for (const source of sources) {
    grouped.get(source.type)?.push(source);
  }

  return (
    <aside className="accesslogs-sources">
      <div className="panel-bar">
        <span>{t('sources')}</span>
        <span className="accesslogs-source-actions">
          <Button size="sm" variant="ghost" onClick={onSelectAll}>
            {t('selectReady')}
          </Button>
          <Button size="sm" variant="ghost" onClick={onRecheck} disabled={isRefreshing}>
            {isRefreshing ? t('rechecking') : t('recheck')}
          </Button>
        </span>
      </div>
      <div className="accesslogs-source-scroll">
        {sources.length === 0 ? (
          <p className="accesslogs-empty-hint">{t('noSourcesFound')}</p>
        ) : null}
        {TYPE_ORDER.map((type) => {
          const items = grouped.get(type) ?? [];
          if (items.length === 0) return null;
          return (
            <section key={type} className="accesslogs-source-group">
              <h3>{sourceTypeLabel(type)}</h3>
              {items.map((source) => {
                const selectable = canSelect(source);
                const checked = selected.has(source.id);
                const badges = sourceBadges(source);
                return (
                  <label
                    key={source.id}
                    className={`accesslogs-source${checked ? ' selected' : ''}${
                      selectable ? '' : ' disabled'
                    }`}
                    title={source.status.issues.join('\n')}
                  >
                    <input
                      type="checkbox"
                      checked={checked}
                      disabled={!selectable}
                      onChange={() => onToggle(source.id)}
                    />
                    <span className="accesslogs-source-body">
                      <span className="accesslogs-source-title">
                        <strong>{source.name}</strong>
                        <span className="accesslogs-source-region">
                          {source.global ? 'Global' : source.region}
                        </span>
                      </span>
                      <span className="accesslogs-source-destination">
                        {destinationLabel(source)}
                      </span>
                      {source.status.sharedBy.length > 0 ? (
                        <span className="accesslogs-source-shared">
                          {t('sharedWith', { names: source.status.sharedBy.join(', ') })}
                        </span>
                      ) : null}
                      {badges.length > 0 ? (
                        <span className="accesslogs-source-badges">
                          {badges.map((badge) => (
                            <span key={badge.label} className={`status ${badge.tone}`}>
                              {badge.label}
                            </span>
                          ))}
                        </span>
                      ) : null}
                      {source.status.issues.length > 0 ? (
                        <span className="accesslogs-source-issues">{source.status.issues[0]}</span>
                      ) : null}
                    </span>
                  </label>
                );
              })}
            </section>
          );
        })}
      </div>
    </aside>
  );
}
