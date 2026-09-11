import React, { useEffect, useRef, useState } from 'react';

export function authorityLabel(t) { return t.locale.startsWith('zh') ? '证书颁发机构' : 'Certificate authorities'; }

function copy(t) {
  return t.locale.startsWith('zh') ? {
    newCA: '创建 CA', caName: 'CA 名称', createCA: '生成证书颁发机构', authority: '颁发机构',
    days: '有效天数', issue: '签发客户端证书', newClient: '签发证书', choose: '选择颁发机构',
    noCA: '先创建证书颁发机构，即可在这里签发客户端证书。', caBody: '集中签发、轮换与吊销客户端身份。签名私钥在服务端加密保存。',
    noAuthorities: '创建第一个证书颁发机构', noAuthoritiesBody: '为应用生成 mTLS 身份，不再需要手工运行证书工具。',
    publicDownload: '下载 CA 证书', bundleTitle: '保存这份凭证', download: '下载凭证包', done: '已保存，关闭',
    once: '私钥仅在这次创建结果中提供。关闭窗口后无法再次下载私钥，请妥善保存凭证包。',
    copied: '凭证包已发起下载。确认文件保存成功后再关闭。', creating: '正在生成…',
    replay: '该操作已完成，私钥不会再次返回。如未保存私钥，请吊销这份凭证后重新创建。',
    revokeCA: '吊销 CA', revokeWarning: '该 CA 签发的所有客户端证书都会立即停止获得授权，此操作不可撤销。',
    clients: '张客户端证书', expired: '已过期', pending: '尚未生效', search: '查找证书', external: '外部 CA',
    emptyClients: '还没有客户端证书', emptyClientsBody: '签发一份应用凭证，或导入已有 CA 签名的公共证书。',
    firstStep: '第一步 · 创建颁发机构', secondStep: '第二步 · 签发应用凭证',
  } : {
    newCA: 'Create CA', caName: 'Authority name', createCA: 'Generate certificate authority', authority: 'Authority',
    days: 'Validity in days', issue: 'Issue client certificate', newClient: 'Issue certificate', choose: 'Choose an authority',
    noCA: 'Create a certificate authority to issue client certificates here.', caBody: 'Issue, rotate, and revoke client identities. Signing keys are encrypted on the server.',
    noAuthorities: 'Create your first certificate authority', noAuthoritiesBody: 'Give applications an mTLS identity without manual certificate tooling.',
    publicDownload: 'Download CA certificate', bundleTitle: 'Save these credentials', download: 'Download credential bundle', done: 'Saved, close',
    once: 'The private key is available only in this creation result. Save the bundle now; closing this window removes access to the private key.',
    copied: 'The download has started. Confirm that the file was saved before closing.', creating: 'Generating…',
    replay: 'This operation already completed. Private keys cannot be returned again. Revoke and replace the credential if the key was not saved.',
    revokeCA: 'Revoke CA', revokeWarning: 'Every client certificate issued by this authority will lose authorization immediately. This cannot be undone.',
    clients: 'client certificates', expired: 'Expired', pending: 'Not yet valid', search: 'Find certificates', external: 'External CA',
    emptyClients: 'No client certificates yet', emptyClientsBody: 'Issue an application credential, or import an existing CA-signed public certificate.',
    firstStep: 'Step 1 · Create an authority', secondStep: 'Step 2 · Issue application credentials',
  };
}

