// Cloud Logging の SUMMARY 列に先頭表示するフィールドを選択するポップオーバー。
// Google Cloud Logs Explorer の「サマリー フィールド」相当。選択順がそのまま表示順になる。
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

export interface SummaryFieldPickerProps {
  // 現在ロード済みの行から集めた選択候補のフィールドキー。
  available: string[];
  // 選択済みフィールドキー (選択順 = 表示順)。
  selected: string[];
  // 選択/解除をトグルする。
  onToggle: (key: string) => void;
  onClear: () => void;
}

export function SummaryFieldPicker({
  available,
  selected,
  onToggle,
  onClear,
}: SummaryFieldPickerProps) {
  const { t } = useTranslation('logviewer');
  const [open, setOpen] = useState(false);
  const wrapperRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: PointerEvent) => {
      if (wrapperRef.current && !wrapperRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('pointerdown', onPointerDown);
    return () => document.removeEventListener('pointerdown', onPointerDown);
  }, [open]);

  const selectedSet = new Set(selected);
  const candidates = available.filter((key) => !selectedSet.has(key));

  return (
    <div className="lv-field-picker" ref={wrapperRef}>
      <button className={`btn sm ${open ? 'active' : ''}`} onClick={() => setOpen((v) => !v)}>
        {t('summaryFieldPicker.button')}
        {selected.length > 0 ? ` (${selected.length})` : ''} ▾
      </button>
      {open && (
        <div className="lv-field-picker-menu">
          <div className="lv-field-picker-head">{t('summaryFieldPicker.head')}</div>
          {selected.length > 0 && (
            <div className="lv-field-picker-selected">
              {selected.map((key, i) => (
                <div key={key} className="lv-field-picker-item selected">
                  <span className="lv-field-picker-order">{i + 1}</span>
                  <span className="lv-field-picker-key">{key}</span>
                  <button
                    className="lv-field-picker-remove"
                    title={t('summaryFieldPicker.removeTitle')}
                    onClick={() => onToggle(key)}
                  >
                    ×
                  </button>
                </div>
              ))}
            </div>
          )}
          <div className="lv-field-picker-candidates">
            {candidates.length === 0 && selected.length === 0 && (
              <div className="lv-field-picker-empty">{t('summaryFieldPicker.empty')}</div>
            )}
            {candidates.map((key) => (
              <label key={key} className="lv-field-picker-item">
                <input type="checkbox" checked={false} onChange={() => onToggle(key)} />
                <span className="lv-field-picker-key">{key}</span>
              </label>
            ))}
          </div>
          {selected.length > 0 && (
            <button className="lv-field-picker-clear" onClick={onClear}>
              {t('summaryFieldPicker.clear')}
            </button>
          )}
        </div>
      )}
    </div>
  );
}
