// Disposable real-service Kubernetes acceptance, invoked by service-smoke.mjs.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createPrivateKey } from 'node:crypto';
import { appendFileSync, chmodSync, readFileSync, unlinkSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';

const run = (command, args, options = {}) => execFileSync(command, args, {
  encoding: 'utf8', stdio: 'pipe', timeout: 180000, maxBuffer: 16 << 20, ...options,
});
const docker = (...args) => run('docker', args);
const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
async function until(label, check, timeout = 150000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    if (await check()) return;
    await delay(1000);
  }
  throw new Error(`Timed out: ${label}`);
}
function kubectl(kubeconfig) {
  assert.ok(kubeconfig, 'An owned service acceptance kubeconfig is required');
  const command = (...args) => run('kubectl', ['--kubeconfig', kubeconfig, ...args]);
  const context = command('config', 'current-context').trim();
  assert.match(context, /^kind-configra-review-service-\d+$/);
  const [node] = JSON.parse(docker('inspect', `${context.slice(5)}-control-plane`));
  assert.equal(node.Config.Labels['io.x-k8s.kind.cluster'], context.slice(5));
  const apply = (...items) => run('kubectl', ['--kubeconfig', kubeconfig, 'apply', '--server-side', '-f', '-'], {
    input: JSON.stringify({ apiVersion: 'v1', kind: 'List', items }),
  });
  const render = (path) => command('create', '--dry-run=client', '-k', path, '-o', 'jsonpath={@}{"\\n"}').trim().split('\n').map((line) => JSON.parse(line));
  return { command, apply, render };
}
function captureLogs(command, directory) {
  for (const namespace of ['configra', 'configra-app', 'configra-provider', 'kube-system']) {
    const pods = JSON.parse(command('-n', namespace, 'get', 'pods', '-o', 'json')).items;
    for (const pod of pods.filter((pod) => namespace !== 'kube-system' || pod.metadata.name.startsWith('csi-secrets-store-'))) {
      if (!pod.status.containerStatuses?.some((status) => status.state.running || status.state.terminated)) continue;
      appendFileSync(join(directory, 'kubernetes-application.log'), command('-n', namespace, 'logs', pod.metadata.name, '--all-containers=true', '--prefix=true'), { mode: 0o600 });
      // A recovered leader exits after losing its lease; keep that error path too.
      for (const status of pod.status.containerStatuses.filter((status) => status.restartCount > 0)) {
        appendFileSync(join(directory, 'kubernetes-application.log'), command('-n', namespace, 'logs', pod.metadata.name, '-c', status.name, '--previous=true', '--prefix=true'), { mode: 0o600 });
      }
    }
  }
}

