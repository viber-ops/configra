#!/usr/bin/env node
// Compose verified component evidence and a file inventory for the delivered root.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { existsSync, lstatSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { join, posix, resolve } from 'node:path';

const [kind, directory, imageName] = process.argv.slice(2);
assert.ok(kind === 'bundle' || kind === 'image', 'Usage: compose-distribution.mjs bundle|image ROOT [IMAGE_COMMAND]');
assert.ok(directory, 'A delivered root directory is required');
assert.ok(kind !== 'image' || ['configra', 'configra-kubernetes'].includes(imageName), 'Name the image command');
const root = resolve(directory);
const licenseRoot = kind === 'bundle' ? 'licenses' : `licenses/${imageName}`;
const sbomPath = kind === 'bundle' ? 'SBOM.cdx.json' : `${licenseRoot}/SBOM.cdx.json`;
const inventoryPath = kind === 'bundle' ? 'INVENTORY.json' : `${licenseRoot}/INVENTORY.json`;
for (const file of [sbomPath, inventoryPath]) assert.ok(!existsSync(join(root, file)), 'Refusing to overwrite existing distribution metadata');
const hash = (bytes) => createHash('sha256').update(bytes).digest('hex');
const safePath = (path) => typeof path === 'string' && path && !path.includes('\\') && !posix.isAbsolute(path) &&
  posix.normalize(path) === path && path !== '..' && !path.startsWith('../');
const read = (path) => {
  assert.ok(safePath(path), 'Metadata must reference a file inside the delivered root');
  return readFileSync(join(root, path));
};
const json = (path) => JSON.parse(read(path));
const properties = (value) => Object.fromEntries((value ?? []).map(({ name, value }) => [name, value]));
const components = new Map();
const dependencies = new Map();
const evidenceParts = [];
const rootDependencies = new Set();

function addComponent(input, origin) {
  const entry = structuredClone(input);
  const ref = entry['bom-ref'];
  assert.ok(ref && entry.name, 'Every component needs an identity');
  // Paths in these properties are relative to the original evidence document.
  // Retain that document unchanged and refer to it instead of rebasing ambiguously.
  entry.properties = (entry.properties ?? []).filter((item) => !['configra:license-files', 'configra:bundle-files'].includes(item.name));
  const existing = components.get(ref);
  if (existing) {
    for (const key of ['type', 'name', 'version', 'purl', 'hashes', 'licenses', 'evidence']) {
      assert.equal(JSON.stringify(existing[key]), JSON.stringify(entry[key]), `Conflicting component evidence: ${ref}`);
    }
    for (const key of ['configra:go:module-sum', 'configra:npm:integrity', 'configra:license-scope']) {
      assert.equal(properties(existing.properties)[key], properties(entry.properties)[key],
        `Conflicting component source identity: ${ref}/${key}`);
    }
    const old = properties(existing.properties)['configra:evidence-documents'];
    const origins = new Set(old ? JSON.parse(old) : []);
    if (origin) origins.add(origin);
    existing.properties = existing.properties.filter((item) => item.name !== 'configra:evidence-documents');
    if (origins.size) existing.properties.push({ name: 'configra:evidence-documents', value: JSON.stringify([...origins].sort()) });
  } else {
    if (origin) entry.properties.push({ name: 'configra:evidence-documents', value: JSON.stringify([origin]) });
    components.set(ref, entry);
  }
  return ref;
}

function addEdges(ref, targets) {
  if (!dependencies.has(ref)) dependencies.set(ref, new Set());
  for (const target of targets) dependencies.get(ref).add(target);
}

function loadPart(path) {
  const bytes = read(path);
  const part = JSON.parse(bytes);
  assert.equal(part.bomFormat, 'CycloneDX');
  assert.equal(part.specVersion, '1.6');
  evidenceParts.push({ path, sha256: hash(bytes) });
  const own = structuredClone(part.metadata.component);
  own.properties = [...(own.properties ?? []), ...(part.metadata.properties ?? [])];
  const ref = addComponent(own, path);
  for (const component of part.components ?? []) addComponent(component, path);
  for (const edge of part.dependencies ?? []) addEdges(edge.ref, edge.dependsOn ?? []);
  return { part, ref };
}

