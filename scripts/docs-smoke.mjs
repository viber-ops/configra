#!/usr/bin/env node
// Maintainer acceptance check for the documented local stack, not a live deployment.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { lookup } from 'node:dns';
import { readFileSync, mkdirSync, writeFileSync } from 'node:fs';
import http from 'node:http';
import https from 'node:https';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { verifyKubernetesConsumers } from './kubernetes-smoke.mjs';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const project = process.env.CONFIGRA_DOCS_PROJECT ?? '';
const mode = process.argv[2] ?? 'setup';
if (!['setup', 'read', 'audit', 'rejections', 'faults', 'recovery', 'kubernetes', 'kubernetes-after-rotation'].includes(mode)) {
  throw new Error(
    'Usage: docs-smoke.mjs [setup | audit | rejections | read|faults|recovery|kubernetes|kubernetes-after-rotation .cache/docs-smoke-<run>]',
  );
}
if (!/^configra-doc-check-[a-z0-9-]+$/.test(project)) {
  throw new Error(
    'Set CONFIGRA_DOCS_PROJECT to an explicitly disposable configra-doc-check-* Compose project.',
  );
}
for (const service of ['mysql', 'nats', 'clickhouse', 'casdoor']) {
  const [container] = JSON.parse(
    execFileSync('docker', ['inspect', `${project}-${service}-1`], {
      encoding: 'utf8',
    }),
  );
  assert.equal(container.Config.Labels['com.docker.compose.project'], project);
  assert.equal(
    container.State.Running,
    true,
    `${service} fixture must be running`,
  );
  if (service === 'mysql') {
    assert.equal(container.Config.Image, 'mysql:8.0.22');
    assert.ok(
      Object.hasOwn(container.HostConfig.Tmpfs, '/var/lib/mysql'),
      'Refuse a persistent database fixture',
    );
  }
}

const management = 'https://localhost:18088';
const identity = process.env.CONFIGRA_DOCS_IDENTITY ?? 'http://localhost:18080';
assert.ok(['http://localhost:18080', 'https://casdoor:18080'].includes(identity));
const machine = 'https://localhost:18089';
const allowedOrigins = new Set([management, identity, machine]);
const tlsCertificate = process.env.CONFIGRA_DOCS_TLS_CERT ?? join(root, '.cache/local-dev/server.crt');
const tls = new https.Agent({
  ca: readFileSync(tlsCertificate),
  keepAlive: true,
  // The image fixture uses Docker's casdoor name internally. Only the test
  // client maps it to the published loopback port; TLS verification stays on.
  lookup(hostname, options, callback) {
    if (hostname === 'casdoor') callback(null, options.all ? [{ address: '127.0.0.1', family: 4 }] : '127.0.0.1', 4);
    else lookup(hostname, options, callback);
  },
});
const results = [];
const record = (name) => {
  results.push(name);
  console.log(`PASS ${name}`);
};

function request(
  url,
  {
    method = 'GET',
    body,
    rawBody,
    jar = new Map(),
    headers = {},
    tlsAgent = tls,
    timeout = 15000,
  } = {},
) {
  const destination = new URL(url);
  if (!allowedOrigins.has(destination.origin))
    throw new Error(
      'Refusing an origin outside the local documentation fixture',
    );
  const data =
    rawBody !== undefined
      ? Buffer.from(rawBody)
      : body === undefined
        ? undefined
        : Buffer.from(JSON.stringify(body));
  const options = {
    method,
    agent: destination.protocol === 'https:' ? tlsAgent : undefined,
    headers: {
      ...(data
        ? { 'Content-Type': 'application/json', 'Content-Length': data.length }
        : {}),
      ...(jar.get(destination.origin)
        ? { Cookie: jar.get(destination.origin) }
        : {}),
      ...headers,
    },
  };
  return new Promise((resolve, reject) => {
    const outgoing = (destination.protocol === 'https:' ? https : http).request(
      destination,
      options,
      (response) => {
        const cookies = response.headers['set-cookie'];
        if (cookies?.length)
          jar.set(
            destination.origin,
            cookies.map((cookie) => cookie.split(';', 1)[0]).join('; '),
          );
        const chunks = [];
        let size = 0;
        response.on('data', (chunk) => {
          size += chunk.length;
          if (size > 8 * 1024 * 1024)
            outgoing.destroy(
              new Error('Documentation response exceeded limit'),
            );
          else chunks.push(chunk);
        });
        response.on('error', reject);
        response.on('end', () => {
          const raw = Buffer.concat(chunks);
          let json;
          if (
            response.headers['content-type']?.includes('application/json') &&
            raw.length
          ) {
            try {
              json = JSON.parse(raw.toString('utf8'));
            } catch {
              reject(new Error('Invalid JSON response'));
              return;
            }
          }
          resolve({
            status: response.statusCode,
            headers: response.headers,
            json,
            raw,
          });
        });
      },
    );
    outgoing.setTimeout(timeout, () =>
      outgoing.destroy(new Error('Documentation request timed out')),
    );
    outgoing.on('error', (error) =>
      reject(
        new Error(
          `Local documentation request failed (${destination.origin}${destination.pathname}; ${/^[A-Z0-9_]{1,64}$/.test(error.code ?? '') ? error.code : 'transport_error'})`,
        ),
      ),
    );
    outgoing.end(data);
  });
}

