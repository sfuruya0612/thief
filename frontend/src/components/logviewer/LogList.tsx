// ログビューアのログ一覧。1 行 1 イベントで、行クリックで詳細 (JSON フィールド) を展開する。
// CloudWatch Logs (GROUP 列) と Cloud Logging (SEVERITY バッジ列) で列構成が異なるため、
// 列見出し・2 列目の描画・詳細の描画を呼び出し側から差し込む汎用コンポーネントにする。
// 実 <table> (auto table layout) + thead sticky で描画し、SUMMARY 列が長い場合はパネル全体
// (ヘッダー含む) が 1 本のスクロールバーで横スクロールする (行ごとの個別スクロールにはしない)。
import { Fragment, type ReactNode, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { formatLogClock } from '../../lib/logFormat';
import type { SeverityLevel } from '../../lib/logSeverity';

export interface LogListProps<T> {
  rows: T[];
  getKey: (row: T) => string;
  getLevel: (row: T) => SeverityLevel;
  getTimestamp: (row: T) => string;
  secondHeader: string;
  secondWidth: number;
  renderSecond: (row: T) => ReactNode;
  messageHeader: string;
  getMessage: (row: T) => string;
  renderDetail: (row: T) => ReactNode;
  // true のときメッセージ本文を severity で色付けする (CloudWatch。GCP は SEVERITY バッジ側で表現)。
  tintMessageByLevel?: boolean;
  // ヘッダー右側、コピー ボタンの直前に差し込む任意の操作 (Cloud Logging のフィールド選択等)。
  headerExtra?: ReactNode;
  copyLabel: string;
  onCopy: () => void;
  footer?: ReactNode;
  emptyMessage: string;
  bodyRef?: React.RefObject<HTMLDivElement>;
  onScroll?: () => void;
}

export function LogList<T>({
  rows,
  getKey,
  getLevel,
  getTimestamp,
  secondHeader,
  secondWidth,
  renderSecond,
  messageHeader,
  getMessage,
  renderDetail,
  tintMessageByLevel,
  headerExtra,
  copyLabel,
  onCopy,
  footer,
  emptyMessage,
  bodyRef,
  onScroll,
}: LogListProps<T>) {
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  const toggle = (key: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  return (
    <div className="lv-list">
      <div className="lv-list-toolbar">
        {headerExtra}
        <button className="btn sm lv-copy-btn" onClick={onCopy} disabled={rows.length === 0}>
          {copyLabel}
        </button>
      </div>
      <div className="lv-table-wrap" ref={bodyRef} onScroll={onScroll}>
        <table className="lv-table">
          <thead>
            <tr>
              <th className="lv-col-ts" style={{ width: 150 }}>
                TIMESTAMP
              </th>
              <th className="lv-col-second" style={{ width: secondWidth }}>
                {secondHeader}
              </th>
              <th className="lv-col-msg">{messageHeader}</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr>
                <td className="lv-list-empty" colSpan={3}>
                  {emptyMessage}
                </td>
              </tr>
            ) : (
              rows.map((row) => {
                const key = getKey(row);
                const level = getLevel(row);
                const isOpen = expanded.has(key);
                return (
                  <Fragment key={key}>
                    <tr
                      className={`lv-row lv-row-${level} ${isOpen ? 'open' : ''}`}
                      onClick={() => toggle(key)}
                    >
                      <td className="lv-row-ts" style={{ width: 150 }}>
                        {formatLogClock(getTimestamp(row))}
                      </td>
                      <td className="lv-row-second" style={{ width: secondWidth }}>
                        {renderSecond(row)}
                      </td>
                      <td className={`lv-row-msg ${tintMessageByLevel ? `tint-${level}` : ''}`}>
                        {getMessage(row)}
                      </td>
                    </tr>
                    {isOpen && (
                      <tr className="lv-row-detail-row">
                        <td colSpan={3}>
                          <div className="lv-row-detail">{renderDetail(row)}</div>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })
            )}
          </tbody>
        </table>
      </div>
      {footer && <div className="lv-list-foot">{footer}</div>}
    </div>
  );
}

// SeverityBadge は Cloud Logging の SEVERITY 列バッジ。level で色を分け、label に元の severity を出す。
export function SeverityBadge({ level, label }: { level: SeverityLevel; label: string }) {
  return <span className={`lv-sev-badge lv-sev-${level}`}>{label}</span>;
}

export interface LogFieldRowProps {
  label: string;
  value: string;
  onAddFilter?: () => void;
  onCopy?: () => void;
  trace?: { label: string; onOpen: () => void };
}

// LogFieldRow は展開時の 1 フィールド行 (ラベル: 値 + 任意のアクション)。
export function LogFieldRow({ label, value, onAddFilter, onCopy, trace }: LogFieldRowProps) {
  const { t } = useTranslation('logviewer');
  return (
    <div className="lv-field">
      <span className="lv-field-label">{label}:</span>
      <span className="lv-field-value">{value}</span>
      {onAddFilter && (
        <button className="lv-field-action" onClick={onAddFilter}>
          {t('logList.addFilter')}
        </button>
      )}
      {onCopy && (
        <button className="lv-field-action" onClick={onCopy} title={t('logList.copyValueTitle')}>
          {t('logList.copy')}
        </button>
      )}
      {trace && (
        <button className="lv-field-action" onClick={trace.onOpen}>
          {trace.label}
        </button>
      )}
    </div>
  );
}