const commands = kind === 'bundle' ? ['configra', 'configra-kubernetes'] : [imageName];
const commandRefs = new Map();
const commandBytes = new Map();
const platforms = new Set();
const goVersions = new Set();
for (const command of commands) {
  const directory = `${licenseRoot}/${command}-modules`;
  const { part, ref } = loadPart(`${directory}/sbom.cdx.json`);
  const executable = read(command);
  assert.equal(part.metadata.component.hashes.find((item) => item.alg === 'SHA-256')?.content, hash(executable), 'Module SBOM must describe the actual executable');
  const metadata = properties(part.metadata.properties);
  platforms.add(metadata['configra:target'].split('/').slice(1).join('/'));
  goVersions.add(metadata['configra:go:version']);
  for (const component of part.components) {
    for (const file of JSON.parse(properties(component.properties)['configra:license-files'])) {
      assert.equal(hash(read(`${directory}/${file.path}`)), file.sha256, `Notice does not match evidence: ${file.path}`);
    }
  }
  commandRefs.set(command, ref);
  commandBytes.set(command, executable);
  rootDependencies.add(ref);
}
assert.equal(platforms.size, 1, 'A delivered root must not mix binary targets');
assert.deepEqual([...goVersions], ['go1.26.7'], 'Runtime evidence requires review for a changed toolchain');
const runtimeRoot = `${licenseRoot}/go1.26.7`;
assert.equal(hash(read(`${runtimeRoot}/LICENSE`)), '911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad');
read(`${runtimeRoot}/SOURCE.md`);
const runtimeRef = addComponent({
  type: 'platform', 'bom-ref': 'pkg:golang/stdlib@go1.26.7', name: 'stdlib', version: 'go1.26.7', purl: 'pkg:golang/stdlib@go1.26.7',
  description: 'Go runtime and standard library compiled into these binaries. The notice collection retains file-specific terms and a conservative source superset.',
  properties: [{ name: 'configra:notice-directory', value: runtimeRoot }],
}, `${runtimeRoot}/SOURCE.md`);
for (const ref of commandRefs.values()) addEdges(ref, [runtimeRef]);

if (commandRefs.has('configra')) {
  const uiRoot = `${licenseRoot}/ui`;
  const { part, ref } = loadPart(`${uiRoot}/sbom.cdx.json`);
  const notices = read(`${uiRoot}/THIRD_PARTY_NOTICES.txt`);
  assert.equal(properties(part.metadata.properties)['configra:notices-sha256'], hash(notices));
  assert.ok(commandBytes.get('configra').includes(notices), 'UI notice copy must match the embedded distribution');
  assert.ok(commandBytes.get('configra').includes(read(`${uiRoot}/sbom.cdx.json`)), 'UI SBOM must match the embedded distribution');
  addEdges(commandRefs.get('configra'), [ref]);
}

if (kind === 'image') {
  const caRoot = `${licenseRoot}/ca-certificates`;
  const ca = json(`${caRoot}/metadata.json`);
  assert.equal(ca.formatVersion, 1);
  assert.equal(ca.bundle.path, '/etc/ssl/certs/ca-certificates.crt');
  assert.equal(hash(read(ca.bundle.path.slice(1))), ca.bundle.sha256, 'CA metadata must describe the runtime trust file');
  for (const file of ca.files) assert.equal(hash(read(`${caRoot}/${file.path}`)), file.sha256, `CA evidence mismatch: ${file.path}`);
  const caRef = addComponent({
    type: 'data', 'bom-ref': `pkg:deb/debian/ca-certificates@${ca.version}?arch=all`, name: ca.package, version: ca.version,
    purl: `pkg:deb/debian/ca-certificates@${ca.version}?arch=all`,
    hashes: [{ alg: 'SHA-256', content: ca.bundle.sha256 }],
    licenses: [{ license: { id: ca.dataLicense } }],
    description: 'Reviewed Mozilla-derived certificate data copied into the runtime trust store; not the Debian update/conversion programs.',
  }, `${caRoot}/metadata.json`);
  const sourceRef = addComponent({
    type: 'file', 'bom-ref': `urn:sha256:${ca.source.sha256}`, name: 'ca-certificates-source', version: ca.version,
    hashes: [{ alg: 'SHA-256', content: ca.source.sha256 }],
    licenses: [{ expression: `${ca.dataLicense} AND ${ca.sourceScriptsLicense}` }],
    description: 'Unmodified source archive retained as license material. GPL terms apply to its packaging/conversion scripts, which are not installed as runtime programs.',
    externalReferences: [{ type: 'distribution', url: ca.source.url }],
  }, `${caRoot}/metadata.json`);
  addEdges(commandRefs.get(imageName), [caRef]);
  rootDependencies.add(sourceRef);
}

