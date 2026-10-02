# Release sets / 完整发布集

A release stream belongs to **one Environment and one Config key**. Its immutable
manifest pins a Config Revision, every Vault Revision used by that Config, and
up to 31 additional File members. Activating a prepared key changes one pointer
with an expected generation; rolling back activates an older key and increments
the generation again. Preparation does not change the active pointer.

发布流绑定“环境＋Config”，不能用一个环境级指针覆盖其他应用。发布集固定配置、
引用值及文件版本；后续 Config/Vault 草稿写入不改变已发布内容。回滚恢复整套内容，
不会把旧版本复制成新 Vault 值，也不会删除历史。

## Prepare, validate and activate

Upload `server.yaml` and File values with the normal scoped writes. Record the
returned revisions. For multiple Files in the same Item, use that Item's **final
revision** for every member. A minimal manifest is:

```json
{
  "config_revision": 12,
  "config_path": "server.yaml",
  "files": [
    {"namespace":"deployment","item":"secrets","field":"signing_key","path":"secrets/signing.key","revision":18}
  ]
}
```

```sh
configractl release prepare server deploy_20261003 --file release.json \
  --operation-id prepare-20261003 --json
configractl release get server --release deploy_20261003 --output-dir ./candidate
configractl release state server --json
configractl release activate server deploy_20261003 --expected-generation 4 \
  --operation-id activate-20261003 --validator /opt/operator/validate-server \
  --validator-arg=--strict --json
configractl release rollback server deploy_20261002 --expected-generation 5 \
  --operation-id rollback-20261003 --validator /opt/operator/validate-server --json
```

The CLI executes the explicitly supplied local executable without a shell. The
candidate's new private directory is available as `CONFIGRA_RELEASE_DIR` and its
Config path as `CONFIGRA_CONFIG_FILE`; the working directory is the candidate
directory. Validator output is discarded, and a nonzero exit, timeout or changed
candidate bytes prevents activation. Supply arguments separately with Cobra's
repeatable `--validator-arg`; remote manifests cannot supply executable commands.
Use the application's own schema/semantic validator and existing CI protections.

验证器来自本机明确指定的程序，不来自服务端。验证失败不会切换版本。正常退出后
才发送带代数检查的切换请求；并发变化返回 409，不自动覆盖。SDK/API 调用方应在
调用激活接口前自行执行业务验证。预发布验证不能代替上线后的业务健康检查：先让
少量实例读取候选 key，确认后切换，异常时激活上一个 key。

`release get` creates a new 0700 directory containing 0600 files. It does not
overwrite a live application tree. Switch the application's directory reference
only after the command succeeds; readers that open files independently across a
directory switch still need an application-level reload boundary. Cached values
cannot be remotely erased after a client has received them.

## API and SDK contract

All paths below start with `/v1/environments/{environment}/configs/{config}` on
the 9443 machine listener. Existing Token, certificate and scope checks apply.

| Method and suffix | Contract |
| --- | --- |
| `PUT /releases/{release}` | Immutable preparation, manifest body above and `Idempotency-Key`; Config/File revisions must still be current |
| `POST /releases/{release}/activate` | Body `{"expected_generation":4}` and `Idempotency-Key`; initial generation is zero |
| `GET /releases/{release}` | Complete candidate bundle for verification or a pinned consumer |
| `GET /release` | Complete active bundle, resolved in one MySQL snapshot; ETag includes activation generation |
| `GET /release-state` | Active key and generation, or empty key/generation zero before first activation |

Preparation and activation require a write-scoped Token. Root Config scope and
every referenced/File namespace must be granted; reads support deployment
read-only credentials. Current archived Config/Environment/Item state, removed
Fields and Environment bindings are checked even for historical releases and
conditional reads. A saved ETag cannot bypass authorization. Reactivating an old
key does not restore removed permissions or deleted Fields.

Bounds: 32 members including Config, 5 MiB **total decoded content**, 64 distinct
Vault Items and 256 referenced/member fields. Paths must be canonical relative
paths, cannot overlap as file/directory, and cannot contain traversal, backslashes,
colon or control characters. Manifests contain identifiers/revisions/paths, not
secret bytes. The retained encrypted Vault history remains the value store, so
the existing Master Key rotation and backup contracts still apply.

Prepare/activate outcomes and authenticated rejections enter the durable Audit
Outbox with the Token ID and source IP. `resource_type=release` identifies
`config.release`; the successful activation revision is its stream generation.
Activation emits value-free `release.activate` notifications. Content reads
produce best-effort Access Events, with the same limitations as existing reads.

`configra-go` provides `PrepareRelease`, `ActivateRelease`, `ReadRelease`, and
`ReadReleaseState`. `ViperHandlerOptions.UseRelease=true` follows the active
stream; `Validate` sees the complete proposed Snapshot before installation.
`Snapshot.Files()` returns independent byte copies. `ReleaseKey()` and
`ReleaseGeneration()` identify the deployment, including File-only changes.

旧的 Config/File 读取接口继续读取当前值；它们不会自动跟随发布指针。需要整套发布
保证的应用必须显式改用发布集。`OnChange` 仍是安装后的通知，业务拒绝应放在
`Validate`；长期使用旧快照的可用性策略及新鲜度告警由应用决定。

## Kubernetes consumers

Use a single source mapping; it cannot be mixed with independently versioned
Config/File mappings:

```yaml
objects:
  - type: release
    environment: production
    config: server
    path: .
```

The `.` marks the release root; actual paths come from the manifest. CSI accepts
the existing safe relative path subset and a 3 MiB batch. Native Secret/ConfigMap
sync requires **flat filenames**, `target.mode: files`, and at most 900 KiB;
choose flat manifest paths when using it. Vault/File data keeps the ConfigMap
sensitivity guard. A whole release is written as one Secret update or CSI batch;
Kubernetes delivery still does not attest application reload. Environment-variable
consumers require Pod replacement and do not support release File members.
