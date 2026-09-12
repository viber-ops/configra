#!/usr/bin/env node
// Check the files recipients download, not the unpacked build staging directories.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const directory = resolve(process.argv[2] ?? '');
const version = basename(directory);
assert.match(version, /^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/, 'Pass a release directory');
const expectedSDK = readFileSync(join(root, 'kubernetes/go.mod'), 'utf8')
  .match(/^\s*github\.com\/viber-ops\/configra-go\s+(\S+)$/m)?.[1];
assert.ok(expectedSDK, 'Kubernetes declares its SDK version');
const checksums = new Map(
  readFileSync(join(directory, 'SHA256SUMS'), 'utf8').trim().split('\n').map((line) => {
    const match = /^([a-f0-9]{64})  (configra_[A-Za-z0-9_.-]+\.tar\.gz)$/.exec(line);
    assert.ok(match, `Invalid checksum entry: ${line}`);
    return [match[2], match[1]];
  }),
);
assert.equal(checksums.size, 4, 'A release contains all four platform archives');
for (const platform of ['darwin', 'linux']) {
  for (const arch of ['amd64', 'arm64']) {
    const name = `configra_${version.slice(1)}_${platform}_${arch}`;
    const archive = join(directory, `${name}.tar.gz`);
    assert.equal(
      createHash('sha256').update(readFileSync(archive)).digest('hex'),
      checksums.get(`${name}.tar.gz`),
      `${name}: archive checksum`,
    );
    const entries = execFileSync('tar', ['-tzf', archive], { encoding: 'utf8' }).trim().split('\n');
    assert.equal(new Set(entries).size, entries.length, `${name}: no duplicate archive paths`);
    assert.ok(entries.every((path) => path.startsWith(`${name}/`) &&
      !path.includes('\\') && !path.split('/').some((part) => part === '.' || part === '..')),
    `${name}: every file stays inside its bundle directory`);
    const detailedEntries = execFileSync('tar', ['-tvzf', archive], { encoding: 'utf8' }).trim().split('\n');
    assert.ok(detailedEntries.every((entry) => entry.startsWith('-') || entry.startsWith('d')),
      'Release archives contain only regular files and directories, not links or devices');
    const read = (path) => {
      assert.ok(entries.includes(`${name}/${path}`), `${name}: missing ${path}`);
      return execFileSync('tar', ['-xOf', archive, `${name}/${path}`], { encoding: 'utf8' });
    };
    assert.match(read('LICENSE'), /Apache License\s+Version 2\.0, January 2004/);
    assert.match(read('NOTICE'), /Copyright 2026 The Configra Authors/);
    assert.match(read('kubernetes/LICENSE'), /Apache License\s+Version 2\.0, January 2004/);
    assert.match(read('kubernetes/NOTICE'), /Copyright 2026 The Configra Authors/);
    assert.match(read('licenses/go1.26.7/LICENSE'), /The Go Authors/);
    for (const file of readFileSync(join(root, 'scripts/go-notices/files.txt'), 'utf8').trim().split('\n')) {
      const stored = /\.(?:go|s|h)$/.test(file) ? `${file}.txt` : file;
      assert.ok(entries.includes(`${name}/licenses/go1.26.7/${stored}`), `${name}: missing Go notice ${file}`);
    }
    assert.match(read('licenses/go1.26.7/src/runtime/memmove_amd64.s.txt'), /Lucent/);
    assert.match(read('licenses/go1.26.7/src/crypto/internal/fips140/edwards25519/scalar.go.txt'), /fiat-crypto/);
    assert.match(read('licenses/go1.26.7/SOURCE.md'), /0ed24eac755105085b89fe9cabc2742b91a0ad7b94b59d3ad364918ebc8956ad/);
    const uiNotices = read('licenses/ui/THIRD_PARTY_NOTICES.txt');
    const uiBOMText = read('licenses/ui/sbom.cdx.json');
    assert.match(uiNotices, /react@19\.2\.8/);
    assert.equal(JSON.parse(uiBOMText).bomFormat, 'CycloneDX');
    const build = JSON.parse(read('BUILD.json'));
    assert.equal(build.version, version);
    assert.equal(build.platform, platform);
    assert.equal(build.arch, arch);
    assert.match(build.commit, /^[a-f0-9]{40}$/);
    assert.equal(build.cgo, false);
    for (const binary of ['configra', 'configra-kubernetes']) {
      assert.ok(entries.includes(`${name}/${binary}`), `${name}: missing ${binary}`);
      const temporary = mkdtempSync(join(tmpdir(), 'configra-release-check-'));
      try {
        const path = join(temporary, binary);
        writeFileSync(path, execFileSync('tar', ['-xOf', archive, `${name}/${binary}`], {
          maxBuffer: 200 * 1024 * 1024,
        }), { mode: 0o600 });
        const info = JSON.parse(execFileSync('go', ['version', '-m', '-json', path], {
          encoding: 'utf8', env: { ...process.env, GOTOOLCHAIN: 'go1.26.7' },
        }));
        const settings = Object.fromEntries(info.Settings.map(({ Key, Value }) => [Key, Value]));
        assert.equal(settings.GOOS, platform, `${name}/${binary}: actual binary platform`);
        assert.equal(settings.GOARCH, arch, `${name}/${binary}: actual binary architecture`);
        assert.equal(settings.CGO_ENABLED, '0', `${name}/${binary}: static Go build`);
        assert.equal(info.GoVersion, build.go.split(/\s+/)[2], `${name}/${binary}: toolchain`);
        assert.ok(info.Deps.every((dep) => !dep.Replace), `${name}/${binary}: no local dependency replacements`);
        const moduleBOM = JSON.parse(read(`licenses/${binary}-modules/sbom.cdx.json`));
        assert.equal(moduleBOM.bomFormat, 'CycloneDX');
        const executable = readFileSync(path);
        assert.equal(moduleBOM.metadata.component.hashes[0].content, createHash('sha256').update(executable).digest('hex'));
        if (binary === 'configra') {
          assert.ok(executable.includes(Buffer.from(uiNotices)), 'UI notice delivery survives Go embedding');
          assert.ok(executable.includes(Buffer.from(uiBOMText)), 'UI SBOM delivery survives Go embedding');
        }
        const recordedModules = moduleBOM.components.filter((entry) => entry.type === 'library');
        assert.deepEqual(recordedModules.map((entry) => `${entry.name}@${entry.version}`).sort(),
          info.Deps.map((entry) => `${entry.Path}@${entry.Version}`).sort(), 'Every linked module has artifact licensing evidence');
        execFileSync('tar', ['-xzf', archive, '-C', temporary, `${name}/licenses/${binary}-modules`]);
        const moduleRoot = join(temporary, name, 'licenses', `${binary}-modules`);
        for (const dependency of recordedModules) {
          const properties = Object.fromEntries(dependency.properties.map(({ name, value }) => [name, value]));
          assert.equal(properties['configra:go:module-sum'], info.Deps.find((entry) => entry.Path === dependency.name).Sum);
          const files = JSON.parse(properties['configra:license-files']);
          assert.ok(files.length > 0, `License files exist for ${dependency.name}`);
          for (const file of files) {
            assert.ok(entries.includes(`${name}/licenses/${binary}-modules/${file.path}`), `Missing ${file.path}`);
            const material = resolve(moduleRoot, file.path);
            assert.ok(material.startsWith(`${moduleRoot}/`), 'Notice stays inside its collection');
            assert.equal(createHash('sha256').update(readFileSync(material)).digest('hex'), file.sha256,
              `Original archive notice bytes: ${file.path}`);
          }
        }
        if (binary === 'configra') {
          const source = read('licenses/configra-modules/SOURCE_ACCESS.md');
          assert.match(source, /github\.com\/go-sql-driver\/mysql/);
          assert.match(source, /MPL-2\.0/);
          const zip = execFileSync('tar', ['-xOf', archive,
            `${name}/licenses/configra-modules/sources/mysql-v1.10.0.zip`], { maxBuffer: 10 * 1024 * 1024 });
          assert.equal(createHash('sha256').update(zip).digest('hex'),
            'dc93f5770556406e82bf750a980d2316f882a19d883a3689eadb820708c2b651', 'Exact upstream MPL source is delivered');
        }
        if (binary === 'configra-kubernetes') {
          const sdk = info.Deps.find((dep) => dep.Path === 'github.com/viber-ops/configra-go');
          assert.equal(sdk?.Version, expectedSDK, `${name}: SDK matches the published module pin`);
          assert.match(sdk.Sum, /^h1:[A-Za-z0-9+/]+=*$/, `${name}: SDK source checksum is recorded`);
        }
      } finally {
        rmSync(temporary, { recursive: true });
      }
    }
    assert.ok(!entries.some((path) => /(?:^|\/)(?:\.git|\.cache|node_modules)(?:\/|$)/.test(path)));
    assert.ok(!entries.some((path) => /\.(?:key|p12|pfx)$/.test(path)), 'No private credential files');
    const extracted = mkdtempSync(join(tmpdir(), 'configra-complete-bundle-'));
    try {
      execFileSync('tar', ['-xzpf', archive, '--no-same-owner', '-C', extracted]);
      execFileSync(process.execPath, [join(root, 'scripts/verify-distribution.mjs'), join(extracted, name), 'INVENTORY.json'], { stdio: 'inherit' });
    } finally {
      rmSync(extracted, { recursive: true });
    }
    console.log(`PASS ${name}: checksums, target/toolchain, SDK, runtime/module/UI materials and source/SBOM binding`);
  }
}