async function login(username, password, beforeCallback) {
  const jar = new Map();
  const start = await request(`${management}/auth/login`, { jar });
  assert.ok(
    start.status === 302 || start.status === 303,
    'OIDC login must redirect',
  );
  const authorize = new URL(start.headers.location);
  assert.equal(authorize.origin, identity);
  const query = new URLSearchParams({ type: 'code' });
  for (const [input, output] of Object.entries({
    client_id: 'clientId',
    response_type: 'responseType',
    redirect_uri: 'redirectUri',
    scope: 'scope',
    state: 'state',
    nonce: 'nonce',
    code_challenge: 'code_challenge',
    code_challenge_method: 'code_challenge_method',
  })) {
    const value = authorize.searchParams.get(input);
    if (value !== null) query.set(output, value);
  }
  assert.equal(query.get('redirectUri'), `${management}/auth/callback`);
  assert.equal(query.get('code_challenge_method'), 'S256');
  assert.ok(
    query.get('state') && query.get('nonce') && query.get('code_challenge'),
  );
  const signedIn = await request(`${identity}/api/login?${query}`, {
    method: 'POST',
    jar,
    body: {
      application: 'app-configra-local',
      organization: 'configra-local',
      username,
      password,
      type: 'code',
      signinMethod: 'Password',
      autoSignin: true,
    },
  });
  assert.equal(signedIn.status, 200, 'Casdoor login HTTP status');
  if (signedIn.json?.status !== 'ok' || typeof signedIn.json.data !== 'string')
    throw new Error(
      'Seeded Casdoor account did not issue an authorization code',
    );
  const callback = new URL(`${management}/auth/callback`);
  callback.searchParams.set('code', signedIn.json.data);
  callback.searchParams.set('state', query.get('state'));
  beforeCallback?.();
  const finish = await request(callback, { jar });
  if (beforeCallback) {
    assert.equal(finish.status, 503, 'unavailable OIDC exchange fails closed');
    assert.equal(finish.json?.error?.code, 'authentication_unavailable');
    assert.equal((await request(`${management}/v1/environments`, { jar })).status, 401);
    return;
  }
  assert.ok(
    finish.status === 302 || finish.status === 303,
    'Configra callback must establish a session',
  );
  return jar;
}

async function api(jar, path, method = 'GET', body) {
  const response = await request(`${management}${path}`, {
    method,
    body,
    jar,
    headers: {
      Origin: management,
      ...(method === 'GET' ? {} : { 'Idempotency-Key': randomUUID() }),
    },
  });
  if (response.status !== 200)
    throw new Error(
      `Documentation API failed: ${method} ${path}, HTTP ${response.status}`,
    );
  return response.json;
}

