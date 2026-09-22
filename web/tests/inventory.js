// List fixtures honor the public paging/filter contract. Returning every row
// would hide client-side truncation and missing server-side filter regressions.
export function inventoryPage(route, items) {
  const { searchParams: query, pathname } = new URL(route.request().url());
  const inactive = ['/v1/api-tokens', '/v1/client-certificates', '/v1/certificate-authorities'].includes(pathname) ? 'revoked' : 'archived';
  const filtered = items.filter(item => {
    if (query.get('status') === inactive && !item[inactive]) return false;
    if (!['all', inactive].includes(query.get('status')) && query.get(`include_${inactive}`) !== 'true' && item[inactive]) return false;
    if (query.get('key') && (item.key || item.public_id || item.id || item.fingerprint_sha256) !== query.get('key')) return false;
    if (query.get('usable') === 'true' && (item.revoked || Date.parse(item.not_before) > Date.now() || Date.parse(item.not_after) <= Date.now())) return false;
    if (query.get('namespace') && item.namespace_key !== query.get('namespace')) return false;
    const environments = item.environments?.map(environment => environment.key) || item.environment_keys || [];
    if (query.get('environment') && !environments.includes(query.get('environment'))) return false;
    if (query.get('unbound') === 'true' && item.environments?.some(environment => !environment.archived)) return false;
    const text = `${item.namespace_key ? `${item.namespace_key}.` : ''}${item.key || item.public_id || item.id} ${item.display_name} ${environments.join(' ')} ${item.fingerprint_sha256 || ''} ${item.subject || ''} ${item.serial_hex || ''} ${item.authority_name || ''} ${item.safe_host || ''}`;
    return !query.get('q') || text.toLowerCase().includes(query.get('q').trim().toLowerCase());
  });
  const offset = Number(query.get('offset') || 0);
  const limit = Number(query.get('limit') || 50);
  return JSON.stringify({ total: filtered.length, items: filtered.slice(offset, offset + limit).map(item => ({
    ...item,
    ...(item.environments && { environments: item.environments.slice(0, 3), environment_count: item.environments.length,
      revision: item.environments.find(environment => environment.key === query.get('environment'))?.revision }),
    ...(['/v1/vault-items', '/v1/api-tokens'].includes(pathname) && { environment_keys: (item.environment_keys || []).slice(0, 3), environment_count: (item.environment_keys || []).length }),
    ...(pathname === '/v1/notification-destinations' && { event_types: item.event_types.slice(0, 3), event_type_count: item.event_types.length }),
  })) });
}
