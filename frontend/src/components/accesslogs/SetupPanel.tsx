import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useAthenaCatalogs,
  useAthenaDatabases,
  useAthenaTables,
  useValidateAccessLogTable,
} from '../../api/queries';
import { accessLogDestinationKey } from '../../lib/normalizeAccessLogs';
import type {
  AccessLogSource,
  AccessLogTableMapping,
  AccessLogTableValidation,
} from '../../types/accesslogs';
import { Button } from '../primitives';

interface SetupPanelProps {
  profile: string;
  sources: AccessLogSource[];
  mappings: Record<string, AccessLogTableMapping>;
  onChange: (key: string, mapping: AccessLogTableMapping) => void;
  workgroup: string;
  onWorkgroupChange: (value: string) => void;
  outputLocation: string;
  onOutputLocationChange: (value: string) => void;
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

export function SetupPanel({
  profile,
  sources,
  mappings,
  onChange,
  workgroup,
  onWorkgroupChange,
  outputLocation,
  onOutputLocationChange,
}: SetupPanelProps) {
  const { t } = useTranslation('accesslogs');
  return (
    <div className="accesslogs-setup">
      <div className="accesslogs-setup-global">
        <label>
          <span>{t('setup.workgroup')}</span>
          <input
            value={workgroup}
            onChange={(event) => onWorkgroupChange(event.target.value)}
            placeholder="primary"
          />
        </label>
        <label>
          <span>{t('setup.outputLocation')}</span>
          <input
            value={outputLocation}
            onChange={(event) => onOutputLocationChange(event.target.value)}
            placeholder="s3://athena-results/"
          />
        </label>
        <p>{t('setup.globalHint')}</p>
      </div>
      {sources.length === 0 ? (
        <p className="accesslogs-empty-hint">{t('noSourcesFound')}</p>
      ) : (
        sources.map((source) => (
          <SetupSource
            key={source.id}
            profile={profile}
            source={source}
            mapping={mappings[accessLogDestinationKey(source)]}
            onChange={(mapping) => onChange(accessLogDestinationKey(source), mapping)}
            workgroup={workgroup}
          />
        ))
      )}
    </div>
  );
}

interface SetupSourceProps {
  profile: string;
  source: AccessLogSource;
  mapping?: AccessLogTableMapping;
  onChange: (mapping: AccessLogTableMapping) => void;
  workgroup: string;
}

function SetupSource({ profile, source, mapping, onChange, workgroup }: SetupSourceProps) {
  const { t } = useTranslation('accesslogs');
  if (!source.status.loggingEnabled) {
    return (
      <section className="accesslogs-setup-card">
        <header>
          <strong>{source.name}</strong>
          <span className="accesslogs-setup-type">{destinationLabel(source)}</span>
        </header>
        <p className="accesslogs-muted">{t('setup.loggingDisabled')}</p>
        {source.status.issues.length > 0 ? (
          <ul className="accesslogs-setup-issues">
            {source.status.issues.map((issue) => (
              <li key={issue}>{issue}</li>
            ))}
          </ul>
        ) : null}
      </section>
    );
  }
  if (source.engine === 'logs_insights') {
    return (
      <section className="accesslogs-setup-card">
        <header>
          <strong>{source.name}</strong>
          <span className="accesslogs-setup-type">WAF / CloudWatch Logs Insights</span>
        </header>
        <p>{t('setup.logsInsights', { logGroup: source.destination.logGroup ?? '' })}</p>
        {source.status.issues.length > 0 ? (
          <ul className="accesslogs-setup-issues">
            {source.status.issues.map((issue) => (
              <li key={issue}>{issue}</li>
            ))}
          </ul>
        ) : null}
      </section>
    );
  }
  if (source.engine !== 'athena') {
    return (
      <section className="accesslogs-setup-card">
        <header>
          <strong>{source.name}</strong>
          <span className="accesslogs-setup-type">{destinationLabel(source)}</span>
        </header>
        <p className="accesslogs-muted">{t('setup.notAvailable')}</p>
        {source.status.issues.length > 0 ? (
          <ul className="accesslogs-setup-issues">
            {source.status.issues.map((issue) => (
              <li key={issue}>{issue}</li>
            ))}
          </ul>
        ) : null}
      </section>
    );
  }
  return (
    <SetupAthenaSource
      profile={profile}
      source={source}
      mapping={mapping}
      onChange={onChange}
      workgroup={workgroup}
    />
  );
}

function SetupAthenaSource({ profile, source, mapping, onChange, workgroup }: SetupSourceProps) {
  const { t } = useTranslation('accesslogs');
  const [catalog, setCatalog] = useState(mapping?.catalog ?? 'AwsDataCatalog');
  const [database, setDatabase] = useState(mapping?.database ?? '');
  const [table, setTable] = useState(mapping?.table ?? '');
  const [validation, setValidation] = useState<AccessLogTableValidation | null>(null);
  const [ddl, setDdl] = useState(source.setupSql ?? '');
  const [showDDL, setShowDDL] = useState(false);
  const [copied, setCopied] = useState(false);

  const catalogs = useAthenaCatalogs(profile, source.queryRegion);
  const databases = useAthenaDatabases(profile, source.queryRegion, catalog);
  const tables = useAthenaTables(profile, source.queryRegion, database || undefined, catalog);
  const validate = useValidateAccessLogTable(profile);

  const catalogOptions = useMemo(() => {
    const names = (catalogs.data ?? []).map((item) => item.name);
    if (names.length === 0) names.push('AwsDataCatalog');
    return names;
  }, [catalogs.data]);

  const updateMapping = (nextCatalog: string, nextDatabase: string, nextTable: string) => {
    onChange({ catalog: nextCatalog, database: nextDatabase, table: nextTable });
  };

  const handleValidate = async () => {
    if (!database || !table) return;
    try {
      const result = await validate.mutateAsync({
        source_id: source.id,
        catalog,
        database,
        table,
        workgroup: workgroup || undefined,
      });
      setValidation(result.validation);
      setDdl(result.setupSql ?? source.setupSql ?? '');
    } catch {
      // エラーは validate.error で表示する。
    }
  };

  const handleCopy = () => {
    if (!ddl) return;
    void navigator.clipboard?.writeText(ddl);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };

  return (
    <section className="accesslogs-setup-card">
      <header>
        <strong>{source.name}</strong>
        <span className="accesslogs-setup-type">Athena / {destinationLabel(source)}</span>
      </header>
      {source.status.sharedBy.length > 0 ? (
        <p className="accesslogs-muted">
          {t('setup.shared', { names: source.status.sharedBy.join(', ') })}
        </p>
      ) : null}
      {source.schemaProfile === 'cloudfront_legacy' ? (
        <p className="accesslogs-warning">{t('setup.legacyCloudFront')}</p>
      ) : null}
      <div className="accesslogs-setup-fields">
        <label>
          <span>Catalog</span>
          <select
            value={catalog}
            onChange={(event) => {
              const value = event.target.value;
              setCatalog(value);
              updateMapping(value, database, table);
            }}
          >
            {catalogOptions.map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>Database</span>
          <select
            value={database}
            onChange={(event) => {
              const value = event.target.value;
              setDatabase(value);
              setTable('');
              updateMapping(catalog, value, '');
            }}
          >
            <option value="">{t('setup.selectDatabase')}</option>
            {(databases.data ?? []).map((item) => (
              <option key={item.name} value={item.name}>
                {item.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>Table</span>
          <select
            value={table}
            onChange={(event) => {
              const value = event.target.value;
              setTable(value);
              updateMapping(catalog, database, value);
            }}
            disabled={!database}
          >
            <option value="">{t('setup.selectTable')}</option>
            {(tables.data ?? []).map((item) => (
              <option key={item.name} value={item.name}>
                {item.name}
              </option>
            ))}
          </select>
        </label>
        <Button
          size="sm"
          onClick={() => void handleValidate()}
          disabled={!database || !table || validate.isPending}
        >
          {validate.isPending ? t('setup.validating') : t('setup.validate')}
        </Button>
        <Button size="sm" variant="ghost" onClick={() => setShowDDL((value) => !value)}>
          {t('setup.createTable')}
        </Button>
      </div>
      {validate.error ? <p className="accesslogs-error">{validate.error.message}</p> : null}
      {validation ? (
        <div className={`accesslogs-validation${validation.valid ? ' valid' : ' invalid'}`}>
          <strong>{validation.valid ? t('setup.valid') : t('setup.invalid')}</strong>
          {validation.schemaProfile ? (
            <span>{t('setup.profile', { profile: validation.schemaProfile })}</span>
          ) : null}
          {validation.missingColumns.length > 0 ? (
            <span>
              {t('setup.missingColumns', { columns: validation.missingColumns.join(', ') })}
            </span>
          ) : null}
          {!validation.attribution ? <span>{t('setup.noAttribution')}</span> : null}
          {validation.issues.map((issue) => (
            <span key={issue}>{issue}</span>
          ))}
        </div>
      ) : null}
      {showDDL ? (
        <div className="accesslogs-ddl">
          <div className="accesslogs-ddl-head">
            <strong>
              {t('setup.ddlTitle', { table: source.suggestedTable || 'access_logs' })}
            </strong>
            <Button size="sm" variant="ghost" onClick={handleCopy} disabled={!ddl}>
              {copied ? t('setup.copied') : t('setup.copyDDL')}
            </Button>
          </div>
          {ddl ? <pre>{ddl}</pre> : <p className="accesslogs-muted">{t('setup.noDDL')}</p>}
          <p className="accesslogs-muted">{t('setup.ddlHint')}</p>
        </div>
      ) : null}
    </section>
  );
}
