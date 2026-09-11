import { createHash } from 'node:crypto';
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';

const digest = (value) => createHash('sha256').update(value).digest('hex');
const json = (path) => JSON.parse(readFileSync(path, 'utf8'));
const legalName = /(?:^|[-_.])(licen[sc]es?|notices?|copying|copyright|authors|patents)(?:$|[-_.])/i;
const virtualOwners = new Map([
  ['\0rolldown/runtime.js', 'rolldown'],
  ['\0vite/modulepreload-polyfill.js', 'vite'],
  ['\0vite/preload-helper.js', 'vite'],
]);

function legalFiles(directory, base = directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory() && entry.name !== 'node_modules') return legalFiles(path, base);
    return entry.isFile() && legalName.test(entry.name) ? [relative(base, path)] : [];
  }).sort();
}

export default function licenseMaterials() {
  let root;
  return {
    name: 'configra-license-materials',
    apply: 'build',
    enforce: 'post',
    configResolved(config) {
      root = resolve(config.root);
      if (config.base !== '/ui/') throw new Error('Configra serves its UI under /ui/; retain that build base');
    },
    generateBundle: {
      order: 'post',
      handler(_options, bundle) {
        const review = json(join(root, 'scripts/reviewed-licenses.json'));
        const lock = json(join(root, 'package-lock.json'));
        const own = json(join(root, 'package.json'));
        const selected = new Map();
        for (const output of Object.values(bundle)) {
          if (output.type !== 'chunk') continue;
          for (const id of Object.keys(output.modules)) {
            let directory;
            let source;
            if (virtualOwners.has(id)) {
              directory = join(root, 'node_modules', virtualOwners.get(id));
              source = `virtual:${id.slice(1)}`;
            } else if (id.includes('/node_modules/') && id.startsWith(`${root}/`)) {
              const file = id.split('?')[0];
              directory = dirname(file);
              while (directory !== root && !existsSync(join(directory, 'package.json'))) directory = dirname(directory);
              if (directory === root) throw new Error('A bundled dependency has no package metadata');
              source = relative(directory, file);
            } else if (id.startsWith(`${root}/src/`) || id.split('?')[0] === join(root, 'index.html')) {
              continue;
            } else {
              throw new Error('Unattributed build input; review the new module or generated helper before release');
            }
            if (!selected.has(directory)) selected.set(directory, new Set());
            selected.get(directory).add(source);
          }
        }

        const collected = new Map();
        for (const [directory, sources] of selected) {
          const installed = json(join(directory, 'package.json'));
          const name = `${installed.name}@${installed.version}`;
          const approved = review.packages.find((entry) => entry.name === installed.name && entry.version === installed.version);
          const locked = lock.packages[relative(root, directory)];
          if (!approved || !locked || installed.version !== locked.version ||
              installed.license !== approved.license || locked.resolved !== approved.archive ||
              locked.integrity !== approved.integrity) {
            throw new Error(`License review required for ${name}: package version, license or source archive changed`);
          }
          if (JSON.stringify(legalFiles(directory)) !== JSON.stringify(approved.files.map((entry) => entry.path).sort())) {
            throw new Error(`License review required for ${name}: license/notice file set changed`);
          }
          const files = approved.files.map((entry) => {
            const bytes = readFileSync(join(directory, entry.path));
            if (digest(bytes) !== entry.sha256) throw new Error(`License review required for ${name}: ${entry.path} changed`);
            return { ...entry, text: bytes.toString('utf8') };
          });
          const existing = collected.get(name);
          collected.set(name, { ...approved, sources: [...new Set([...(existing?.sources ?? []), ...sources])].sort(), files });
        }
        const records = [...collected.values()].sort((a, b) => `${a.name}@${a.version}` < `${b.name}@${b.version}` ? -1 : 1);
        if (!records.length) throw new Error('The UI build produced no attributable runtime dependency inventory');

        const notices = [
          'Configra Management UI — third-party notices\n',
          'Packages below have modules assigned to emitted browser chunks, including lazy-loaded chunks.\n',
          'Vite/Rolldown entries cover emitted helpers. Their upstream license appendices are retained as an attribution superset; they do not assert that the full build tool or all of its dependencies run in the browser.\n',
          ...records.flatMap((entry) => [
            `\n===== ${entry.name}@${entry.version} =====\nSource archive: ${entry.archive}\nIntegrity: ${entry.integrity}\n`,
            ...entry.files.map((file) => `\n--- ${file.path} (SHA-256 ${file.sha256}) ---\n${file.text}`),
          ]),
        ].join('');
        const noticeFile = `assets/third-party-notices-${digest(notices).slice(0, 16)}.txt`;
        const components = records.map((entry) => {
          const purl = `pkg:npm/${entry.name.split('/').map(encodeURIComponent).join('/')}@${encodeURIComponent(entry.version)}`;
          return {
            type: 'library', 'bom-ref': purl, name: entry.name, version: entry.version, purl,
            licenses: [{ license: { id: entry.license } }],
            externalReferences: [{ type: 'distribution', url: entry.archive }],
            properties: [
              { name: 'configra:npm:integrity', value: entry.integrity },
              { name: 'configra:bundled-modules', value: JSON.stringify(entry.sources) },
              { name: 'configra:license-files', value: JSON.stringify(entry.files.map(({ path, sha256 }) => ({ path, sha256 }))) },
            ],
          };
        });
        const artifactHashes = Object.values(bundle).filter((entry) => entry.type === 'chunk' || entry.fileName.endsWith('.css'))
          .map((entry) => ({ path: entry.fileName, sha256: digest(entry.type === 'chunk' ? entry.code : entry.source) }))
          .sort((a, b) => a.path < b.path ? -1 : 1);
        const bom = {
          $schema: 'http://cyclonedx.org/schema/bom-1.6.schema.json', bomFormat: 'CycloneDX', specVersion: '1.6', version: 1,
          metadata: {
            component: { type: 'application', 'bom-ref': 'configra-management-ui', name: own.name, version: own.version,
              description: 'Browser bundle inputs only; excludes server Go code, container roots and unbundled build dependencies.' },
            properties: [
              { name: 'configra:bundle-files', value: JSON.stringify(artifactHashes) },
              { name: 'configra:notices-sha256', value: digest(notices) },
            ],
          },
          components,
          dependencies: [{ ref: 'configra-management-ui', dependsOn: components.map((entry) => entry['bom-ref']) }],
        };
        const bomText = JSON.stringify(bom, null, 2) + '\n';
        const bomFile = `assets/ui-sbom-${digest(bomText).slice(0, 16)}.cdx.json`;
        this.emitFile({ type: 'asset', fileName: noticeFile, source: notices });
        this.emitFile({ type: 'asset', fileName: bomFile, source: bomText });
        const index = bundle['index.html'];
        if (!index || index.type !== 'asset' || !String(index.source).includes('</head>')) {
          throw new Error('Cannot attach license materials to the final UI document');
        }
        index.source = String(index.source).replace('</head>',
          `<link rel="license" href="/ui/${noticeFile}" />\n<link rel="describedby" type="application/vnd.cyclonedx+json" href="/ui/${bomFile}" />\n</head>`);
      },
    },
  };
}
