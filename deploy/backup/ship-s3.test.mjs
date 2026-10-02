import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';

test('S3 shipment verifies remote bytes before recording a recovery point; failures retain prior success', () => {
  const root = mkdtempSync(join(tmpdir(), 'configra-backup-test-'));
  try {
    const bin = join(root, 'bin'), artifact = join(root, 'artifact');
    mkdirSync(bin); mkdirSync(artifact); mkdirSync(join(root, 'remote'));
    const body = Buffer.from('private-backup-value');
    writeFileSync(join(artifact, 'mysql.sql.gz'), body);
    writeFileSync(join(artifact, 'manifest.sha256'), `${createHash('sha256').update(body).digest('hex')}  mysql.sql.gz\n`);
    const point = Math.floor(Date.now() / 1000) - 60;
    writeFileSync(join(artifact, 'recovery-point.timestamp'), `${point}\n`);
    writeFileSync(join(artifact, 'metadata.txt'), 'schema=4\nkey_reference=escrow-fixture\n');
    writeFileSync(join(bin, 'aws'), `#!${process.execPath}\n` + `
const fs=require('node:fs'),path=require('node:path'),args=process.argv.slice(2);
if(args[0]!=='s3'||args[1]!=='cp')process.exit(2);
const remote=x=>path.join(process.env.FIXTURE_ROOT,'remote',path.basename(x));
let data=fs.readFileSync(args[2].startsWith('s3://')?remote(args[2]):args[2]);
if(process.env.FIXTURE_CORRUPT && args[2].startsWith('s3://'))data=Buffer.from('corrupted');
fs.writeFileSync(args[3].startsWith('s3://')?remote(args[3]):args[3],data);
`, { mode: 0o700 });
    writeFileSync(join(bin, 'curl'), `#!${process.execPath}\n` + `
const fs=require('node:fs'),path=require('node:path'),args=process.argv.slice(2);
if(args[args.indexOf('--request')+1]!=='POST')process.exit(2);
fs.copyFileSync(args[args.indexOf('--data-binary')+1].slice(1),path.join(process.env.FIXTURE_ROOT,'metrics'));
`, { mode: 0o700 });
    const env = { ...process.env, PATH: `${bin}:${process.env.PATH}`, BACKUP_ARTIFACT_DIRECTORY: artifact, BACKUP_S3_PREFIX: 's3://fixture/configra', POD_UID: 'fixture-job', PUSHGATEWAY_URL: 'http://fixture', FIXTURE_ROOT: root };
    const script = join(dirname(fileURLToPath(import.meta.url)), 'ship-s3.sh');
    const success = spawnSync('sh', [script], { env, encoding: 'utf8' });
    assert.equal(success.status, 0, success.stderr);
    assert.match(readFileSync(join(root, 'metrics'), 'utf8'), new RegExp(`configra_backup_recovery_point_timestamp_seconds\\{store="mysql"\\} ${point}`));
    const failed = spawnSync('sh', [script], { env: { ...env, POD_UID: 'second-job', FIXTURE_CORRUPT: '1' }, encoding: 'utf8' });
    assert.notEqual(failed.status, 0);
    const metrics = readFileSync(join(root, 'metrics'), 'utf8');
    assert.match(metrics, /last_attempt_succeeded\{store="mysql"\} 0/);
    assert.doesNotMatch(metrics, /last_success|recovery_point/);
    assert.doesNotMatch(success.stdout + success.stderr + failed.stdout + failed.stderr, /private-backup-value/);
  } finally { rmSync(root, { recursive: true, force: true }); }
});
