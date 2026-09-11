import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');

test('a production UI build delivers runtime dependency notices and a matching SBOM', () => {
  const output = mkdtempSync(join(tmpdir(), 'configra-ui-license-check-'));
  try {
    execFileSync('npm', ['run', 'build', '--', '--outDir', output, '--emptyOutDir'], { cwd: root });
    const html = readFileSync(join(output, 'index.html'), 'utf8');
    const noticesLink = html.match(/<link rel="license" href="(\/ui\/assets\/[^"<>]+\.txt)"/);
    assert.ok(noticesLink, 'Readers can discover the license asset from the delivered HTML');
    const notices = readFileSync(join(output, noticesLink[1].slice('/ui/'.length)), 'utf8');
    for (const name of ['react', 'scheduler', '@codemirror/state', '@lezer/common']) {
      const metadata = JSON.parse(readFileSync(join(root, 'node_modules', name, 'package.json'), 'utf8'));
      assert.ok(notices.includes(`${name}@${metadata.version}`), `Includes ${name}, including transitives`);
      assert.ok(notices.includes(readFileSync(join(root, 'node_modules', name, 'LICENSE'), 'utf8')),
        `Retains the original ${name} license text`);
    }
    assert.ok(notices.includes('Copyright (c) 2020 Evan Wallace'), 'Retains emitted bundler-helper attribution');
    const bomLink = html.match(/<link rel="describedby" type="application\/vnd\.cyclonedx\+json" href="(\/ui\/assets\/[^"<>]+\.json)"/);
    assert.ok(bomLink, 'Readers can find the matching component inventory');
    const bom = JSON.parse(readFileSync(join(output, bomLink[1].slice('/ui/'.length)), 'utf8'));
    assert.equal(bom.bomFormat, 'CycloneDX');
    assert.equal(bom.specVersion, '1.6');
    const names = new Set(bom.components.map((entry) => entry.name));
    for (const name of ['react', 'react-dom', 'scheduler', 'vite', 'rolldown', '@lezer/common']) {
      assert.ok(names.has(name), `Records actual runtime input ${name}`);
    }
    assert.ok(!names.has('@playwright/test'), 'Does not label the unbundled test runner as browser code');
    assert.doesNotMatch(notices + JSON.stringify(bom), /\/Users\/|\/node_modules\//, 'No workstation paths');
    const exported = join(output, 'delivery');
    execFileSync(process.execPath, [join(root, 'scripts/export-licenses.mjs'), output, exported]);
    assert.equal(readFileSync(join(exported, 'THIRD_PARTY_NOTICES.txt'), 'utf8'), notices);
    assert.deepEqual(JSON.parse(readFileSync(join(exported, 'sbom.cdx.json'), 'utf8')), bom);
  } finally {
    rmSync(output, { recursive: true });
  }
});

test('unreviewed dependency versions and modified license texts stop the real build', () => {
  for (const change of ['version', 'license']) {
    const fixture = mkdtempSync(join(tmpdir(), 'configra-ui-license-input-'));
    try {
      mkdirSync(join(fixture, 'src'));
      mkdirSync(join(fixture, 'scripts'));
      cpSync(join(root, 'scripts/reviewed-licenses.json'), join(fixture, 'scripts/reviewed-licenses.json'));
      cpSync(join(root, 'package-lock.json'), join(fixture, 'package-lock.json'));
      cpSync(join(root, 'package.json'), join(fixture, 'package.json'));
      cpSync(join(root, 'node_modules/react'), join(fixture, 'node_modules/react'), { recursive: true });
      for (const name of ['vite', 'rolldown']) {
        mkdirSync(join(fixture, 'node_modules', name), { recursive: true });
        const files = name === 'vite' ? ['package.json', 'LICENSE.md'] : ['package.json', 'LICENSE', 'THIRD-PARTY-LICENSE'];
        for (const file of files) cpSync(join(root, 'node_modules', name, file), join(fixture, 'node_modules', name, file));
      }
      writeFileSync(join(fixture, 'index.html'), '<html><head></head><body><script type="module" src="/src/main.js"></script></body></html>');
      writeFileSync(join(fixture, 'src/main.js'), 'import React from "react"; document.body.textContent = React.version;');
      if (change === 'license') writeFileSync(join(fixture, 'node_modules/react/LICENSE'), 'unreviewed license terms');
      else {
        const path = join(fixture, 'node_modules/react/package.json');
        const pkg = JSON.parse(readFileSync(path, 'utf8'));
        pkg.version = '999.0.0';
        writeFileSync(path, JSON.stringify(pkg));
      }
      const result = spawnSync(process.execPath, [join(root, 'node_modules/vite/bin/vite.js'), 'build', fixture,
        '--config', join(root, 'vite.config.mjs')], { cwd: fixture, encoding: 'utf8' });
      assert.notEqual(result.status, 0, `${change}: must reject the unreviewed input`);
      assert.match(result.stderr, /License review required for react@/);
      assert.equal(existsSync(join(fixture, 'dist/index.html')), false, 'No publishable page on review failure');
    } finally {
      rmSync(fixture, { recursive: true });
    }
  }
});

test('an altered built asset cannot be paired with stale UI license metadata', () => {
  const fixture = mkdtempSync(join(tmpdir(), 'configra-ui-license-stale-'));
  try {
    execFileSync('npm', ['run', 'build', '--', '--outDir', fixture, '--emptyOutDir'], { cwd: root });
    const html = readFileSync(join(fixture, 'index.html'), 'utf8');
    const script = html.match(/src="\/ui\/(assets\/[^"<>]+\.js)"/)[1];
    writeFileSync(join(fixture, script), 'different build');
    const output = join(fixture, 'delivery');
    const result = spawnSync(process.execPath, [join(root, 'scripts/export-licenses.mjs'), fixture, output], { encoding: 'utf8' });
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /SBOM does not match/);
    assert.equal(existsSync(output), false, 'Validate before creating delivery material');
  } finally {
    rmSync(fixture, { recursive: true });
  }
});
