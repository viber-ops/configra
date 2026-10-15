import React, { useEffect, useRef, useState } from 'react';

// Full text stays visible/selectable; only an explicit click writes to the
// clipboard. Browser errors never echo the value (which can be a one-time Token).
export function CopyValue({ value, label, t }) {
  const [notice, setNotice] = useState('');
  const timer = useRef(null);
  const current = useRef(value);
  current.current = value;
  useEffect(() => {
    setNotice('');
    return () => clearTimeout(timer.current);
  }, [value]);
  if (!value) return <span>—</span>;
  const copy = async () => {
    clearTimeout(timer.current);
    setNotice('');
    try {
      await navigator.clipboard.writeText(value);
      if (current.current !== value) return;
      setNotice('copied');
      timer.current = setTimeout(() => setNotice(''), 3000);
    } catch {
      if (current.current === value) setNotice('failed');
    }
  };
  const zh = t.locale.startsWith('zh');
  return <span className="copy-value">
    <button type="button" className="copy-value-button" aria-label={`${t.copy} ${label}`} title={`${t.copy} ${label}`} onClick={copy}>
      <code>{value}</code>
      <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true"><rect x="8" y="8" width="12" height="12" rx="2" /><path d="M15 8V4H4v11h4" /></svg>
    </button>
    {notice && <span role="status" className={`copy-feedback ${notice === 'failed' ? 'copy-failed' : ''}`}>{notice === 'copied' ? (zh ? '已复制' : 'Copied') : (zh ? '无法复制，请选中完整文本手动复制。' : 'Copy unavailable. Select the full text and copy manually.')}</span>}
  </span>;
}
