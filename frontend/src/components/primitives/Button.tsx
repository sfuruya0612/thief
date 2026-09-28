// 枠付きボタン (primitives.css の .btn)。size / variant の組み合わせをここで決め、
// 呼び出し側が className を組み立てないようにする (docs/issues/closed/0203)。
// <select> / <a> にボタンの見た目を付ける箇所は例外として className="btn sm" の直書きを残す
// (一覧は primitives.css の .btn のコメント)。
import { forwardRef } from 'react';
import type { ButtonHTMLAttributes } from 'react';

export type ButtonSize = 'md' | 'sm';
export type ButtonVariant = 'default' | 'primary' | 'ghost';

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  // 既定 md = 高さ 28px。sm = 高さ 24px (.btn.sm)。
  size?: ButtonSize;
  // default = 枠付き、primary = accent の面 (.btn.primary)、ghost = 枠なし (.btn.ghost)。
  variant?: ButtonVariant;
}

// クラス名は primitives.css の順 (btn → sm → variant → 追加クラス) で連結する。
function buttonClassName(size: ButtonSize, variant: ButtonVariant, className?: string): string {
  return ['btn', size === 'sm' ? 'sm' : '', variant === 'default' ? '' : variant, className ?? '']
    .filter(Boolean)
    .join(' ');
}

// type の既定は "button" にする。現行の呼び出しはすべて <form> の外にあり、submit に
// なることは無いが、将来 <form> に置かれても誤って送信しないようにする。
export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { size = 'md', variant = 'default', className, type = 'button', ...rest },
  ref,
) {
  return (
    <button ref={ref} type={type} className={buttonClassName(size, variant, className)} {...rest} />
  );
});