export async function withKubernetesServices({ root, directory, project, image, kubernetesImage, fixture, ready }, check) {
  const cluster = `configra-review-service-${Date.now()}`;
  const kubeconfig = join(directory, 'kubeconfig');
  const kind = process.env.CONFIGRA_TEST_KIND ?? 'kind';
  assert.ok(!run(kind, ['get', 'clusters']).split('\n').includes(cluster));
  const config = join(directory, 'kind.json');
  writeFileSync(config, JSON.stringify({ kind: 'Cluster', apiVersion: 'kind.x-k8s.io/v1alpha4', nodes: [{ role: 'control-plane',
    extraPortMappings: [
      { containerPort: 30443, hostPort: 18088, listenAddress: '127.0.0.1' },
      { containerPort: 30943, hostPort: 18089, listenAddress: '127.0.0.1' },
    ],
  }, { role: 'worker' }, { role: 'worker' }] }), { mode: 0o600 });
  console.log(`Starting disposable Kubernetes cluster ${cluster}`);
  let kube;
  let passed = false;
  const driverImages = [];
  try {
    run(kind, ['create', 'cluster', '--name', cluster, '--kubeconfig', kubeconfig, '--config', config, '--image', 'kindest/node:v1.35.0', '--wait', '120s']);
    chmodSync(kubeconfig, 0o600);
    kube = kubectl(kubeconfig);
    const { command, apply, render } = kube;
    const nodes = run(kind, ['get', 'nodes', '--name', cluster]).trim().split('\n');
    assert.equal(nodes.length, 3);
    for (const node of nodes) {
      const [info] = JSON.parse(docker('inspect', node));
      assert.equal(info.Config.Labels['io.x-k8s.kind.cluster'], cluster);
      docker('network', 'connect', fixture.networks.default.name, node);
    }
    // kind's control-plane readiness does not imply both workers are ready.
    command('wait', 'node', '--all', '--for=condition=Ready', '--timeout=120s');
    const loadImage = (tag) => {
      const [info] = JSON.parse(docker('image', 'inspect', tag));
      const platform = `${info.Os}/${info.Architecture}`;
      const archive = join(directory, 'image.tar');
      docker('image', 'save', '--platform', platform, '--output', archive, tag);
      // Explicit platform import also works with Docker's multi-platform cache.
      for (const node of nodes) run('docker', ['exec', '-i', node, 'ctr', '--namespace', 'k8s.io', 'images', 'import', '--platform', platform, '-'], { input: readFileSync(archive) });
      unlinkSync(archive);
      return info.Id;
    };
    for (const tag of [image, kubernetesImage, 'busybox:1.37.0']) loadImage(tag);
    apply({ apiVersion: 'v1', kind: 'Namespace', metadata: { name: 'configra' } });
    const secret = (name, stringData) => ({ apiVersion: 'v1', kind: 'Secret', metadata: { name, namespace: 'configra' }, stringData });
    const env = fixture.services.management.environment;
    const cert = readFileSync(join(directory, 'tls.crt'), 'utf8');
    const key = readFileSync(join(directory, 'tls.key'), 'utf8');
    apply(
      secret('configra-runtime', { 'mysql-dsn': env.CONFIGRA_MYSQL_DSN, 'clickhouse-dsn': env.CONFIGRA_CLICKHOUSE_DSN, 'oidc-client-secret': env.CONFIGRA_OIDC_CLIENT_SECRET }),
      secret('configra-management-tls', { 'tls.crt': cert, 'tls.key': key }),
      secret('configra-api-tls', { 'tls.crt': cert, 'tls.key': key }),
      secret('configra-master-key', { 'master-key': readFileSync(join(directory, 'master-key'), 'utf8') }),
      secret('configra-nats', {}), // This isolated fixture uses unauthenticated NATS.
    );
    for (const [name, port] of [['mysql', 3306], ['nats', 4222], ['clickhouse', 9000], ['casdoor', 18080]]) {
      const [container] = JSON.parse(docker('inspect', `${project}-${name}-1`));
      assert.equal(container.Config.Labels['com.docker.compose.project'], project);
      const address = container.NetworkSettings.Networks[fixture.networks.default.name].IPAddress;
      apply(
        { apiVersion: 'v1', kind: 'Service', metadata: { name, namespace: 'configra' }, spec: { ports: [{ name: 'tcp', port, targetPort: port }] } },
        { apiVersion: 'discovery.k8s.io/v1', kind: 'EndpointSlice', metadata: { name: `${name}-external`, namespace: 'configra', labels: { 'kubernetes.io/service-name': name, 'endpointslice.kubernetes.io/managed-by': 'configra-acceptance' } }, addressType: 'IPv4', ports: [{ name: 'tcp', protocol: 'TCP', port }], endpoints: [{ addresses: [address], conditions: { ready: true } }] },
      );
    }
    for (const object of render(join(root, 'deploy/kubernetes/base'))) {
      object.metadata.namespace = 'configra';
      const mode = object.metadata.name.includes('management') ? 'management' : 'api';
      if (object.kind === 'ConfigMap') {
        const config = JSON.parse(readFileSync(join(directory, `${mode}.yaml`), 'utf8'));
        config.listen = mode === 'management' ? ':8443' : ':9443';
        config.tls = { certificate_file: '/run/configra/server-tls/tls.crt', private_key_file: '/run/configra/server-tls/tls.key' };
        config.key_provider.master_key_file = '/run/configra/master-key/master-key';
        object.data['config.yaml'] = JSON.stringify(config);
      } else if (object.kind === 'Deployment') {
        object.spec.template.spec.containers[0].image = image;
        if (mode === 'management') {
          object.spec.replicas = 2;
          object.spec.template.spec.containers[0].env.push({ name: 'SSL_CERT_FILE', value: '/run/configra/server-tls/tls.crt' });
        }
        else object.spec.replicas = 0; // Management initializes/verifies schema first.
      } else if (object.kind === 'Service') {
        object.spec.type = 'NodePort';
        object.spec.ports[0].nodePort = mode === 'management' ? 30443 : 30943;
      }
      apply(object);
    }
    for (const mode of ['management', 'api']) {
      if (mode === 'api') command('-n', 'configra', 'scale', 'deployment/configra-api', '--replicas=2');
      command('-n', 'configra', 'rollout', 'status', `deployment/configra-${mode}`, '--timeout=150s');
      await ready(mode === 'management' ? 18088 : 18089);
    }
    const services = JSON.parse(command('-n', 'configra', 'get', 'pods', '-o', 'json')).items;
    assert.equal(services.length, 4);
    for (const mode of ['management', 'api']) {
      const placed = services.filter((pod) => pod.metadata.labels['app.kubernetes.io/component'] === mode);
      assert.equal(new Set(placed.map((pod) => pod.spec.nodeName)).size, 2, `${mode} replicas must occupy both workers`);
    }
    for (const pod of services) {
      assert.equal(pod.spec.automountServiceAccountToken, false);
      assert.equal(pod.spec.securityContext.runAsUser, 65532);
      assert.equal(pod.spec.containers[0].securityContext.readOnlyRootFilesystem, true);
      assert.deepEqual(pod.spec.containers[0].securityContext.capabilities.drop, ['ALL']);
    }
    console.log('PASS hardened Configra Management/API Pods and verified HTTPS Service routes');

    // Fixed upstream driver source, not a copy maintained by Configra.
    const revision = '0e4ceeb734246e1de5267d8968023a3c8dcff39a'; // v1.6.1
    for (const file of ['secrets-store.csi.x-k8s.io_secretproviderclasses.yaml', 'secrets-store.csi.x-k8s.io_secretproviderclasspodstatuses.yaml', 'rbac-secretproviderclass.yaml', 'csidriver.yaml', 'secrets-store-csi-driver.yaml']) {
      let manifest = run('curl', ['--fail', '--silent', '--show-error', '--max-time', '30', `https://raw.githubusercontent.com/kubernetes-sigs/secrets-store-csi-driver/${revision}/deploy/${file}`]);
      manifest = manifest.replace('--enable-secret-rotation=false', '--enable-secret-rotation=true').replace('--rotation-poll-interval=2m', '--rotation-poll-interval=5s');
      if (file === 'secrets-store-csi-driver.yaml') {
        const driver = JSON.parse(run('kubectl', ['--kubeconfig', kubeconfig, 'create', '--dry-run=client', '--validate=false', '-f', '-', '-o', 'json'], { input: manifest }));
        assert.equal(driver.kind, 'DaemonSet');
        // Fetch the upstream manifest's images once, before starting its Pods.
        for (const container of driver.spec.template.spec.containers) {
          docker('pull', container.image);
          driverImages.push({ reference: container.image, id: loadImage(container.image) });
        }
      }
      run('kubectl', ['--kubeconfig', kubeconfig, 'apply', '--server-side', '-f', '-'], { input: manifest });
    }
    command('patch', 'csidriver', 'secrets-store.csi.k8s.io', '--type=merge', '-p', JSON.stringify({ spec: { fsGroupPolicy: 'File' } }));
    // Driver provisioning is separate from Configra's recovery assertions.
    run('kubectl', ['--kubeconfig', kubeconfig, '-n', 'kube-system', 'rollout', 'status', 'daemonset/csi-secrets-store', '--timeout=300s'], { timeout: 320000 });
    command('apply', '-f', join(root, 'kubernetes/deploy/crd.yaml'));
    for (const [path, namespace] of [['sync', 'configra-app'], ['provider', 'configra-provider']]) {
      apply({ apiVersion: 'v1', kind: 'Namespace', metadata: { name: namespace } });
      apply({ apiVersion: 'v1', kind: 'Secret', metadata: { name: 'configra-server-trust', namespace }, stringData: { 'ca.crt': cert } });
      for (const object of render(join(root, 'kubernetes/deploy', path))) {
        if (object.spec?.template) object.spec.template.spec.containers[0].image = kubernetesImage;
        apply(object);
      }
      command('-n', namespace, 'rollout', 'status', path === 'sync' ? 'deployment/configra-sync' : 'daemonset/configra-provider', '--timeout=150s');
    }
    const controllers = JSON.parse(command('-n', 'configra-app', 'get', 'pods', '-l', 'app.kubernetes.io/name=configra-sync', '-o', 'json')).items;
    assert.equal(new Set(controllers.map((pod) => pod.spec.nodeName)).size, 2, 'sync replicas must occupy both workers');
    assert.throws(() => command('auth', 'can-i', 'get', 'secrets', '-n', 'configra', '--as=system:serviceaccount:configra-app:configra-sync'),
      (error) => error.status === 1 && error.stdout.trim() === 'no', 'sync cannot read bootstrap Secrets');
    assert.throws(() => command('auth', 'can-i', 'get', 'secrets', '-n', 'configra', '--as=system:serviceaccount:kube-system:secrets-store-csi-driver'),
      (error) => error.status === 1 && error.stdout.trim() === 'no', 'file-only CSI needs no Kubernetes Secret-reading grant');
    await check(kubeconfig);
    passed = true;
  } finally {
    try {
      if (kube) {
        writeFileSync(join(directory, 'kubernetes-pods.json'), kube.command('get', 'pods', '-A', '-o', 'json'), { mode: 0o600 });
        writeFileSync(join(directory, 'kubernetes-events.json'), kube.command('get', 'events', '-A', '-o', 'json'), { mode: 0o600 });
        writeFileSync(join(directory, 'kubernetes-status.json'), kube.command('get', 'configrabindings', '-A', '-o', 'json'), { mode: 0o600 });
        captureLogs(kube.command, directory);
      }
    } catch (error) {
      writeFileSync(join(directory, 'kubernetes-capture-failure.txt'), String(error.stack ?? error), { mode: 0o600 });
      const wasPassed = passed;
      passed = false;
      if (wasPassed) throw error;
    } finally {
      run(kind, ['delete', 'cluster', '--name', cluster, '--kubeconfig', kubeconfig]);
      writeFileSync(join(directory, 'kubernetes-result.json'), JSON.stringify({ cluster, passed, node: 'kindest/node:v1.35.0', nodes: 3,
        image: JSON.parse(docker('image', 'inspect', image))[0].Id,
        adapterImage: JSON.parse(docker('image', 'inspect', kubernetesImage))[0].Id,
        csiDriver: 'v1.6.1', csiSource: '0e4ceeb734246e1de5267d8968023a3c8dcff39a', driverImages,
      }, null, 2), { mode: 0o600 });
    }
  }
}

