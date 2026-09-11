#!/usr/bin/env node
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';

assert.equal(process.argv.length, 4, 'Usage: node export-licenses.mjs BUILT_UI NEW_OUTPUT_DIRECTORY');
const [source, output] = process.argv.slice(2).map((path) => resolve(path));
assert.ok(!existsSync(output), 'Refusing to mix UI license materials with an existing output directory');
const digest = (bytes) => createHash('sha256').update(bytes).digest('hex');
const html = readFileSync(join(source, 'index.html'), 'utf8');
const noticeLinks = [...html.matchAll(/<link rel="license" href="\/ui\/(assets\/third-party-notices-([a-f0-9]{16})\.txt)"/g)];
const bomLinks = [...html.matchAll(/<link rel="describedby" type="application\/vnd\.cyclonedx\+json" href="\/ui\/(assets\/ui-sbom-([a-f0-9]{16})\.cdx\.json)"/g)];
assert.equal(noticeLinks.length, 1, 'The built HTML must identify exactly one notice asset');
assert.equal(bomLinks.length, 1, 'The built HTML must identify exactly one UI SBOM');
const notices = readFileSync(join(source, noticeLinks[0][1]));
const bomBytes = readFileSync(join(source, bomLinks[0][1]));
assert.equal(digest(notices).slice(0, 16), noticeLinks[0][2], 'Notice filename matches its contents');
assert.equal(digest(bomBytes).slice(0, 16), bomLinks[0][2], 'SBOM filename matches its contents');
const bom = JSON.parse(bomBytes);
assert.equal(bom.bomFormat, 'CycloneDX');
assert.equal(bom.specVersion, '1.6');
const properties = Object.fromEntries(bom.metadata.properties.map(({ name, value }) => [name, value]));
assert.equal(properties['configra:notices-sha256'], digest(notices), 'Notices belong to this SBOM');
const files = JSON.parse(properties['configra:bundle-files']);
assert.ok(Array.isArray(files) && files.length > 0, 'SBOM binds the actual UI assets');
const paths = new Set();
for (const file of files) {
  assert.match(file.path, /^assets\/[A-Za-z0-9][A-Za-z0-9_.-]*\.(?:js|css)$/);
  assert.ok(!paths.has(file.path), 'No duplicate UI asset identities');
  paths.add(file.path);
  assert.equal(digest(readFileSync(join(source, file.path))), file.sha256, `SBOM does not match ${file.path}`);
}
const actual = readdirSync(join(source, 'assets')).filter((name) => /\.(?:js|css)$/.test(name)).map((name) => `assets/${name}`);
assert.deepEqual(actual.sort(), [...paths].sort(), 'No unrecorded or stale JS/CSS assets');
for (const match of html.matchAll(/(?:src|href)="\/ui\/(assets\/[^"<>]+\.(?:js|css))"/g)) {
  assert.ok(paths.has(match[1]), 'HTML must load the assets identified by its SBOM');
}
mkdirSync(output, { recursive: true });
writeFileSync(join(output, 'THIRD_PARTY_NOTICES.txt'), notices);
writeFileSync(join(output, 'sbom.cdx.json'), bomBytes);
writeFileSync(join(output, 'README.txt'),
  'This collection describes the UI embedded in Configra. The same notices and SBOM are linked from its HTML.\n' +
  'JS/CSS paths in the SBOM are relative to that embedded UI, not this directory.\n' +
  'It excludes server Go code, Go runtime notices and container CA roots; those have separate materials.\n');
console.log(`Verified and exported UI notices for ${bom.components.length} bundled package versions.`);
