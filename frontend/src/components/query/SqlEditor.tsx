// CodeMirror 6 ベースの SQL エディタ。
// ハイライト色は app.css の --sql-* CSS 変数を参照するため、テーマ切替時の再構成は不要。
// タブ切替は親側で key を変えて再マウントする前提 (doc の差し替えは value 同期 effect が拾う)。
import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react';
import { autocompletion, completionKeymap } from '@codemirror/autocomplete';
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands';
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { sql, type SQLNamespace } from '@codemirror/lang-sql';
import { Compartment, EditorState } from '@codemirror/state';
import { drawSelection, EditorView, keymap, lineNumbers } from '@codemirror/view';
import { tags } from '@lezer/highlight';

export interface SqlEditorHandle {
  insertText: (text: string) => void;
  replaceAll: (text: string) => void;
  focus: () => void;
}

export interface SqlEditorProps {
  value: string;
  onChange: (value: string) => void;
  onRun?: () => void;
  schema?: SQLNamespace;
}

const editorTheme = EditorView.theme({
  '&': { fontSize: '12.5px', backgroundColor: 'transparent' },
  '&.cm-focused': { outline: 'none' },
  '.cm-scroller': {
    fontFamily: 'var(--font-mono)',
    lineHeight: '1.75',
    padding: '10px 0',
  },
  '.cm-content': { caretColor: 'var(--text-1)', color: 'var(--sql-text)' },
  '.cm-gutters': {
    backgroundColor: 'transparent',
    color: 'var(--sql-linenum)',
    border: 'none',
  },
  '.cm-lineNumbers .cm-gutterElement': { minWidth: '38px', paddingRight: '12px' },
  '.cm-activeLine': { backgroundColor: 'transparent' },
  '.cm-activeLineGutter': { backgroundColor: 'transparent' },
  '.cm-cursor': { borderLeftColor: 'var(--text-1)' },
  '.cm-selectionBackground, &.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground':
    {
      backgroundColor: 'var(--qe-selection)',
    },
  '.cm-tooltip': {
    backgroundColor: 'var(--bg-1)',
    border: '1px solid var(--line-2)',
    color: 'var(--text-1)',
    borderRadius: '7px',
    overflow: 'hidden',
  },
  '.cm-tooltip.cm-tooltip-autocomplete > ul > li': { fontFamily: 'var(--font-mono)' },
  '.cm-tooltip.cm-tooltip-autocomplete > ul > li[aria-selected]': {
    backgroundColor: 'var(--accent-dim)',
    color: 'var(--text-1)',
  },
});

// デザイントークン: keyword 青 / 関数・型 紫 / 文字列 緑 / 数値 橙 / コメント 灰
const sqlHighlight = HighlightStyle.define([
  { tag: tags.keyword, color: 'var(--sql-kw)', fontWeight: '600' },
  { tag: [tags.function(tags.variableName), tags.standard(tags.name)], color: 'var(--sql-fn)' },
  { tag: tags.typeName, color: 'var(--sql-fn)' },
  { tag: [tags.string, tags.special(tags.string)], color: 'var(--sql-str)' },
  { tag: tags.number, color: 'var(--sql-num)' },
  { tag: tags.bool, color: 'var(--sql-num)' },
  { tag: [tags.lineComment, tags.blockComment], color: 'var(--sql-comment)', fontStyle: 'italic' },
]);

export const SqlEditor = forwardRef<SqlEditorHandle, SqlEditorProps>(function SqlEditor(
  { value, onChange, onRun, schema },
  ref,
) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const viewRef = useRef<EditorView | null>(null);
  const schemaCompartmentRef = useRef(new Compartment());
  const initialValueRef = useRef(value);
  const onChangeRef = useRef(onChange);
  const onRunRef = useRef(onRun);
  onChangeRef.current = onChange;
  onRunRef.current = onRun;

  useEffect(() => {
    if (!containerRef.current) return;
    const state = EditorState.create({
      doc: initialValueRef.current,
      extensions: [
        lineNumbers(),
        history(),
        drawSelection(),
        autocompletion(),
        keymap.of([
          {
            key: 'Mod-Enter',
            run: () => {
              onRunRef.current?.();
              return true;
            },
          },
          ...completionKeymap,
          ...defaultKeymap,
          ...historyKeymap,
        ]),
        schemaCompartmentRef.current.of(sql({ schema: undefined })),
        editorTheme,
        syntaxHighlighting(sqlHighlight),
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            onChangeRef.current(update.state.doc.toString());
          }
        }),
      ],
    });
    const view = new EditorView({ state, parent: containerRef.current });
    viewRef.current = view;
    return () => {
      view.destroy();
      viewRef.current = null;
    };
  }, []);

  // 外部からの value 変更 (タブ復元等) をエディタへ反映する。
  // 通常の入力では onChange 経由で value と doc が一致するため何もしない。
  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;
    const current = view.state.doc.toString();
    if (current !== value) {
      view.dispatch({ changes: { from: 0, to: current.length, insert: value } });
    }
  }, [value]);

  // スキーマツリーの読み込みに応じて補完候補を更新する
  useEffect(() => {
    viewRef.current?.dispatch({
      effects: schemaCompartmentRef.current.reconfigure(sql({ schema })),
    });
  }, [schema]);

  useImperativeHandle(
    ref,
    () => ({
      insertText: (text: string) => {
        const view = viewRef.current;
        if (!view) return;
        const { from, to } = view.state.selection.main;
        view.dispatch({
          changes: { from, to, insert: text },
          selection: { anchor: from + text.length },
        });
        view.focus();
      },
      replaceAll: (text: string) => {
        const view = viewRef.current;
        if (!view) return;
        view.dispatch({
          changes: { from: 0, to: view.state.doc.length, insert: text },
        });
        view.focus();
      },
      focus: () => viewRef.current?.focus(),
    }),
    [],
  );

  return <div className="qe-sql-editor" ref={containerRef} />;
});