export async function rotateKubernetesMasterKey({ kubeconfig, artifacts, backup }) {
  const { command, apply } = kubectl(kubeconfig);
  const directory = dirname(kubeconfig);
  const get = (kind, name) => JSON.parse(command('-n', 'configra', 'get', kind, ...(name ? [name] : []), '-o', 'json'));
  assert.equal(get('hpa').items.length, 0, 'no autoscaler may restart an offline service');
  const deployments = ['management', 'api'].map((mode) => get('deployment', `configra-${mode}`));
  captureLogs(command, directory);
  command('-n', 'configra', 'scale', ...deployments.map((item) => `deployment/${item.metadata.name}`), '--replicas=0');
  await until('every Configra service Pod has exited', () => get('pods').items.every((pod) => pod.metadata.labels?.['app.kubernetes.io/name'] !== 'configra'));
  await backup();

  const replacementKey = readFileSync(join(directory, 'new-master-key'), 'utf8');
  apply({ apiVersion: 'v1', kind: 'Secret', metadata: { name: 'configra-replacement-key', namespace: 'configra' }, stringData: { 'master-key': replacementKey } });
  const data = Object.fromEntries(['old', 'new'].map((name) => [`${name}.yaml`, JSON.stringify({
    version: 1, mysql: { dsn_env: 'CONFIGRA_MYSQL_DSN' }, key_provider: { master_key_file: `/run/configra/${name}/master-key` },
  })]));
  apply({ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'configra-maintenance', namespace: 'configra' }, data });
  const spec = structuredClone(deployments[0].spec.template.spec);
  spec.restartPolicy = 'Never';
  spec.activeDeadlineSeconds = 180;
  const container = spec.containers[0];
  for (const name of ['ports', 'startupProbe', 'readinessProbe', 'livenessProbe', 'lifecycle']) delete container[name];
  container.env = container.env.filter((entry) => entry.name === 'CONFIGRA_MYSQL_DSN');
  container.volumeMounts = [
    { name: 'config', mountPath: '/etc/configra', readOnly: true },
    { name: 'old', mountPath: '/run/configra/old', readOnly: true },
    { name: 'new', mountPath: '/run/configra/new', readOnly: true },
  ];
  spec.volumes = [
    { name: 'config', configMap: { name: 'configra-maintenance' } },
    { name: 'old', secret: { secretName: 'configra-master-key' } },
    { name: 'new', secret: { secretName: 'configra-replacement-key' } },
  ];
  let invocation = 0;
  const cli = async (args, expectedExit = 0) => {
    const name = `configra-maintenance-${++invocation}`;
    container.args = args;
    // No service labels: these short-lived CLI Pods must not become endpoints.
    apply({ apiVersion: 'v1', kind: 'Pod', metadata: { name, namespace: 'configra', labels: { 'app.kubernetes.io/name': 'configra-maintenance' } }, spec });
    await until(`maintenance Pod ${name}`, () => ['Succeeded', 'Failed'].includes(get('pod', name).status.phase), 200000);
    const output = command('-n', 'configra', 'logs', name);
    appendFileSync(join(directory, 'kubernetes-application.log'), output, { mode: 0o600 });
    assert.equal(get('pod', name).status.containerStatuses[0].state.terminated.exitCode, expectedExit, 'maintenance command exit status');
    return output;
  };
  const doctor = (key) => ['doctor', '--config', `/etc/configra/${key}.yaml`, '--verify-vault'];
  const expectedCounts = { vault_revisions: 1, certificate_authorities: 1, notification_destinations: 1 };
  assert.deepEqual(JSON.parse(await cli(doctor('old'))), expectedCounts);
  const { operation_id: operation, ...counts } = JSON.parse(await cli([
    'rotate-master-key', '--config', '/etc/configra/old.yaml', '--new-key-file', '/run/configra/new/master-key',
    '--confirm-database', 'configra_local', '--confirm-offline',
  ]));
  assert.match(operation, /^key-rotation-[0-9a-f]{32}$/);
  assert.deepEqual(counts, expectedCounts);
  writeFileSync(join(artifacts, 'rotation-operation'), operation, { mode: 0o600 });
  assert.match(await cli(doctor('old'), 1), /crypto Sentinel failed; check the Master Key and database backup/);
  assert.deepEqual(JSON.parse(await cli(doctor('new'))), expectedCounts);
  // Only a successful full scan authorizes replacing the live bootstrap Secret.
  apply({ apiVersion: 'v1', kind: 'Secret', metadata: { name: 'configra-master-key', namespace: 'configra' }, stringData: { 'master-key': replacementKey } });
  for (const deployment of deployments) {
    command('-n', 'configra', 'scale', `deployment/${deployment.metadata.name}`, `--replicas=${deployment.spec.replicas}`);
    command('-n', 'configra', 'rollout', 'status', `deployment/${deployment.metadata.name}`, '--timeout=150s');
  }
  console.log('PASS offline Kubernetes maintenance Pods, old-key rejection, new-key verification and bootstrap Secret replacement');
}

