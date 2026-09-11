#!/usr/bin/env node
// Maintainer acceptance check for the documented local stack, not a live deployment.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { readFileSync, mkdirSync, writeFileSync } from 'node:fs';
import http from 'node:http';
import https from 'node:https';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const project = process.env.CONFIGRA_DOCS_PROJECT ?? '';
const mode = process.argv[2] ?? 'setup';
if (!['setup', 'read'].includes(mode)) {
  throw new Error('Usage: docs-smoke.mjs [setup | read .cache/docs-smoke-<run>]');
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
const identity = 'http://localhost:18080';
const machine = 'https://localhost:18089';
const allowedOrigins = new Set([management, identity, machine]);
const tls = new https.Agent({
  ca: readFileSync(join(root, '.cache/local-dev/server.crt')),
  keepAlive: true,
});
const results = [];
const record = (name) => {
  results.push(name);
  console.log(`PASS ${name}`);
};

function request(
  url,
  { method = 'GET', body, jar = new Map(), headers = {}, tlsAgent = tls } = {},
) {
  const destination = new URL(url);
  if (!allowedOrigins.has(destination.origin))
    throw new Error(
      'Refusing an origin outside the local documentation fixture',
    );
  const data =
    body === undefined ? undefined : Buffer.from(JSON.stringify(body));
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
          });
        });
      },
    );
    outgoing.setTimeout(15000, () =>
      outgoing.destroy(new Error('Documentation request timed out')),
    );
    outgoing.on('error', () =>
      reject(
        new Error(
          `Local documentation request failed (${destination.origin}${destination.pathname})`,
        ),
      ),
    );
    outgoing.end(data);
  });
}

async function login(username, password) {
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
  const finish = await request(callback, { jar });
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

async function verifyReads(directory) {
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
    ca: readFileSync(join(root, '.cache/local-dev/server.crt')),
    cert: readFileSync(join(output, 'client.crt')),
    key: readFileSync(join(output, 'client.key')),
    keepAlive: true,
  });
  const endpoint = `${machine}/v1/environments/development/configs/payment`;
  const options = {
    tlsAgent: clientTLS,
    headers: { Authorization: `Bearer ${token}` },
  };
  try {
    const resolved = await request(endpoint, options);
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
    const unchanged = await request(endpoint, {
      ...options,
      headers: { ...options.headers, 'If-None-Match': resolved.headers.etag },
    });
    assert.equal(unchanged.status, 304);
    record('the returned ETag produces an unchanged 304 read');
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

try {
  if (mode === 'read') {
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
    await api(admin, '/v1/vault-items/platform/database', 'PUT', {
      display_name: 'Application database',
      expected_revision: 0,
      snapshot: {
        fields: [
          { key: 'username', name: 'Username', type: 'text' },
          { key: 'password', name: 'Password', type: 'secret' },
        ],
        variants: [
          {
            id: randomBytes(16).toString('hex'),
            environments: ['development'],
            values: {
              username: { text: 'payment' },
              password: { text: password },
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

    const output = join(root, '.cache', `docs-smoke-${Date.now()}`);
    mkdirSync(output, { recursive: true, mode: 0o700 });
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