function walk(directory = root, relative = '') {
  return readdirSync(directory).sort().flatMap((name) => {
    const full = join(directory, name);
    const path = relative ? `${relative}/${name}` : name;
    const stat = lstatSync(full);
    assert.ok(!stat.isSymbolicLink(), 'Distribution roots must not contain symlinks');
    if (stat.isDirectory()) return walk(full, path);
    assert.ok(stat.isFile(), 'Distribution roots contain only regular files and directories');
    return [{ path, bytes: stat.size, mode: (stat.mode & 0o777).toString(8).padStart(4, '0'), sha256: hash(readFileSync(full)) }];
  });
}
const payload = walk().sort((a, b) => a.path < b.path ? -1 : 1);
const payloadDigest = hash(JSON.stringify(payload));
const rootRef = `urn:configra:payload:${payloadDigest}`;
const build = kind === 'bundle' ? json('BUILD.json') : undefined;
if (build) assert.equal(`${build.platform}/${build.arch}`, [...platforms][0]);
const rootComponent = {
  type: kind === 'image' ? 'container' : 'application', 'bom-ref': rootRef,
  name: kind === 'image' ? imageName : 'configra-release-bundle',
  ...(build ? { version: build.version } : {}),
  description: 'Distributed payload composition. The payload-file digest is not an OCI manifest digest. Host/cluster-provided services and runtime-generated container files are outside this inventory.',
};
addEdges(rootRef, rootDependencies);
for (const [ref, targets] of dependencies) {
  assert.ok(ref === rootRef || components.has(ref), `Unknown dependency source: ${ref}`);
  for (const target of targets) assert.ok(components.has(target), `Unknown dependency target: ${target}`);
}
const bom = {
  $schema: 'http://cyclonedx.org/schema/bom-1.6.schema.json', bomFormat: 'CycloneDX', specVersion: '1.6', version: 1,
  metadata: { component: rootComponent, properties: [
    { name: 'configra:payload-files-sha256', value: payloadDigest },
    { name: 'configra:inventory', value: inventoryPath },
    { name: 'configra:evidence-parts', value: JSON.stringify(evidenceParts) },
    { name: 'configra:target', value: [...platforms][0] },
    ...(build ? [{ name: 'configra:source-commit', value: build.commit }] : []),
  ] },
  components: [...components.values()].sort((a, b) => a['bom-ref'] < b['bom-ref'] ? -1 : 1),
  dependencies: [...dependencies].sort(([a], [b]) => a < b ? -1 : 1).map(([ref, targets]) => ({ ref, dependsOn: [...targets].sort() })),
};
const sbom = JSON.stringify(bom, null, 2) + '\n';
writeFileSync(join(root, sbomPath), sbom, { mode: 0o644 });
const sbomMode = (lstatSync(join(root, sbomPath)).mode & 0o777).toString(8).padStart(4, '0');
const inventory = { formatVersion: 1, kind, target: [...platforms][0], payloadDigest, sbom: sbomPath,
  excludedSelf: inventoryPath, files: [...payload, { path: sbomPath, bytes: Buffer.byteLength(sbom), mode: sbomMode, sha256: hash(sbom) }].sort((a, b) => a.path < b.path ? -1 : 1) };
writeFileSync(join(root, inventoryPath), JSON.stringify(inventory, null, 2) + '\n', { mode: 0o644 });
console.log(`Composed ${kind} inventory: ${bom.components.length} components, ${inventory.files.length} files.`);
