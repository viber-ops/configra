import React, { useEffect, useRef } from 'react';
import { basicSetup, EditorView } from 'codemirror';
import { EditorState } from '@codemirror/state';
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { json } from '@codemirror/lang-json';
import { yaml } from '@codemirror/lang-yaml';
import { MergeView } from '@codemirror/merge';
import { tags } from '@lezer/highlight';

const configEditorTheme = EditorView.theme({
  '&': { height: '100%', backgroundColor: 'var(--editor-surface)', color: 'var(--ink)' },
  '.cm-scroller': { fontFamily: 'ui-monospace, "SFMono-Regular", Consolas, monospace', lineHeight: '1.65' },
  '.cm-content': { padding: '14px 0' },
  '.cm-gutters': { borderRight: '1px solid var(--line)', backgroundColor: 'var(--surface-muted)', color: 'var(--muted)' },
  '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: 'var(--surface-muted)' },
  '&.cm-focused': { outline: 'none' },
});

const configHighlightStyle = HighlightStyle.define([
  { tag: [tags.propertyName, tags.attributeName, tags.typeName], color: 'var(--syntax-key)', fontWeight: '650' },
  { tag: [tags.string, tags.character, tags.attributeValue], color: 'var(--syntax-string)' },
  { tag: [tags.number, tags.integer, tags.float], color: 'var(--syntax-number)' },
  { tag: [tags.keyword, tags.atom, tags.bool, tags.null], color: 'var(--syntax-keyword)', fontWeight: '650' },
  { tag: [tags.comment, tags.docComment], color: 'var(--syntax-comment)', fontStyle: 'italic' },
  { tag: [tags.punctuation, tags.bracket, tags.operator, tags.separator], color: 'var(--syntax-punctuation)' },
  { tag: tags.invalid, color: 'var(--danger)', textDecoration: 'underline' },
]);

const configEditorNonce = document.querySelector('meta[name="csp-nonce"]')?.content;
const configLanguage = format => format === 'json' ? json() : yaml();
const configEditorExtensions = (format, readOnly, label) => [
  basicSetup,
  configLanguage(format),
  configEditorTheme,
  syntaxHighlighting(configHighlightStyle),
  EditorState.tabSize.of(2),
  EditorState.readOnly.of(readOnly),
  EditorView.editable.of(!readOnly),
  EditorView.contentAttributes.of({ 'aria-label': label, 'aria-readonly': String(readOnly), spellcheck: 'false' }),
  ...(configEditorNonce ? [EditorView.cspNonce.of(configEditorNonce)] : []),
];

export function ConfigCodeEditor({ value, format, label, readOnly = true, onChange, className = '' }) {
  const host = useRef(null);
  const view = useRef(null);
  const change = useRef(onChange);
  const syncing = useRef(false);
  change.current = onChange;

  useEffect(() => {
    const update = EditorView.updateListener.of(event => {
      if (event.docChanged && !syncing.current) change.current?.(event.state.doc.toString());
    });
    view.current = new EditorView({
      doc: value,
      extensions: [...configEditorExtensions(format, readOnly, label), update],
      parent: host.current,
    });
    return () => {
      view.current?.destroy();
      view.current = null;
    };
  }, [format, label, readOnly]);

  useEffect(() => {
    if (!view.current || view.current.state.doc.toString() === value) return;
    syncing.current = true;
    view.current.dispatch({ changes: { from: 0, to: view.current.state.doc.length, insert: value } });
    syncing.current = false;
  }, [value]);

  return <div className={`config-code-editor ${readOnly ? 'read-only ' : ''}${className}`} data-format={format} ref={host} />;
}

export function ConfigDiffEditor({ source, target, sourceFormat, targetFormat, sourceLabel, targetLabel, t }) {
  const host = useRef(null);
  useEffect(() => {
    const merge = new MergeView({
      a: { doc: source, extensions: configEditorExtensions(sourceFormat, true, sourceLabel) },
      b: { doc: target, extensions: configEditorExtensions(targetFormat, true, targetLabel) },
      parent: host.current,
      highlightChanges: true,
      gutter: true,
      collapseUnchanged: { margin: 3, minSize: 8 },
    });
    return () => merge.destroy();
  }, [source, sourceFormat, sourceLabel, target, targetFormat, targetLabel]);

  return <section className="config-diff-panel">
    <header className="config-diff-heading">
      <strong>{sourceLabel}<code>{sourceFormat.toUpperCase()}</code></strong>
      <span className="diff-legend"><i className="removed" />{t.diffRemoved}<i className="added" />{t.diffAdded}</span>
      <strong>{targetLabel}<code>{targetFormat.toUpperCase()}</code></strong>
    </header>
    <div className="config-diff-editor" ref={host} />
  </section>;
}
