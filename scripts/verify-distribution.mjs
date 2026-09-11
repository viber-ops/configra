#!/usr/bin/env node
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { lstatSync, readFileSync, readdirSync } from 'node:fs';
import { join, posix, resolve } from 'node:path';

const [directory, inventoryPath, option] = process.argv.slice(2);
assert.ok(directory && inventoryPath && (!option || option === '--container-export'),
  'Usage: verify-distribution.mjs ROOT INVENTORY_PATH [--container-export]');
const root = resolve(directory);
const safe = (path) => typeof path === 'string' && path && !posix.isAbsolute(path) && !path.includes('\\') &&
  posix.normalize(path) === path && path !== '..' && !path.startsWith('../');
const read = (path) => { assert.ok(safe(path), `Unsafe inventory path: ${path}`); return readFileSync(join(root, path)); };
const hash = (bytes) => createHash('sha256').update(bytes).digest('hex');
const inventory = JSON.parse(read(inventoryPath));
assert.equal(inventory.formatVersion, 1);
assert.equal(inventory.excludedSelf, inventoryPath);
assert.ok(inventory.kind === 'image' || inventory.kind === 'bundle');
const expected = new Set([inventoryPath]);
for (const file of inventory.files) {
  assert.ok(!expected.has(file.path), `Duplicate file identity: ${file.path}`);
  expected.add(file.path);
  const bytes = read(file.path);
  const stat = lstatSync(join(root, file.path));
  assert.ok(stat.isFile() && !stat.isSymbolicLink());
  assert.equal(bytes.length, file.bytes, `File size mismatch: ${file.path}`);
  assert.equal(hash(bytes), file.sha256, `File digest mismatch: ${file.path}`);
  assert.equal((stat.mode & 0o777).toString(8).padStart(4, '0'), file.mode, `File mode mismatch: ${file.path}`);
}
const runtimeFiles = option === '--container-export' && inventory.kind === 'image'
  ? new Set(['.dockerenv', 'etc/hostname', 'etc/hosts', 'etc/resolv.conf', 'dev/console']) : new Set();
function walk(directory, prefix = '') {
  return readdirSync(directory).flatMap((name) => {
    const path = prefix ? `${prefix}/${name}` : name;
    if (runtimeFiles.has(path)) return [];
    const full = join(directory, name);
    const stat = lstatSync(full);
    assert.ok(!stat.isSymbolicLink(), `Unexpected symlink: ${path}`);
    if (stat.isDirectory()) return walk(full, path);
    assert.ok(stat.isFile(), `Unexpected special file: ${path}`);
    return [path];
  });
}
assert.deepEqual(walk(root).sort(), [...expected].sort(), 'Every delivered file is covered; no missing or extra payload');
const payload = inventory.files.filter((file) => file.path !== inventory.sbom)
  .map(({ path, bytes, mode, sha256 }) => ({ path, bytes, mode, sha256 }));
assert.equal(hash(JSON.stringify(payload)), inventory.payloadDigest);
const bom = JSON.parse(read(inventory.sbom));
assert.equal(bom.bomFormat, 'CycloneDX');
assert.equal(bom.specVersion, '1.6');
const properties = Object.fromEntries(bom.metadata.properties.map(({ name, value }) => [name, value]));
assert.equal(properties['configra:payload-files-sha256'], inventory.payloadDigest);
assert.equal(properties['configra:inventory'], inventoryPath);
const components = new Map(bom.components.map((component) => [component['bom-ref'], component]));
assert.equal(components.size, bom.components.length, 'No duplicate component identities');
const rootRef = bom.metadata.component['bom-ref'];
assert.ok(!components.has(rootRef));
const edges = new Map(bom.dependencies.map((edge) => [edge.ref, new Set(edge.dependsOn ?? [])]));
assert.equal(edges.size, bom.dependencies.length, 'No duplicate dependency source identities');
for (const edge of bom.dependencies) {
  assert.ok(edge.ref === rootRef || components.has(edge.ref));
  for (const ref of edge.dependsOn ?? []) assert.ok(components.has(ref), `Missing dependency component: ${ref}`);
}
for (const evidence of JSON.parse(properties['configra:evidence-parts'])) {
  const bytes = read(evidence.path);
  assert.equal(hash(bytes), evidence.sha256, `Changed component evidence: ${evidence.path}`);
  const part = JSON.parse(bytes);
  for (const edge of part.dependencies ?? []) {
    for (const target of edge.dependsOn ?? []) {
      assert.ok(edges.get(edge.ref)?.has(target), `Lost original dependency relationship: ${edge.ref} -> ${target}`);
    }
  }
  for (const component of [part.metadata.component, ...(part.components ?? [])]) {
    const aggregate = components.get(component['bom-ref']);
    assert.ok(aggregate, `Lost source component: ${component.name}`);
    for (const key of ['type', 'name', 'version', 'purl', 'hashes', 'licenses', 'evidence']) {
      assert.deepEqual(aggregate[key], component[key], `Changed component identity/evidence: ${component.name}/${key}`);
    }
    const actualProperties = Object.fromEntries((aggregate.properties ?? []).map(({ name, value }) => [name, value]));
    const originalProperties = Object.fromEntries((component.properties ?? []).map(({ name, value }) => [name, value]));
    for (const key of ['configra:go:module-sum', 'configra:npm:integrity', 'configra:license-scope']) {
      assert.equal(actualProperties[key], originalProperties[key], `Changed source identity: ${component.name}/${key}`);
    }
  }
}
console.log(`PASS ${inventory.kind} composition: ${inventory.files.length} files and ${components.size} components.`);
