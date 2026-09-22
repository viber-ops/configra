import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const render = (files) =>
  JSON.parse(
    execFileSync(
      'docker',
      [
        'compose',
        '--project-name',
        'configra-profile-check',
        ...files.flatMap((file) => ['-f', file]),
        'config',
        '--format',
        'json',
      ],
      { cwd: root, encoding: 'utf8' },
    ),
  );

test('development avoids native AIO without changing the acceptance baseline', () => {
  const baseline = render(['deploy/compose.test.yaml']);
  const development = render([
    'deploy/compose.test.yaml',
    'deploy/compose.local.yaml',
  ]);
  assert.equal(baseline.services.mysql.image, 'mysql:8.0.22');
  assert.equal(development.services.mysql.image, 'mysql:8.0.22');
  assert.ok(
    !baseline.services.mysql.command,
    'production acceptance fixture keeps the original server command',
  );
  assert.deepEqual(development.services.mysql.command, [
    '--innodb-use-native-aio=0',
  ]);
  for (const service of Object.values(development.services)) {
    for (const port of service.ports ?? [])
      assert.equal(port.host_ip, '127.0.0.1');
  }
  assert.deepEqual(development.services.mysql.tmpfs, ['/var/lib/mysql']);
  assert.ok(
    development.services.clickhouse.tmpfs.includes('/var/lib/clickhouse'),
  );
});

test('local startup and stop use an explicit separate project', () => {
  for (const target of ['local-dependencies-up', 'local-stop']) {
    const output = execFileSync(
      'make',
      ['-n', target, 'LOCAL_PROJECT=configra-profile-check'],
      { cwd: root, encoding: 'utf8' },
    );
    const commands = output
      .split('\n')
      .filter((line) => line.startsWith('docker compose'));
    assert.ok(commands.length > 0);
    for (const command of commands) {
      assert.ok(command.includes("--project-name 'configra-profile-check'"));
      assert.ok(command.includes('-f deploy/compose.local.yaml'));
    }
  }
});

test('integration passes every external dependency to the E2E process', () => {
  const output = execFileSync(
    'make',
    ['-n', '-o', 'dependencies-test-up', 'test-integration',
      'TEST_MYSQL_ROOT_DSN=mysql-fixture', 'TEST_NATS_URL=nats-fixture',
      'TEST_CLICKHOUSE_DSN=clickhouse-fixture'],
    { cwd: root, encoding: 'utf8' },
  );
  const command = output.replace(/\\\n\s*/g, ' ').split('\n')
    .find((line) => line.includes('go -C e2e test'));
  assert.ok(command, 'E2E tests must run, not just the root module');
  for (const [name, value] of Object.entries({
    MYSQL_ROOT_DSN: 'mysql-fixture', NATS_URL: 'nats-fixture',
    CLICKHOUSE_DSN: 'clickhouse-fixture',
  })) {
    assert.ok(command.includes(`CONFIGRA_TEST_${name}='${value}'`), name);
  }
});