export async function verifyKubernetesConsumers({ root, kubeconfig, directory, api, read, afterRotation = false }) {
  const { command, apply } = kubectl(kubeconfig);
  const namespace = 'configra-app';
  const get = (kind, name, ns = namespace) => JSON.parse(command('-n', ns, 'get', kind, ...(name ? [name] : []), '-o', 'json'));
  const applyInNamespace = (object) => apply({ ...object, metadata: { ...object.metadata, namespace } });
  const consumer = 'configra-file-reader';
  const consumers = [consumer, `${consumer}-other-node`];
  const file = (path, pod = consumer) => run('kubectl', ['--kubeconfig', kubeconfig, '-n', namespace, 'exec', pod, '--', 'cat', `/etc/application/${path}`], { encoding: 'buffer' });
  const password = readFileSync(join(directory, 'expected-password'), 'utf8');
  const fileBytes = readFileSync(join(directory, 'expected-file'));
  const publish = async (revision) => {
    for (const key of ['kubernetes_file', 'kubernetes_env']) {
      await api(`/v1/environments/development/configs/${key}`, 'PUT', {
        name: 'Kubernetes acceptance', format: 'yaml', expected_revision: revision - 1,
        content: `VERSION: ${revision}\nPORT: ${8080 + revision}\n` + (key.endsWith('file') ? 'SECRET: "{vault.platform.database.password}"\n' : ''),
      });
    }
  };
  const delivered = (revision, readers = consumers) => {
    const native = get('secret', 'native-file');
    return Buffer.from(native.data['app.yaml'], 'base64').toString().includes(`VERSION: ${revision}\n`) &&
      get('configmap', 'native-env').data.PORT === String(8080 + revision) && readers.every((pod) => file('app.yaml', pod).includes(`VERSION: ${revision}\n`));
  };
  const verifyBytes = (readers = consumers) => {
    for (const pod of readers) {
      assert.ok(file('app.yaml', pod).includes(password));
      assert.ok(file('credentials.txt', pod).equals(fileBytes), 'CSI preserves binary File bytes');
    }
    assert.ok(Buffer.from(get('secret', 'native-file').data['app.yaml'], 'base64').includes(password));
    assert.ok(Buffer.from(get('secret', 'native-file').data['credentials.txt'], 'base64').equals(fileBytes), 'native Secret preserves binary File bytes');
  };
  if (afterRotation) {
    await publish(7);
    await until('both adapters consume a new revision after Master Key rotation', () => delivered(7));
    verifyBytes();
    for (const pod of consumers) assert.ok(get('pod', pod).status.containerStatuses.every((status) => status.restartCount === 0));
    console.log('PASS post-rotation native/CSI refresh with decrypted Vault values and exact binary File bytes');
    return;
  }
  const privateValues = [];
  const authorities = await api('/v1/certificate-authorities?limit=100');
  const authority = authorities.items.find((item) => item.display_name === 'Documentation CA');
  assert.ok(authority);
  const issue = async () => {
    const issued = await api('/v1/client-certificates/issue', 'POST', { authority_id: authority.id, display_name: 'Kubernetes acceptance', valid_days: 1 });
    const token = await api('/v1/api-tokens', 'POST', { display_name: 'Kubernetes acceptance', environment_keys: ['development'], allow_without_mtls: false });
    assert.ok(issued.certificate?.fingerprint_sha256 && token.public_id && token.token);
    const bundle = join(directory, `kubernetes-client-${privateValues.length}.zip`);
    writeFileSync(bundle, Buffer.from(issued.export_bundle, 'base64'), { mode: 0o600 });
    const cert = run('unzip', ['-p', bundle, 'client.crt']);
    const key = run('unzip', ['-p', bundle, 'client.key']);
    privateValues.push(Buffer.from(token.token).toString('base64'), Buffer.from(key).toString('base64'), createPrivateKey(key).export({ type: 'pkcs8', format: 'der' }).toString('base64'));
    writeFileSync(join(directory, 'kubernetes-private.json'), JSON.stringify(privateValues), { mode: 0o600 });
    return { token, issued, data: { token: token.token, 'tls.crt': cert, 'tls.key': key } };
  };
  const credentials = (value) => applyInNamespace({ apiVersion: 'v1', kind: 'Secret', metadata: { name: 'configra-credentials' }, stringData: value.data });
  const initial = await issue();
  credentials(initial);
  await publish(1);
  const objects = [{ type: 'config', environment: 'development', config: 'kubernetes_file', path: 'app.yaml' },
    { type: 'file', environment: 'development', namespace: 'platform', item: 'database', field: 'credentials', path: 'credentials.txt' }];
  for (const [name, kind, mode, sources] of [
    ['native-file', 'Secret', 'files', objects],
    ['native-env', 'ConfigMap', 'env', [{ type: 'config', environment: 'development', config: 'kubernetes_env', path: 'env.yaml' }]],
    ['unsafe-configmap', 'ConfigMap', 'files', objects],
  ]) applyInNamespace({ apiVersion: 'configra.viber-ops.github.io/v1alpha1', kind: 'ConfigraBinding', metadata: { name }, spec: {
    credentialsSecretRef: { name: 'configra-credentials' }, target: { name, kind, mode }, refreshInterval: '5s', objects: sources,
  } });
  command('-n', namespace, 'wait', 'configrabinding/native-file', '--for=condition=Ready', '--timeout=150s');
  command('-n', namespace, 'wait', 'configrabinding/native-env', '--for=condition=Ready', '--timeout=150s');
  command('-n', namespace, 'wait', 'configrabinding/unsafe-configmap', '--for=condition=Ready=false', '--timeout=150s');
  assert.equal(command('-n', namespace, 'get', 'configmap/unsafe-configmap', '--ignore-not-found'), '');
  const workers = JSON.parse(command('get', 'nodes', '-l', '!node-role.kubernetes.io/control-plane', '-o', 'json')).items.map((node) => node.metadata.name).sort();
  assert.equal(workers.length, 2);
  const example = command('create', '--dry-run=client', '-f', join(root, 'kubernetes/examples/provider.yaml'), '-o', 'jsonpath={@}{"\\n"}').trim().split('\n').map((line) => JSON.parse(line));
  for (const object of example) {
    if (object.kind === 'SecretProviderClass') object.spec.parameters.objects = JSON.stringify(objects);
    else {
      object.spec.containers[0].envFrom = [{ configMapRef: { name: 'native-env' } }];
      object.spec.nodeSelector = { 'kubernetes.io/hostname': workers[0] };
    }
    applyInNamespace(object);
  }
  const second = structuredClone(example.find((object) => object.kind === 'Pod'));
  second.metadata.name = consumers[1];
  second.spec.nodeSelector = { 'kubernetes.io/hostname': workers[1] };
  applyInNamespace(second);
  for (const pod of consumers) command('-n', namespace, 'wait', `pod/${pod}`, '--for=condition=Ready', '--timeout=150s');
  await until('initial native and CSI delivery', () => delivered(1));
  verifyBytes();
  console.log('PASS real mTLS Config/File reads, non-root CSI mount and sensitive ConfigMap rejection');

  const adapters = ['configra-app', 'configra-provider'].flatMap((ns) => get('pods', '', ns).items.filter((pod) => !consumers.includes(pod.metadata.name)));
  await publish(2);
  await until('native and CSI update', () => delivered(2));
  const rotated = await issue();
  await api(`/v1/api-tokens/${initial.token.public_id}/revoke`, 'POST');
  await api(`/v1/client-certificates/${initial.issued.certificate.fingerprint_sha256}/revoke`, 'POST');
  const deniedSince = new Date().toISOString();
  await publish(3);
  command('-n', namespace, 'wait', 'configrabinding/native-file', '--for=condition=Ready=false', '--timeout=150s');
  const rotationFailed = (since) => consumers.every((pod) => {
    const node = get('pod', pod).spec.nodeName;
    const driver = get('pods', '', 'kube-system').items.find((item) => item.metadata.labels?.app === 'csi-secrets-store' && item.spec.nodeName === node);
    assert.ok(driver, 'the consumer node has a CSI driver');
    return /failed to rotate|failed to mount secrets store/i.test(command('-n', 'kube-system', 'logs', driver.metadata.name, '-c', 'secrets-store', `--since-time=${since}`));
  });
  await until('CSI observes revoked credentials', () => rotationFailed(deniedSince));
  assert.ok(delivered(2), 'revocation must not replace already delivered bytes');
  credentials(rotated);
  await until('credential rotation without restarts', () => delivered(3));
  for (const previous of adapters) {
    const pod = get('pod', previous.metadata.name, previous.metadata.namespace);
    assert.equal(pod.metadata.uid, previous.metadata.uid);
    assert.ok(pod.status.containerStatuses.every((status) => status.restartCount === 0));
  }
  console.log('PASS real Token/certificate revocation, retained values and live Secret credential rotation');

  const outageSince = new Date().toISOString();
  captureLogs(command, dirname(kubeconfig));
  command('-n', 'configra', 'scale', 'deployment/configra-api', '--replicas=0');
  command('-n', 'configra', 'wait', 'pod', '-l', 'app.kubernetes.io/component=api', '--for=delete', '--timeout=60s');
  await publish(4); // Management remains independently usable.
  command('-n', namespace, 'wait', 'configrabinding/native-file', '--for=condition=Ready=false', '--timeout=150s');
  await until('CSI observes API outage', () => rotationFailed(outageSince));
  assert.ok(delivered(3));
  const pending = structuredClone(example.find((object) => object.kind === 'Pod'));
  pending.metadata.name = 'new-csi-during-outage';
  applyInNamespace(pending);
  await until('a new mount reports failure during outage', () => get('events', '').items.some((event) => event.involvedObject?.name === pending.metadata.name && event.reason === 'FailedMount' && event.message.includes('Configra objects could not be fetched')));
  assert.ok(!get('pod', pending.metadata.name).status.containerStatuses?.some((status) => status.ready));
  command('-n', namespace, 'delete', `pod/${pending.metadata.name}`, '--wait=true', '--timeout=30s');
  command('-n', 'configra', 'scale', 'deployment/configra-api', '--replicas=2');
  command('-n', 'configra', 'rollout', 'status', 'deployment/configra-api', '--timeout=150s');
  await until('service recovery reaches both adapters', () => delivered(4));
  console.log('PASS API outage retention, failed new CSI mount and automatic recovery');

  const leader = get('lease', 'configra-binding-controller').spec.holderIdentity;
  captureLogs(command, dirname(kubeconfig));
  command('-n', namespace, 'delete', `pod/${leader.split('_')[0]}`, '--wait=false');
  await until('replacement sync leader', () => get('lease', 'configra-binding-controller').spec.holderIdentity !== leader);
  await publish(5);
  await until('delivery after controller leader replacement', () => delivered(5));
  for (const pod of consumers) {
    assert.equal(command('-n', namespace, 'exec', pod, '--', 'printenv', 'PORT').trim(), '8081');
    assert.ok(get('pod', pod).status.containerStatuses.every((status) => status.restartCount === 0));
  }
  console.log('PASS controller failover; running envFrom remains unchanged as documented');

  let reads = 0;
  for (let attempt = 0; attempt < 3; attempt++) {
    captureLogs(command, dirname(kubeconfig));
    const before = get('pods', '', 'configra').items.filter((pod) => pod.metadata.labels['app.kubernetes.io/component'] === 'api');
    command('-n', 'configra', 'rollout', 'restart', 'deployment/configra-api');
    const deadline = Date.now() + 90000;
    let samples = 0;
    while (true) {
      assert.equal(await read(), 200, 'mTLS read during rolling API replacement');
      samples++; reads++;
      const deployment = get('deployment', 'configra-api', 'configra');
      const pods = get('pods', '', 'configra').items;
      const oldPodsGone = before.every((old) => !pods.some((pod) => pod.metadata.uid === old.metadata.uid));
      if (samples >= 5 && oldPodsGone && deployment.status.observedGeneration === deployment.metadata.generation && deployment.status.updatedReplicas === 2 && deployment.status.availableReplicas === 2 && deployment.status.replicas === 2) break;
      assert.ok(Date.now() < deadline, 'rolling replacement completes');
      await delay(250);
    }
  }
  console.log(`PASS ${reads} verified mTLS reads across three complete API rolling replacements (no retries)`);

  const holder = get('lease', 'configra-binding-controller').spec.holderIdentity;
  const failedNode = get('pod', holder.split('_')[0]).spec.nodeName;
  const [nodeContainer] = JSON.parse(docker('inspect', failedNode));
  assert.equal(nodeContainer.Config.Labels['io.x-k8s.kind.cluster'], command('config', 'current-context').trim().slice(5));
  assert.equal(nodeContainer.Config.Labels['io.x-k8s.kind.role'], 'worker');
  const survivor = consumers.find((pod) => get('pod', pod).spec.nodeName !== failedNode);
  assert.ok(survivor);
  const healthyNode = get('pod', survivor).spec.nodeName;
  const started = Date.now();
  captureLogs(command, dirname(kubeconfig));
  try {
    docker('pause', failedNode);
    await until('worker is marked unavailable', () => {
      const node = JSON.parse(command('get', 'node', failedNode, '-o', 'json'));
      return node.status.conditions.some((condition) => condition.type === 'Ready' && condition.status !== 'True');
    });
    await until('unavailable worker leaves API and Management endpoints', () => ['configra-api', 'configra-management'].every((service) => {
      const endpoints = get('endpointslices', '', 'configra').items
        .filter((item) => item.metadata.labels['kubernetes.io/service-name'] === service).flatMap((item) => item.endpoints);
      return endpoints.some((endpoint) => endpoint.nodeName === healthyNode && endpoint.conditions.ready) &&
        !endpoints.some((endpoint) => endpoint.nodeName === failedNode && endpoint.conditions.ready);
    }));
    await until('sync leader moves to the healthy worker', () => {
      const current = get('lease', 'configra-binding-controller').spec.holderIdentity;
      return current && current !== holder && get('pod', current.split('_')[0]).spec.nodeName === healthyNode;
    });
    await until('Management session reaches its surviving replica', async () => {
      try { await api('/v1/environments?limit=1'); return true; } catch { return false; }
    });
    await publish(6);
    await until('native and surviving CSI update during worker loss', () => delivered(6, [survivor]));
    verifyBytes([survivor]);
    const fresh = structuredClone(second);
    fresh.metadata.name = 'new-csi-during-node-outage';
    fresh.spec.nodeSelector = { 'kubernetes.io/hostname': healthyNode };
    applyInNamespace(fresh);
    command('-n', namespace, 'wait', `pod/${fresh.metadata.name}`, '--for=condition=Ready', '--timeout=150s');
    assert.ok(delivered(6, [fresh.metadata.name]), 'new mount fetches the latest revision while a worker is unavailable');
    verifyBytes([fresh.metadata.name]);
    for (let sample = 0; sample < 20; sample++) assert.equal(await read(), 200, 'mTLS read after failed-node endpoint removal');
    writeFileSync(join(dirname(kubeconfig), 'kubernetes-node-failure.json'), JSON.stringify({
      failedNode, healthyNode, elapsedMilliseconds: Date.now() - started,
      revision: 6, reads: 20, nativeSync: true, existingCSIMount: true, newCSIMount: true,
    }, null, 2), { mode: 0o600 });
  } finally {
    if (JSON.parse(docker('inspect', failedNode))[0].State.Paused) docker('unpause', failedNode);
  }
  command('wait', `node/${failedNode}`, '--for=condition=Ready', '--timeout=150s');
  for (const pod of consumers) command('-n', namespace, 'wait', `pod/${pod}`, '--for=condition=Ready', '--timeout=150s');
  await until('both workers converge after recovery', () => delivered(6));
  verifyBytes();
  console.log('PASS abrupt worker loss, cross-node sync leadership, native/CSI updates, fresh mount and node recovery');
}
