import React, { useEffect, useRef } from 'react';
import { basicSetup, EditorView } from 'codemirror';
import { EditorState } from '@codemirror/state';
import { HighlightStyle, syntaxHighlighting, syntaxTree } from '@codemirror/language';
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

// Nodes that hold one YAML or JSON scalar in the bundled grammars. Keys are scalars too: YAML wraps them in Key and
// JSON names them PropertyName.
const scalarNodes = new Set(['QuotedLiteral', 'Literal', 'BlockLiteral', 'String', 'Number', 'True', 'False', 'Null']);
const keyNodes = new Set(['Key', 'PropertyName']);

function enclosingNode(state, position) {
  for (const side of [-1, 1]) {
    for (let node = syntaxTree(state).resolveInner(position, side); node; node = node.parent) {
      if (keyNodes.has(node.name) || node.name === 'Comment') return node;
      if (scalarNodes.has(node.name)) return keyNodes.has(node.parent?.name) ? node.parent : node;
    }
  }
  return null;
}

// Where a new value may start: after a colon, a YAML sequence dash, or an opening bracket or comma, with nothing but
// a closing bracket, comma or comment after it on the line.
const valueBefore = /(?::|(?:^|\s)-|[[,])[ \t]*$/;
const valueAfter = /^[ \t]*(?:$|#|[,\]}])/;

// A Vault reference only resolves as a whole scalar value, so it goes into an empty pair of quotes, over a selected
// value, or in as a new quoted value where a value belongs. It is never spliced into other text, a key or a comment.
// Returns the change, or null when the selection is not a place for a value.
export function placeReference(state, reference) {
  const { from, to } = state.selection.main;
  const node = enclosingNode(state, from);
  if (node && (keyNodes.has(node.name) || node.name === 'Comment')) return null;
  const quoted = node && /^["']$/.test(state.sliceDoc(node.from, node.from + 1)) && node.to - node.from >= 2;
  if (from !== to) {
    if (node && from === node.from && to === node.to) return { from, to, insert: `"${reference}"` };
    if (quoted && from === node.from + 1 && to === node.to - 1) return { from, to, insert: reference };
    return null;
  }
  if (node) return quoted && node.to - node.from === 2 && from === node.from + 1 ? { from, to, insert: reference } : null;
  const line = state.doc.lineAt(from);
  const before = state.sliceDoc(line.from, from);
  if (!valueBefore.test(before) || !valueAfter.test(state.sliceDoc(from, line.to))) return null;
  // A more indented line after "key:" or "-" means the value is the nested block below, so it is not a place for one.
  if (/[:-][ \t]*$/.test(before)) {
    const indent = text => /^[ \t]*/.exec(text)[0].length;
    for (let number = line.number + 1; number <= state.doc.lines; number += 1) {
      const next = state.doc.line(number).text;
      if (!next.trim() || next.trim().startsWith('#')) continue;
      if (indent(next) > indent(line.text)) return null;
      break;
    }
  }
  // A value straight after a colon or dash needs a separating space in YAML; JSON accepts it too.
  return { from, to, insert: `${/[:-]$/.test(before) ? ' ' : ''}"${reference}"` };
}

export function ConfigCodeEditor({ value, format, label, readOnly = true, onChange, controller, className = '' }) {
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

  // Lets the page insert a Vault reference at the caret as one undoable edit (#51).
  useEffect(() => {
    if (!controller) return undefined;
    controller.current = {
      insertReference(reference) {
        const target = view.current;
        const change = target && !readOnly ? placeReference(target.state, reference) : null;
        if (!change) return false;
        target.dispatch({ changes: change, selection: { anchor: change.from + change.insert.length }, userEvent: 'input', scrollIntoView: true });
        target.focus();
        return true;
      },
    };
    return () => { controller.current = null; };
  }, [controller, readOnly]);

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
