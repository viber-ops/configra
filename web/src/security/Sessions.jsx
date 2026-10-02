import React, { useRef, useState } from 'react';
import { request, managementError, ActionError, useInventory, InventoryPagination } from '../management.jsx';

export function sessionLabel(t) { return t.locale.startsWith('zh') ? '管理员会话' : 'Administrator sessions'; }

export function Sessions({ t, principal }) {
  const zh = t.locale.startsWith('zh');
  const labels = zh ? {
    issuer: 'OIDC 签发方', subject: '用户标识（sub）', action: '操作', invalidate: '使现有会话失效', block: '使会话失效并阻止新登录', unblock: '解除阻止，要求重新登录', review: '核对变更', confirm: '确认变更', saved: '会话策略已更新。', hint: '使用身份提供方的准确 issuer 和 sub；邮箱或显示名称不一定是 sub。需要管理员 MFA。已有旧会话不会因解除阻止而恢复。', blocked: '已阻止', allowed: '允许新登录', target: '确认目标',
  } : {
    issuer: 'OIDC issuer', subject: 'Subject (sub)', action: 'Action', invalidate: 'Invalidate existing sessions', block: 'Invalidate and block new sign-ins', unblock: 'Unblock; require a fresh sign-in', review: 'Review change', confirm: 'Confirm change', saved: 'Session policy updated.', hint: 'Use the exact issuer and sub from your identity provider; email/display name may differ. Administrator MFA is required. Unblocking never restores old sessions.', blocked: 'Blocked', allowed: 'Fresh sign-in allowed', target: 'Confirm target',
  };
  const [draft, setDraft] = useState({ issuer: principal?.issuer || '', subject: '', action: 'invalidate' });
  const [confirmed, setConfirmed] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState(null);
  const [search, setSearch] = useState('');
  const [refresh, setRefresh] = useState(0);
  const operation = useRef(null);
  const collection = useInventory(`/v1/session-policies?q=${encodeURIComponent(search)}`, refresh);
  const change = (key, value) => { setDraft(current => ({ ...current, [key]: value })); setConfirmed(false); setSaved(false); operation.current = null; };
  const submit = async event => {
    event.preventDefault();
    if (!confirmed) { setConfirmed(true); return; }
    if (saving) return;
    const body = JSON.stringify(draft);
    if (operation.current?.body !== body) operation.current = { body, id: crypto.randomUUID() };
    setSaving(true); setError(null);
    try {
      await request('/v1/session-policies', { method: 'POST', headers: { 'Content-Type': 'application/json', 'Idempotency-Key': operation.current.id }, body });
      setSaved(true); setConfirmed(false); operation.current = null; setRefresh(value => value + 1);
    } catch (failure) { setError(managementError(failure, t, t.operationFailed)); }
    finally { setSaving(false); }
  };
  return <section className="admin-panel">
    <h2>{sessionLabel(t)}</h2><p>{labels.hint}</p>
    <form className="token-form" onSubmit={submit}>
      <label>{labels.issuer}<input type="url" required maxLength="512" value={draft.issuer} disabled={saving} onChange={event => change('issuer', event.target.value)} /></label>
      <label>{labels.subject}<input required maxLength="255" autoComplete="off" value={draft.subject} disabled={saving} onChange={event => change('subject', event.target.value)} /></label>
      <label>{labels.action}<select value={draft.action} disabled={saving} onChange={event => change('action', event.target.value)}>{['invalidate', 'block', 'unblock'].map(action => <option key={action} value={action}>{labels[action]}</option>)}</select></label>
      {confirmed && <p role="status">{labels.target}: <code>{draft.issuer}</code> / <code>{draft.subject}</code> — {labels[draft.action]}</p>}
      <div className="form-actions"><ActionError error={error} t={t} /><button className="primary-action compact-action" type="submit" disabled={saving}>{confirmed ? labels.confirm : labels.review}</button></div>
      {saved && <p role="status">{labels.saved}</p>}
    </form>
    <label>{t.search}<input type="search" value={search} onChange={event => setSearch(event.target.value)} /></label>
    <div className="table-frame"><table><thead><tr><th>{labels.issuer}</th><th>{labels.subject}</th><th>{t.status}</th></tr></thead><tbody>{collection.items.map(policy => <tr key={policy.identity_id}><td>{policy.issuer}</td><td>{policy.subject}</td><td>{policy.blocked ? labels.blocked : labels.allowed}</td></tr>)}</tbody></table></div>
    <InventoryPagination collection={collection} t={t} />
  </section>;
}
