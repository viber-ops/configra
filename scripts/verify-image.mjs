#!/usr/bin/env node
// Verify the actual scratch filesystem and CLI without starting Configra services.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdirSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const [image, binary] = process.argv.slice(2);
assert.match(image ?? '', /^[A-Za-z0-9][A-Za-z0-9./:@_-]*$/, 'Pass a locally built image');
assert.ok(['configra', 'configra-kubernetes'].includes(binary), 'Name the binary in the image');
const run = (args) => execFileSync('docker', args, { encoding: 'utf8', timeout: 30_000 }).trim();
const metadata = JSON.parse(run(['image', 'inspect', '--format',
  '{"id":{{json .Id}},"user":{{json .Config.User}},"arch":{{json .Architecture}},"os":{{json .Os}}}', image]));
assert.equal(metadata.user, '65532:65532', 'Image runs as the documented non-root user');
assert.equal(metadata.os, 'linux');
const temporary = mkdtempSync(join(tmpdir(), 'configra-image-check-'));
let container;
try {
  // Resolve the tag first and use the immutable ID throughout the check.
  container = run(['create', '--read-only', '--network=none', '--cap-drop=ALL',
    '--security-opt=no-new-privileges', metadata.id, '--version']);
  assert.match(container, /^[a-f0-9]{64}$/);
  const source = binary === 'configra' ? root : join(root, 'kubernetes');
  for (const file of ['LICENSE', 'NOTICE']) {
    run(['cp', `${container}:/licenses/${binary}/${file}`, join(temporary, file)]);
    assert.deepEqual(readFileSync(join(temporary, file)), readFileSync(join(source, file)),
      `${binary}: image ${file} matches reviewed source`);
  }
  const goEnv = { ...process.env, GOTOOLCHAIN: 'go1.26.7' };
  const goRoot = execFileSync('go', ['env', 'GOROOT'], { encoding: 'utf8', env: goEnv }).trim();
  run(['cp', `${container}:/licenses/${binary}/go1.26.7`, join(temporary, 'go1.26.7')]);
  for (const file of readFileSync(join(root, 'scripts/go-notices/files.txt'), 'utf8').trim().split('\n')) {
    const stored = /\.(?:go|s|h)$/.test(file) ? `${file}.txt` : file;
    assert.deepEqual(readFileSync(join(temporary, 'go1.26.7', stored)), readFileSync(join(goRoot, file)),
      `${binary}: image preserves original Go notice ${file}`);
  }
  assert.deepEqual(readFileSync(join(temporary, 'go1.26.7/SOURCE.md')),
    readFileSync(join(root, 'scripts/go-notices/SOURCE.md')));
  run(['cp', `${container}:/${binary}`, join(temporary, binary)]);
  const info = JSON.parse(execFileSync('go', ['version', '-m', '-json', join(temporary, binary)], {
    encoding: 'utf8', timeout: 30_000, env: { ...process.env, GOTOOLCHAIN: 'go1.26.7' },
  }));
  const settings = Object.fromEntries(info.Settings.map(({ Key, Value }) => [Key, Value]));
  assert.equal(settings.GOOS, metadata.os);
  assert.equal(settings.GOARCH, metadata.arch);
  assert.equal(settings.CGO_ENABLED, '0');
  assert.ok(info.Deps.every((dep) => !dep.Replace), 'Image contains no locally replaced dependency');
  const moduleDirectory = join(temporary, 'modules');
  run(['cp', `${container}:/licenses/${binary}/${binary}-modules`, moduleDirectory]);
  const moduleBOM = JSON.parse(readFileSync(join(moduleDirectory, 'sbom.cdx.json'), 'utf8'));
  assert.deepEqual(moduleBOM.components.map((entry) => `${entry.name}@${entry.version}`).sort(),
    info.Deps.map((entry) => `${entry.Path}@${entry.Version}`).sort());
  const hash = (bytes) => createHash('sha256').update(bytes).digest('hex');
  const caDirectory = join(temporary, 'ca-certificates');
  run(['cp', `${container}:/licenses/${binary}/ca-certificates`, caDirectory]);
  run(['cp', `${container}:/etc/ssl/certs/ca-certificates.crt`, join(temporary, 'system-ca.crt')]);
  const caMetadata = JSON.parse(readFileSync(join(caDirectory, 'metadata.json'), 'utf8'));
  assert.equal(caMetadata.package, 'ca-certificates');
  assert.equal(caMetadata.version, '20250419~deb12u1');
  assert.equal(caMetadata.bundle.sha256, hash(readFileSync(join(temporary, 'system-ca.crt'))));
  assert.equal(hash(readFileSync(join(caDirectory, 'source.tar.xz'))),
    'b2a431cbab9a0ece921cffacbe238dc27a3e382ad4a1806dc8968c5eff30471d');
  assert.match(readFileSync(join(caDirectory, 'MPL-2.0.txt'), 'utf8'), /Mozilla Public License Version 2.0/);
  assert.match(readFileSync(join(caDirectory, 'GPL-2.0.txt'), 'utf8'), /GNU GENERAL PUBLIC LICENSE/);
  for (const file of caMetadata.files) {
    assert.ok(!file.path.includes('/') && file.path !== '..');
    assert.equal(hash(readFileSync(join(caDirectory, file.path))), file.sha256);
  }
  run(['cp', `${container}:/licenses/${binary}/SBOM.cdx.json`, join(temporary, 'SBOM.cdx.json')]);
  run(['cp', `${container}:/licenses/${binary}/INVENTORY.json`, join(temporary, 'INVENTORY.json')]);
  const completeBOM = JSON.parse(readFileSync(join(temporary, 'SBOM.cdx.json'), 'utf8'));
  const inventory = JSON.parse(readFileSync(join(temporary, 'INVENTORY.json'), 'utf8'));
  assert.equal(completeBOM.bomFormat, 'CycloneDX');
  for (const name of [binary, 'stdlib', 'ca-certificates']) {
    assert.ok(completeBOM.components.some((entry) => entry.name === name), `Whole-image inventory includes ${name}`);
  }
  assert.ok(inventory.files.some((entry) => entry.path === 'etc/ssl/certs/ca-certificates.crt'));
  const exported = join(temporary, 'image.tar');
  const unpacked = join(temporary, 'rootfs');
  run(['export', '--output', exported, container]);
  const paths = execFileSync('tar', ['-tf', exported], { encoding: 'utf8' }).trim().split('\n');
  assert.ok(paths.every((path) => !path.startsWith('/') && !path.includes('\\') && !path.split('/').includes('..')));
  const kinds = execFileSync('tar', ['-tvf', exported], { encoding: 'utf8' }).trim().split('\n');
  // Docker's init layer adds this mount-table alias; never extract its absolute target.
  assert.deepEqual(kinds.filter((entry) => !entry.startsWith('-') && !entry.startsWith('d') &&
    !(entry.startsWith('l') && entry.endsWith(' etc/mtab -> /proc/mounts'))), [], 'Unexpected exported file kinds');
  mkdirSync(unpacked);
  execFileSync('tar', ['-xpf', exported, '--no-same-owner', '--exclude=etc/mtab', '-C', unpacked]);
  execFileSync(process.execPath, [join(root, 'scripts/verify-distribution.mjs'), unpacked,
    `licenses/${binary}/INVENTORY.json`, '--container-export'], { stdio: 'inherit' });
  assert.equal(moduleBOM.metadata.component.hashes[0].content, hash(readFileSync(join(temporary, binary))),
    'The module SBOM describes this exact executable');
  for (const dependency of moduleBOM.components) {
    const properties = Object.fromEntries(dependency.properties.map(({ name, value }) => [name, value]));
    const actual = info.Deps.find((entry) => entry.Path === dependency.name);
    assert.equal(properties['configra:go:module-sum'], actual.Sum);
    for (const file of JSON.parse(properties['configra:license-files'])) {
      assert.ok(file.path.startsWith('files/') && !file.path.split('/').includes('..') && !file.path.includes('\\'));
      assert.equal(hash(readFileSync(join(moduleDirectory, file.path))), file.sha256, `Original notice: ${file.path}`);
    }
  }
  if (binary === 'configra') {
    assert.equal(hash(readFileSync(join(moduleDirectory, 'sources/mysql-v1.10.0.zip'))),
      'dc93f5770556406e82bf750a980d2316f882a19d883a3689eadb820708c2b651');
    const uiDirectory = join(temporary, 'ui');
    run(['cp', `${container}:/licenses/configra/ui`, uiDirectory]);
    const notices = readFileSync(join(uiDirectory, 'THIRD_PARTY_NOTICES.txt'));
    const uiBOM = readFileSync(join(uiDirectory, 'sbom.cdx.json'));
    const executable = readFileSync(join(temporary, binary));
    assert.ok(executable.includes(notices), 'UI notices are present in the embedded distribution too');
    assert.ok(executable.includes(uiBOM), 'UI SBOM is present in the embedded distribution too');
    assert.equal(JSON.parse(uiBOM).bomFormat, 'CycloneDX');
  }
  if (binary === 'configra-kubernetes') {
    const expectedSDK = readFileSync(join(root, 'kubernetes/go.mod'), 'utf8')
      .match(/^\s*github\.com\/viber-ops\/configra-go\s+(\S+)$/m)?.[1];
    const sdk = info.Deps.find((dep) => dep.Path === 'github.com/viber-ops/configra-go');
    assert.equal(sdk?.Version, expectedSDK, 'Image uses the SDK declared in the module manifest');
    assert.match(sdk.Sum, /^h1:[A-Za-z0-9+/]+=*$/);
  }
  const output = run(['start', '--attach', container]);
  assert.equal(run(['inspect', '--format', '{{.State.ExitCode}}', container]), '0');
  assert.ok(output.startsWith(`${binary} `), 'The documented binary handles --version');
  console.log(`PASS ${binary}: ${metadata.id}; project/runtime/module notices, source/SBOM binding and non-root/read-only CLI`);
} finally {
  try {
    if (container && /^[a-f0-9]{64}$/.test(container)) run(['rm', '--force', container]);
  } finally {
    rmSync(temporary, { recursive: true });
  }
}
