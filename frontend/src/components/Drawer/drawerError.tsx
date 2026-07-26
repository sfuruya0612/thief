// Drawer サブタブ共通のエラー表示。ApiError ならステータス・コード・メッセージを、
// それ以外は文字列化した内容を表示する。本文が AWS 由来の英語メッセージのため
// ラベルも英語ハードコードとし i18n に載せない。
// 表示規則: query の data が無いときはこの部品のみを出し、data があるときは
// 既存表示の上部に出して表示中のデータを消さない (issues/0075 で確定)。
import { ApiError } from '../../types/common';

export interface DrawerErrorProps {
  error: unknown;
}

export function DrawerError({ error }: DrawerErrorProps) {
  const text =
    error instanceof ApiError
      ? `Error ${error.statusCode}${error.code ? ` (${error.code})` : ''}: ${error.message}`
      : String(error);
  return <div style={{ padding: '8px 0', color: 'var(--err)' }}>{text}</div>;
}
