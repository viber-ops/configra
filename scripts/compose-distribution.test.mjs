import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const scripts = dirname(fileURLToPath(import.meta.url));
const compose = join(scripts, 'compose-distribution.mjs');
const verify = join(scripts, 'verify-distribution.mjs');
const hash = (bytes) => createHash('sha256').update(bytes).digest('hex');
const encode = (value) => JSON.stringify(value);

function fixture(root, conflicting = false) {
  const put = (path, bytes) => { mkdirSync(dirname(join(root, path)), { recursive: true }); writeFileSync(join(root, path), bytes); };
  const goRoot = execFileSync('go', ['env', 'GOROOT'], { encoding: 'utf8', env: { ...process.env, GOTOOLCHAIN: 'go1.26.7' } }).trim();
  put('licenses/go1.26.7/LICENSE', readFileSync(join(goRoot, 'LICENSE')));
  put('licenses/go1.26.7/SOURCE.md', 'Runtime notice fixture');
  put('BUILD.json', encode({ version: 'v0.1.0-test', commit: 'a'.repeat(40), platform: 'linux', arch: 'amd64' }));
  const notice = 'Browser notice fixture';
  const ui = { bomFormat: 'CycloneDX', specVersion: '1.6', metadata: { component: {
    type: 'application', 'bom-ref': 'ui', name: 'ui', version: '1.0.0',
  }, properties: [{ name: 'configra:notices-sha256', value: hash(notice) }] }, components: [], dependencies: [] };
  put('licenses/ui/THIRD_PARTY_NOTICES.txt', notice);
  put('licenses/ui/sbom.cdx.json', encode(ui));
  for (const name of ['configra', 'configra-kubernetes']) {
    const binary = name === 'configra' ? name + notice + encode(ui) : name;
    put(name, binary);
    const moduleRoot = `licenses/${name}-modules`;
    put(`${moduleRoot}/files/shared/LICENSE`, 'Dependency notice');
    const part = { bomFormat: 'CycloneDX', specVersion: '1.6', metadata: { component: {
      type: 'application', 'bom-ref': name, name, hashes: [{ alg: 'SHA-256', content: hash(binary) }],
    }, properties: [{ name: 'configra:target', value: `${name}/linux/amd64` }, { name: 'configra:go:version', value: 'go1.26.7' }] },
    components: [{ type: 'library', 'bom-ref': 'pkg:golang/shared@v1.0.0', name: 'shared', version: 'v1.0.0', properties: [
      { name: 'configra:go:module-sum', value: conflicting && name === 'configra-kubernetes' ? 'h1:changed' : 'h1:original' },
      { name: 'configra:license-files', value: encode([{ path: 'files/shared/LICENSE', sha256: hash('Dependency notice') }]) },
    ] }], dependencies: [{ ref: name, dependsOn: ['pkg:golang/shared@v1.0.0'] }] };
    put(`${moduleRoot}/sbom.cdx.json`, encode(part));
  }
}

test('the delivered bundle preserves shared component evidence and detects changed or extra files', () => {
  const root = mkdtempSync(join(tmpdir(), 'configra-composition-'));
  try {
    fixture(root);
    execFileSync(process.execPath, [compose, 'bundle', root]);
    execFileSync(process.execPath, [verify, root, 'INVENTORY.json']);
    const bom = JSON.parse(readFileSync(join(root, 'SBOM.cdx.json'), 'utf8'));
    assert.equal(bom.components.filter((component) => component.name === 'shared').length, 1);
    assert.ok(bom.components.some((component) => component.name === 'stdlib'));
    writeFileSync(join(root, 'unexpected.txt'), 'unrecorded content');
    let result = spawnSync(process.execPath, [verify, root, 'INVENTORY.json'], { encoding: 'utf8' });
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /no missing or extra payload/);
    rmSync(join(root, 'unexpected.txt'));
    writeFileSync(join(root, 'configra-kubernetes'), 'changed binary');
    result = spawnSync(process.execPath, [verify, root, 'INVENTORY.json'], { encoding: 'utf8' });
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /File (size|digest) mismatch/);
  } finally { rmSync(root, { recursive: true }); }
});

test('a same-version shared module with contradictory source identity is not merged', () => {
  const root = mkdtempSync(join(tmpdir(), 'configra-composition-conflict-'));
  try {
    fixture(root, true);
    const result = spawnSync(process.execPath, [compose, 'bundle', root], { encoding: 'utf8' });
    assert.notEqual(result.status, 0, 'Conflicting source checksums require review');
    assert.match(result.stderr, /Conflicting component source identity/);
  } finally { rmSync(root, { recursive: true }); }
});
