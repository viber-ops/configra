#!/usr/bin/env node
// Real, disposable service/image acceptance. Never accepts an existing project.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createPrivateKey, randomBytes } from 'node:crypto';
import { chmodSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import https from 'node:https';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { gunzipSync } from 'node:zlib';
import { rotateKubernetesMasterKey, withKubernetesServices } from './kubernetes-smoke.mjs';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const image = process.argv[2];
const kubernetesImage = process.argv[3] === '--kubernetes' ? process.argv[4] : undefined;
if (!image || !(process.argv.length === 3 || (kubernetesImage && process.argv.length === 5)))
  throw new Error('Usage: node scripts/service-smoke.mjs LOCAL_CONFIGRA_IMAGE [--kubernetes LOCAL_ADAPTER_IMAGE]');
const project = `configra-doc-check-image-${Date.now()}`;
mkdirSync(join(root, '.cache'), { recursive: true });
const directory = mkdtempSync(join(root, '.cache', 'service-smoke-'));
chmodSync(directory, 0o755); // The unprivileged image reads its fixture mounts.
const run = (command, args, options = {}) => execFileSync(command, args, {
  cwd: root, encoding: 'utf8', stdio: 'pipe', timeout: 180000, maxBuffer: 16 << 20, ...options,
});
const docker = (...args) => run('docker', args);
const save = (name, value, mode = 0o444) => {
  const path = join(directory, name);
  writeFileSync(path, value, { mode });
  chmodSync(path, mode); // The non-root fixture must also work with umask 077.
};
const [inspectedImage] = JSON.parse(docker('image', 'inspect', image));
assert.equal(inspectedImage.Config.User, '65532:65532');
assert.equal(docker('ps', '--all', '--quiet', '--filter', `label=com.docker.compose.project=${project}`).trim(), '');
const composeFile = join(directory, 'compose.json');
const compose = (...args) => docker('compose', '--project-name', project, '-f', composeFile, ...args);
const fixture = JSON.parse(docker('compose', '-f', 'deploy/compose.test.yaml', '-f', 'deploy/compose.local.yaml', 'config', '--format', 'json'));
fixture.name = project;
// Rendered networks have a resolved project name; this run owns a fresh one.
fixture.networks.default.name = `${project}_default`;
const network = fixture.networks.default.name;
assert.equal(fixture.services.mysql.image, 'mysql:8.0.22');
assert.deepEqual(fixture.services.mysql.tmpfs, ['/var/lib/mysql']);
run('openssl', ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-sha256', '-days', '2',
  '-subj', '/CN=Configra service acceptance', '-addext', 'subjectAltName=DNS:localhost,DNS:casdoor,DNS:configra-api.configra.svc,IP:127.0.0.1',
  '-keyout', join(directory, 'tls.key'), '-out', join(directory, 'tls.crt')]);