async function verifyReads(directory, unavailable = false) {
  const output = resolve(directory ?? '');
  if (
    !output.startsWith(join(root, '.cache') + '/') ||
    !/\/docs-smoke-\d+$/.test(output)
  )
    throw new Error('Expected an owned .cache/docs-smoke-* artifact directory');
  const prior = JSON.parse(readFileSync(join(output, 'result.json'), 'utf8'));
  assert.equal(
    prior.project,
    project,
    'Credential artifacts must belong to this fixture',
  );
  const token = readFileSync(join(output, 'token'), 'utf8').trim();
  const expected = readFileSync(join(output, 'expected-password'), 'utf8');
  const clientTLS = new https.Agent({
    ca: readFileSync(tlsCertificate),
    cert: readFileSync(join(output, 'client.crt')),
    key: readFileSync(join(output, 'client.key')),
    keepAlive: true,
  });
  const endpoint = `${machine}/v1/environments/development/configs/payment`;
  const options = {
    tlsAgent: clientTLS,
    headers: { Authorization: `Bearer ${token}` },
    timeout: 25000,
  };
  try {
    const resolved = await request(endpoint, options);
    if (unavailable) {
      assert.equal(resolved.status, 503);
      assert.equal(resolved.json?.error?.code, 'service_unavailable');
      assert.ok(!resolved.raw.includes(token) && !resolved.raw.includes(expected));
      record('MySQL stall returns a bounded, value-free Machine read failure');
      return;
    }
    assert.equal(resolved.status, 200);
    assert.equal(resolved.json?.config_revision, 1);
    assert.equal(resolved.json?.vault_revisions?.['platform.database'], 1);
    assert.ok(
      resolved.json.content.includes(expected),
      'Resolved Vault value matches without printing it',
    );
    assert.ok(
      !resolved.json.content.includes('{vault.'),
      'References were fully resolved',
    );
    record(
      'verified mTLS returns the documented Config and resolved Vault value',
    );
    assert.ok(resolved.headers.etag);
    if (prior.configETag) assert.equal(resolved.headers.etag, prior.configETag, 'unchanged Config/Vault revisions keep their ETag across recovery and key rotation');
    const unchanged = await request(endpoint, {
      ...options,
      headers: { ...options.headers, 'If-None-Match': resolved.headers.etag },
    });
    assert.equal(unchanged.status, 304);
    record('the returned ETag produces an unchanged 304 read');
    const file = await request(`${machine}/v1/environments/development/vault-items/platform/database/fields/credentials/content`, options);
    assert.equal(file.status, 200);
    assert.ok(file.raw.equals(readFileSync(join(output, 'expected-file'))), 'File bytes survive the real API path');
    record('authenticated File bytes match without printing their content');
    const missingCertificate = await request(endpoint, {
      headers: options.headers,
    });
    assert.equal(missingCertificate.status, 401);
    const available = await request(endpoint, options);
    assert.equal(
      available.status,
      200,
      'API remains available to the authorized client',
    );
    record('the same Token without its client certificate is denied');
    writeFileSync(
      join(output, 'result.json'),
      JSON.stringify(
        {
          ...prior,
          configETag: resolved.headers.etag,
          readCheckedAt: new Date().toISOString(),
          checks: [...prior.checks, ...results],
        },
        null,
        2,
      ) + '\n',
      { mode: 0o600 },
    );
  } finally {
    clientTLS.destroy();
  }
}

