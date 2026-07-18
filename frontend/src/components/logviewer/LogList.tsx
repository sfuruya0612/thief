// ログビューアのログ一覧。1 行 1 イベントで、行クリックで詳細 (JSON フィールド) を展開する。
// CloudWatch Logs (GROUP 列) と Cloud Logging (SEVERITY バッジ列) で列構成が異なるため、
// 列見出し・2 列目の描画・詳細の描画を呼び出し側から差し込む汎用コンポーネントにする。
import { type ReactNode, useState } from 'react';
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
      <div className="lv-list-head">
        <span className="lv-col-ts" style={{ width: 150 }}>
          TIMESTAMP
        </span>
        <span className="lv-col-second" style={{ width: secondWidth }}>
          {secondHeader}
        </span>
        <span className="lv-col-msg">{messageHeader}</span>
        <button className="btn sm lv-copy-btn" onClick={onCopy} disabled={rows.length === 0}>
          {copyLabel}
        </button>
      </div>
      <div className="lv-list-body" ref={bodyRef} onScroll={onScroll}>
        {rows.length === 0 ? (
          <div className="lv-list-empty">{emptyMessage}</div>
        ) : (
          rows.map((row) => {
            const key = getKey(row);
            const level = getLevel(row);
            const isOpen = expanded.has(key);
            return (
              <div key={key} className={`lv-row lv-row-${level} ${isOpen ? 'open' : ''}`}>
                <div className="lv-row-line" onClick={() => toggle(key)}>
                  <span className="lv-row-ts" style={{ width: 150 }}>
                    {formatLogClock(getTimestamp(row))}
                  </span>
                  <span className="lv-row-second" style={{ width: secondWidth }}>
                    {renderSecond(row)}
                  </span>
                  <span className={`lv-row-msg ${tintMessageByLevel ? `tint-${level}` : ''}`}>
                    {getMessage(row)}
                  </span>
                </div>
                {isOpen && <div className="lv-row-detail">{renderDetail(row)}</div>}
              </div>
            );
          })
        )}
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
  return (
    <div className="lv-field">
      <span className="lv-field-label">{label}:</span>
      <span className="lv-field-value">{value}</span>
      {onAddFilter && (
        <button className="lv-field-action" onClick={onAddFilter}>
          ＋ フィルタ
        </button>
      )}
      {onCopy && (
        <button className="lv-field-action" onClick={onCopy} title="値をコピー">
          コピー
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