function download(bytes, filename, type) {
  const url = URL.createObjectURL(new Blob([bytes], { type }));
  const link = document.createElement('a');
  link.href = url; link.download = filename;
  document.body.append(link); link.click(); link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function ExportDialog({ bundle, filename, onClose, t }) {
  const dialog = useRef(null);
  const [downloaded, setDownloaded] = useState(false);
  const s = copy(t);
  useEffect(() => { const node = dialog.current; node.showModal(); return () => node.close(); }, []);
  const save = () => {
    const bytes = Uint8Array.from(atob(bundle), character => character.charCodeAt(0));
    download(bytes, filename, 'application/zip');
    bytes.fill(0);
    setDownloaded(true);
  };
  return <dialog className="credential-dialog" ref={dialog} onCancel={event => { event.preventDefault(); onClose(); }} aria-labelledby="credential-export-title">
    <div className="credential-dialog-icon" aria-hidden="true">↓</div>
    <h2 id="credential-export-title">{s.bundleTitle}</h2><p>{s.once}</p>
    <div className="credential-bundle-name"><span aria-hidden="true">▤</span><code>{filename}</code><span>ZIP</span></div>
    {downloaded && <p role="status" className="success-note">{s.copied}</p>}
    <div className="dialog-actions"><button type="button" onClick={onClose}>{downloaded ? s.done : t.cancel}</button><button className="primary-action compact-action" type="button" onClick={save}>{s.download}</button></div>
  </dialog>;
}

function useMutation(request) {
  const attempt = useRef(null);
  const mutate = (path, body) => {
    const signature = JSON.stringify([path, body]);
    if (attempt.current?.signature !== signature) attempt.current = { signature, id: crypto.randomUUID() };
    return request(path, { method: 'POST', headers: { 'Content-Type': 'application/json', 'Idempotency-Key': attempt.current.id }, ...(body ? { body: JSON.stringify(body) } : {}) });
  };
  return [mutate, () => { attempt.current = null; }];
}

function Status({ record, t }) {
  const s = copy(t);
  const expired = Date.parse(record.not_after) <= Date.now();
  const pending = Date.parse(record.not_before) > Date.now();
  return <span className={`status ${record.revoked || expired ? 'archived' : pending ? 'pending-status' : 'active'}`}>{record.revoked ? t.revoked : expired ? s.expired : pending ? s.pending : t.statusActive}</span>;
}

function date(value, t) { return new Intl.DateTimeFormat(t.locale, { dateStyle: 'medium' }).format(new Date(value)); }

export function AuthorityManagement({ request, t, onCertificates }) {
  const s = copy(t);
  const [items, setItems] = useState([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [confirming, setConfirming] = useState('');
  const [exported, setExported] = useState(null);
  const [refresh, setRefresh] = useState(0);
  const [mutate, resetMutation] = useMutation(request);
  useEffect(() => {
    let live = true;
    request('/v1/certificate-authorities?include_revoked=true').then(result => { if (live) setItems(result.items); }).catch(() => { if (live) setError(t.operationFailed); }).finally(() => { if (live) setLoading(false); });
    return () => { live = false; };
  }, [refresh]);
  const create = async event => {
    event.preventDefault();
    if (busy) return;
    const data = new FormData(event.currentTarget);
    setBusy(true); setError('');
    try {
      const result = await mutate('/v1/certificate-authorities', { display_name: data.get('display_name'), valid_days: Number(data.get('valid_days')) });
      setCreating(false); setRefresh(value => value + 1);
      if (result.export_bundle) setExported({ bundle: result.export_bundle, filename: `configra-ca-${result.authority.id.slice(0, 8)}.zip` });
      else setError(s.replay);
    } catch { setError(t.operationFailed); } finally { setBusy(false); }
  };
  const revoke = async authority => {
    if (busy) return;
    setBusy(true); setError('');
    try { await mutate(`/v1/certificate-authorities/${authority.id}/revoke`); setConfirming(''); setRefresh(value => value + 1); }
    catch { setError(t.operationFailed); } finally { setBusy(false); }
  };
  return <section className="admin-panel authority-panel">
    <div className="security-panel-heading"><div><span className="section-kicker">{s.firstStep}</span><h2>{authorityLabel(t)}</h2><p>{s.caBody}</p></div><button className="primary-action compact-action" type="button" disabled={busy} onClick={() => { if (!creating) resetMutation(); setCreating(value => !value); setError(''); }}>{s.newCA}</button></div>
    {creating && <form className="credential-create-form" onSubmit={create}>
      <label>{s.caName}<input name="display_name" required maxLength="128" autoComplete="off" placeholder={t.locale.startsWith('zh') ? '例如：生产应用' : 'e.g. Production applications'} /></label>
      <label>{s.days}<input name="valid_days" type="number" min="1" max="3650" defaultValue="1825" required /></label>
      <div className="form-actions"><button type="button" disabled={busy} onClick={() => setCreating(false)}>{t.cancel}</button><button className="primary-action compact-action" disabled={busy} type="submit">{busy ? s.creating : s.createCA}</button></div>
    </form>}
    {error && <p className="inline-error" role="alert">{error}</p>}
    {loading ? <div className="collection-state"><span className="loading-line" /></div> : items.length === 0 ? <div className="credential-empty"><div className="credential-dialog-icon" aria-hidden="true">◇</div><h3>{s.noAuthorities}</h3><p>{s.noAuthoritiesBody}</p></div> : <div className="authority-grid">{items.map(authority => <article className="authority-card" key={authority.id}>
      <header><div className="resource-symbol" data-kind="authority" aria-hidden="true">CA</div><div><h3>{authority.display_name}</h3><Status record={authority} t={t} /></div></header>
      <dl><div><dt>{t.validUntil}</dt><dd>{date(authority.not_after, t)}</dd></div><div><dt>{t.fingerprint}</dt><dd><code title={authority.fingerprint_sha256}>{authority.fingerprint_sha256.slice(0, 16)}…{authority.fingerprint_sha256.slice(-8)}</code></dd></div><div><dt>{t.clientCertificates}</dt><dd>{authority.client_certificate_count} {s.clients}</dd></div></dl>
      <div className="authority-actions"><button type="button" onClick={() => download(authority.certificate_pem, `configra-ca-${authority.id.slice(0, 8)}.crt`, 'application/x-pem-file')}>{s.publicDownload}</button>{!authority.revoked && <button className="danger-link" type="button" onClick={() => setConfirming(authority.id)}>{s.revokeCA}</button>}</div>
      {confirming === authority.id && <div className="credential-confirmation" role="alert"><p>{s.revokeWarning}</p><div><button type="button" disabled={busy} onClick={() => setConfirming('')}>{t.cancel}</button><button className="danger-action" type="button" disabled={busy} onClick={() => revoke(authority)}>{t.confirmRevoke}</button></div></div>}
    </article>)}</div>}
    {items.some(item => !item.revoked && Date.parse(item.not_after) > Date.now()) && <div className="security-next-step"><span>{s.secondStep}</span><button type="button" onClick={onCertificates}>{s.issue} →</button></div>}
    {exported && <ExportDialog {...exported} t={t} onClose={() => setExported(null)} />}
  </section>;
}

export function CertificateManagement({ request, t, onAuthorities }) {
  const s = copy(t);
  const [items, setItems] = useState([]);
  const [authorities, setAuthorities] = useState([]);
  const [loading, setLoading] = useState(true);
  const [form, setForm] = useState('');
  const [busy, setBusy] = useState(false);
  const [search, setSearch] = useState('');
  const [confirming, setConfirming] = useState('');
  const [error, setError] = useState('');
  const [exported, setExported] = useState(null);
  const [refresh, setRefresh] = useState(0);
  const [mutate, resetMutation] = useMutation(request);
  useEffect(() => {
    let live = true;
    request('/v1/client-certificates?include_revoked=true').then(result => { if (live) setItems(result.items); }).catch(() => { if (live) setError(t.operationFailed); }).finally(() => { if (live) setLoading(false); });
    request('/v1/certificate-authorities?include_revoked=true').then(result => { if (live) setAuthorities(result.items || []); }).catch(() => {});
    return () => { live = false; };
  }, [refresh]);
  const active = authorities.filter(item => !item.revoked && Date.parse(item.not_after) > Date.now() && Date.parse(item.not_before) <= Date.now());
  const submit = async event => {
    event.preventDefault(); if (busy) return;
    const data = new FormData(event.currentTarget);
    setBusy(true); setError('');
    const issued = form === 'issue';
    try {
      const body = issued ? { display_name: data.get('display_name'), authority_id: data.get('authority_id'), valid_days: Number(data.get('valid_days')) } : { display_name: data.get('display_name'), certificate_pem: data.get('certificate_pem') };
      const result = await mutate(issued ? '/v1/client-certificates/issue' : '/v1/client-certificates', body);
      setForm(''); setRefresh(value => value + 1);
      if (issued) {
        if (result.export_bundle) setExported({ bundle: result.export_bundle, filename: `configra-client-${result.certificate.fingerprint_sha256.slice(0, 8)}.zip` });
        else setError(s.replay);
      }
    } catch { setError(t.operationFailed); } finally { setBusy(false); }
  };
  const revoke = async certificate => {
    if (busy) return; setBusy(true); setError('');
    try { await mutate(`/v1/client-certificates/${certificate.fingerprint_sha256}/revoke`); setConfirming(''); setRefresh(value => value + 1); }
    catch { setError(t.operationFailed); } finally { setBusy(false); }
  };
  const visible = items.filter(item => `${item.display_name} ${item.subject} ${item.fingerprint_sha256}`.toLowerCase().includes(search.toLowerCase()));
  return <section className="admin-panel certificates-panel">
    <div className="security-panel-heading"><div><span className="section-kicker">{s.secondStep}</span><h2>{t.clientCertificates}</h2><p>{t.caVerifiedCertificates}</p></div><div className="row-actions"><button className="secondary-action compact-action" type="button" disabled={busy} onClick={() => { resetMutation(); setForm(form === 'import' ? '' : 'import'); setError(''); }}>{t.importClientCertificate}</button><button className="primary-action compact-action" type="button" disabled={busy} onClick={() => { resetMutation(); setForm(form === 'issue' ? '' : 'issue'); setError(''); }}>{s.newClient}</button></div></div>
    {form === 'issue' && active.length === 0 ? <div className="security-next-step"><p>{s.noCA}</p><button type="button" onClick={onAuthorities}>{s.newCA} →</button></div> : form && <form className="credential-create-form" onSubmit={submit}>
      {form === 'import' && <p>{t.publicCertificateOnly}</p>}
      <label>{t.certificateName}<input name="display_name" required maxLength={form === 'issue' ? 128 : 255} autoComplete="off" /></label>
      {form === 'issue' ? <><label>{s.authority}<select name="authority_id" required defaultValue={active[0]?.id}>{active.map(authority => <option value={authority.id} key={authority.id}>{authority.display_name}</option>)}</select></label><label>{s.days}<input name="valid_days" type="number" min="1" max="365" defaultValue="90" required /></label></> : <label className="full-width">{t.publicCertificatePEM}<textarea name="certificate_pem" required rows="7" spellCheck="false" /></label>}
      <div className="form-actions"><button type="button" disabled={busy} onClick={() => setForm('')}>{t.cancel}</button><button className="primary-action compact-action" disabled={busy} type="submit">{busy ? s.creating : form === 'issue' ? s.issue : t.registerCertificate}</button></div>
    </form>}
    {error && <p className="inline-error" role="alert">{error}</p>}
    {!loading && items.length > 0 && <label className="credential-search"><span>{s.search}</span><input type="search" value={search} onChange={event => setSearch(event.target.value)} /></label>}
    {loading ? <div className="collection-state"><span className="loading-line" /></div> : items.length === 0 ? <div className="credential-empty"><h3>{s.emptyClients}</h3><p>{s.emptyClientsBody}</p></div> : <div className="table-frame credential-table"><table><thead><tr><th>{t.certificateName}</th><th>{s.authority}</th><th>{t.fingerprint}</th><th>{t.validUntil}</th><th>{t.status}</th><th /></tr></thead><tbody>{visible.map(certificate => <tr key={certificate.fingerprint_sha256}>
      <td><strong>{certificate.display_name}</strong><small className="row-subkey">{certificate.subject}</small></td><td>{authorities.find(authority => authority.id === certificate.authority_id)?.display_name || s.external}</td><td><code className="fingerprint" title={certificate.fingerprint_sha256}>{certificate.fingerprint_sha256.slice(0, 12)}…{certificate.fingerprint_sha256.slice(-8)}</code><small className="row-subkey">{t.serial} {certificate.serial_hex}</small></td><td><time>{date(certificate.not_after, t)}</time></td><td><Status record={certificate} t={t} /></td><td>{!certificate.revoked && <button className="danger-link" type="button" disabled={busy} aria-label={`${confirming === certificate.fingerprint_sha256 ? t.confirmRevoke : t.revoke} ${certificate.display_name}`} onClick={() => confirming === certificate.fingerprint_sha256 ? revoke(certificate) : setConfirming(certificate.fingerprint_sha256)}>{confirming === certificate.fingerprint_sha256 ? t.confirmRevoke : t.revoke}</button>}</td>
    </tr>)}</tbody></table></div>}
    {exported && <ExportDialog {...exported} t={t} onClose={() => setExported(null)} />}
  </section>;
}
