import React, { useEffect, useState } from 'react';

export async function request(path, options = {}) {
  const response = await fetch(path, {
    ...options,
    headers: { Accept: 'application/json', ...options.headers },
  });
  if (!response.ok) {
    const error = new Error(`request failed: ${response.status}`);
    error.status = response.status;
    try {
      const failure = (await response.json()).error;
      error.code = failure?.code;
      error.requestID = failure?.request_id;
    } catch {
      // The HTTP status remains enough for non-JSON failures.
    }
    throw error;
  }
  const result = response.status === 204 ? null : await response.json();
  if (options.method && !['GET', 'HEAD', 'OPTIONS'].includes(options.method) &&
      /^\/v1\/(environments|configs|vault-items)(\/|$)/.test(path) && !/\/(validate|transfer-preview|impact-preview)(\?|$)/.test(path)) {
    window.dispatchEvent(new Event('configra:inventory-changed'));
  }
  return result;
}

export function managementError(error, t, fallback) {
  return { message: t[error?.code] || fallback, requestID: error?.requestID || '' };
}

export function ActionError({ error, t }) {
  if (!error) return null;
  return <span className="action-error" role="alert"><span>{error.message}</span>{error.requestID && <button type="button" aria-label={`${t.copyRequestID} ${error.requestID}`} onClick={() => navigator.clipboard.writeText(error.requestID).catch(() => {})}><span>{t.requestID}</span><code>{error.requestID}</code></button>}</span>;
}

export function useInventory(path, refresh = 0, initialPage = 1, body = null) {
  const key = `${path}@${refresh}@${initialPage}@${body}`;
  const [navigation, setNavigation] = useState({ key, page: initialPage });
  const [state, setState] = useState({});
  const [retry, setRetry] = useState(0);
  const page = navigation.key === key ? navigation.page : initialPage;
  useEffect(() => {
    if (!path) return undefined;
    const controller = new AbortController();
    setNavigation(current => current.key === key ? current : { key, page: initialPage });
    setState({ key, page, status: 'loading', items: [], total: 0 });
    // Debounce searches and abort obsolete replies, without accumulating pages.
    const timer = window.setTimeout(() => {
      const separator = path.includes('?') ? '&' : '?';
      request(`${path}${separator}limit=50&offset=${(page - 1) * 50}`, { signal: controller.signal, ...(body !== null ? { method: 'POST', headers: { 'Content-Type': 'application/json' }, body } : {}) })
        .then(result => {
          if (!Array.isArray(result?.items) || !Number.isSafeInteger(result.total) || result.total < 0) throw new Error('invalid inventory response');
          if (!controller.signal.aborted) setState({ key, page, status: 'ready', items: result.items, total: result.total });
        })
        .catch(error => !controller.signal.aborted && setState({ key, page, status: 'failed', items: [], total: 0, error }));
    }, 150);
    return () => { window.clearTimeout(timer); controller.abort(); };
  }, [path, key, page, initialPage, retry, body]);
  return {
    ...(state.key === key && state.page === page ? state : { status: 'loading', items: [], total: 0 }),
    page,
    setPage: next => setNavigation({ key, page: next }),
    retry: () => setRetry(value => value + 1),
  };
}

export function InventoryPagination({ collection, t, onPageChange, hideSinglePage = false }) {
  const change = page => { collection.setPage(page); onPageChange?.(page); };
  return <>
    {collection.status === 'failed' && <p><ActionError error={managementError(collection.error, t, t.loadFailed)} t={t} /> <button type="button" onClick={collection.retry}>{t.retry}</button></p>}
    {(!hideSinglePage || collection.page > 1 || collection.total > 50) && <nav className="config-pagination" aria-label={t.pagination}>
      <span>{collection.status === 'ready' ? `${collection.items.length ? (collection.page - 1) * 50 + 1 : 0}–${collection.items.length ? (collection.page - 1) * 50 + collection.items.length : 0} / ${collection.total}` : '…'}</span>
      <div><button type="button" disabled={collection.page === 1 || collection.status === 'loading'} onClick={() => change(collection.page - 1)}>{t.previous}</button><code>{t.pageNumber.replace('{page}', collection.page)}</code><button type="button" disabled={collection.status !== 'ready' || collection.page * 50 >= collection.total} onClick={() => change(collection.page + 1)}>{t.next}</button></div>
    </nav>}
  </>;
}
