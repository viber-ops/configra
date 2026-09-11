#!/usr/bin/env node
// The same cross-build path is used locally and in GitHub Actions.
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import {
  cpSync,
  existsSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
} from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const version = process.argv[2];
if (!/^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version ?? '')) {
  throw new Error('Usage: node scripts/build-release.mjs v0.1.0-rc.1');
}
const run = (command, args, options = {}) =>
  execFileSync(command, args, { cwd: root, stdio: 'inherit', ...options });
const read = (command, args, options = {}) =>
  execFileSync(command, args, {
    cwd: root,
    encoding: 'utf8',
    ...options,
  }).trim();
const commit = read('git', ['rev-parse', 'HEAD']);
if (read('git', ['status', '--porcelain', '--untracked-files=normal'])) {
  throw new Error(
    'Release builds require a clean worktree; commit or isolate your work first.',
  );
}
const output = join(root, 'dist', version);
if (existsSync(output))
  throw new Error(
    `Refusing to overwrite ${output}; choose a new version or move that artifact directory.`,
  );
const env = {
  ...process.env,
  GOTOOLCHAIN: process.env.GOTOOLCHAIN || 'go1.26.7',
  GOWORK: 'off',
  GOFLAGS: '-mod=readonly',
  CGO_ENABLED: '0',
};
run(process.execPath, ['--test', join(root, 'scripts/go-notices/collect.test.mjs')], { env });
run('npm', ['--prefix', 'web', 'ci', '--ignore-scripts']);
run('npm', ['--prefix', 'web', 'run', 'test:licenses']);
run('npm', ['--prefix', 'web', 'run', 'build']);
mkdirSync(output, { recursive: true });
const [hostOS, hostArch] = read('go', ['env', 'GOHOSTOS', 'GOHOSTARCH'], { env }).split(/\s+/);
const licenseTool = join(output, '.build-tools', 'configra-go-licenses');
run('go', ['build', '-trimpath', '-buildvcs=false', '-o', licenseTool, './scripts/go-licenses'], {
  env: { ...env, GOOS: hostOS, GOARCH: hostArch },
});
const hashes = [];
for (const platform of ['darwin', 'linux']) {
  for (const arch of ['amd64', 'arm64']) {
    const name = `configra_${version.slice(1)}_${platform}_${arch}`;
    const stage = join(output, name);
    mkdirSync(stage);
    const buildEnv = { ...env, GOOS: platform, GOARCH: arch };
    const flags = [
      '-trimpath',
      '-buildvcs=false',
      '-ldflags',
      `-s -w -X main.version=${version} -X main.commit=${commit}`,
    ];
    run(
      'go',
      ['build', ...flags, '-o', join(stage, 'configra'), './cmd/configra'],
      { env: buildEnv },
    );
    run(
      'go',
      [
        '-C',
        'kubernetes',
        'build',
        ...flags,
        '-o',
        join(stage, 'configra-kubernetes'),
        './cmd/configra-kubernetes',
      ],
      { env: buildEnv },
    );
    cpSync(join(root, 'deploy'), join(stage, 'deploy'), {
      recursive: true,
      filter: (path) =>
        !path.endsWith('management.local.yaml') &&
        !path.endsWith('compose.test.yaml') &&
        !path.endsWith('compose.local.yaml') &&
        !path.includes('/casdoor/'),
    });
    cpSync(
      join(root, 'kubernetes', 'deploy'),
      join(stage, 'kubernetes', 'deploy'),
      { recursive: true },
    );
    cpSync(
      join(root, 'kubernetes', 'examples'),
      join(stage, 'kubernetes', 'examples'),
      { recursive: true },
    );
    cpSync(
      join(root, 'kubernetes', 'README.md'),
      join(stage, 'kubernetes', 'README.md'),
    );
    cpSync(
      join(root, 'docs', 'release-installation.md'),
      join(stage, 'README.md'),
    );
    for (const file of ['LICENSE', 'NOTICE']) {
      cpSync(join(root, file), join(stage, file));
      cpSync(join(root, 'kubernetes', file), join(stage, 'kubernetes', file));
    }
    run('sh', [join(root, 'scripts/go-notices/collect.sh'), join(stage, 'licenses/go1.26.7')], { env: buildEnv });
    run(process.execPath, [join(root, 'web/scripts/export-licenses.mjs'), join(root, 'web/dist'), join(stage, 'licenses/ui')]);
    for (const binary of ['configra', 'configra-kubernetes']) {
      run(licenseTool, ['-binary', join(stage, binary), '-out', join(stage, 'licenses', `${binary}-modules`)], {
        cwd: binary === 'configra' ? root : join(root, 'kubernetes'), env,
      });
    }
    writeFileSync(
      join(stage, 'BUILD.json'),
      JSON.stringify(
        {
          version,
          commit,
          platform,
          arch,
          go: read('go', ['version'], { env }),
          cgo: false,
          signed: false,
          notarized: false,
        },
        null,
        2,
      ) + '\n',
    );
    const archive = `${name}.tar.gz`;
    run('tar', ['-czf', join(output, archive), '-C', output, name], {
      env: { ...process.env, COPYFILE_DISABLE: '1' },
    });
    hashes.push(
      `${createHash('sha256')
        .update(readFileSync(join(output, archive)))
        .digest('hex')}  ${archive}`,
    );
  }
}
writeFileSync(join(output, 'SHA256SUMS'), hashes.join('\n') + '\n');
run(process.execPath, [join(root, 'scripts', 'verify-release.mjs'), output]);
console.log(`Release artifacts: ${output}`);
