// 検索アイコン付きの入力 (primitives.css の .chip-search)。
// <span class="chip-search"><Icons.search /><input /></span> の形をここで固定し、
// props はすべて <input> に渡す (docs/issues/closed/0203)。
import { forwardRef } from 'react';
import type { InputHTMLAttributes } from 'react';
import { Icons } from '../icons/Icons';

export type SearchFieldProps = InputHTMLAttributes<HTMLInputElement>;

export const SearchField = forwardRef<HTMLInputElement, SearchFieldProps>(function SearchField(
  { className, ...rest },
  ref,
) {
  return (
    <span className={className ? `chip-search ${className}` : 'chip-search'}>
      <Icons.search size={12} />
      <input ref={ref} {...rest} />
    </span>
  );
});
