const resourceKey = /^[a-z][a-z0-9_-]{0,62}$/;

export function validateTokenInput(body, maxDays, now = Date.now()) {
  if (!body.display_name || new TextEncoder().encode(body.display_name).length > 255) return { field: 'display_name', reason: 'token_name_invalid' };
  for (const field of ['config_keys', 'namespace_keys']) {
    const keys = body[field] || [];
    if (keys.length > 256) return { field, reason: 'scope_keys_too_many' };
    if (keys.some(key => !resourceKey.test(key))) return { field, reason: 'scope_key_invalid' };
    if (new Set(keys).size !== keys.length) return { field, reason: 'scope_keys_duplicate' };
  }
  if (body.kind === 'write-scoped' && !body.environment_keys.length) return { field: 'environment_keys', reason: 'environment_required' };
  if (body.kind === 'write-scoped' && !body.expires_at) return { field: 'expires_at', reason: 'expiry_required' };
  if (body.expires_at) {
    const expiry = new Date(body.expires_at).getTime();
    if (!Number.isFinite(expiry)) return { field: 'expires_at', reason: 'expiry_invalid' };
    if (expiry <= now) return { field: 'expires_at', reason: 'expiry_in_past' };
    if (body.kind === 'write-scoped' && expiry > now + maxDays * 86400000) return { field: 'expires_at', reason: 'write_token_expiry_limit', maxTTLDays: maxDays };
  }
  return null;
}

export function tokenValidationMessage(error, t, maxDays) {
  const labels = { display_name: t.tokenName, expires_at: t.expiresAt, config_keys: t.configAllowlist, namespace_keys: t.namespaceAllowlist, environment_keys: t.allowedEnvironments, kind: t.writeScoped, allow_without_mtls: t.allowWithoutMTLS };
  const field = Object.hasOwn(labels, error.field) ? error.field : '';
  const key = `tokenValidation_${error.reason}`;
  const detail = field && Object.hasOwn(t, key) ? t[key] : '';
  const days = Number.isInteger(error.maxTTLDays) && error.maxTTLDays >= 1 && error.maxTTLDays <= 90 ? error.maxTTLDays : maxDays;
  return {
    field: detail ? field : '',
    message: detail ? `${labels[field]}: ${detail.replace('{days}', days)}` : (error.code === 'validation_failed' ? t.tokenValidationFallback : t[error.code] || t.operationFailed),
    requestID: error.requestID || '',
  };
}

export function localDateTime(date) {
  const pad = value => String(value).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}