chmodSync(join(directory, 'tls.key'), 0o444);
chmodSync(join(directory, 'tls.crt'), 0o444);
save('master-key', randomBytes(32).toString('base64'));
save('new-master-key', randomBytes(32).toString('base64'));
const ca = readFileSync(join(directory, 'tls.crt'));
const casdoorConfig = readFileSync(join(root, 'deploy/casdoor/app.conf'), 'utf8').replaceAll('http://localhost:18080', 'https://casdoor:18080');
save('casdoor.conf', `${casdoorConfig}\nenablehttps = true\nhttpsport = 18080\nhttpscertfile = /run/configra/tls.crt\nhttpskeyfile = /run/configra/tls.key\n`);
fixture.services.casdoor.ports = [{ target: 18080, published: '18080', host_ip: '127.0.0.1', protocol: 'tcp' }];
fixture.services.casdoor.volumes.find((volume) => volume.target === '/conf/app.conf').source = join(directory, 'casdoor.conf');
const mount = { type: 'bind', source: directory, target: '/run/configra', read_only: true };
for (const name of ['tls.crt', 'tls.key']) fixture.services.casdoor.volumes.push({ type: 'bind', source: join(directory, name), target: `/run/configra/${name}`, read_only: true });
const credentials = JSON.parse(readFileSync(join(root, 'deploy/casdoor/init_data.json'), 'utf8'));
const oidcSecret = credentials.applications[0].clientSecret;
const dsn = (database) => `configra:configra-test@tcp(mysql:3306)/${database}?parseTime=true&charset=utf8mb4&collation=utf8mb4_0900_ai_ci`;
for (const [mode, port] of [['management', 18088], ['api', 18089]]) {
  fixture.services[mode] = {
    image: inspectedImage.Id, command: [mode, '--config', `/run/configra/${mode}.yaml`],
    read_only: true, cap_drop: ['ALL'], security_opt: ['no-new-privileges:true'],
    volumes: [mount], ports: [{ target: port, published: String(port), host_ip: '127.0.0.1', protocol: 'tcp' }],
    environment: {
      CONFIGRA_MYSQL_DSN: dsn('configra_local'),
      ...(mode === 'management' ? {
        CONFIGRA_CLICKHOUSE_DSN: 'clickhouse://configra:configra-test@clickhouse:9000/configra_local?dial_timeout=5s&compress=lz4',
        CONFIGRA_OIDC_CLIENT_SECRET: oidcSecret, SSL_CERT_FILE: '/run/configra/tls.crt',
      } : {}),
    },
  };
}
function saveServerConfigs(key) {
  for (const [mode, port] of [['management', 18088], ['api', 18089]]) {
    const config = {
      version: 1, listen: `:${port}`,
      tls: { certificate_file: '/run/configra/tls.crt', private_key_file: '/run/configra/tls.key' },
      mysql: { dsn_env: 'CONFIGRA_MYSQL_DSN' }, key_provider: { master_key_file: `/run/configra/${key}` },
      nats: { urls: ['nats://nats:4222'] }, logging: { level: 'info' },
    };
    if (mode === 'management') {
      config.clickhouse = { dsn_env: 'CONFIGRA_CLICKHOUSE_DSN' };
      config.oidc = {
        issuer: 'https://casdoor:18080', client_id: 'configra-local', client_secret_env: 'CONFIGRA_OIDC_CLIENT_SECRET',
        redirect_url: 'https://localhost:18088/auth/callback', role_source: 'id_token', role_claim: 'groups',
        viewer_values: ['configra-local/configra-viewers'], admin_values: ['configra-local/configra-admins'],
      };
    }
    const path = join(directory, `${mode}.yaml`);
    // Existing fixture configs are owner-controlled, never application-managed.
    try { chmodSync(path, 0o600); } catch (error) { if (error.code !== 'ENOENT') throw error; }
    writeFileSync(path, JSON.stringify(config), { mode: 0o444 });
    chmodSync(path, 0o444);
  }
}
const saveCompose = () => writeFileSync(composeFile, JSON.stringify(fixture, null, 2), { mode: 0o600 });
saveServerConfigs('master-key');
saveCompose();
save('mysql.cnf', '[client]\nhost=mysql\nprotocol=tcp\nuser=root\npassword=configra-test-root\n', 0o600);
const mysql = (sql) => compose('exec', '-T', '-e', 'MYSQL_PWD=configra-test-root', 'mysql', 'mysql', '--user=root', '--batch', '--skip-column-names', '--execute', sql);
const ready = async (port) => {
  const deadline = Date.now() + 45000;
  while (Date.now() < deadline) {
    const status = await new Promise((resolve) => {
      const request = https.get(`https://localhost:${port}/health/ready`, { ca, timeout: 2500 }, (response) => {
        response.resume(); resolve(response.statusCode);
      });
      request.on('timeout', () => request.destroy());
      request.on('error', () => resolve(0));
    });
    if (status === 204) return;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`Service port ${port} did not become ready`);
};
const docs = (mode, artifacts, extraEnv = {}) => {
  try {
    return run(process.execPath, ['scripts/docs-smoke.mjs', mode, ...(artifacts ? [artifacts] : [])], {
      timeout: mode === 'kubernetes' ? 900000 : 180000,
      env: { ...process.env, CONFIGRA_DOCS_PROJECT: project, CONFIGRA_DOCS_IDENTITY: 'https://casdoor:18080', CONFIGRA_DOCS_TLS_CERT: join(directory, 'tls.crt'), ...extraEnv },
    });
  } catch (error) {
    save(`docs-${mode}-failure.txt`, String(error.stdout ?? '') + String(error.stderr ?? ''), 0o600);
    throw error;
  }
};
const backupTool = (script, ...args) => docker('run', '--rm', '--platform', 'linux/amd64', '--network', network,
  '--user', `${process.getuid()}:${process.getgid()}`,
  '--mount', `type=bind,src=${join(root, 'deploy/backup')},dst=/tools,readonly`,
  '--mount', `type=bind,src=${directory},dst=/work`, '--entrypoint', '/bin/sh', 'mysql:8.0.22', `/tools/${script}`, '/work/mysql.cnf', ...args);
