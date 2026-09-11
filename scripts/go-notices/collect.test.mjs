import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const script = join(dirname(fileURLToPath(import.meta.url)), 'collect.sh');
const env = { ...process.env, GOTOOLCHAIN: 'go1.26.7', GOWORK: 'off', GOFLAGS: '-mod=readonly',
  CGO_ENABLED: '0', GOEXPERIMENT: '', GOOS: 'linux', GOARCH: 'amd64' };

test('collecting original Go notices does not add buildable packages to the checkout', () => {
  const fixture = mkdtempSync(join(tmpdir(), 'configra-go-notices-'));
  try {
    writeFileSync(join(fixture, 'go.mod'), 'module example.test/release-check\n\ngo 1.25.13\n');
    writeFileSync(join(fixture, 'main.go'), 'package main\nfunc main() {}\n');
    const output = join(fixture, 'dist/go1.26.7');
    execFileSync('sh', [script, output], { cwd: fixture, env });
    assert.equal(execFileSync('go', ['list', './...'], { cwd: fixture, env, encoding: 'utf8' }).trim(),
      'example.test/release-check');
    const goRoot = execFileSync('go', ['env', 'GOROOT'], { cwd: fixture, env, encoding: 'utf8' }).trim();
    for (const file of ['src/runtime/memmove_amd64.s', 'src/crypto/internal/fips140/edwards25519/scalar.go']) {
      assert.deepEqual(readFileSync(join(output, `${file}.txt`)), readFileSync(join(goRoot, file)),
        'Source-embedded notices are retained byte for byte');
    }
  } finally {
    rmSync(fixture, { recursive: true });
  }
});