async function verifyCredentialAudit() {
  const admin = await login('admin', 'configra-admin');
  const certificates = await api(admin, '/v1/client-certificates');
  const certificate = certificates.items.find(
    (item) => item.display_name === 'Documentation client',
  );
  assert.ok(certificate, 'Run the setup smoke first');
  const fingerprint = certificate.fingerprint_sha256;
  assert.match(fingerprint, /^[0-9a-f]{64}$/);
  const deadline = Date.now() + 10000;
  while (true) {
    const page = await api(admin, `/v1/audit?q=${fingerprint}&limit=100`);
    const matching = page.items.filter(
      (item) =>
        item.action === 'client_certificate.issue' &&
        item.resource === fingerprint,
    );
    if (matching.length > 0) {
      assert.equal(
        matching.length,
        1,
        'Issued client has one logical Audit Event',
      );
      assert.equal(matching[0].outcome, 'success');
      record(
        'issued client certificate is visible in Audit with its complete fingerprint',
      );
      return;
    }
    if (Date.now() >= deadline)
      throw new Error(
        'Issued client certificate never appeared in Management Audit',
      );
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
}

async function verifyRecoveredAuthority(directory) {
  await verifyReads(directory); // Validates fixture ownership before any writes.
  const admin = await login('admin', 'configra-admin');
  const rotation = readFileSync(join(directory, 'rotation-operation'), 'utf8');
  assert.match(rotation, /^key-rotation-[0-9a-f]{32}$/);
  const deadline = Date.now() + 30000;
  while (true) {
    const page = await api(admin, `/v1/audit?q=${rotation}&limit=100`);
    if (page.items.length) {
      assert.equal(page.items.length, 1);
      assert.equal(page.items[0].action, 'master_key.rotate');
      assert.equal(page.items[0].outcome, 'success');
      assert.equal(page.items[0].actor_type, 'system');
      record('offline rotation Audit Event arrives after the worker restarts');
      break;
    }
    assert.ok(Date.now() < deadline, 'rotation Audit Event is available after restart');
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  const authorities = await api(admin, '/v1/certificate-authorities?limit=100');
  const authority = authorities.items.find((item) => item.display_name === 'Documentation CA');
  assert.ok(authority);
  const issued = await api(admin, '/v1/client-certificates/issue', 'POST', {
    authority_id: authority.id, display_name: 'After database/key recovery', valid_days: 1,
  });
  assert.ok(issued.export_bundle && issued.certificate?.fingerprint_sha256);
  const bundle = join(directory, 'recovered-client.zip');
  writeFileSync(bundle, Buffer.from(issued.export_bundle, 'base64'), { mode: 0o600 });
  const cert = execFileSync('unzip', ['-p', bundle, 'client.crt']);
  const key = execFileSync('unzip', ['-p', bundle, 'client.key']);
  writeFileSync(join(directory, 'recovered-client.key'), key, { mode: 0o600 });
  const clientTLS = new https.Agent({ ca: readFileSync(tlsCertificate), cert, key, keepAlive: true });
  const options = { tlsAgent: clientTLS, headers: { Authorization: `Bearer ${readFileSync(join(directory, 'token'), 'utf8').trim()}` } };
  const endpoint = `${machine}/v1/environments/development/configs/payment`;
  try {
    assert.equal((await request(endpoint, options)).status, 200);
    await api(admin, `/v1/client-certificates/${issued.certificate.fingerprint_sha256}/revoke`, 'POST');
    assert.equal((await request(endpoint, options)).status, 401);
    record('restored CA issues a usable client; revocation rejects the same client/HTTP agent');
  } finally {
    clientTLS.destroy();
  }
  await verifyReads(directory);
}

async function verifyRejectedAudit() {
  const viewer = await login('viewer', 'configra-viewer');
  const admin = await login('admin', 'configra-admin');
  const marker = `never-log-${randomBytes(12).toString('hex')}`;
  const cases = [
    {
      name: 'Viewer rejection',
      jar: viewer,
      status: 403,
      code: 'forbidden',
      body: { key: 'viewer_denied', display_name: marker },
    },
    {
      name: 'malformed JSON',
      rawBody: `{"display_name":"${marker}",`,
      status: 400,
      code: 'invalid_request',
    },
    {
      name: 'unknown JSON field',
      body: {
        key: 'rejected',
        display_name: 'Rejected',
        private_value: marker,
      },
      status: 400,
      code: 'invalid_request',
    },
    {
      name: 'missing OperationID',
      body: { key: 'missing_op', display_name: marker },
      noOperation: true,
      status: 422,
      code: 'validation_failed',
    },
    {
      name: 'invalid OperationID',
      body: { key: 'invalid_op', display_name: marker },
      operation: 'bad',
      status: 422,
      code: 'validation_failed',
    },
    {
      name: 'cross-origin rejection',
      body: { key: 'csrf_rejected', display_name: marker },
      origin: 'https://untrusted.example',
      status: 403,
      code: 'csrf_rejected',
    },
    {
      name: 'unsupported content type',
      rawBody: marker,
      contentType: 'text/plain',
      status: 415,
      code: 'unsupported_media_type',
    },
  ];
  for (const scenario of cases) {
    const denied = await request(`${management}/v1/environments`, {
      method: 'POST',
      jar: scenario.jar ?? admin,
      body: scenario.body,
      rawBody: scenario.rawBody,
      headers: {
        Origin: scenario.origin ?? management,
        ...(scenario.noOperation
          ? {}
          : { 'Idempotency-Key': scenario.operation ?? randomUUID() }),
        ...(scenario.contentType
          ? { 'Content-Type': scenario.contentType }
          : {}),
      },
    });
    assert.equal(denied.status, scenario.status, scenario.name);
    assert.equal(denied.json?.error?.code, scenario.code, scenario.name);
    const id = denied.json?.error?.request_id;
    assert.match(id, /^[0-9a-f]{24}$/);
    const deadline = Date.now() + 10000;
    while (true) {
      const page = await api(admin, `/v1/audit?q=${id}&limit=100`);
      assert.ok(
        !JSON.stringify(page).includes(marker),
        'Audit excludes request values',
      );
      const matching = page.items.filter((item) => item.request_id === id);
      if (matching.length) {
        assert.equal(matching.length, 1, scenario.name);
        assert.equal(matching[0].error_code, scenario.code);
        assert.equal(matching[0].outcome, 'validation_failed');
        assert.equal(matching[0].operation_id, '');
        record(
          `${scenario.name} is searchable by its returned Request ID without values`,
        );
        break;
      }
      if (Date.now() >= deadline)
        throw new Error(`${scenario.name} has no searchable Audit Event`);
      await new Promise((resolve) => setTimeout(resolve, 250));
    }
  }
  const operation = randomUUID();
  const invalidCertificate = await request(
    `${management}/v1/client-certificates/${marker}/revoke`,
    {
      method: 'POST',
      jar: admin,
      headers: { Origin: management, 'Idempotency-Key': operation },
    },
  );
  assert.equal(invalidCertificate.status, 422);
  const deadline = Date.now() + 10000;
  while (true) {
    const page = await api(admin, `/v1/audit?q=${operation}&limit=100`);
    if (page.items.length) {
      assert.equal(
        page.items.length,
        1,
        'Domain validation is not audited twice',
      );
      assert.equal(page.items[0].outcome, 'validation_failed');
      assert.equal(
        page.items[0].resource,
        '',
        'Invalid credential identifiers are not logged',
      );
      assert.ok(!JSON.stringify(page).includes(marker));
      record(
        'invalid certificate input has one value-free logical Operation audit',
      );
      break;
    }
    if (Date.now() >= deadline)
      throw new Error('Invalid certificate validation never appeared in Audit');
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
}

async function verifyDependencies(directory) {
  for (const service of ['management', 'api']) {
    const [container] = JSON.parse(execFileSync('docker', ['inspect', `${project}-${service}-1`], { encoding: 'utf8' }));
    assert.equal(container.Config.Labels['com.docker.compose.project'], project);
    assert.equal(container.Config.User, '65532:65532');
    assert.equal(container.HostConfig.ReadonlyRootfs, true);
    assert.equal(container.State.Running, true);
  }
  const paused = new Set();
  const pause = (service) => {
    assert.ok(['mysql', 'nats', 'clickhouse', 'casdoor', 'management'].includes(service));
    execFileSync('docker', ['pause', `${project}-${service}-1`], { timeout: 15000 });
    paused.add(service);
  };
  const resume = (service) => {
    execFileSync('docker', ['unpause', `${project}-${service}-1`], { timeout: 15000 });
    paused.delete(service);
  };
  const admin = await login('admin', 'configra-admin');
  try {
    for (const service of ['management', 'nats']) {
      pause(service);
      await verifyReads(directory);
      resume(service);
      record(`Machine reads survive ${service} unavailability`);
    }
    await login('admin', 'configra-admin', () => pause('casdoor'));
    assert.equal((await request(`${management}/v1/environments`, { jar: admin })).status, 200);
    await verifyReads(directory);
    resume('casdoor');
    record('OIDC outage rejects new login without breaking existing sessions or Machine reads');

    pause('clickhouse');
    const operations = [];
    for (let index = 0; index < 3; index++) {
      const operation = randomUUID();
      const response = await request(`${management}/v1/environments`, {
        method: 'POST', jar: admin, headers: { Origin: management, 'Idempotency-Key': operation },
        body: { key: `recovery_${randomBytes(6).toString('hex')}`, display_name: 'Audit recovery fixture' },
      });
      assert.equal(response.status, 200, 'mutation succeeds with ClickHouse unavailable');
      operations.push(operation);
    }
    await verifyReads(directory);
    resume('clickhouse');
    for (const operation of operations) {
      const deadline = Date.now() + 30000;
      while (true) {
        const page = await api(admin, `/v1/audit?q=${operation}&limit=100`);
        if (page.items.length) {
          assert.equal(page.items.length, 1, 'one logical Audit Event after recovery');
          assert.equal(page.items[0].operation_id, operation);
          assert.equal(page.items[0].outcome, 'success');
          break;
        }
        assert.ok(Date.now() < deadline, 'durable Audit Event delivered after ClickHouse recovery');
        await new Promise((resolve) => setTimeout(resolve, 250));
      }
    }
    record('three successful mutations retain and deliver Audit Events after ClickHouse recovery');
    pause('mysql');
    assert.equal((await request(`${machine}/health/ready`)).status, 503);
    assert.equal((await request(`${machine}/health/live`)).status, 204);
    assert.equal((await request(`${management}/health/ready`)).status, 503);
    await verifyReads(directory, true);
    // Even a malformed resource path can contain a secret; session failure
    // logging must not echo arbitrary URLs before routing/validation runs.
    const privatePath = readFileSync(join(directory, 'expected-password'), 'utf8');
    const failedSession = await request(`${management}/v1/client-certificates/${privatePath}/revoke`, { jar: admin });
    assert.equal(failedSession.status, 500);
    assert.equal(failedSession.json?.error?.code, 'session_unavailable');
    resume('mysql');
    await verifyReads(directory);
    assert.equal((await request(`${management}/v1/environments`, { jar: admin })).status, 200);
    record('MySQL stall removes readiness, rejects session work, preserves liveness and recovers');
  } finally {
    for (const service of paused) resume(service);
  }
}

try {
  if (mode === 'kubernetes' || mode === 'kubernetes-after-rotation') {
    const directory = process.argv[3];
    await verifyReads(directory);
    const admin = await login('admin', 'configra-admin');
    const clientTLS = new https.Agent({ ca: readFileSync(tlsCertificate), cert: readFileSync(join(directory, 'client.crt')), key: readFileSync(join(directory, 'client.key')), keepAlive: false });
    try {
      await verifyKubernetesConsumers({ root, directory, kubeconfig: process.env.CONFIGRA_DOCS_KUBECONFIG,
        afterRotation: mode === 'kubernetes-after-rotation',
        api: (...args) => api(admin, ...args),
        read: async () => (await request(`${machine}/v1/environments/development/configs/payment`, { tlsAgent: clientTLS, headers: { Authorization: `Bearer ${readFileSync(join(directory, 'token'), 'utf8').trim()}` } })).status,
      });
    } finally {
      clientTLS.destroy();
    }
    await verifyReads(directory);
  } else if (mode === 'recovery') {
    await verifyRecoveredAuthority(process.argv[3]);
  } else if (mode === 'faults') {
    await verifyDependencies(process.argv[3]);
  } else if (mode === 'rejections') {
    await verifyRejectedAudit();
  } else if (mode === 'audit') {
    await verifyCredentialAudit();
  } else if (mode === 'read') {
    await verifyReads(process.argv[3]);
  } else {
    assert.equal((await request(`${management}/health/ready`)).status, 204);
    record('Management readiness over verified HTTPS');
    const anonymous = await request(`${management}/v1/environments`);
    assert.equal(anonymous.status, 401);
    record('unauthenticated management access is rejected');
    const admin = await login('admin', 'configra-admin');
    const environments = await api(admin, '/v1/environments');
    assert.equal(
      environments.items.length,
      0,
      'Use a fresh fixture; do not overwrite existing workspace data',
    );
    record('real Casdoor Admin login with state, nonce and PKCE');
    const viewer = await login('viewer', 'configra-viewer');
    const forbidden = await request(`${management}/v1/environments`, {
      method: 'POST',
      jar: viewer,
      headers: { Origin: management, 'Idempotency-Key': randomUUID() },
      body: { key: 'forbidden', display_name: 'Forbidden' },
    });
    assert.equal(forbidden.status, 403);
    record('real Casdoor Viewer cannot create an Environment');

    await api(admin, '/v1/environments', 'POST', {
      key: 'development',
      display_name: 'Development',
    });
    const password = `docs-${randomBytes(18).toString('hex')}`;
    const fileBytes = Buffer.concat([Buffer.from([0, 255, 13, 10]), Buffer.from(`docs-file-${randomBytes(18).toString('hex')}`)]);
    await api(admin, '/v1/vault-items/platform/database', 'PUT', {
      display_name: 'Application database',
      expected_revision: 0,
      snapshot: {
        fields: [
          { key: 'username', name: 'Username', type: 'text' },
          { key: 'password', name: 'Password', type: 'secret' },
          { key: 'credentials', name: 'Credentials file', type: 'file' },
        ],
        variants: [
          {
            id: randomBytes(16).toString('hex'),
            environments: ['development'],
            values: {
              username: { text: 'payment' },
              password: { text: password },
              credentials: { file: { filename: 'credentials.bin', content_type: 'application/octet-stream', bytes: fileBytes.toString('base64') } },
            },
          },
        ],
      },
    });
    const content =
      'server:\n  port: 8080\ndatabase:\n  username: "{vault.platform.database.username}"\n  password: "{vault.platform.database.password}"\n';
    await api(admin, '/v1/environments/development/configs/payment', 'PUT', {
      name: 'Payment',
      format: 'yaml',
      content,
      expected_revision: 0,
    });
    record(
      'documented Environment, Vault fields and Config references created',
    );

    const ca = await api(admin, '/v1/certificate-authorities', 'POST', {
      display_name: 'Documentation CA',
      valid_days: 7,
    });
    assert.ok(ca.authority?.id && ca.export_bundle);
    const client = await api(admin, '/v1/client-certificates/issue', 'POST', {
      authority_id: ca.authority.id,
      display_name: 'Documentation client',
      valid_days: 1,
    });
    assert.ok(client.certificate?.fingerprint_sha256 && client.export_bundle);
    const token = await api(admin, '/v1/api-tokens', 'POST', {
      display_name: 'Documentation reader',
      environment_keys: ['development'],
      allow_without_mtls: false,
    });
    assert.ok(token.token?.startsWith('cfg_'));
    record('managed CA, one-time client export and mTLS-required Token issued');
    const notificationSecret = `docs-notification-${randomBytes(18).toString('hex')}`;
    await api(admin, '/v1/notification-destinations/recovery', 'PUT', {
      display_name: 'Disabled recovery fixture', provider: 'generic_webhook',
      url: `https://hooks.example.test/${notificationSecret}`, secret: notificationSecret,
      enabled: false, event_types: [],
    });

    const output = join(root, '.cache', `docs-smoke-${Date.now()}`);
    mkdirSync(output, { mode: 0o700 });
    writeFileSync(join(output, 'ca.zip'), Buffer.from(ca.export_bundle, 'base64'), { mode: 0o600 });
    writeFileSync(
      join(output, 'client.zip'),
      Buffer.from(client.export_bundle, 'base64'),
      { mode: 0o600 },
    );
    for (const name of ['client.crt', 'client.key']) {
      const data = execFileSync('unzip', [
        '-p',
        join(output, 'client.zip'),
        name,
      ]);
      writeFileSync(join(output, name), data, { mode: 0o600 });
    }
    writeFileSync(join(output, 'token'), token.token, { mode: 0o600 });
    writeFileSync(join(output, 'expected-password'), password, { mode: 0o600 });
    writeFileSync(join(output, 'expected-file'), fileBytes, { mode: 0o600 });
    writeFileSync(join(output, 'notification-secret'), notificationSecret, { mode: 0o600 });
    writeFileSync(
      join(output, 'result.json'),
      JSON.stringify(
        { date: new Date().toISOString(), project, checks: results },
        null,
        2,
      ) + '\n',
      { mode: 0o600 },
    );
    console.log(
      `Documentation fixture artifacts saved to ${output}; private values were not printed.`,
    );
  }
} finally {
  tls.destroy();
}