const cliOutput = [];
const cli = (...args) => {
  try {
    const output = compose('run', '--rm', '--no-deps', '-T', 'management', ...args);
    cliOutput.push(output);
    return output;
  } catch (error) {
    cliOutput.push(String(error.stdout ?? ''), String(error.stderr ?? ''));
    throw error;
  }
};
let artifacts;
let beforeRestartLogs = '';
const checks = [];
let failure;
let stage = 'startup';
const sdkRead = () => {
  const env = Object.fromEntries(Object.entries(process.env).filter(([name]) => !name.startsWith('CONFIGRA_')));
  Object.assign(env, {
    GOWORK: 'off', GOFLAGS: '-mod=readonly', GOTOOLCHAIN: 'go1.26.7',
    CONFIGRA_URL: 'https://localhost:18089', CONFIGRA_TOKEN_FILE: join(artifacts, 'token'),
    CONFIGRA_CLIENT_CERT: join(artifacts, 'client.crt'), CONFIGRA_CLIENT_KEY: join(artifacts, 'client.key'),
    CONFIGRA_SERVER_CA: join(directory, 'tls.crt'),
  });
  const output = run('go', ['-C', join(root, '..', 'configra-go'), 'run', './examples/basic'], { env });
  assert.equal(output.trim(), 'Loaded Config revision 1 (yaml)');
  cliOutput.push(output);
  console.log('PASS sibling Go SDK example reads the actual image over mTLS');
};
try {
  console.log(`Starting disposable service fixture ${project}`);
  compose('up', '-d', '--wait', 'mysql', 'nats', 'clickhouse');
  assert.equal(mysql('SELECT VERSION()').trim(), '8.0.22');
  mysql("CREATE DATABASE configra_local CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci; CREATE DATABASE casdoor CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci; GRANT ALL ON configra_local.* TO 'configra'@'%'; GRANT ALL ON casdoor.* TO 'configra'@'%';");
  compose('exec', '-T', 'clickhouse', 'clickhouse-client', '--user', 'configra', '--password', 'configra-test', '--query', 'CREATE DATABASE configra_local');
  compose('up', '-d', '--wait', 'casdoor');
  if (kubernetesImage) {
    stage = 'kubernetes';
    await withKubernetesServices({ root, directory, project, image, kubernetesImage, fixture, ready }, async (kubeconfig) => {
      const setup = docs('setup');
      console.log(setup.trim());
      artifacts = /Documentation fixture artifacts saved to (.+); private values/.exec(setup)?.[1];
      assert.ok(artifacts?.startsWith(join(root, '.cache/docs-smoke-')));
      // These API replicas predate CA creation. Managed public trust refreshes
      // every five seconds; initial CA propagation is not a rollout failure.
      await new Promise((resolve) => setTimeout(resolve, 6000));
      sdkRead();
      for (const mode of ['read', 'audit', 'rejections', 'kubernetes']) {
        console.log(`Checking ${mode} against Configra Pods`);
        const output = docs(mode, artifacts, { CONFIGRA_DOCS_KUBECONFIG: kubeconfig });
        console.log(output.trim());
        save(`kubernetes-${mode}.txt`, output, 0o600);
        cliOutput.push(output);
        checks.push(`kubernetes-${mode}`);
      }
      stage = 'kubernetes-master-key-rotation';
      await rotateKubernetesMasterKey({ kubeconfig, artifacts, backup: () => {
        mkdirSync(join(directory, 'backup'), { mode: 0o700 });
        backupTool('mysql-backup.sh', 'configra_local', '/work/backup');
      } });
      await ready(18088);
      await ready(18089);
      sdkRead();
      for (const mode of ['recovery', 'kubernetes-after-rotation']) {
        const output = docs(mode, artifacts, { CONFIGRA_DOCS_KUBECONFIG: kubeconfig });
        console.log(output.trim());
        save(`kubernetes-${mode}.txt`, output, 0o600);
        cliOutput.push(output);
      }
      checks.push('kubernetes-backup', 'kubernetes-offline-key-rotation', 'kubernetes-rotated-ca-issue-revoke', 'kubernetes-post-rotation-delivery');
    });
    checks.push('kubernetes-services', 'sibling-sdk');
  } else {
    compose('up', '-d', 'management');
    await ready(18088);
    const setup = docs('setup');
    console.log(setup.trim());
    artifacts = /Documentation fixture artifacts saved to (.+); private values/.exec(setup)?.[1];
    assert.ok(artifacts?.startsWith(join(root, '.cache/docs-smoke-')));
    compose('up', '-d', 'api');
    await ready(18089);
    sdkRead();
    for (const mode of ['read', 'audit', 'rejections', 'faults']) {
      stage = mode;
      console.log(`Checking ${mode}`);
      console.log(docs(mode, artifacts).trim());
      checks.push(mode);
    }

    stage = 'backup-restore-rotation';
    compose('stop', '--timeout', '15', 'api', 'management');
    for (const service of ['api', 'management']) {
      const [stopped] = JSON.parse(docker('inspect', `${project}-${service}-1`));
      assert.equal(stopped.State.ExitCode, 0, `${service} graceful shutdown`);
    }
    // Recreating a container discards its old Docker logs. Keep the fault-phase
    // logs too, otherwise the final leak check would only cover fresh instances.
    beforeRestartLogs = compose('logs', '--no-color', 'management', 'api');
    mkdirSync(join(directory, 'backup'), { mode: 0o700 });
    backupTool('mysql-backup.sh', 'configra_local', '/work/backup');
    backupTool('mysql-restore.sh', '/work/backup', 'configra_recovered');
    mysql("GRANT ALL ON configra_recovered.* TO 'configra'@'%';");
    for (const service of ['management', 'api']) fixture.services[service].environment.CONFIGRA_MYSQL_DSN = dsn('configra_recovered');
    saveCompose();
    const expectedCounts = { vault_revisions: 1, certificate_authorities: 1, notification_destinations: 1 };
    assert.deepEqual(JSON.parse(cli('doctor', '--config', '/run/configra/management.yaml', '--verify-vault')), expectedCounts);
    const rotated = JSON.parse(cli('rotate-master-key', '--config', '/run/configra/management.yaml', '--new-key-file', '/run/configra/new-master-key', '--confirm-database', 'configra_recovered', '--confirm-offline'));
    const { operation_id: rotationOperation, ...counts } = rotated;
    assert.ok(rotationOperation);
    assert.deepEqual(counts, expectedCounts);
    writeFileSync(join(artifacts, 'rotation-operation'), rotationOperation, { mode: 0o600 });
    assert.throws(() => cli('doctor', '--config', '/run/configra/management.yaml', '--verify-vault'), 'old key no longer accepted');
    saveServerConfigs('new-master-key');
    assert.deepEqual(JSON.parse(cli('doctor', '--config', '/run/configra/management.yaml', '--verify-vault')), expectedCounts);
    compose('up', '-d', 'management');
    await ready(18088);
    compose('up', '-d', 'api');
    await ready(18089);
    sdkRead();
    console.log(docs('recovery', artifacts).trim());
    console.log(docs('audit', artifacts).trim());
    checks.push('backup', 'restore', 'image-doctor', 'image-rotation', 'restored-mtls-reads', 'restored-ca-issue-revoke', 'sibling-sdk');
    console.log('PASS actual-image backup, restore, offline rotation and restarted mTLS reads');
  }
} catch (error) {
  // Subprocess errors can embed entire output and command arguments. Preserve
  // fixture evidence privately; the terminal gets a bounded, value-free error.
  failure = error;
  save('failure.txt', String(error.stack ?? error), 0o600);
} finally {
  try {
    if (!failure) stage = 'leak-scan';
    let clusterLogs = '';
    try { clusterLogs = readFileSync(join(directory, 'kubernetes-application.log'), 'utf8'); } catch (error) { if (error.code !== 'ENOENT') throw error; }
    for (const name of ['kubernetes-events.json', 'kubernetes-status.json']) {
      try { cliOutput.push(readFileSync(join(directory, name), 'utf8')); } catch (error) { if (error.code !== 'ENOENT') throw error; }
    }
    const applicationLogs = beforeRestartLogs + compose('logs', '--no-color', 'management', 'api') + clusterLogs;
    save('application.log', applicationLogs, 0o600);
    if (artifacts) {
      const forbidden = ['token', 'expected-password', 'expected-file', 'notification-secret'].map((name) => readFileSync(join(artifacts, name)));
      // Also catch a binary File logged as escaped/replacement-character text.
      const fileMarker = readFileSync(join(artifacts, 'expected-file')).toString('latin1').match(/docs-file-[a-f0-9]{36}/)?.[0];
      assert.ok(fileMarker);
      forbidden.push(Buffer.from(fileMarker));
      forbidden.push(Buffer.from(oidcSecret));
      try {
        for (const value of JSON.parse(readFileSync(join(artifacts, 'kubernetes-private.json'), 'utf8'))) forbidden.push(Buffer.from(value, 'base64'));
      } catch (error) { if (error.code !== 'ENOENT') throw error; }
      const caPrivate = run('unzip', ['-p', join(artifacts, 'ca.zip'), 'ca.key'], { encoding: 'buffer' });
      for (const key of [caPrivate, readFileSync(join(artifacts, 'client.key')), readFileSync(join(directory, 'tls.key'))]) {
        forbidden.push(key, createPrivateKey(key).export({ type: 'pkcs8', format: 'der' }));
      }
      try {
        const key = readFileSync(join(artifacts, 'recovered-client.key'));
        forbidden.push(key, createPrivateKey(key).export({ type: 'pkcs8', format: 'der' }));
      } catch (error) { if (error.code !== 'ENOENT') throw error; }
      for (const name of ['master-key', 'new-master-key']) {
        const encoded = readFileSync(join(directory, name));
        forbidden.push(encoded, Buffer.from(encoded.toString(), 'base64'));
      }
      const dumpPath = join(directory, 'backup/mysql.sql.gz');
      let dump;
      try { dump = gunzipSync(readFileSync(dumpPath)); } catch (error) { if (error.code !== 'ENOENT') throw error; }
      const lowerDump = dump?.toString('latin1').toLowerCase();
      const observed = Buffer.from(applicationLogs + cliOutput.join('\n'));
      for (const value of forbidden) {
        assert.ok(!observed.includes(value) && !observed.includes(value.toString('base64')) && !observed.includes(value.toString('hex')), 'application/CLI logs contain fixture credential material');
        if (dump) assert.ok(!dump.includes(value) && !dump.includes(value.toString('base64')) && !lowerDump.includes(value.toString('hex')), 'database backup contains plaintext credential material');
      }
      checks.push(kubernetesImage ? 'kubernetes-log-status-backup-leak-scan' : 'log-and-backup-leak-scan');
    }
  } catch (error) { failure ??= error; }
  // Only this newly created, uniquely named project's tmpfs services are removed.
  try { compose('down'); } catch (error) { failure ??= error; }
  chmodSync(directory, 0o700);
  if (failure) save('failure.txt', String(failure.stack ?? failure), 0o600);
  save('result.json', JSON.stringify({ date: new Date().toISOString(), project, image: inspectedImage.Id, checks, passed: !failure, failed_stage: failure ? stage : null }, null, 2), 0o600);
}
if (failure) throw new Error(`Service acceptance failed at ${stage} (${failure.code ?? failure.name}); private diagnostics: ${directory}`);
console.log(`PASS service acceptance; containers removed, evidence in ${directory}`);
