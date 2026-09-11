import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { basicSetup, EditorView } from 'codemirror';
import { EditorState } from '@codemirror/state';
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { json } from '@codemirror/lang-json';
import { yaml } from '@codemirror/lang-yaml';
import { MergeView } from '@codemirror/merge';
import { tags } from '@lezer/highlight';
import './styles.css';

const messages = {
  en: {
    access: 'Access',
    accessBody: 'Recent content responses observed in ClickHouse through the best-effort Access event path.',
    observedResponses: 'Observed responses',
    accessIncomplete: '304 checks and dropped events are not included.',
    addField: 'Add field',
    addVariant: 'Add Variant',
    administration: 'Administration',
    administrationBody: 'Business credentials and read-only runtime information. Cold-start YAML is deliberately not editable here.',
    allowWithoutMTLS: 'Allow this token without mTLS',
    allowedEnvironments: 'Allowed environments',
    apiTokens: 'API tokens',
    audit: 'Audit',
    auditBody: 'Durable metadata for authenticated mutation attempts and their outcomes.',
    auditAtLeastOnce: 'At-least-once delivery may produce duplicate rows.',
    searchAudit: 'Search audit logs',
    searchAuditPlaceholder: 'Operation, actor, action, outcome, or resource',
    search: 'Search',
    noMatchingAudits: 'No audit records match this search.',
    auditPagination: 'Audit pagination',
    pageNumber: 'Page {page}',
    archive: 'Archive',
    cancel: 'Cancel',
    clone: 'Clone',
    cloneComplete: 'Clone created.',
    cloneConfig: 'Clone config',
    compare: 'Compare',
    compareDragHint: 'Drag any two revisions into Source and Target. Use one Environment for history, or two for a cross-Environment comparison.',
    transferDragHint: 'Choose any Source Revision and any Target Revision. A preview appears after both slots are filled.',
    compareRevision: 'Compare v{revision}',
    compareRevisions: 'Compare revisions',
    diffAdded: 'Added',
    diffRemoved: 'Removed',
    configured: 'Configured',
    confirmArchive: 'Confirm archive',
    confirmRestore: 'Confirm restore v{revision}',
    configs: 'Configs',
    configBody: 'One Config identity, with an independent Revision history in every Environment where it exists.',
    configIdentity: 'Config identity',
    configContexts: 'Environment contexts',
    configContextBody: 'Choose the Environment context whose independent Revision you want to inspect or change.',
    configContextHint: 'Each Environment is a peer context with its own immutable Revision history.',
    current: 'Current',
    readOnly: 'Read-only',
    noConfigContexts: 'This Config does not exist in any Environment.',
    openConfig: 'Open Config',
    openContext: 'Open context',
    searchConfigs: 'Search configs',
    searchConfigsPlaceholder: 'Resource key, name, or Environment',
    allStatuses: 'All statuses',
    allEnvironments: 'All environments',
    environmentFilter: 'Environment filter',
    noMatchingConfigs: 'No Config matches the current search and status filter.',
    searchVault: 'Search Vault',
    searchVaultPlaceholder: 'Namespace, resource key, or name',
    allNamespaces: 'All namespaces',
    namespaceFilter: 'Filter by namespace',
    noMatchingVaultItems: 'No Vault item matches the current filters.',
    moreContexts: 'more Environment contexts',
    previous: 'Previous',
    next: 'Next',
    pagination: 'Config pagination',
    copy: 'Copy',
    copyReference: 'Copy reference',
    copyValue: 'Copy value',
    copyRequestID: 'Copy Request ID',
    requestID: 'Request ID',
    currentReferences: 'Current references',
    currentReferencesBody: 'Active current Config revisions that reference this Vault Item.',
    noCurrentReferences: 'No active current Config references this Vault Item.',
    vaultImpactWarning: 'This change would leave the following current Config references unresolved. Save again to confirm.',
    confirmImpactSave: 'Confirm save with impact',
    configurationSource: 'Configuration source',
    created: 'Created',
    createConfig: 'Create config',
    createEnvironment: 'Create environment',
    createVaultItem: 'Create Vault item',
    createAPIToken: 'Create API token',
    clientCertificates: 'Client certificates',
    certificateName: 'Certificate name',
    chooseFile: 'Choose file',
    confirmRevoke: 'Confirm revoke',
    deploymentStatus: 'Deployment status',
    managementReady: 'Management ready',
    managementNotReady: 'Management not ready',
    observedNow: 'Observed now',
    configuredArchitecture: 'Configured architecture',
    coldStartYAML: 'Cold-start YAML',
    apiScalesIndependently: 'API scales independently',
    compatibilityBaseline: 'Compatibility baseline',
    destinationKey: 'Destination key',
    destinationName: 'Destination name',
    deliveryHistory: 'Delivery history',
    displayName: 'Display name',
    download: 'Download',
    editEnvironments: 'Edit environments',
    editConfig: 'Edit config',
    editItem: 'Edit item',
    environment: 'Environment',
    environments: 'Environments',
    fields: 'Fields',
    fieldKey: 'Field key',
    fieldName: 'Field name',
    fieldType: 'Field type',
    fieldIdentityLocked: 'Field keys and types are immutable after creation.',
    fileTooLarge: 'Files must be 5 MiB or smaller.',
    fileType: 'File',
    fileReadPath: 'Client ReadFile path',
    forbiddenTitle: 'Administrator access required',
    forbiddenBody: 'Your Viewer role can inspect configuration metadata but cannot manage credentials or outbound destinations.',
    environmentBody: 'Peer runtime contexts. Environment is not a deployment stage or an ordering.',
    environmentDetailBody: 'Configs and current Vault bindings available in this peer runtime context.',
    openEnvironment: 'Open Environment',
    configsInEnvironment: 'Configs in this Environment',
    vaultInEnvironment: 'Vault Items in this Environment',
    noConfigsInEnvironment: 'No Config currently exists in this Environment.',
    noVaultInEnvironment: 'No current Vault Variant is bound to this Environment.',
    viewAllConfigsIn: 'View all Configs in {environment}',
    viewAllVaultIn: 'View all Vault Items in {environment}',
    boundEnvironments: 'Bound environments',
    environmentRevisions: 'Environment revisions',
    environmentBinding: 'Environment binding',
    environmentMoveHint: 'Selecting an Environment here moves it from any other Variant.',
    inventory: 'Managed resources',
    itemDetails: 'Item details',
    globalSearch: 'Workspace search',
    globalSearchPlaceholder: 'Search Configs, Vault, Environments',
    configurationGaps: 'Configuration gaps',
    noConfigurationGaps: 'Every active Config has an active Environment Revision.',
    noActiveConfigContext: 'No active Environment Revision',
    language: '中文',
    useDarkTheme: 'Use dark theme',
    useLightTheme: 'Use light theme',
    loadFailed: 'Configra could not load this workspace.',
    leftEnvironment: 'Environment A',
    loginBody: 'Version YAML or JSON together with Vault fields, then deliver one resolved response through the API.',
    loginButton: 'Continue with SSO',
    loginEyebrow: 'Config + Vault / versioned together',
    loginTitle: 'Exact configuration, for every environment.',
    loginAuthHint: 'Authentication stays with your identity provider. Configra never receives your password.',
    storedConfiguration: 'Stored configuration',
    deliveredConfiguration: 'Delivered response',
    resolvedAtRead: 'Resolved when the client reads',
    revisionEvidence: 'Revision evidence',
    notifications: 'Notifications',
    notificationsBody: 'Encrypted outbound destinations for selected mutation events. Lists expose only safe host metadata.',
    newDestination: 'New destination',
    newConfig: 'New config',
    newVaultItem: 'New Vault item',
    provider: 'Provider',
    genericWebhook: 'Generic webhook',
    feishuBot: 'Feishu bot',
    webhookURL: 'Webhook URL',
    signingSecret: 'Signing secret',
    eventTypes: 'Event types',
    saveDestination: 'Save destination',
    sendTest: 'Send test',
    viewDeliveries: 'View deliveries',
    viewRevision: 'View v{revision}',
    redeliver: 'Redeliver',
    enabled: 'Enabled',
    disabled: 'Disabled',
    newEnvironment: 'New environment',
    newAPIToken: 'New API token',
    neverExpires: 'Never expires',
    mtlsRequired: 'mTLS required',
    importClientCertificate: 'Import client certificate',
    publicCertificatePEM: 'Public certificate PEM',
    publicCertificateOnly: 'Public certificate only. Private keys are never accepted.',
    registerCertificate: 'Register certificate',
    revoke: 'Revoke',
    revoked: 'Revoked',
    tokenOnlyAllowed: 'Token-only allowed',
    tokenName: 'Token name',
    targetConfig: 'Target config',
    targetEnvironment: 'Target environment',
    targetRevision: 'Target revision',
    target: 'Target',
    targetConflictHint: 'The target Environment was current at v{revision} when previewed. If it changes before commit, this operation stops for review.',
    mergeFormatMismatch: 'Cannot merge: Source is {source}, while Target is {target}. Convert them to the same format first.',
    targetName: 'Target name',
    textType: 'Text',
    tokenOnce: 'This token will not be shown again.',
    noResources: 'No managed resources yet.',
    noFileSelected: 'No file selected',
    overview: 'Overview',
    overviewBody: 'Service status, recent changes, and configuration gaps.',
    recentChanges: 'Recent changes',
    noRecentChanges: 'No mutation records have been observed yet.',
    available: 'Available',
    unavailable: 'Unavailable',
    raw: 'Raw',
    resolvedConfiguration: 'Resolved configuration',
    resolvedPreview: 'Resolved preview',
    resolvedWarning: 'This view can contain plaintext secrets. Opening it creates an Access event.',
    revealResolvedValues: 'Reveal resolved values',
    history: 'History',
    revision: 'Revision',
    retry: 'Try again',
    resourceKey: 'Resource key',
    resourceKeyHint: 'Immutable after creation · lowercase letters, numbers, _ or -',
    namespace: 'Namespace',
    namespaceHint: 'Immutable identity segment for organization only; it does not grant access.',
    revisionPool: 'Revision pool',
    rightEnvironment: 'Environment B',
    roleAdmin: 'Administrator',
    roleViewer: 'Viewer',
    signedInAs: 'Signed in as',
    signOut: 'Sign out',
    saveChanges: 'Save changes',
    saveItem: 'Save item',
    saveName: 'Save name',
    saveEnvironmentGrants: 'Save environment grants',
    saveConflict: 'This Config changed after you opened it. Reload before saving again.',
    saveFailed: 'The Config was not saved. Check the source and try again.',
    savedRevision: 'Saved as v{revision}.',
    unsavedChanges: 'You have unsaved changes. Discard them?',
    revision_conflict: 'This resource changed after you opened it. Review the latest Revision and try again.',
    validation_failed: 'The input is invalid. Review the highlighted fields and try again.',
    request_too_large: 'The request exceeds the supported size limit.',
    service_unavailable: 'The service is temporarily unavailable. Try again shortly.',
    operation_id_reused: 'This operation identifier was already used for different content.',
    unresolved_vault_reference: 'One or more Vault references cannot be resolved in this Environment.',
    crypto_integrity_failure: 'Vault integrity verification failed. Contact an administrator.',
    not_found: 'The requested resource no longer exists.',
    statusActive: 'Active',
    statusArchived: 'Archived',
    secretType: 'Secret',
    sourceConfig: 'Source config',
    sourceEnvironment: 'Source environment',
    sourceRevision: 'Source revision',
    source: 'Source',
    dropRevisionHere: 'Drop a revision here',
    setSource: 'Set Source',
    setTarget: 'Set Target',
    useAsSource: 'Use {revision} as Source',
    useAsTarget: 'Use {revision} as Target',
    snapshotHint: 'Complete Snapshot JSON. File bytes use base64.',
    snapshotJSON: 'Snapshot JSON',
    snapshotIncomplete: 'Complete every Field value and use a unique valid Field key.',
    transfer: 'Transfer',
    transferMode: 'Transfer mode',
    transferPreview: 'Transfer preview',
    result: 'Result',
    targetBefore: 'Target · before',
    resultAfter: 'Result · after',
    previewTransfer: 'Preview transfer',
    merge: 'Merge',
    moreActions: 'More actions',
    previewMerge: 'Preview merge',
    previewReplace: 'Preview replace',
    replace: 'Replace',
    replaceFile: 'Replace file',
    removeField: 'Remove field',
    removeVariant: 'Remove Variant',
    mergeInto: 'Merge into {environment}',
    replaceInto: 'Replace {environment}',
    rename: 'Rename',
    restore: 'Restore',
    restoreRevision: 'Restore v{revision}',
    updated: 'Updated',
    unarchive: 'Unarchive',
    vault: 'Vault',
    vaultBody: 'Versioned Items group complete Field sets into Environment-bound Variants.',
    reveal: 'Reveal',
    hide: 'Hide',
    revealItemValues: 'Reveal item values',
    valueAccessWarning: 'Values may contain plaintext secrets and files. Revealing them creates an Access event.',
    variant: 'Variant',
    variants: 'Variants',
    vaultRevisions: 'Vault revisions',
    versionIdentity: 'Version identity',
    action: 'Action',
    actor: 'Actor',
    applicationLogs: 'Access / Audit logs',
    attempt: 'Attempt',
    authentication: 'Authentication',
    caVerifiedCertificates: 'CA-verified public certificates',
    configView: 'Config view',
    currentState: 'current state',
    currentRevisionLabel: 'Current revision v{revision}',
    delivery: 'Delivery',
    endpoint: 'Endpoint',
    error: 'Error',
    expiresAt: 'Expires at',
    fingerprint: 'Fingerprint SHA-256',
    format: 'Format',
    formatSource: 'Format source',
    formatComplete: 'Source formatted.',
    validationFailed: 'The source is invalid and was not formatted.',
    missing_vault_item: 'Vault item does not exist',
    missing_vault_field: 'Vault field does not exist',
    missing_vault_environment_variant: 'No matching Environment variant',
    invalidSnapshot: 'Snapshot JSON is invalid.',
    latencyMS: 'Latency ms',
    locale: 'en',
    managementSingleReplica: 'Management remains single replica',
    observedReads: 'Observed reads',
    nonBlockingAccessEvents: 'non-blocking Access events',
    operation: 'Operation',
    operationFailed: 'The operation failed. Check the input and try again.',
    outcome: 'Outcome',
    prefix: 'Prefix',
    primaryNavigation: 'Primary navigation',
    principal: 'Principal',
    readOnlyContract: 'read-only V1 contract',
    reference: 'Reference',
    referenceKey: 'Reference key',
    resource: 'Resource',
    systemStatus: 'System status',
    restartToApply: 'restart to apply',
    revisionHistory: 'Revision history',
    revisionLabel: 'Revision v{revision}',
    serial: 'Serial',
    status: 'Status',
    subject: 'Subject',
    time: 'Time',
    tokenEnvironmentGrant: 'Token + Environment grant',
    validUntil: 'Valid until',
    value: 'Value',
    valueFor: '{field} value',
    vaultView: 'Vault view',
    notFoundTitle: 'Page not found',
    notFoundBody: 'This route does not exist in Configra V1.',
  },
  zh: {
    access: '访问记录',
    accessBody: '通过最大努力 Access 事件链路在 ClickHouse 中观测到的近期内容响应。',
    observedResponses: '已观测响应',
    accessIncomplete: '不包含 304 检查与已丢失事件。',
    addField: '新增字段',
    addVariant: '新增 Variant',
    administration: '管理',
    administrationBody: '管理业务凭据并只读查看运行信息；冷启动 YAML 不会在这里编辑。',
    allowWithoutMTLS: '允许此 Token 不使用 mTLS',
    allowedEnvironments: '允许访问的 Environment',
    apiTokens: 'API Token',
    audit: '审计日志',
    auditBody: '已认证 Mutation 尝试及结果的持久化元数据。',
    auditAtLeastOnce: '至少一次投递可能产生重复行。',
    searchAudit: '搜索审计日志',
    searchAuditPlaceholder: '操作 ID、操作者、动作、结果或资源',
    search: '搜索',
    noMatchingAudits: '没有符合当前搜索条件的审计记录。',
    auditPagination: '审计日志分页',
    pageNumber: '第 {page} 页',
    archive: 'Archive',
    cancel: '取消',
    clone: '克隆',
    cloneComplete: '克隆已创建。',
    cloneConfig: '克隆 Config',
    compare: '对比',
    compareDragHint: '把任意两个 Revision 拖入 Source 和 Target；同一 Environment 是历史对比，不同 Environment 是跨环境对比。',
    transferDragHint: '选择任意 Source Revision 和任意 Target Revision；两边就位后自动生成预览。',
    compareRevision: '对比 v{revision}',
    compareRevisions: '对比 Revision',
    diffAdded: '新增',
    diffRemoved: '删除',
    configured: '已配置',
    confirmArchive: '确认 Archive',
    confirmRestore: '确认恢复 v{revision}',
    configs: '配置',
    configBody: '同一个 Config 身份，在每个所在 Environment 中拥有独立的 Revision 历史。',
    configIdentity: 'Config 身份',
    configContexts: 'Environment 上下文',
    configContextBody: '选择要查看或修改其独立 Revision 的 Environment 上下文。',
    configContextHint: '每个 Environment 都彼此平级，并拥有独立的不可变 Revision 历史。',
    current: '当前',
    readOnly: '只读',
    noConfigContexts: '这个 Config 尚未存在于任何 Environment。',
    openConfig: '打开 Config',
    openContext: '打开上下文',
    searchConfigs: '搜索配置',
    searchConfigsPlaceholder: '资源键、名称或 Environment',
    allStatuses: '全部状态',
    allEnvironments: '全部 Environment',
    environmentFilter: 'Environment 筛选',
    noMatchingConfigs: '没有符合当前搜索和状态筛选的 Config。',
    searchVault: '搜索 Vault',
    searchVaultPlaceholder: 'Namespace、资源键或名称',
    allNamespaces: '全部 Namespace',
    namespaceFilter: '按 Namespace 筛选',
    noMatchingVaultItems: '没有符合当前筛选条件的 Vault Item。',
    moreContexts: '个其他 Environment 上下文',
    previous: '上一页',
    next: '下一页',
    pagination: 'Config 分页',
    copy: '复制',
    copyReference: '复制引用',
    copyValue: '复制值',
    copyRequestID: '复制 Request ID',
    requestID: 'Request ID',
    currentReferences: '当前引用',
    currentReferencesBody: '当前启用的 Config Revision 对此 Vault Item 的引用。',
    noCurrentReferences: '当前没有启用的 Config 引用此 Vault Item。',
    vaultImpactWarning: '此变更会让以下当前 Config 引用无法解析；请再次保存以确认。',
    confirmImpactSave: '确认影响并保存',
    configurationSource: '配置原文',
    created: '创建时间',
    createConfig: '创建 Config',
    createEnvironment: '创建环境',
    createVaultItem: '创建 Vault Item',
    createAPIToken: '创建 API Token',
    clientCertificates: '客户端证书',
    certificateName: '证书名称',
    chooseFile: '选择文件',
    confirmRevoke: '确认吊销',
    deploymentStatus: '部署状态',
    managementReady: 'Management 已就绪',
    managementNotReady: 'Management 未就绪',
    observedNow: '当前观测',
    configuredArchitecture: '配置架构',
    coldStartYAML: '冷启动 YAML',
    apiScalesIndependently: 'API 独立扩容',
    compatibilityBaseline: '兼容基线',
    destinationKey: 'Destination 键',
    destinationName: 'Destination 名称',
    deliveryHistory: '投递历史',
    displayName: '显示名称',
    download: '下载',
    editEnvironments: '编辑 Environment',
    editConfig: '编辑配置',
    editItem: '编辑 Item',
    environment: 'Environment',
    environments: '环境',
    fields: '字段',
    fieldKey: '字段键',
    fieldName: '字段名称',
    fieldType: '字段类型',
    fieldIdentityLocked: '字段键和类型创建后不可修改。',
    fileTooLarge: '文件不能超过 5 MiB。',
    fileType: '文件',
    fileReadPath: '客户端 ReadFile 路径',
    forbiddenTitle: '需要管理员权限',
    forbiddenBody: 'Viewer 可以查看配置元数据，但不能管理凭据或出站通知。',
    environmentBody: '彼此平级的运行上下文；Environment 不是发布阶段，也不存在顺序。',
    environmentDetailBody: '查看此运行上下文中的 Config 与当前 Vault 绑定。',
    openEnvironment: '打开 Environment',
    configsInEnvironment: '此 Environment 中的 Config',
    vaultInEnvironment: '此 Environment 中的 Vault Item',
    noConfigsInEnvironment: '此 Environment 当前没有 Config。',
    noVaultInEnvironment: '当前没有 Vault Variant 绑定到此 Environment。',
    viewAllConfigsIn: '查看 {environment} 中的全部 Config',
    viewAllVaultIn: '查看 {environment} 中的全部 Vault Item',
    boundEnvironments: '绑定的 Environment',
    environmentRevisions: 'Environment Revision',
    environmentBinding: 'Environment 绑定',
    environmentMoveHint: '在这里选择 Environment 时，会自动从其他 Variant 移入当前 Variant。',
    inventory: '托管资源',
    itemDetails: 'Item 信息',
    globalSearch: '工作区搜索',
    globalSearchPlaceholder: '搜索 Config、Vault、Environment',
    configurationGaps: '配置缺口',
    noConfigurationGaps: '所有启用的 Config 都有可用的 Environment Revision。',
    noActiveConfigContext: '没有可用的 Environment Revision',
    language: 'EN',
    useDarkTheme: '使用暗色主题',
    useLightTheme: '使用亮色主题',
    loadFailed: 'Configra 无法加载当前工作区。',
    leftEnvironment: 'Environment A',
    loginBody: '把 YAML 或 JSON 与 Vault Field 一起版本管理，再通过 API 交付解析完成的最终响应。',
    loginButton: '使用 SSO 继续',
    loginEyebrow: 'Config + Vault / 统一版本管理',
    loginTitle: '让每个 Environment 都拿到准确配置。',
    loginAuthHint: '认证由你的身份提供方完成，Configra 不会接收账号密码。',
    storedConfiguration: '存储的配置',
    deliveredConfiguration: '交付的响应',
    resolvedAtRead: '客户端读取时解析',
    revisionEvidence: 'Revision 证据',
    notifications: '通知',
    notificationsBody: '为指定变更事件配置加密的出站通知；列表只展示安全 Host 元数据。',
    newDestination: '新建 Destination',
    newConfig: '新建 Config',
    newVaultItem: '新建 Vault Item',
    provider: 'Provider',
    genericWebhook: '通用 Webhook',
    feishuBot: '飞书机器人',
    webhookURL: 'Webhook URL',
    signingSecret: '签名 Secret',
    eventTypes: '事件类型',
    saveDestination: '保存 Destination',
    sendTest: '发送测试',
    viewDeliveries: '查看投递',
    viewRevision: '查看 v{revision}',
    redeliver: '重新投递',
    enabled: '已启用',
    disabled: '未启用',
    newEnvironment: '新建环境',
    newAPIToken: '新建 API Token',
    neverExpires: '永不过期',
    mtlsRequired: '必须使用 mTLS',
    importClientCertificate: '导入客户端证书',
    publicCertificatePEM: '公钥证书 PEM',
    publicCertificateOnly: '仅接受公钥证书，绝不接受 Private Key。',
    registerCertificate: '登记证书',
    revoke: '吊销',
    revoked: '已吊销',
    tokenOnlyAllowed: '允许仅 Token',
    tokenName: 'Token 名称',
    targetConfig: '目标 Config',
    targetEnvironment: '目标 Environment',
    targetRevision: '目标 Revision',
    target: '目标',
    targetConflictHint: '生成预览时，目标 Environment 当前为 v{revision}；提交前若发生变化，本次操作会停止并要求重新确认。',
    mergeFormatMismatch: '无法合并：源是 {source}，目标是 {target}。请先将两者统一为同一种格式。',
    targetName: '目标名称',
    textType: '文本',
    tokenOnce: '此 Token 不会再次显示。',
    noResources: '还没有托管资源。',
    noFileSelected: '尚未选择文件',
    overview: '概览',
    overviewBody: '查看服务状态、最近变更和配置缺口。',
    recentChanges: '最近变更',
    noRecentChanges: '尚未观测到变更记录。',
    available: '可用',
    unavailable: '不可用',
    raw: '原始配置',
    resolvedConfiguration: '解析后配置',
    resolvedPreview: '解析预览',
    resolvedWarning: '此视图可能包含明文 Secret；打开后会产生一条访问事件。',
    revealResolvedValues: '显示解析值',
    history: '历史版本',
    revision: 'Revision',
    retry: '重试',
    resourceKey: '资源键',
    resourceKeyHint: '创建后不可修改 · 仅小写字母、数字、_ 或 -',
    namespace: 'Namespace',
    namespaceHint: '仅用于组织资源的不可变身份段，不参与访问授权。',
    revisionPool: 'Revision 池',
    rightEnvironment: 'Environment B',
    roleAdmin: '管理员',
    roleViewer: '查看者',
    signedInAs: '当前用户',
    signOut: '退出登录',
    saveChanges: '保存更改',
    saveItem: '保存 Item',
    saveName: '保存名称',
    saveEnvironmentGrants: '保存 Environment 授权',
    saveConflict: '打开后该 Config 已发生变化，请重新加载再保存。',
    saveFailed: 'Config 未保存，请检查原文后重试。',
    savedRevision: '已保存为 v{revision}。',
    unsavedChanges: '有尚未保存的更改，确定放弃吗？',
    revision_conflict: '打开后资源已发生变化，请查看最新 Revision 后重试。',
    validation_failed: '输入不合法，请检查标记的字段后重试。',
    request_too_large: '请求超过系统支持的大小限制。',
    service_unavailable: '服务暂时不可用，请稍后重试。',
    operation_id_reused: '此操作标识已用于其他内容。',
    unresolved_vault_reference: '此 Environment 中有 Vault 引用无法解析。',
    crypto_integrity_failure: 'Vault 完整性校验失败，请联系管理员。',
    not_found: '请求的资源已不存在。',
    statusActive: '启用',
    statusArchived: '已归档',
    secretType: 'Secret',
    sourceConfig: '源 Config',
    sourceEnvironment: '源 Environment',
    sourceRevision: '源 Revision',
    source: '源',
    dropRevisionHere: '将 Revision 拖到这里',
    setSource: '设为源',
    setTarget: '设为目标',
    useAsSource: '将 {revision} 设为 Source',
    useAsTarget: '将 {revision} 设为 Target',
    snapshotHint: '填写完整 Snapshot JSON；文件 bytes 使用 base64。',
    snapshotJSON: 'Snapshot JSON',
    snapshotIncomplete: '请填写每个字段的值，并确保字段键合法且不重复。',
    transfer: '迁移',
    transferMode: '迁移方式',
    transferPreview: '迁移预览',
    result: '结果',
    targetBefore: '目标 · 变更前',
    resultAfter: '结果 · 变更后',
    previewTransfer: '生成迁移预览',
    merge: '合并',
    moreActions: '更多操作',
    previewMerge: '预览合并',
    previewReplace: '预览替换',
    replace: '替换',
    replaceFile: '替换文件',
    removeField: '移除字段',
    removeVariant: '移除 Variant',
    mergeInto: '合并到 {environment}',
    replaceInto: '替换 {environment}',
    rename: '重命名',
    restore: '恢复',
    restoreRevision: '恢复 v{revision}',
    updated: '更新时间',
    unarchive: '取消归档',
    vault: 'Vault',
    vaultBody: '版本化 Item 将完整 Field 集合组织为绑定 Environment 的 Variant。',
    reveal: '显示',
    hide: '隐藏',
    revealItemValues: '显示 Item 值',
    valueAccessWarning: '值可能包含明文 Secret 和文件；显示后会产生一条访问事件。',
    variant: 'Variant',
    variants: 'Variants',
    vaultRevisions: 'Vault Revision',
    versionIdentity: '版本身份',
    action: '操作',
    actor: '操作者',
    applicationLogs: '访问 / 审计日志',
    attempt: '尝试次数',
    authentication: '认证方式',
    caVerifiedCertificates: '经 CA 验证的公钥证书',
    configView: 'Config 视图',
    currentState: '当前状态',
    currentRevisionLabel: '当前 Revision v{revision}',
    delivery: '投递',
    endpoint: 'Endpoint',
    error: '错误',
    expiresAt: '过期时间',
    fingerprint: '指纹 SHA-256',
    format: '格式',
    formatSource: '格式化原文',
    formatComplete: '原文已格式化。',
    validationFailed: '原文无效，未执行格式化。',
    missing_vault_item: 'Vault Item 不存在',
    missing_vault_field: 'Vault Field 不存在',
    missing_vault_environment_variant: '没有匹配的 Environment Variant',
    invalidSnapshot: 'Snapshot JSON 无效。',
    latencyMS: '延迟 ms',
    locale: 'zh-CN',
    managementSingleReplica: 'Management 保持单副本',
    observedReads: '已观测读取',
    nonBlockingAccessEvents: '非阻塞 Access 事件',
    operation: '操作 ID',
    operationFailed: '操作失败，请检查输入后重试。',
    outcome: '结果',
    prefix: '前缀',
    primaryNavigation: '主导航',
    principal: '主体',
    readOnlyContract: '只读 V1 合约',
    reference: '引用',
    referenceKey: '引用键',
    resource: '资源',
    systemStatus: '系统状态',
    restartToApply: '重启后生效',
    revisionHistory: 'Revision 历史',
    revisionLabel: 'Revision v{revision}',
    serial: '序列号',
    status: '状态',
    subject: 'Subject',
    time: '时间',
    tokenEnvironmentGrant: 'Token + Environment 授权',
    validUntil: '有效期至',
    value: '值',
    valueFor: '{field} 的值',
    vaultView: 'Vault 视图',
    notFoundTitle: '页面不存在',
    notFoundBody: 'Configra V1 中不存在该路径。',
  },
};

const navigation = [
  ['overview', 'overview'],
  ['environments', 'environments'],
  ['configs', 'configs'],
  ['vault', 'vault'],
  ['notifications', 'notifications'],
  ['access', 'access'],
  ['audit', 'audit'],
  ['administration', 'administration'],
];

const iconPaths = {
  overview: 'M4 4h6v6H4zM14 4h6v10h-6zM4 14h6v6H4zM14 18h6v2h-6z',
  environments: 'M12 3v5m0 0-6 4m6-4 6 4M6 12v5m12-5v5M3 17h6v4H3zM15 17h6v4h-6z',
  configs: 'M6 3h9l4 4v14H6zM15 3v5h4M9 12h7M9 16h7',
  vault: 'M12 3 5 6v5c0 4.7 2.8 8.1 7 10 4.2-1.9 7-5.3 7-10V6zM9 12h6M12 9v6',
  notifications: 'M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4',
  access: 'M14 8a4 4 0 1 1-7.7 1.5L3 13v4h4v-2h2v-2h2l1.2-1.2A4 4 0 0 1 14 8z',
  audit: 'M5 4h14v17H5zM8 8h8M8 12h8M8 16h5',
  administration: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8m0-5v2m0 14v2M3 12h2m14 0h2M5.6 5.6 7 7m10 10 1.4 1.4M18.4 5.6 17 7M7 17l-1.4 1.4',
  search: 'M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14m5.5 12.5L21 21',
  eye: 'M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6S2 12 2 12zM12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6',
  eyeOff: 'M2 12s3.5-6 10-6c2.2 0 4.1.7 5.6 1.7M22 12s-3.5 6-10 6c-2.2 0-4.1-.7-5.6-1.7M3 3l18 18',
  sun: 'M12 3v2m0 14v2M3 12h2m14 0h2M5.6 5.6 7 7m10 10 1.4 1.4M18.4 5.6 17 7M7 17l-1.4 1.4M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8',
  moon: 'M20 15.2A8 8 0 0 1 8.8 4 8.2 8.2 0 1 0 20 15.2z',
};

async function request(path, options = {}) {
  const response = await fetch(path, {
    ...options,
    headers: { Accept: 'application/json', ...options.headers },
  });
  if (!response.ok) {
    const error = new Error(`request failed: ${response.status}`);
    error.status = response.status;
    try {
      const failure = (await response.json()).error;
      error.code = failure?.code;
      error.requestID = failure?.request_id;
    } catch {
      // The HTTP status remains enough for non-JSON failures.
    }
    throw error;
  }
  return response.status === 204 ? null : response.json();
}

function managementError(error, t, fallback) {
  return { message: t[error?.code] || fallback, requestID: error?.requestID || '' };
}

function ActionError({ error, t }) {
  if (!error) return null;
  return <span className="action-error" role="alert"><span>{error.message}</span>{error.requestID && <button type="button" aria-label={`${t.copyRequestID} ${error.requestID}`} onClick={() => navigator.clipboard.writeText(error.requestID).catch(() => {})}><span>{t.requestID}</span><code>{error.requestID}</code></button>}</span>;
}

function useUnsavedChanges(dirty, message) {
  const allowingHashChange = useRef(false);
  useEffect(() => {
    if (!dirty) return undefined;
    const editorURL = window.location.href;
    const unload = event => {
      event.preventDefault();
      event.returnValue = '';
    };
    const followLink = event => {
      if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      const link = event.target.closest?.('a[href]');
      if (!link) return;
      if (!window.confirm(message)) return event.preventDefault();
      const destination = new URL(link.href, window.location.href);
      allowingHashChange.current = destination.origin === window.location.origin
        && destination.pathname === window.location.pathname
        && destination.search === window.location.search
        && destination.hash !== window.location.hash;
    };
    const changeHash = event => {
      if (allowingHashChange.current) {
        allowingHashChange.current = false;
        return;
      }
      if (window.confirm(message)) return;
      allowingHashChange.current = true;
      window.history.pushState(window.history.state, '', event.oldURL);
      window.dispatchEvent(new HashChangeEvent('hashchange', { oldURL: event.newURL, newURL: event.oldURL }));
    };
    const traverseHistory = () => {
      const leave = window.confirm(message);
      allowingHashChange.current = true;
      if (!leave) window.history.pushState(window.history.state, '', editorURL);
      window.setTimeout(() => { allowingHashChange.current = false; }, 0);
    };
    window.addEventListener('beforeunload', unload);
    window.addEventListener('popstate', traverseHistory, true);
    window.addEventListener('hashchange', changeHash, true);
    document.addEventListener('click', followLink, true);
    return () => {
      window.removeEventListener('beforeunload', unload);
      window.removeEventListener('popstate', traverseHistory, true);
      window.removeEventListener('hashchange', changeHash, true);
      document.removeEventListener('click', followLink, true);
    };
  }, [dirty, message]);
  return () => !dirty || window.confirm(message);
}

function useRoute() {
  const read = () => {
    const [path, search = ''] = window.location.hash.slice(2).split('?');
    const segments = path.split('/').filter(Boolean);
    const candidate = segments[0] || 'overview';
    const section = navigation.some(([key]) => key === candidate) ? candidate : 'overview';
    return { section, segments: section === candidate ? segments : ['overview'], query: new URLSearchParams(search) };
  };
  const [route, setRoute] = useState(read);
  useEffect(() => {
    const changed = () => setRoute(read());
    window.addEventListener('hashchange', changed);
    return () => window.removeEventListener('hashchange', changed);
  }, []);
  return route;
}

function replaceHashQuery(changes) {
  const [path, search = ''] = window.location.hash.slice(2).split('?');
  const query = new URLSearchParams(search);
  for (const [key, value] of Object.entries(changes)) {
    if (value === '' || value == null || value === 1 || value === 'all') query.delete(key);
    else query.set(key, String(value));
  }
  const suffix = query.size ? `?${query}` : '';
  window.history.replaceState(null, '', `${window.location.pathname}${window.location.search}#/${path}${suffix}`);
}

function Mark({ small = false }) {
  return (
    <span className={small ? 'mark mark-small' : 'mark'} aria-hidden="true">
      <span>C</span>
    </span>
  );
}

function Icon({ name }) {
  return <svg className="icon" viewBox="0 0 24 24" aria-hidden="true"><path d={iconPaths[name]} /></svg>;
}

function LanguageButton({ language, setLanguage, t }) {
  return (
    <button className="language-button" type="button" onClick={() => setLanguage(language === 'en' ? 'zh' : 'en')}>
      <span aria-hidden="true">文</span>
      {t.language}
    </button>
  );
}

function ThemeButton({ theme, setTheme, t }) {
  const nextTheme = theme === 'dark' ? 'light' : 'dark';
  const label = nextTheme === 'dark' ? t.useDarkTheme : t.useLightTheme;
  return (
    <button className="icon-action theme-button" type="button" aria-label={label} title={label} onClick={() => setTheme(nextTheme)}>
      <Icon name={theme === 'dark' ? 'sun' : 'moon'} />
    </button>
  );
}

function Login({ language, setLanguage, theme, setTheme, t }) {
  return (
    <main className="login-layout">
      <div className="login-topline">
        <a className="wordmark" href="/" aria-label={`Configra ${t.overview}`}><Mark />Configra</a>
        <div className="login-top-actions">
          <ThemeButton theme={theme} setTheme={setTheme} t={t} />
          <LanguageButton language={language} setLanguage={setLanguage} t={t} />
        </div>
      </div>
      <section className="login-panel">
        <div className="login-copy">
          <p className="eyebrow">{t.loginEyebrow}</p>
          <h1>{t.loginTitle}</h1>
          <p>{t.loginBody}</p>
          <div className="login-action">
            <a className="primary-action" href="/auth/login?return_to=/">{t.loginButton}<span aria-hidden="true">→</span></a>
            <small>{t.loginAuthHint}</small>
          </div>
        </div>
        <div className="login-resolution" aria-hidden="true">
          <section className="login-source">
            <header><span>{t.storedConfiguration}</span><code>production / payment @ v19</code></header>
            <div className="login-code">
              <span><b>database</b>:</span>
              <span className="code-indent"><b>host</b>: <i>mysql.production</i></span>
              <span className="code-indent"><b>username</b>: <i>{'{vault.platform.mysql.username}'}</i></span>
              <span className="code-indent code-reference"><b>password</b>: <i>{'{vault.platform.mysql.password}'}</i></span>
            </div>
          </section>
          <div className="login-resolve"><span>{t.resolvedAtRead}</span><i /><b>→</b></div>
          <section className="login-delivery">
            <header><span>{t.deliveredConfiguration}</span><code>200 OK</code></header>
            <div className="login-code delivered-code">
              <span><b>database</b>:</span>
              <span className="code-indent"><b>host</b>: <i>mysql.production</i></span>
              <span className="code-indent"><b>username</b>: <i>payment_user</i></span>
              <span className="code-indent"><b>password</b>: <i>••••••••••••</i></span>
            </div>
            <footer><span>{t.revisionEvidence}</span><code>config v19</code><code>platform.mysql v8</code></footer>
          </section>
        </div>
      </section>
    </main>
  );
}

function Loading() {
  return <main className="center-state"><Mark /><span className="loading-line" /></main>;
}

function Failure({ retry, t }) {
  return (
    <main className="center-state error-state">
      <Mark />
      <h1>{t.loadFailed}</h1>
      <button className="secondary-action" type="button" onClick={retry}>{t.retry}</button>
    </main>
  );
}

function Shell({ principal, language, setLanguage, theme, setTheme, t }) {
  const currentRoute = useRoute();
  const route = currentRoute.section;
  const [inventory, setInventory] = useState({ status: 'loading' });
  const [search, setSearch] = useState('');
  const visibleNavigation = principal.role === 'admin' ? navigation : navigation.filter(([key]) => key !== 'notifications' && key !== 'administration');
  useEffect(() => {
    let live = true;
    Promise.all([request('/v1/environments'), request('/v1/configs'), request('/v1/vault-items')])
      .then(([environments, configs, vault]) => live && setInventory({
        status: 'ready', environments: environments.items, configs: configs.items, vault: vault.items,
      }))
      .catch(() => live && setInventory({ status: 'failed' }));
    return () => { live = false; };
  }, []);
  const searchResults = useMemo(() => {
    const needle = search.trim().toLowerCase();
    if (!needle || inventory.status !== 'ready') return [];
    return [
      ...inventory.configs.map(item => ({ type: 'CFG', key: item.key, name: item.display_name, href: `#/configs/${item.key}` })),
      ...inventory.vault.map(item => ({ type: 'VLT', key: `${item.namespace_key}.${item.key}`, name: item.display_name, href: `#/vault/${item.namespace_key}/${item.key}` })),
      ...inventory.environments.map(item => ({ type: 'ENV', key: item.key, name: item.display_name, href: `#/environments/${item.key}` })),
    ].filter(item => item.key.toLowerCase().includes(needle) || item.name.toLowerCase().includes(needle)).slice(0, 7);
  }, [inventory, search]);

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a className="wordmark" href="#/overview" aria-label={`Configra ${t.overview}`}><Mark />Configra</a>
        <nav aria-label={t.primaryNavigation}>
          {visibleNavigation.map(([key, icon]) => (
            <a key={key} href={`#/${key}`} aria-current={route === key ? 'page' : undefined}>
              <Icon name={icon} /><span>{t[key]}</span>
            </a>
          ))}
        </nav>
        <div className="identity">
          <span className="avatar">{(principal.email || principal.subject).slice(0, 1).toUpperCase()}</span>
          <span title={principal.email || principal.subject}><strong>{principal.email || principal.subject}</strong><small>{principal.role === 'admin' ? t.roleAdmin : t.roleViewer}</small></span>
        </div>
      </aside>
      <main className="workspace">
        <header className="topbar">
          <div className="global-search">
            <Icon name="search" />
            <input type="search" aria-label={t.globalSearch} placeholder={t.globalSearchPlaceholder} value={search} onChange={event => setSearch(event.target.value)} />
            {search.trim() && <div className="global-search-results">
              {searchResults.map(item => <a key={`${item.type}-${item.key}`} href={item.href} onClick={() => setSearch('')}><span>{item.type}</span><strong title={item.name}>{item.name}</strong><code title={item.key}>{item.key}</code></a>)}
              {searchResults.length === 0 && <p>{t.noResources}</p>}
            </div>}
          </div>
          <div className="topbar-actions">
            <ThemeButton theme={theme} setTheme={setTheme} t={t} />
            <LanguageButton language={language} setLanguage={setLanguage} t={t} />
            <form action="/auth/logout" method="post"><button className="logout-button" type="submit">{t.signOut}</button></form>
          </div>
        </header>
        {route === 'overview' && <Overview inventory={inventory} t={t} />}
        {route === 'environments' && currentRoute.segments.length >= 2
          ? <EnvironmentDetail environmentKey={currentRoute.segments[1]} t={t} />
          : route === 'environments' && <Environments principal={principal} t={t} />}
        {route === 'configs' && currentRoute.segments.length >= 3
          ? <ConfigDetail principal={principal} configKey={currentRoute.segments[1]} environmentKey={currentRoute.segments[2]} inventory={inventory} t={t} />
          : route === 'configs' && currentRoute.segments.length === 2
            ? <ConfigHome configKey={currentRoute.segments[1]} t={t} />
            : route === 'configs' && <Configs principal={principal} filters={currentRoute.query} t={t} />}
        {route === 'vault' && currentRoute.segments.length >= 3
          ? <VaultDetail principal={principal} namespaceKey={currentRoute.segments[1]} itemKey={currentRoute.segments[2]} t={t} />
          : route === 'vault' && <VaultItems principal={principal} filters={currentRoute.query} t={t} />}
        {route === 'administration' && principal.role === 'admin' && <Administration t={t} />}
        {route === 'notifications' && principal.role === 'admin' && <Notifications t={t} />}
        {principal.role !== 'admin' && ['administration', 'notifications'].includes(route) && <Forbidden t={t} />}
        {route === 'access' && <AccessPage t={t} />}
        {route === 'audit' && <AuditPage principal={principal} t={t} />}
        {!['overview', 'environments', 'configs', 'vault', 'administration', 'notifications', 'access', 'audit'].includes(route) && <Pending route={route} t={t} />}
      </main>
    </div>
  );
}

function Overview({ inventory, t }) {
  const [operations, setOperations] = useState({ audit: 'loading', audits: [], ready: null });
  useEffect(() => {
    let live = true;
    Promise.allSettled([request('/v1/audit?limit=8'), request('/health/ready')]).then(([audit, ready]) => {
      if (!live) return;
      setOperations({
        audit: audit.status === 'fulfilled' ? 'ready' : 'failed',
        audits: audit.status === 'fulfilled' ? audit.value.items : [],
        ready: ready.status === 'fulfilled',
      });
    });
    return () => { live = false; };
  }, []);
  const recentChanges = useMemo(() => {
    const seen = new Set();
    return operations.audits.filter(record => {
      if (seen.has(record.id)) return false;
      seen.add(record.id);
      return true;
    }).slice(0, 6);
  }, [operations.audits]);
  const gaps = inventory.status === 'ready'
    ? inventory.configs.filter(item => !(item.environments || []).some(environment => !environment.archived))
    : [];

  return (
    <div className="page overview-page">
      <div className="page-heading"><div><h1>{t.overview}</h1><p>{t.overviewBody}</p></div></div>
      <div className="overview-layout">
        <div className="overview-main">
          <section className="panel overview-status" aria-labelledby="system-status-title">
            <div className="section-heading"><h2 id="system-status-title">{t.systemStatus}</h2><span>{t.observedNow}</span></div>
            <div className="overview-status-grid">
              <article><span>MySQL</span><strong className={operations.ready === false ? 'health-bad' : 'health-good'}>{operations.ready === null ? '…' : operations.ready ? t.available : t.unavailable}</strong><small>/health/ready</small></article>
              <article><span>ClickHouse</span><strong className={operations.audit === 'ready' ? 'health-good' : operations.audit === 'loading' ? '' : 'health-bad'}>{operations.audit === 'loading' ? '…' : operations.audit === 'ready' ? t.available : t.unavailable}</strong><small>{t.audit}</small></article>
            </div>
          </section>
          <section className="panel overview-change-panel" aria-labelledby="recent-changes-title">
            <div className="section-heading"><h2 id="recent-changes-title">{t.recentChanges}</h2><a href="#/audit">{t.audit}</a></div>
            {operations.audit === 'loading' && <p className="overview-empty">…</p>}
            {operations.audit === 'failed' && <p className="overview-empty health-bad">{t.unavailable}</p>}
            {operations.audit === 'ready' && recentChanges.length === 0 && <p className="overview-empty">{t.noRecentChanges}</p>}
            {recentChanges.length > 0 && <div className="table-frame"><table className="overview-change-table"><thead><tr><th>{t.time}</th><th>{t.action}</th><th>{t.resource}</th><th>{t.revision}</th><th>{t.outcome}</th></tr></thead><tbody>
              {recentChanges.map(record => {
                const identity = record.namespace ? `${record.namespace}.${record.resource}` : record.environment && record.resource_type === 'config' ? `${record.environment}/${record.resource}` : record.resource;
                const href = record.resource_type === 'config' && record.environment ? `#/configs/${record.resource}/${record.environment}` : record.resource_type === 'vault_item' && record.namespace ? `#/vault/${record.namespace}/${record.resource}` : '#/audit';
                return <tr key={record.id}><td><time>{formatDate(record.time, t.locale)}</time></td><td><code>{record.action}</code></td><td><span>{record.resource_type}</span><a href={href}><code>{identity}</code></a></td><td>{record.revision ? `v${record.revision}` : '—'}</td><td><span className={record.outcome === 'success' ? 'status active' : 'status archived'}>{record.outcome}</span></td></tr>;
              })}
            </tbody></table></div>}
          </section>
        </div>
        <aside className="overview-side">
          <section className="panel overview-inventory">
            <div className="section-heading"><h2>{t.inventory}</h2></div>
            <nav aria-label={t.inventory}><a href="#/environments"><span>{t.environments}</span><strong>{inventory.status === 'ready' ? inventory.environments.length : '—'}</strong></a><a href="#/configs"><span>{t.configs}</span><strong>{inventory.status === 'ready' ? inventory.configs.length : '—'}</strong></a><a href="#/vault"><span>{t.vault}</span><strong>{inventory.status === 'ready' ? inventory.vault.length : '—'}</strong></a></nav>
          </section>
          <section className="panel overview-gaps">
            <div className="section-heading"><h2>{t.configurationGaps}</h2><span>{gaps.length}</span></div>
            {gaps.length === 0 ? <p>{t.noConfigurationGaps}</p> : gaps.slice(0, 6).map(item => <a href={`#/configs/${item.key}`} key={item.key}><code>{item.key}</code><span>{t.noActiveConfigContext}</span></a>)}
          </section>
        </aside>
      </div>
    </div>
  );
}

function Environments({ principal, t }) {
  const [state, setState] = useState({ status: 'loading', items: [] });
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState('');
  const [confirming, setConfirming] = useState('');
  const [refresh, setRefresh] = useState(0);
  const [mutationError, setMutationError] = useState('');
  useEffect(() => {
    let live = true;
    request('/v1/environments?include_archived=true')
      .then(result => live && setState({ status: 'ready', items: result.items }))
      .catch(() => live && setState({ status: 'failed', items: [] }));
    return () => { live = false; };
  }, [refresh]);

  const create = async event => {
    event.preventDefault();
    setMutationError('');
    const data = new FormData(event.currentTarget);
    try {
      await request('/v1/environments', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({ key: data.get('key'), display_name: data.get('display_name') }),
      });
      setCreating(false);
      setRefresh(value => value + 1);
    } catch {
      setMutationError(t.operationFailed);
    }
  };
  const rename = async event => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    try {
      await request(`/v1/environments/${encodeURIComponent(editing)}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({ display_name: data.get('display_name') }),
      });
      setEditing('');
      setRefresh(value => value + 1);
    } catch {
      setMutationError(t.operationFailed);
    }
  };
  const changeLifecycle = async (environment, action) => {
    try {
      await request(`/v1/environments/${encodeURIComponent(environment.key)}/${action}`, { method: 'POST', headers: { 'Idempotency-Key': crypto.randomUUID() } });
      setConfirming('');
      setRefresh(value => value + 1);
    } catch {
      setMutationError(t.operationFailed);
    }
  };

  return (
    <ResourcePage eyebrow="ENV / peer contexts" title={t.environments} body={t.environmentBody}
      action={principal.role === 'admin' && <button className="primary-action compact-action" type="button" onClick={() => setCreating(value => !value)}>{t.newEnvironment}</button>}>
      {creating && (
        <form className="create-panel" onSubmit={create}>
          <label>{t.resourceKey}<input name="key" required maxLength="63" pattern="[a-z][a-z0-9_-]{0,62}" autoComplete="off" /><small>{t.resourceKeyHint}</small></label>
          <label>{t.displayName}<input name="display_name" required maxLength="255" autoComplete="off" /></label>
          <button className="primary-action compact-action" type="submit">{t.createEnvironment}</button>
          {mutationError && <span className="inline-error" role="alert">{mutationError}</span>}
        </form>
      )}
      {(!creating || state.items.length > 0) && <CollectionState state={state} t={t}>
        <div className="table-frame environment-index-table"><table>
          <thead><tr><th>{t.environment}</th><th>{t.updated}</th><th>{t.status}</th>{principal.role === 'admin' && <th />}</tr></thead>
          <tbody>{state.items.map(environment => <tr key={environment.key}>
            <td>{editing === environment.key
              ? <form className="environment-inline-rename" onSubmit={rename}><code>{environment.key}</code><label>{t.displayName}<input name="display_name" required maxLength="255" defaultValue={environment.display_name} autoComplete="off" /></label><button type="submit">{t.saveName}</button><button type="button" onClick={() => setEditing('')}>{t.cancel}</button></form>
              : <a className="environment-resource-link" href={`#/environments/${environment.key}`} aria-label={`${t.openEnvironment} ${environment.display_name}`}><code>{environment.key}</code><h2 title={environment.display_name}>{environment.display_name}</h2></a>}</td>
            <td><time>{formatDate(environment.updated_at, t.locale)}</time></td>
            <td><span className={environment.archived ? 'status archived' : 'status active'}>{environment.archived ? t.statusArchived : t.statusActive}</span></td>
            {principal.role === 'admin' && <td>{editing !== environment.key && <div className="row-actions">
              {!environment.archived && <button type="button" aria-label={`${t.rename} ${environment.display_name}`} onClick={() => setEditing(environment.key)}>{t.rename}</button>}
              {environment.archived
                ? <button type="button" aria-label={`${t.unarchive} ${environment.display_name}`} onClick={() => changeLifecycle(environment, 'unarchive')}>{t.unarchive}</button>
                : confirming === environment.key
                  ? <button className="danger-link" type="button" aria-label={`${t.confirmArchive} ${environment.display_name}`} onClick={() => changeLifecycle(environment, 'archive')}>{t.confirmArchive}</button>
                  : <button className="danger-link" type="button" aria-label={`${t.archive} ${environment.display_name}`} onClick={() => setConfirming(environment.key)}>{t.archive}</button>}
            </div>}</td>}
          </tr>)}</tbody>
        </table></div>
      </CollectionState>}
      {!creating && mutationError && <p className="inline-error" role="alert">{mutationError}</p>}
    </ResourcePage>
  );
}

function EnvironmentDetail({ environmentKey, t }) {
  const [state, setState] = useState({ status: 'loading' });
  useEffect(() => {
    let live = true;
    Promise.all([
      request('/v1/environments?include_archived=true'),
      request('/v1/configs?include_archived=true'),
      request('/v1/vault-items?include_archived=true'),
    ])
      .then(([environments, configs, vault]) => live && setState({
        status: 'ready',
        environment: environments.items.find(item => item.key === environmentKey),
        configs: configs.items,
        vault: vault.items,
      }))
      .catch(() => live && setState({ status: 'failed' }));
    return () => { live = false; };
  }, [environmentKey]);

  if (state.status === 'loading') return <div className="page collection-state"><span className="loading-line" /></div>;
  if (state.status === 'failed') return <div className="page collection-state"><p>{t.loadFailed}</p></div>;
  if (!state.environment) return <Pending route={`environments/${environmentKey}`} t={t} />;

  const configs = state.configs.flatMap(config => {
    const context = config.environments.find(environment => environment.key === environmentKey);
    return context ? [{ ...config, context }] : [];
  });
  const vault = state.vault.filter(item => (item.environment_keys || []).includes(environmentKey));
  const environment = state.environment;
  return <ResourcePage className="environment-detail-page" eyebrow={`ENV / ${environment.key}`} title={environment.display_name} body={t.environmentDetailBody}
    action={<span className={environment.archived ? 'status archived' : 'status active'}>{environment.archived ? t.statusArchived : t.statusActive}</span>}>
    <dl className="environment-summary"><div><dt>{t.resourceKey}</dt><dd><code>{environment.key}</code></dd></div><div><dt>{t.configs}</dt><dd>{configs.length}</dd></div><div><dt>{t.vault}</dt><dd>{vault.length}</dd></div><div><dt>{t.updated}</dt><dd><time>{formatDate(environment.updated_at, t.locale)}</time></dd></div></dl>
    <div className="environment-resource-columns">
      <section className="panel environment-resource-section">
        <div className="section-heading"><h2>{t.configsInEnvironment}</h2><a href={`#/configs?environment=${encodeURIComponent(environment.key)}`} aria-label={t.viewAllConfigsIn.replace('{environment}', environment.display_name)}>{t.viewAllConfigsIn.replace('{environment}', environment.display_name)}</a></div>
        {configs.length === 0 ? <p className="environment-empty">{t.noConfigsInEnvironment}</p> : <div className="table-frame"><table><thead><tr><th>{t.configIdentity}</th><th>{t.revision}</th><th>{t.status}</th></tr></thead><tbody>{configs.map(config => <tr key={config.key}><td><a href={`#/configs/${config.key}/${environment.key}`} aria-label={`${config.display_name} v${config.context.revision}`}><code>{config.key}</code><strong title={config.display_name}>{config.display_name}</strong></a></td><td><strong>v{config.context.revision}</strong></td><td><span className={config.archived || config.context.archived ? 'status archived' : 'status active'}>{config.archived || config.context.archived ? t.statusArchived : t.statusActive}</span></td></tr>)}</tbody></table></div>}
      </section>
      <section className="panel environment-resource-section">
        <div className="section-heading"><h2>{t.vaultInEnvironment}</h2><a href={`#/vault?environment=${encodeURIComponent(environment.key)}`} aria-label={t.viewAllVaultIn.replace('{environment}', environment.display_name)}>{t.viewAllVaultIn.replace('{environment}', environment.display_name)}</a></div>
        {vault.length === 0 ? <p className="environment-empty">{t.noVaultInEnvironment}</p> : <div className="table-frame"><table><thead><tr><th>{t.resource}</th><th>{t.revision}</th><th>{t.status}</th></tr></thead><tbody>{vault.map(item => <tr key={`${item.namespace_key}.${item.key}`}><td><a href={`#/vault/${item.namespace_key}/${item.key}`} aria-label={`${item.namespace_key}.${item.key} v${item.revision}`}><code>{item.namespace_key}.{item.key}</code><strong title={item.display_name}>{item.display_name}</strong></a></td><td><strong>v{item.revision}</strong></td><td><span className={item.archived ? 'status archived' : 'status active'}>{item.archived ? t.statusArchived : t.statusActive}</span></td></tr>)}</tbody></table></div>}
      </section>
    </div>
  </ResourcePage>;
}

function Configs({ principal, filters = new URLSearchParams(), t }) {
  const [state, setState] = useState({ status: 'loading', items: [], environments: [] });
  const [creating, setCreating] = useState(false);
  const [confirming, setConfirming] = useState('');
  const [error, setError] = useState('');
  const [warnings, setWarnings] = useState([]);
  const [refresh, setRefresh] = useState(0);
  const [query, setQuery] = useState(() => filters.get('q') || '');
  const [statusFilter, setStatusFilter] = useState(() => filters.get('status') || 'all');
  const [environmentFilter, setEnvironmentFilter] = useState(() => filters.get('environment') || '');
  const [page, setPage] = useState(() => Math.max(1, Number(filters.get('page')) || 1));
  const [createFormat, setCreateFormat] = useState('yaml');
  const [createContent, setCreateContent] = useState('');
  useEffect(() => {
    let live = true;
    Promise.all([
      request('/v1/configs?include_archived=true'),
      principal.role === 'admin' ? request('/v1/environments') : Promise.resolve({ items: [] }),
    ])
      .then(([configs, environments]) => live && setState({ status: 'ready', items: configs.items, environments: environments.items }))
      .catch(() => live && setState({ status: 'failed', items: [], environments: [] }));
    return () => { live = false; };
  }, [principal.role, refresh]);
  const filterKey = filters.toString();
  useEffect(() => {
    setQuery(filters.get('q') || '');
    setStatusFilter(filters.get('status') || 'all');
    setEnvironmentFilter(filters.get('environment') || '');
    setPage(Math.max(1, Number(filters.get('page')) || 1));
  }, [filterKey]);
  const create = async event => {
    event.preventDefault();
    setError('');
    const data = new FormData(event.currentTarget);
    if (!createContent.trim()) return setError(t.saveFailed);
    try {
      const result = await request(`/v1/environments/${encodeURIComponent(data.get('environment'))}/configs/${encodeURIComponent(data.get('key'))}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({ name: data.get('display_name'), expected_revision: 0, format: createFormat, content: createContent }),
      });
      setWarnings(result.warnings || []);
      setCreating(false);
      setCreateContent('');
      setRefresh(value => value + 1);
    } catch {
      setError(t.saveFailed);
    }
  };
  const changeLifecycle = async (config, action) => {
    try {
      await request(`/v1/configs/${encodeURIComponent(config.key)}/${action}`, { method: 'POST', headers: { 'Idempotency-Key': crypto.randomUUID() } });
      setConfirming('');
      setRefresh(value => value + 1);
    } catch {
      setError(t.operationFailed);
    }
  };
  const filteredItems = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return state.items.filter(config => {
      if (statusFilter === 'active' && config.archived) return false;
      if (statusFilter === 'archived' && !config.archived) return false;
      if (environmentFilter && !config.environments.some(environment => environment.key === environmentFilter)) return false;
      return !needle || config.key.toLowerCase().includes(needle) || config.display_name.toLowerCase().includes(needle)
        || config.environments.some(environment => environment.key.toLowerCase().includes(needle));
    });
  }, [environmentFilter, query, state.items, statusFilter]);
  const environmentKeys = [...new Set(state.items.flatMap(config => config.environments.map(environment => environment.key)))].sort();
  const pageSize = 50;
  const totalPages = Math.max(1, Math.ceil(filteredItems.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const firstIndex = (currentPage - 1) * pageSize;
  const pageItems = filteredItems.slice(firstIndex, firstIndex + pageSize);
  return (
    <ResourcePage className="config-index-page" eyebrow="CFG / canonical YAML + JSON" title={t.configs} body={t.configBody}
      action={principal.role === 'admin' && <button className="primary-action compact-action" type="button" onClick={() => { setCreating(value => !value); setCreateFormat('yaml'); setCreateContent(''); setWarnings([]); }}>{t.newConfig}</button>}>
      {creating && <form className="config-create-form" onSubmit={create}>
        <label>{t.environment}<select name="environment" required>{state.environments.map(environment => <option key={environment.key} value={environment.key}>{environment.display_name} · {environment.key}</option>)}</select></label>
        <label>{t.resourceKey}<input name="key" required maxLength="63" pattern="[a-z][a-z0-9_-]{0,62}" autoComplete="off" /></label>
        <label>{t.displayName}<input name="display_name" required maxLength="255" autoComplete="off" /></label>
        <label>{t.format}<select name="format" value={createFormat} onChange={event => setCreateFormat(event.target.value)}><option value="yaml">YAML</option><option value="json">JSON</option></select></label>
        <div className="form-source"><span>{t.configurationSource}</span><ConfigCodeEditor label={t.configurationSource} value={createContent} format={createFormat} readOnly={false} onChange={setCreateContent} /></div>
        <div className="form-actions"><span className="inline-error" role="alert">{error}</span><button className="primary-action compact-action" type="submit" disabled={!createContent.trim()}>{t.createConfig}</button></div>
      </form>}
      {(!creating || state.items.length > 0) && <CollectionState state={state} t={t}>
        <div className="config-index">
          <div className="config-index-toolbar">
            <label><span>{t.searchConfigs}</span><input type="search" value={query} placeholder={t.searchConfigsPlaceholder} onChange={event => { setQuery(event.target.value); setPage(1); replaceHashQuery({ q: event.target.value, page: 1 }); }} /></label>
            <label><span>{t.environment}</span><select aria-label={t.environmentFilter} value={environmentFilter} onChange={event => { setEnvironmentFilter(event.target.value); setPage(1); replaceHashQuery({ environment: event.target.value, page: 1 }); }}><option value="">{t.allEnvironments}</option>{environmentKeys.map(environment => <option key={environment} value={environment}>{environment}</option>)}</select></label>
            <label><span>{t.status}</span><select value={statusFilter} onChange={event => { setStatusFilter(event.target.value); setPage(1); replaceHashQuery({ status: event.target.value, page: 1 }); }}><option value="all">{t.allStatuses}</option><option value="active">{t.statusActive}</option><option value="archived">{t.statusArchived}</option></select></label>
            <output>{filteredItems.length} {t.configs}</output>
          </div>
          {pageItems.length === 0 ? <p className="config-index-empty">{t.noMatchingConfigs}</p> : <div className="table-frame config-index-frame">
            <table className="config-index-table">
              <thead><tr><th>{t.configIdentity}</th><th>{t.environmentRevisions}</th><th>{t.updated}</th><th>{t.status}</th>{principal.role === 'admin' && <th />}</tr></thead>
              <tbody>{pageItems.map(config => (
                <tr key={config.key}>
                  <td><a className="config-resource-link" href={`#/configs/${config.key}`} aria-label={`${t.openConfig} ${config.display_name}`}><code title={config.key}>{config.key}</code><strong title={config.display_name}>{config.display_name}</strong></a></td>
                  <td><div className="environment-links config-environment-links">{config.environments.slice(0, 3).map(environment => {
                    const label = `${environment.key} v${environment.revision}${environment.archived ? ` ${t.statusArchived}` : ''}`;
                    const identity = <><code>{environment.key}</code><span>v{environment.revision}</span>{environment.archived && <i>{t.statusArchived}</i>}</>;
                    return config.archived || environment.archived
                      ? <span key={environment.key} aria-label={label}>{identity}</span>
                      : <a key={environment.key} href={`#/configs/${config.key}/${environment.key}`} aria-label={label}>{identity}</a>;
                  })}{config.environments.length > 3 && <span aria-label={`${config.environments.length - 3} ${t.moreContexts}`}>+{config.environments.length - 3}</span>}</div></td>
                  <td><time>{formatDate(config.updated_at, t.locale)}</time></td>
                  <td><span className={config.archived ? 'status archived' : 'status active'}>{config.archived ? t.statusArchived : t.statusActive}</span></td>
                  {principal.role === 'admin' && <td>{config.archived
                    ? <button type="button" aria-label={`${t.unarchive} ${config.display_name}`} onClick={() => changeLifecycle(config, 'unarchive')}>{t.unarchive}</button>
                    : confirming === config.key
                      ? <button className="danger-link" type="button" aria-label={`${t.confirmArchive} ${config.display_name}`} onClick={() => changeLifecycle(config, 'archive')}>{t.confirmArchive}</button>
                      : <button className="danger-link" type="button" aria-label={`${t.archive} ${config.display_name}`} onClick={() => setConfirming(config.key)}>{t.archive}</button>}</td>}
                </tr>
              ))}</tbody>
            </table>
          </div>}
          <nav className="config-pagination" aria-label={t.pagination}>
            <span>{filteredItems.length === 0 ? 0 : firstIndex + 1}–{Math.min(firstIndex + pageSize, filteredItems.length)} / {filteredItems.length}</span>
            <div><button type="button" disabled={currentPage === 1} onClick={() => { setPage(currentPage - 1); replaceHashQuery({ page: currentPage - 1 }); }}>{t.previous}</button><code>{currentPage} / {totalPages}</code><button type="button" disabled={currentPage === totalPages} onClick={() => { setPage(currentPage + 1); replaceHashQuery({ page: currentPage + 1 }); }}>{t.next}</button></div>
          </nav>
        </div>
      </CollectionState>}
      {warnings.length > 0 && <div className="config-warning-list" role="status"><ConfigWarningLines warnings={warnings} t={t} /></div>}
      {!creating && error && <p className="inline-error" role="alert">{error}</p>}
    </ResourcePage>
  );
}

function ConfigHome({ configKey, t }) {
  const [state, setState] = useState({ status: 'loading' });
  useEffect(() => {
    let live = true;
    Promise.all([request('/v1/configs?include_archived=true'), request('/v1/environments?include_archived=true')])
      .then(([configs, environments]) => {
        if (!live) return;
        const item = configs.items.find(candidate => candidate.key === configKey);
        const environmentByKey = new Map(environments.items.map(environment => [environment.key, environment]));
        setState({ status: item ? 'ready' : 'not-found', item, environmentByKey });
      })
      .catch(() => live && setState({ status: 'failed' }));
    return () => { live = false; };
  }, [configKey]);

  if (state.status === 'not-found') return <Pending route={`configs/${configKey}`} t={t} />;
  if (state.status !== 'ready') return <CollectionState state={{ status: state.status, items: [] }} t={t} />;
  const { item, environmentByKey } = state;
  return (
    <div className="page config-home">
      <div className="resource-heading config-home-heading">
        <div>
          <p className="eyebrow"><a href="#/configs">{t.configs}</a> / <code>{item.key}</code></p>
          <h1 title={item.display_name}>{item.display_name}</h1>
          <p>{t.configContextBody}</p>
        </div>
        <div className="config-identity"><code>{item.key}</code><span className={item.archived ? 'status archived' : 'status active'}>{item.archived ? t.statusArchived : t.statusActive}</span></div>
      </div>
      <div className="config-home-summary">
        <span><small>{t.configIdentity}</small><code>{item.key}</code></span>
        <span><small>{t.environments}</small><strong>{item.environments.length}</strong></span>
        <span><small>{t.updated}</small><time>{formatDate(item.updated_at, t.locale)}</time></span>
      </div>
      <section className="panel config-context-section" aria-labelledby="config-context-title">
        <div className="section-heading"><h2 id="config-context-title">{t.configContexts}</h2><span>{t.configContextHint}</span></div>
        {item.environments.length === 0 ? <p className="config-context-empty">{t.noConfigContexts}</p> : <div className="table-frame config-context-table"><table>
          <thead><tr><th>{t.environment}</th><th>{t.displayName}</th><th>{t.revision}</th><th>{t.status}</th><th /></tr></thead>
          <tbody>{item.environments.map(environment => {
            const metadata = environmentByKey.get(environment.key);
            const displayName = metadata?.display_name || environment.key;
            const archived = item.archived || environment.archived || metadata?.archived;
            const href = `#/configs/${item.key}/${environment.key}`;
            const label = `${displayName} · ${environment.key} · v${environment.revision}${archived ? ` · ${t.statusArchived}` : ''}`;
            return <tr key={environment.key} aria-label={archived ? label : undefined}>
              <td>{archived ? <code>{environment.key}</code> : <a href={href} aria-label={`${t.openContext}: ${label}`}><code>{environment.key}</code></a>}</td>
              <td><strong title={displayName}>{displayName}</strong></td>
              <td><strong>v{environment.revision}</strong></td>
              <td><span className={archived ? 'status archived' : 'status active'}>{archived ? t.statusArchived : t.statusActive}</span></td>
              <td>{!archived && <a className="table-action" href={href}>{t.openContext} →</a>}</td>
            </tr>;
          })}</tbody>
        </table></div>}
      </section>
    </div>
  );
}

const vaultResourceKeyPattern = /^[a-z][a-z0-9_-]{0,62}$/;

function vaultEditorID() {
  return crypto.randomUUID().replaceAll('-', '');
}

function emptyVaultValue(type) {
  return type === 'file' ? { file: null } : { text: '' };
}

function newVaultDraft(environments) {
  const field = { key: '', name: '', type: 'secret', _id: vaultEditorID(), _existing: false };
  return {
    fields: [field],
    variants: [{
      id: vaultEditorID(),
      environments: environments.slice(0, 1).map(environment => environment.key),
      values: { [field._id]: emptyVaultValue(field.type) },
    }],
  };
}

function vaultDraftFromSnapshot(snapshot) {
  const fields = snapshot.fields.map(field => ({ ...field, _id: vaultEditorID(), _existing: true }));
  return {
    fields,
    variants: snapshot.variants.map(variant => ({
      id: variant.id,
      environments: [...variant.environments],
      values: Object.fromEntries(fields.map(field => [field._id, variant.values?.[field.key] ? structuredClone(variant.values[field.key]) : emptyVaultValue(field.type)])),
    })),
  };
}

function serializeVaultDraft(draft) {
  return {
    fields: draft.fields.map(({ key, name, type }) => ({ key, name, type })),
    variants: draft.variants.map(variant => ({
      id: variant.id,
      environments: [...variant.environments],
      values: Object.fromEntries(draft.fields.map(field => [field.key, structuredClone(variant.values[field._id])])),
    })),
  };
}

function vaultUsageImpacts(draft, usages) {
  const fields = new Set(draft.fields.map(field => field.key));
  const environments = new Set(draft.variants.flatMap(variant => variant.environments));
  return usages.filter(usage => !fields.has(usage.field_key) || !environments.has(usage.environment_key));
}

function base64ByteLength(value) {
  if (!value) return 0;
  return Math.floor(value.length * 3 / 4) - (value.endsWith('==') ? 2 : value.endsWith('=') ? 1 : 0);
}

function validateVaultDraft(draft, t) {
  if (!draft.fields.length || !draft.variants.length) return t.snapshotIncomplete;
  const keys = new Set();
  for (const field of draft.fields) {
    if (!vaultResourceKeyPattern.test(field.key) || !field.name.trim() || keys.has(field.key)) return t.snapshotIncomplete;
    keys.add(field.key);
  }
  for (const variant of draft.variants) {
    for (const field of draft.fields) {
      const value = variant.values[field._id];
      if ((field.type === 'text' || field.type === 'secret') && (typeof value?.text !== 'string' || new TextEncoder().encode(value.text).length > 512 * 1024)) return t.snapshotIncomplete;
      if (field.type === 'file' && (!value?.file?.filename || typeof value.file.bytes !== 'string' || base64ByteLength(value.file.bytes) > 5 * 1024 * 1024)) return t.snapshotIncomplete;
    }
  }
  return new Blob([JSON.stringify(serializeVaultDraft(draft))]).size > 10 * 1024 * 1024 ? t.snapshotIncomplete : '';
}

function VaultSnapshotEditor({ draft, onChange, environments, namespaceKey, itemKey, t }) {
  const [selected, setSelected] = useState(draft.variants[0]?.id || '');
  const [revealed, setRevealed] = useState({});
  const [fileError, setFileError] = useState('');
  useEffect(() => {
    if (!draft.variants.some(variant => variant.id === selected)) setSelected(draft.variants[0]?.id || '');
  }, [draft.variants, selected]);
  const environmentOptions = useMemo(() => {
    const options = new Map(environments.map(environment => [environment.key, environment]));
    draft.variants.flatMap(variant => variant.environments).forEach(key => {
      if (!options.has(key)) options.set(key, { key, display_name: key, archived: false });
    });
    return [...options.values()];
  }, [draft.variants, environments]);
  const selectedIndex = draft.variants.findIndex(variant => variant.id === selected);
  const selectedVariant = draft.variants[selectedIndex];
  const copy = text => navigator.clipboard.writeText(text).catch(() => {});
  const updateField = (fieldID, property, value) => onChange(current => {
    const field = current.fields.find(candidate => candidate._id === fieldID);
    const fields = current.fields.map(candidate => candidate._id === fieldID ? { ...candidate, [property]: value } : candidate);
    const variants = property === 'type' && field.type !== value
      ? current.variants.map(variant => ({ ...variant, values: { ...variant.values, [fieldID]: emptyVaultValue(value) } }))
      : current.variants;
    return { ...current, fields, variants };
  });
  const addField = () => {
    const field = { key: '', name: '', type: 'secret', _id: vaultEditorID(), _existing: false };
    onChange(current => ({
      ...current,
      fields: [...current.fields, field],
      variants: current.variants.map(variant => ({ ...variant, values: { ...variant.values, [field._id]: emptyVaultValue(field.type) } })),
    }));
  };
  const removeField = fieldID => onChange(current => ({
    ...current,
    fields: current.fields.filter(field => field._id !== fieldID),
    variants: current.variants.map(variant => {
      const values = { ...variant.values };
      delete values[fieldID];
      return { ...variant, values };
    }),
  }));
  const addVariant = () => {
    const id = vaultEditorID();
    onChange(current => ({
      ...current,
      variants: [...current.variants, { id, environments: [], values: Object.fromEntries(current.fields.map(field => [field._id, emptyVaultValue(field.type)])) }],
    }));
    setSelected(id);
  };
  const removeVariant = variantID => {
    onChange(current => ({ ...current, variants: current.variants.filter(variant => variant.id !== variantID) }));
    setSelected(draft.variants.find(variant => variant.id !== variantID)?.id || '');
  };
  const setEnvironment = (environment, checked) => onChange(current => ({
    ...current,
    variants: current.variants.map(variant => ({
      ...variant,
      environments: checked
        ? (variant.id === selected ? [...variant.environments.filter(key => key !== environment), environment] : variant.environments.filter(key => key !== environment))
        : (variant.id === selected ? variant.environments.filter(key => key !== environment) : variant.environments),
    })),
  }));
  const setValue = (fieldID, value) => onChange(current => ({
    ...current,
    variants: current.variants.map(variant => variant.id === selected ? { ...variant, values: { ...variant.values, [fieldID]: value } } : variant),
  }));
  const selectFile = (field, file) => {
    if (!file) return;
    if (file.size > 5 * 1024 * 1024) {
      setFileError(t.fileTooLarge);
      return;
    }
    setFileError('');
    const reader = new FileReader();
    reader.onload = () => setValue(field._id, { file: {
      filename: file.name,
      content_type: file.type || 'application/octet-stream',
      bytes: String(reader.result).split(',', 2)[1] || '',
    } });
    reader.readAsDataURL(file);
  };

  return <div className="vault-snapshot-editor">
    <section className="vault-field-definitions">
      <div className="vault-editor-heading"><div><h3>{t.fields}</h3><p>{t.fieldIdentityLocked}</p></div><button className="secondary-action compact-action" type="button" onClick={addField}>{t.addField}</button></div>
      <div className="vault-definition-list">
        {draft.fields.map((field, index) => {
          const reference = vaultResourceKeyPattern.test(namespaceKey) && vaultResourceKeyPattern.test(itemKey) && vaultResourceKeyPattern.test(field.key)
            ? field.type === 'file' ? `/v1/environments/<environment>/vault-items/${namespaceKey}/${itemKey}/fields/${field.key}/content` : `{vault.${namespaceKey}.${itemKey}.${field.key}}`
            : '—';
          return <div className="vault-definition-row" key={field._id}>
            <label><span>{t.fieldKey}</span><input required readOnly={field._existing} aria-label={`${t.fieldKey} ${index + 1}`} maxLength="63" pattern="[a-z][a-z0-9_-]{0,62}" value={field.key} onChange={event => updateField(field._id, 'key', event.target.value)} /></label>
            <label><span>{t.fieldName}</span><input required aria-label={`${t.fieldName} ${index + 1}`} maxLength="255" value={field.name} onChange={event => updateField(field._id, 'name', event.target.value)} /></label>
            <label><span>{t.fieldType}</span><select disabled={field._existing} aria-label={`${t.fieldType} ${index + 1}`} value={field.type} onChange={event => updateField(field._id, 'type', event.target.value)}><option value="text">{t.textType}</option><option value="secret">{t.secretType}</option><option value="file">{t.fileType}</option></select></label>
            <div className="vault-reference-preview"><span>{field.type === 'file' ? t.fileReadPath : t.referenceKey}</span><code>{reference}</code>{reference !== '—' && <button type="button" aria-label={`${t.copyReference} ${reference}`} onClick={() => copy(reference)}>{t.copy}</button>}</div>
            <button className="danger-link" type="button" disabled={draft.fields.length === 1} aria-label={`${t.removeField} ${field.name || index + 1}`} onClick={() => removeField(field._id)}>{t.removeField}</button>
          </div>;
        })}
      </div>
    </section>
    <section className="vault-variant-workspace">
      <aside className="vault-editor-variants">
        <div className="vault-editor-heading"><h3>{t.variants}</h3><button type="button" onClick={addVariant}>{t.addVariant}</button></div>
        {draft.variants.map((variant, index) => <button className="vault-editor-variant" type="button" key={variant.id} aria-pressed={variant.id === selected} onClick={() => setSelected(variant.id)}><strong>{t.variant} {index + 1}</strong><span>{variant.environments.map(environment => <code key={environment}>{environment}</code>)}</span><small>{draft.fields.length}/{draft.fields.length} {t.fields}</small></button>)}
      </aside>
      {selectedVariant && <div className="vault-variant-form">
        <div className="vault-editor-heading"><div><h3>{t.variant} {selectedIndex + 1}</h3><p>{t.environmentMoveHint}</p></div><button className="danger-link" type="button" disabled={draft.variants.length === 1} onClick={() => removeVariant(selectedVariant.id)}>{t.removeVariant}</button></div>
        <fieldset className="vault-environment-picker"><legend>{t.environmentBinding}</legend><div className="choice-grid">{environmentOptions.map(environment => <label className={environment.archived ? 'choice-disabled' : ''} key={environment.key}><input type="checkbox" checked={selectedVariant.environments.includes(environment.key)} disabled={environment.archived} aria-label={`${t.variant} ${selectedIndex + 1} ${environment.display_name} ${environment.key}`} onChange={event => setEnvironment(environment.key, event.target.checked)} /><span>{environment.display_name}<code>{environment.key}</code></span></label>)}</div></fieldset>
        <div className="vault-editor-values"><h4>{t.value}</h4>{draft.fields.map((field, index) => {
          const value = selectedVariant.values[field._id];
          const name = field.name || `${t.fields} ${index + 1}`;
          const valueLabel = `${t.variant} ${selectedIndex + 1} ${t.valueFor.replace('{field}', name)}`;
          return <div className="vault-value-editor" key={field._id}>
            <div><strong>{name}</strong><code>{field.key || '—'}</code><span>{field.type}</span></div>
            {field.type === 'text' && <textarea required aria-label={valueLabel} maxLength={512 * 1024} rows="2" value={value?.text || ''} onChange={event => setValue(field._id, { text: event.target.value })} />}
            {field.type === 'secret' && <div className="vault-secret-editor"><input required aria-label={valueLabel} maxLength={512 * 1024} type={revealed[`${selected}-${field._id}`] ? 'text' : 'password'} value={value?.text || ''} onChange={event => setValue(field._id, { text: event.target.value })} /><button type="button" onClick={() => setRevealed(current => ({ ...current, [`${selected}-${field._id}`]: !current[`${selected}-${field._id}`] }))}>{revealed[`${selected}-${field._id}`] ? t.hide : t.reveal}</button><button type="button" disabled={!value?.text} onClick={() => copy(value.text)}>{t.copy}</button></div>}
            {field.type === 'file' && <div className="vault-file-editor"><label className="vault-file-picker"><input required={!value?.file} aria-label={valueLabel} type="file" onChange={event => selectFile(field, event.target.files[0])} /><span>{value?.file ? t.replaceFile : t.chooseFile}</span></label><strong>{value?.file?.filename || t.noFileSelected}</strong><small>{value?.file?.content_type || '—'}</small></div>}
          </div>;
        })}</div>
      </div>}
    </section>
    {fileError && <p className="inline-error" role="alert">{fileError}</p>}
  </div>;
}

function VaultItems({ principal, filters = new URLSearchParams(), t }) {
  const [state, setState] = useState({ status: 'loading', items: [], environments: [] });
  const [creating, setCreating] = useState(false);
  const [draft, setDraft] = useState(null);
  const [newNamespaceKey, setNewNamespaceKey] = useState('');
  const [newItemKey, setNewItemKey] = useState('');
  const [namespaceFilter, setNamespaceFilter] = useState(() => filters.get('namespace') || '');
  const [environmentFilter, setEnvironmentFilter] = useState(() => filters.get('environment') || '');
  const [search, setSearch] = useState(() => filters.get('q') || '');
  const [confirming, setConfirming] = useState('');
  const [error, setError] = useState('');
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    let live = true;
    Promise.all([
      request('/v1/vault-items?include_archived=true'),
      principal.role === 'admin' ? request('/v1/environments') : Promise.resolve({ items: [] }),
    ])
      .then(([items, environments]) => live && setState({ status: 'ready', items: items.items, environments: environments.items }))
      .catch(() => live && setState({ status: 'failed', items: [], environments: [] }));
    return () => { live = false; };
  }, [refresh]);
  const filterKey = filters.toString();
  useEffect(() => {
    setNamespaceFilter(filters.get('namespace') || '');
    setEnvironmentFilter(filters.get('environment') || '');
    setSearch(filters.get('q') || '');
  }, [filterKey]);
  const toggleCreate = () => {
    if (!creating) {
      setDraft(newVaultDraft(state.environments));
      setNewNamespaceKey('');
      setNewItemKey('');
    }
    setCreating(value => !value);
    setError('');
  };
  const create = async event => {
    event.preventDefault();
    setError('');
    const data = new FormData(event.currentTarget);
    const validationError = validateVaultDraft(draft, t);
    if (validationError) return setError(validationError);
    const snapshot = serializeVaultDraft(draft);
    try {
      await request(`/v1/vault-items/${encodeURIComponent(newNamespaceKey)}/${encodeURIComponent(newItemKey)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({ display_name: data.get('display_name'), expected_revision: 0, snapshot }),
      });
      setCreating(false);
      setDraft(null);
      setRefresh(value => value + 1);
    } catch {
      setError(t.operationFailed);
    }
  };
  const changeLifecycle = async (item, action) => {
    try {
      await request(`/v1/vault-items/${encodeURIComponent(item.namespace_key)}/${encodeURIComponent(item.key)}/${action}`, { method: 'POST', headers: { 'Idempotency-Key': crypto.randomUUID() } });
      setConfirming('');
      setRefresh(value => value + 1);
    } catch {
      setError(t.operationFailed);
    }
  };
  const namespaces = [...new Set(state.items.map(item => item.namespace_key))].sort();
  const environmentKeys = [...new Set(state.items.flatMap(item => item.environment_keys || []))].sort();
  const needle = search.trim().toLowerCase();
  const visibleItems = state.items.filter(item => (!namespaceFilter || item.namespace_key === namespaceFilter)
    && (!environmentFilter || (item.environment_keys || []).includes(environmentFilter))
    && (!needle || `${item.namespace_key}.${item.key} ${item.display_name}`.toLowerCase().includes(needle)));
  return (
    <ResourcePage eyebrow="VLT / encrypted revisions" title={t.vault} body={t.vaultBody}
      action={principal.role === 'admin' && <button className="primary-action compact-action" type="button" onClick={toggleCreate}>{t.newVaultItem}</button>}>
      {creating && draft && <form className="vault-structured-form" onSubmit={create}>
        <section className="vault-item-basics"><div className="vault-editor-heading"><h2>{t.itemDetails}</h2></div><div className="vault-item-identity"><label>{t.namespace}<input required list="vault-namespace-options" maxLength="63" pattern="[a-z][a-z0-9_-]{0,62}" autoComplete="off" value={newNamespaceKey} onChange={event => setNewNamespaceKey(event.target.value)} /><small>{t.namespaceHint}</small></label><datalist id="vault-namespace-options">{namespaces.map(namespace => <option key={namespace} value={namespace} />)}</datalist><label>{t.resourceKey}<input required maxLength="63" pattern="[a-z][a-z0-9_-]{0,62}" autoComplete="off" value={newItemKey} onChange={event => setNewItemKey(event.target.value)} /><small>{t.resourceKeyHint}</small></label><label>{t.displayName}<input name="display_name" required maxLength="255" autoComplete="off" /></label></div></section>
        <VaultSnapshotEditor draft={draft} onChange={setDraft} environments={state.environments} namespaceKey={newNamespaceKey} itemKey={newItemKey} t={t} />
        <div className="vault-editor-footer"><span className="inline-error" role="alert">{error}</span><div className="row-actions"><button type="button" onClick={toggleCreate}>{t.cancel}</button><button className="primary-action compact-action" type="submit">{t.createVaultItem}</button></div></div>
      </form>}
      {state.status === 'ready' && state.items.length > 0 && <div className="vault-index-tools">
        <label><span>{t.searchVault}</span><input type="search" aria-label={t.searchVault} placeholder={t.searchVaultPlaceholder} value={search} onChange={event => { setSearch(event.target.value); replaceHashQuery({ q: event.target.value }); }} /></label>
        <label><span>{t.environment}</span><select aria-label={t.environmentFilter} value={environmentFilter} onChange={event => { setEnvironmentFilter(event.target.value); replaceHashQuery({ environment: event.target.value }); }}><option value="">{t.allEnvironments}</option>{environmentKeys.map(environment => <option key={environment} value={environment}>{environment}</option>)}</select></label>
        <label><span>{t.namespace}</span><select aria-label={t.namespaceFilter} value={namespaceFilter} onChange={event => { setNamespaceFilter(event.target.value); replaceHashQuery({ namespace: event.target.value }); }}><option value="">{t.allNamespaces}</option>{namespaces.map(namespace => <option key={namespace} value={namespace}>{namespace}</option>)}</select></label>
      </div>}
      {(!creating || state.items.length > 0) && <CollectionState state={state} t={t}>
        {visibleItems.length === 0 ? <div className="collection-state"><p>{t.noMatchingVaultItems}</p></div> : <div className="table-frame vault-index-table"><table><thead><tr><th>{t.namespace}</th><th>{t.resource}</th><th>{t.displayName}</th><th>{t.boundEnvironments}</th><th>{t.revision}</th><th>{t.updated}</th><th>{t.status}</th>{principal.role === 'admin' && <th />}</tr></thead>
          <tbody>{visibleItems.map(item => <tr key={`${item.namespace_key}.${item.key}`}>
            <td><code>{item.namespace_key}</code></td><td><a href={`#/vault/${item.namespace_key}/${item.key}`} aria-label={`${item.namespace_key}.${item.key} · ${item.display_name} · v${item.revision}`}><code>{item.key}</code></a></td>
            <td><strong title={item.display_name}>{item.display_name}</strong></td><td><div className="environment-links">{(item.environment_keys || []).map(environment => <a href={`#/environments/${environment}`} key={environment}><code>{environment}</code></a>)}</div></td><td><strong>v{item.revision}</strong></td><td><time>{formatDate(item.updated_at, t.locale)}</time></td>
            <td><span className={item.archived ? 'status archived' : 'status active'}>{item.archived ? t.statusArchived : t.statusActive}</span></td>
            {principal.role === 'admin' && <td>{item.archived
              ? <button type="button" aria-label={`${t.unarchive} ${item.display_name}`} onClick={() => changeLifecycle(item, 'unarchive')}>{t.unarchive}</button>
              : confirming === `${item.namespace_key}.${item.key}`
                ? <button className="danger-link" type="button" aria-label={`${t.confirmArchive} ${item.display_name}`} onClick={() => changeLifecycle(item, 'archive')}>{t.confirmArchive}</button>
                : <button className="danger-link" type="button" aria-label={`${t.archive} ${item.display_name}`} onClick={() => setConfirming(`${item.namespace_key}.${item.key}`)}>{t.archive}</button>}</td>}
          </tr>)}</tbody>
        </table></div>}
      </CollectionState>}
      {!creating && error && <p className="inline-error" role="alert">{error}</p>}
    </ResourcePage>
  );
}

function VaultDetail({ principal, namespaceKey, itemKey, t }) {
  const [state, setState] = useState({ status: 'loading' });
  const [editorEnvironments, setEditorEnvironments] = useState([]);
  const [tab, setTab] = useState('fields');
  const [selected, setSelected] = useState('');
  const [viewedRevision, setViewedRevision] = useState(0);
  const [inspection, setInspection] = useState({ status: 'idle' });
  const [values, setValues] = useState({ status: 'idle' });
  const [revealed, setRevealed] = useState({});
  const [editor, setEditor] = useState({ status: 'idle' });
  const [notice, setNotice] = useState('');
  const [restoring, setRestoring] = useState(0);
  const [refresh, setRefresh] = useState(0);
  const editorDirty = editor.status === 'ready' && editor.original !== JSON.stringify({ displayName: editor.displayName, snapshot: serializeVaultDraft(editor.draft) });
  const confirmDiscard = useUnsavedChanges(editorDirty, t.unsavedChanges);
  useEffect(() => {
    let live = true;
    Promise.all([request(`/v1/vault-items/${encodeURIComponent(namespaceKey)}/${encodeURIComponent(itemKey)}`), request(`/v1/vault-items/${encodeURIComponent(namespaceKey)}/${encodeURIComponent(itemKey)}/revisions`), request(`/v1/vault-items/${encodeURIComponent(namespaceKey)}/${encodeURIComponent(itemKey)}/usages`)])
      .then(([item, revisions, usages]) => {
        if (!live) return;
        setState({ status: 'ready', item, revisions: revisions.items, usages: usages.items || [] });
        setViewedRevision(item.revision);
        setInspection({ status: 'idle' });
      })
      .catch(() => live && setState({ status: 'failed' }));
    return () => { live = false; };
  }, [namespaceKey, itemKey, refresh]);
  useEffect(() => {
    if (state.status !== 'ready' || !viewedRevision || viewedRevision === state.item.revision) return undefined;
    let live = true;
    setInspection({ status: 'loading', revision: viewedRevision });
    request(`/v1/vault-items/${encodeURIComponent(namespaceKey)}/${encodeURIComponent(itemKey)}/revisions/${viewedRevision}`)
      .then(item => live && setInspection({ status: 'ready', revision: viewedRevision, item }))
      .catch(() => live && setInspection({ status: 'failed', revision: viewedRevision }));
    return () => { live = false; };
  }, [itemKey, namespaceKey, state.status, state.item?.revision, viewedRevision]);
  useEffect(() => {
    if (principal.role !== 'admin') return undefined;
    let live = true;
    request('/v1/environments?include_archived=true').then(result => live && setEditorEnvironments(result.items)).catch(() => {});
    return () => { live = false; };
  }, [principal.role]);
  const readValues = async () => {
    const revision = viewedRevision;
    setValues({ status: 'loading', revision });
    try {
      const revisionPath = revision === state.item.revision ? 'values' : `revisions/${revision}/values`;
      const item = await request(`/v1/vault-items/${encodeURIComponent(namespaceKey)}/${encodeURIComponent(itemKey)}/${revisionPath}`);
      setValues({ status: 'ready', revision, item });
      return item;
    } catch {
      setValues({ status: 'failed', revision });
      return null;
    }
  };
  const startEdit = async () => {
    setNotice('');
    setEditor({ status: 'loading' });
    const item = values.status === 'ready' && values.revision === state.item.revision ? values.item : await readValues();
    if (!item) {
      setEditor({ status: 'failed' });
      return;
    }
    const draft = vaultDraftFromSnapshot(item.snapshot);
    setEditor({ status: 'ready', displayName: item.display_name, draft, original: JSON.stringify({ displayName: item.display_name, snapshot: serializeVaultDraft(draft) }), error: null, impactConfirmed: false, saving: false });
  };
  const saveItem = async event => {
    event.preventDefault();
    const validationError = validateVaultDraft(editor.draft, t);
    if (validationError) return setEditor(current => ({ ...current, error: { message: validationError, requestID: '' } }));
    const impacts = vaultUsageImpacts(editor.draft, state.usages);
    if (impacts.length > 0 && !editor.impactConfirmed) {
      setEditor(current => ({ ...current, impactConfirmed: true }));
      return;
    }
    const snapshot = serializeVaultDraft(editor.draft);
    setEditor(current => ({ ...current, saving: true, error: null }));
    try {
      const result = await request(`/v1/vault-items/${encodeURIComponent(namespaceKey)}/${encodeURIComponent(itemKey)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({ display_name: editor.displayName, expected_revision: state.item.revision, snapshot }),
      });
      setState(current => ({ ...current, item: { ...current.item, display_name: editor.displayName, revision: result.revision } }));
      setEditor({ status: 'idle' });
      setValues({ status: 'idle' });
      setNotice(t.savedRevision.replace('{revision}', result.revision));
      setRefresh(value => value + 1);
    } catch (error) {
      setEditor(current => ({ ...current, saving: false, error: managementError(error, t, t.operationFailed) }));
    }
  };
  const restoreRevision = async revision => {
    try {
      const result = await request(`/v1/vault-items/${encodeURIComponent(namespaceKey)}/${encodeURIComponent(itemKey)}/restore`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({ source_revision: revision, expected_revision: state.item.revision }),
      });
      setState(current => ({ ...current, item: { ...current.item, revision: result.revision } }));
      setRestoring(0);
      setValues({ status: 'idle' });
      setRefresh(value => value + 1);
    } catch {
      setEditor({ status: 'failed' });
    }
  };
  if (state.status !== 'ready') return <CollectionState state={{ status: state.status, items: [] }} t={t} />;
  const viewingCurrent = viewedRevision === state.item.revision;
  const viewedItem = viewingCurrent ? state.item : inspection.item;
  const usageImpacts = editor.status === 'ready' ? vaultUsageImpacts(editor.draft, state.usages) : [];
  const variantID = viewedItem?.snapshot.variants.some(candidate => candidate.id === selected) ? selected : viewedItem?.snapshot.variants[0]?.id;
  const valuesReady = values.status === 'ready' && values.revision === viewedRevision;
  const valuedVariant = valuesReady
    ? values.item.snapshot.variants.find(candidate => candidate.id === variantID)
    : null;
  const inspectRevision = revision => {
    setViewedRevision(revision);
    setInspection({ status: revision === state.item.revision ? 'idle' : 'loading', revision });
    setValues({ status: 'idle' });
    setRevealed({});
    setSelected('');
    setTab('fields');
  };
  return (
    <div className={editor.status === 'ready' ? 'page vault-detail vault-editing' : 'page vault-detail'}>
      <div className="config-detail-heading">
        <div><p className="eyebrow">VLT / <code>{namespaceKey}</code> / <code>{itemKey}</code></p><h1 title={viewedItem?.display_name || state.item.display_name}>{viewedItem?.display_name || state.item.display_name}</h1></div>
        <div className="detail-heading-actions"><div className="config-identity"><span>{viewedItem?.snapshot.fields.length ?? state.item.snapshot.fields.length} {t.fields}</span><code>v{viewedRevision}</code><span>{viewingCurrent ? t.current : t.readOnly}</span>{state.item.archived && <span className="status archived">{t.statusArchived}</span>}</div>{principal.role === 'admin' && viewingCurrent && !state.item.archived && editor.status !== 'ready' && <button className="secondary-action compact-action" type="button" disabled={editor.status === 'loading'} onClick={startEdit}>{t.editItem}</button>}</div>
      </div>
      {notice && <p className="success-note vault-save-notice" role="status">{notice}</p>}
      {editor.status === 'ready' && <form className="vault-structured-form vault-detail-editor" onSubmit={saveItem}>
        <div className="vault-edit-warning">{t.valueAccessWarning}</div>
        <section className="vault-item-basics"><div className="vault-editor-heading"><h2>{t.itemDetails}</h2></div><div className="vault-item-identity"><label>{t.namespace}<input readOnly value={namespaceKey} /><small>{t.namespaceHint}</small></label><label>{t.resourceKey}<input readOnly value={itemKey} /></label><label>{t.displayName}<input required maxLength="255" value={editor.displayName} onChange={event => setEditor(current => ({ ...current, displayName: event.target.value, error: null, impactConfirmed: false }))} autoComplete="off" /></label></div></section>
        <VaultSnapshotEditor draft={editor.draft} onChange={update => setEditor(current => ({ ...current, draft: typeof update === 'function' ? update(current.draft) : update, error: null, impactConfirmed: false }))} environments={editorEnvironments} namespaceKey={namespaceKey} itemKey={itemKey} t={t} />
        {usageImpacts.length > 0 && <div className="vault-impact-warning" role="alert"><strong>{t.vaultImpactWarning}</strong>{usageImpacts.map(usage => <a href={`#/configs/${usage.config_key}/${usage.environment_key}`} key={`${usage.field_key}-${usage.environment_key}-${usage.config_key}`}><span>{usage.config_name}</span><code>{usage.environment_key} / {usage.config_key} @ v{usage.config_revision} · {usage.field_key}</code></a>)}</div>}
        <div className="vault-editor-footer"><ActionError error={editor.error} t={t} /><div className="row-actions"><button type="button" onClick={() => { if (confirmDiscard()) setEditor({ status: 'idle' }); }}>{t.cancel}</button><button className="primary-action compact-action" type="submit" disabled={!editorDirty || editor.saving}>{editor.impactConfirmed && usageImpacts.length > 0 ? t.confirmImpactSave : t.saveItem}</button></div></div>
      </form>}
      {editor.status === 'failed' && <p className="inline-error" role="alert">{t.operationFailed}</p>}
      <div className="revision-track vault-revision-track" aria-label={t.revisionHistory}>
        {state.revisions.slice(0, 8).reverse().map(revision => <button type="button" className={`${revision.revision === state.item.revision ? 'current-revision ' : ''}${revision.revision === viewedRevision ? 'selected-revision' : ''}`} aria-pressed={revision.revision === viewedRevision} onClick={() => inspectRevision(revision.revision)} key={revision.revision}><b>v{revision.revision}</b><small>{revision.revision === state.item.revision ? t.current : formatDate(revision.created_at, t.locale).split(',')[0]}</small></button>)}
      </div>
      <div className="tabbar" role="tablist" aria-label={t.vaultView}>
        <button type="button" role="tab" aria-selected={tab === 'fields'} onClick={() => setTab('fields')}>{t.fields}</button>
        <button type="button" role="tab" aria-selected={tab === 'history'} onClick={() => setTab('history')}>{t.history}</button>
      </div>
      {tab === 'fields' && viewingCurrent && editor.status !== 'ready' && <VaultUsagePanel usages={state.usages} t={t} />}
      {tab === 'history' ? (
        <div className="table-frame history-table"><table><thead><tr><th>{t.revision}</th><th>{t.actor}</th><th>{t.created}</th>{principal.role === 'admin' && <th />}</tr></thead>
          <tbody>{state.revisions.map(revision => <tr className={revision.revision === viewedRevision ? 'selected-history-row' : ''} key={revision.revision}><td><button className="revision-link" type="button" aria-label={t.viewRevision.replace('{revision}', revision.revision)} onClick={() => inspectRevision(revision.revision)}>v{revision.revision}</button></td><td><ActorIdentity actorID={revision.actor_id} principal={principal} /></td><td><time>{formatDate(revision.created_at, t.locale)}</time></td>{principal.role === 'admin' && <td>{!state.item.archived && revision.revision !== state.item.revision && (restoring === revision.revision
            ? <button className="danger-link" type="button" aria-label={t.confirmRestore.replace('{revision}', revision.revision)} onClick={() => restoreRevision(revision.revision)}>{t.confirmRestore.replace('{revision}', revision.revision)}</button>
            : <button type="button" aria-label={t.restoreRevision.replace('{revision}', revision.revision)} onClick={() => setRestoring(revision.revision)}>{t.restore}</button>)}</td>}</tr>)}</tbody>
        </table></div>
      ) : !viewingCurrent && inspection.status === 'loading' ? <div className="collection-state"><span className="loading-line" /></div>
        : !viewingCurrent && inspection.status === 'failed' ? <p className="inline-error" role="alert">{t.operationFailed}</p>
          : viewedItem && (
        <div className="vault-layout">
          <aside className="variant-list" aria-label={t.variants}>
            {viewedItem.snapshot.variants.map((candidate, index) => (
              <button type="button" key={candidate.id} aria-label={`${t.variant} ${index + 1} · ${candidate.environments.join(', ')}`} aria-pressed={candidate.id === variantID} onClick={() => { setSelected(candidate.id); setRevealed({}); }}>
                <strong>{t.variant} {index + 1}</strong><span className="variant-environments">{candidate.environments.map(environment => <code key={environment}>{environment}</code>)}</span><small>{viewedItem.snapshot.fields.length}/{viewedItem.snapshot.fields.length} {t.fields}</small>
              </button>
            ))}
          </aside>
          <section className="field-panel">
            {principal.role === 'admin' && !state.item.archived && !valuesReady && (
              <div className="value-gate"><p>{values.status === 'failed' && values.revision === viewedRevision ? t.operationFailed : t.valueAccessWarning}</p><button className="primary-action compact-action" type="button" disabled={values.status === 'loading' && values.revision === viewedRevision} onClick={readValues}>{t.revealItemValues}</button></div>
            )}
            <div className="field-table" role="table" aria-label={`${state.item.display_name} ${t.fields}`}>
              <div className="field-table-header" role="row"><span role="columnheader">{t.fields}</span><span role="columnheader">{t.reference}</span><span role="columnheader">{t.value}</span></div>
              {viewedItem.snapshot.fields.map(field => <VaultField key={field.key} field={field} namespaceKey={namespaceKey} itemKey={itemKey} value={valuedVariant?.values?.[field.key]} revealed={Boolean(revealed[field.key])} setRevealed={value => setRevealed(current => ({ ...current, [field.key]: value }))} t={t} />)}
            </div>
          </section>
          <aside className="vault-meta-rail">
            <section><h2>{t.versionIdentity}</h2><dl><div><dt>{t.namespace}</dt><dd><code>{namespaceKey}</code></dd></div><div><dt>{t.resourceKey}</dt><dd><code>{itemKey}</code></dd></div><div><dt>{t.revision}</dt><dd><strong>v{viewedRevision}</strong>{viewingCurrent && ` · ${t.current}`}</dd></div><div><dt>{t.fields}</dt><dd>{viewedItem.snapshot.fields.length}</dd></div></dl></section>
            <section><h2>{t.environmentBinding}</h2>{viewedItem.snapshot.variants.map((candidate, index) => <div className="binding-row" key={candidate.id}><span>{t.variant} {index + 1}</span><div>{candidate.environments.map(environment => <code key={environment}>{environment}</code>)}</div></div>)}</section>
            <section><h2>{t.history}</h2>{state.revisions.slice(0, 4).map(revision => <div className="history-row" key={revision.revision}><strong>v{revision.revision}</strong><time>{formatDate(revision.created_at, t.locale)}</time></div>)}</section>
          </aside>
        </div>
      )}
    </div>
  );
}

function VaultUsagePanel({ usages, t }) {
  const groups = usages.reduce((result, usage) => {
    (result[usage.field_key] ||= []).push(usage);
    return result;
  }, {});
  return <section className="panel vault-usage-panel" aria-labelledby="vault-usage-title">
    <div className="section-heading"><div><h2 id="vault-usage-title">{t.currentReferences}</h2><p>{t.currentReferencesBody}</p></div><span>{usages.length}</span></div>
    {usages.length === 0 ? <p className="environment-empty">{t.noCurrentReferences}</p> : <div className="vault-usage-groups">{Object.entries(groups).map(([field, items]) => <section key={field}><h3><code>{field}</code><span>{items.length}</span></h3>{items.map(usage => <a href={`#/configs/${usage.config_key}/${usage.environment_key}`} aria-label={`${usage.config_name} ${usage.environment_key} v${usage.config_revision}`} key={`${usage.environment_key}-${usage.config_key}`}><strong title={usage.config_name}>{usage.config_name}</strong><code>{usage.environment_key} / {usage.config_key} @ v{usage.config_revision}</code></a>)}</section>)}</div>}
  </section>;
}

function VaultField({ field, namespaceKey, itemKey, value, revealed, setRevealed, t }) {
  const reference = `{vault.${namespaceKey}.${itemKey}.${field.key}}`;
  const filePath = `/v1/environments/<environment>/vault-items/${namespaceKey}/${itemKey}/fields/${field.key}/content`;
  const referenceValue = field.type === 'file' ? filePath : reference;
  const copy = text => navigator.clipboard.writeText(text).catch(() => {});
  return (
    <div className="field-row" role="row">
      <div className="field-name" role="cell"><strong title={field.name}>{field.name}</strong><code title={field.key}>{field.key}</code><span>{field.type}</span></div>
      <div className="field-reference" role="cell">
        <small>{field.type === 'file' ? t.fileReadPath : t.referenceKey}</small>
        <code title={referenceValue}>{referenceValue}</code>
        <button type="button" aria-label={`${t.copyReference} ${referenceValue}`} onClick={() => copy(referenceValue)}>{t.copy}</button>
      </div>
      <div className="field-current" role="cell">
        {field.type === 'text' && <span>{value?.text ?? '—'}</span>}
        {field.type === 'secret' && value?.text !== undefined && <><label className="source-label" htmlFor={`vault-${field.key}`}>{field.name}</label><input id={`vault-${field.key}`} type={revealed ? 'text' : 'password'} readOnly value={value.text} /><button className="field-icon-button" type="button" title={`${revealed ? t.hide : t.reveal} ${field.name}`} aria-label={`${revealed ? t.hide : t.reveal} ${field.name}`} aria-pressed={revealed} onClick={() => setRevealed(!revealed)}><Icon name={revealed ? 'eyeOff' : 'eye'} /></button><button type="button" aria-label={`${t.copyValue} ${field.name}`} onClick={() => copy(value.text)}>{t.copy}</button></>}
        {field.type === 'secret' && value?.text === undefined && <span>••••••••</span>}
        {field.type === 'file' && <><span>{value?.file?.filename || '—'}</span>{value?.file && <button type="button" aria-label={`${t.download} ${field.name}`} onClick={() => downloadFile(value.file)}>{t.download}</button>}</>}
      </div>
    </div>
  );
}

function downloadFile(file) {
  try {
    const bytes = Uint8Array.from(atob(file.bytes), character => character.charCodeAt(0));
    const url = URL.createObjectURL(new Blob([bytes], { type: file.content_type || 'application/octet-stream' }));
    const link = document.createElement('a');
    link.href = url;
    link.download = file.filename;
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 0);
  } catch {}
}

function Administration({ t }) {
  const [tab, setTab] = useState('tokens');
  return (
    <div className="page administration-page">
      <div className="resource-heading"><div><p className="eyebrow">ADM / business settings</p><h1>{t.administration}</h1><p>{t.administrationBody}</p></div></div>
      <div className="tabbar" role="tablist" aria-label={t.administration}>
        <button type="button" role="tab" aria-selected={tab === 'tokens'} onClick={() => setTab('tokens')}>{t.apiTokens}</button>
        <button type="button" role="tab" aria-selected={tab === 'certificates'} onClick={() => setTab('certificates')}>{t.clientCertificates}</button>
        <button type="button" role="tab" aria-selected={tab === 'notifications'} onClick={() => setTab('notifications')}>{t.notifications}</button>
        <button type="button" role="tab" aria-selected={tab === 'deployment'} onClick={() => setTab('deployment')}>{t.deploymentStatus}</button>
      </div>
      {tab === 'tokens' ? <TokensPanel t={t} /> : tab === 'certificates' ? <CertificatesPanel t={t} /> : tab === 'notifications' ? <Notifications t={t} embedded /> : <DeploymentPanel t={t} />}
    </div>
  );
}

function TokensPanel({ t }) {
  const [state, setState] = useState({ status: 'loading', items: [], environments: [] });
  const [creating, setCreating] = useState(false);
  const [neverExpires, setNeverExpires] = useState(true);
  const [created, setCreated] = useState(null);
  const [editing, setEditing] = useState('');
  const [confirming, setConfirming] = useState('');
  const [error, setError] = useState('');
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    let live = true;
    Promise.all([request('/v1/api-tokens?include_revoked=true'), request('/v1/environments')])
      .then(([tokens, environments]) => live && setState({ status: 'ready', items: tokens.items, environments: environments.items }))
      .catch(() => live && setState({ status: 'failed', items: [], environments: [] }));
    return () => { live = false; };
  }, [refresh]);
  const create = async event => {
    event.preventDefault();
    setError('');
    const data = new FormData(event.currentTarget);
    const body = {
      display_name: data.get('display_name'),
      environment_keys: state.environments.filter(environment => data.has(`environment:${environment.key}`)).map(environment => environment.key),
      allow_without_mtls: data.has('allow_without_mtls'),
      never_expires: neverExpires,
    };
    if (!neverExpires) body.expires_at = new Date(data.get('expires_at')).toISOString();
    try {
      const result = await request('/v1/api-tokens', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify(body),
      });
      setCreated(result.token);
      setCreating(false);
      setRefresh(value => value + 1);
    } catch {
      setError(t.operationFailed);
    }
  };
  const saveGrants = async event => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    try {
      await request(`/v1/api-tokens/${encodeURIComponent(editing)}/environments`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({ environment_keys: state.environments.filter(environment => data.has(`grant:${environment.key}`)).map(environment => environment.key) }),
      });
      setEditing('');
      setRefresh(value => value + 1);
    } catch {
      setError(t.operationFailed);
    }
  };
  const revoke = async publicID => {
    try {
      await request(`/v1/api-tokens/${encodeURIComponent(publicID)}/revoke`, { method: 'POST', headers: { 'Idempotency-Key': crypto.randomUUID() } });
      setConfirming('');
      setRefresh(value => value + 1);
    } catch {
      setError(t.operationFailed);
    }
  };
  return (
    <section className="admin-panel tokens-panel">
      <div className="panel-actions"><span>{t.tokenEnvironmentGrant}</span><button className="primary-action compact-action" type="button" onClick={() => setCreating(value => !value)}>{t.newAPIToken}</button></div>
      {created && <div className="one-time-secret" role="alert"><strong>{t.tokenOnce}</strong><code>{created}</code><button type="button" onClick={() => navigator.clipboard.writeText(created).catch(() => {})}>{t.copy}</button></div>}
      {creating && (
        <form className="token-form" onSubmit={create}>
          <label>{t.tokenName}<input name="display_name" required maxLength="255" autoComplete="off" /></label>
          <fieldset><legend>{t.allowedEnvironments}</legend><div className="choice-grid">{state.environments.map(environment => <label key={environment.key}><input type="checkbox" name={`environment:${environment.key}`} /><span>{environment.display_name}<code>{environment.key}</code></span></label>)}</div></fieldset>
          <label className="check-line"><input type="checkbox" name="allow_without_mtls" />{t.allowWithoutMTLS}</label>
          <label className="check-line"><input type="checkbox" checked={neverExpires} onChange={event => setNeverExpires(event.target.checked)} />{t.neverExpires}</label>
          {!neverExpires && <label>{t.expiresAt}<input type="datetime-local" name="expires_at" required /></label>}
          <div className="form-actions"><span className="inline-error" role="alert">{error}</span><button className="primary-action compact-action" type="submit">{t.createAPIToken}</button></div>
        </form>
      )}
      {editing && (() => {
        const token = state.items.find(item => item.public_id === editing);
        return token && <form className="grant-form" onSubmit={saveGrants}><strong>{token.display_name}</strong><div className="choice-grid">{state.environments.map(environment => <label key={environment.key}><input type="checkbox" name={`grant:${environment.key}`} defaultChecked={token.environment_keys.includes(environment.key)} /><span>{environment.display_name}<code>{environment.key}</code></span></label>)}</div><button className="primary-action compact-action" type="submit">{t.saveEnvironmentGrants}</button></form>;
      })()}
      {!creating && error && <p className="inline-error" role="alert">{error}</p>}
      <CollectionState state={state} t={t}>
        <div className="table-frame"><table><thead><tr><th>{t.tokenName}</th><th>{t.prefix}</th><th>{t.environments}</th><th>mTLS</th><th>{t.expiresAt}</th><th>{t.status}</th><th /></tr></thead>
          <tbody>{state.items.map(token => <tr key={token.public_id}><td><strong title={token.display_name}>{token.display_name}</strong><code className="row-subkey">{token.public_id}</code></td><td><code title={token.display_prefix}>{token.display_prefix}</code></td><td><div className="environment-links">{token.environment_keys.map(environment => <code key={environment}>{environment}</code>)}</div></td><td>{token.allow_without_mtls ? t.tokenOnlyAllowed : t.mtlsRequired}</td><td><time>{token.expires_at ? formatDate(token.expires_at, t.locale) : t.neverExpires}</time></td><td><span className={token.revoked ? 'status archived' : 'status active'}>{token.revoked ? t.revoked : t.statusActive}</span></td><td>{!token.revoked && <div className="row-actions"><button type="button" aria-label={`${t.editEnvironments} ${token.display_name}`} onClick={() => setEditing(token.public_id)}>{t.editEnvironments}</button>{confirming === token.public_id ? <button className="danger-link" type="button" aria-label={`${t.confirmRevoke} ${token.display_name}`} onClick={() => revoke(token.public_id)}>{t.confirmRevoke}</button> : <button className="danger-link" type="button" aria-label={`${t.revoke} ${token.display_name}`} onClick={() => setConfirming(token.public_id)}>{t.revoke}</button>}</div>}</td></tr>)}</tbody>
        </table></div>
      </CollectionState>
    </section>
  );
}

function CertificatesPanel({ t }) {
  const [state, setState] = useState({ status: 'loading', items: [] });
  const [importing, setImporting] = useState(false);
  const [confirming, setConfirming] = useState('');
  const [error, setError] = useState('');
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    let live = true;
    request('/v1/client-certificates?include_revoked=true')
      .then(result => live && setState({ status: 'ready', items: result.items }))
      .catch(() => live && setState({ status: 'failed', items: [] }));
    return () => { live = false; };
  }, [refresh]);
  const register = async event => {
    event.preventDefault();
    setError('');
    const data = new FormData(event.currentTarget);
    try {
      await request('/v1/client-certificates', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({ display_name: data.get('display_name'), certificate_pem: data.get('certificate_pem') }),
      });
      setImporting(false);
      setRefresh(value => value + 1);
    } catch {
      setError(t.operationFailed);
    }
  };
  const revoke = async fingerprint => {
    try {
      await request(`/v1/client-certificates/${fingerprint}/revoke`, { method: 'POST', headers: { 'Idempotency-Key': crypto.randomUUID() } });
      setConfirming('');
      setRefresh(value => value + 1);
    } catch {
      setError(t.operationFailed);
    }
  };
  return (
    <section className="admin-panel certificates-panel">
      <div className="panel-actions"><span>{t.caVerifiedCertificates}</span><button className="primary-action compact-action" type="button" onClick={() => setImporting(value => !value)}>{t.importClientCertificate}</button></div>
      {importing && (
        <form className="certificate-form" onSubmit={register}>
          <p>{t.publicCertificateOnly}</p>
          <label>{t.certificateName}<input name="display_name" required maxLength="255" autoComplete="off" /></label>
          <label>{t.publicCertificatePEM}<textarea name="certificate_pem" required rows="8" spellCheck="false" /></label>
          <div className="form-actions"><span className="inline-error" role="alert">{error}</span><button className="primary-action compact-action" type="submit">{t.registerCertificate}</button></div>
        </form>
      )}
      {!importing && error && <p className="inline-error" role="alert">{error}</p>}
      <CollectionState state={state} t={t}>
        <div className="table-frame"><table><thead><tr><th>{t.certificateName}</th><th>{t.subject}</th><th>{t.fingerprint}</th><th>{t.validUntil}</th><th>{t.status}</th><th /></tr></thead>
          <tbody>{state.items.map(certificate => <tr key={certificate.fingerprint_sha256}><td><strong title={certificate.display_name}>{certificate.display_name}</strong><code className="row-subkey">{t.serial} {certificate.serial_hex}</code></td><td title={certificate.subject}>{certificate.subject}</td><td><code className="fingerprint" title={certificate.fingerprint_sha256}>{certificate.fingerprint_sha256}</code></td><td><time>{formatDate(certificate.not_after, t.locale)}</time></td><td><span className={certificate.revoked ? 'status archived' : 'status active'}>{certificate.revoked ? t.revoked : t.statusActive}</span></td><td>{!certificate.revoked && (confirming === certificate.fingerprint_sha256 ? <button className="danger-link" type="button" aria-label={`${t.confirmRevoke} ${certificate.display_name}`} onClick={() => revoke(certificate.fingerprint_sha256)}>{t.confirmRevoke}</button> : <button className="danger-link" type="button" aria-label={`${t.revoke} ${certificate.display_name}`} onClick={() => setConfirming(certificate.fingerprint_sha256)}>{t.revoke}</button>)}</td></tr>)}</tbody>
        </table></div>
      </CollectionState>
    </section>
  );
}

function DeploymentPanel({ t }) {
  const [ready, setReady] = useState(null);
  useEffect(() => {
    let live = true;
    request('/health/ready').then(() => live && setReady(true)).catch(() => live && setReady(false));
    return () => { live = false; };
  }, []);
  return (
    <section className="deployment-panel">
      <div className={ready === false ? 'deployment-observation observation-failed' : 'deployment-observation'}><span>{t.observedNow}</span><strong>{ready === null ? '…' : ready ? t.managementReady : t.managementNotReady}</strong><small>/health/ready</small></div>
      <div className="section-heading"><h2>{t.configuredArchitecture}</h2><span>{t.readOnlyContract}</span></div>
      <div className="deployment-grid">
        <article><span>SQL</span><strong>MySQL 8.0.22</strong><small>{t.compatibilityBaseline}</small></article>
        <article><span>LOG</span><strong>ClickHouse</strong><small>{t.applicationLogs}</small></article>
        <article><span>EVT</span><strong>NATS</strong><small>{t.nonBlockingAccessEvents}</small></article>
        <article><span>CFG</span><strong>{t.coldStartYAML}</strong><small>{t.restartToApply}</small></article>
        <article><span>API</span><strong>{t.apiScalesIndependently}</strong><small>{t.managementSingleReplica}</small></article>
      </div>
    </section>
  );
}

function Notifications({ t, embedded = false }) {
  const [state, setState] = useState({ status: 'loading', items: [] });
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState('');
  const [deliveries, setDeliveries] = useState({ status: 'idle', destination: '', items: [] });
  const [confirming, setConfirming] = useState('');
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    let live = true;
    request('/v1/notification-destinations?include_archived=true')
      .then(result => live && setState({ status: 'ready', items: result.items }))
      .catch(() => live && setState({ status: 'failed', items: [] }));
    return () => { live = false; };
  }, [refresh]);
  const save = async event => {
    event.preventDefault();
    setError('');
    const data = new FormData(event.currentTarget);
    const key = data.get('key');
    try {
      await request(`/v1/notification-destinations/${encodeURIComponent(key)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({
          display_name: data.get('display_name'),
          provider: data.get('provider'),
          url: data.get('url'),
          secret: data.get('secret'),
          enabled: data.has('enabled'),
          event_types: data.get('event_types').split(',').map(value => value.trim()).filter(Boolean),
        }),
      });
      setCreating(false);
      setRefresh(value => value + 1);
    } catch {
      setError(t.operationFailed);
    }
  };
  const mutate = async path => {
    try {
      await request(path, { method: 'POST', headers: { 'Idempotency-Key': crypto.randomUUID() } });
      return true;
    } catch {
      setError(t.operationFailed);
      return false;
    }
  };
  const loadDeliveries = async destination => {
    setDeliveries({ status: 'loading', destination, items: [] });
    try {
      const result = await request(`/v1/notification-destinations/${encodeURIComponent(destination)}/deliveries?limit=50`);
      setDeliveries({ status: 'ready', destination, items: result.items });
    } catch {
      setDeliveries({ status: 'failed', destination, items: [] });
    }
  };
  const archive = async destination => {
    if (await mutate(`/v1/notification-destinations/${encodeURIComponent(destination)}/archive`)) {
      setConfirming('');
      setRefresh(value => value + 1);
    }
  };
  const unarchive = async destination => {
    if (await mutate(`/v1/notification-destinations/${encodeURIComponent(destination)}/unarchive`)) setRefresh(value => value + 1);
  };
  return (
    <ResourcePage className={embedded ? 'notifications-page embedded-notifications' : 'notifications-page'} eyebrow="NTF / durable outbound" title={t.notifications} body={t.notificationsBody}
      action={<button className="primary-action compact-action" type="button" onClick={() => setCreating(value => !value)}>{t.newDestination}</button>}>
      {creating && (
        <form className="destination-form" onSubmit={save}>
          <label>{t.destinationKey}<input name="key" required maxLength="63" pattern="[a-z][a-z0-9_-]{0,62}" autoComplete="off" /></label>
          <label>{t.destinationName}<input name="display_name" required maxLength="255" autoComplete="off" /></label>
          <label>{t.provider}<select name="provider" defaultValue="generic_webhook"><option value="generic_webhook">{t.genericWebhook}</option><option value="feishu_bot">{t.feishuBot}</option></select></label>
          <label>{t.webhookURL}<input name="url" required type="url" autoComplete="off" /></label>
          <label>{t.signingSecret}<input name="secret" type="password" maxLength="16384" autoComplete="new-password" /></label>
          <label>{t.eventTypes}<input name="event_types" required placeholder="config.created, vault.updated" autoComplete="off" /></label>
          <label className="check-line"><input name="enabled" type="checkbox" defaultChecked />{t.enabled}</label>
          <div className="form-actions"><span className="inline-error" role="alert">{error}</span><button className="primary-action compact-action" type="submit">{t.saveDestination}</button></div>
        </form>
      )}
      {!creating && error && <p className="inline-error" role="alert">{error}</p>}
      <CollectionState state={state} t={t}>
        <div className="table-frame"><table><thead><tr><th>{t.destinationName}</th><th>{t.provider}</th><th>{t.endpoint}</th><th>{t.eventTypes}</th><th>{t.status}</th><th /></tr></thead>
          <tbody>{state.items.map(destination => <tr key={destination.key}><td><strong title={destination.display_name}>{destination.display_name}</strong><code className="row-subkey">{destination.key}</code></td><td><code>{destination.provider}</code></td><td><strong title={destination.safe_host}>{destination.safe_host}</strong> <code>{destination.masked_suffix}</code></td><td><div className="environment-links">{destination.event_types.map(eventType => <code key={eventType}>{eventType}</code>)}</div></td><td><span className={destination.archived || !destination.enabled ? 'status archived' : 'status active'}>{destination.archived ? t.statusArchived : destination.enabled ? t.enabled : t.disabled}</span></td><td>{destination.archived ? <button type="button" aria-label={`${t.unarchive} ${destination.display_name}`} onClick={() => unarchive(destination.key)}>{t.unarchive}</button> : <div className="row-actions"><button type="button" aria-label={`${t.sendTest} ${destination.display_name}`} onClick={() => mutate(`/v1/notification-destinations/${encodeURIComponent(destination.key)}/test`)}>{t.sendTest}</button><button type="button" aria-label={`${t.viewDeliveries} ${destination.display_name}`} onClick={() => loadDeliveries(destination.key)}>{t.viewDeliveries}</button>{confirming === destination.key ? <button className="danger-link" type="button" aria-label={`${t.confirmArchive} ${destination.display_name}`} onClick={() => archive(destination.key)}>{t.confirmArchive}</button> : <button className="danger-link" type="button" aria-label={`${t.archive} ${destination.display_name}`} onClick={() => setConfirming(destination.key)}>{t.archive}</button>}</div>}</td></tr>)}</tbody>
        </table></div>
      </CollectionState>
      {deliveries.status === 'ready' && <section className="delivery-section"><div className="section-heading"><h2>{t.deliveryHistory}</h2><code>{deliveries.destination}</code></div><div className="table-frame"><table><thead><tr><th>{t.eventTypes}</th><th>{t.attempt}</th><th>{t.status}</th><th>HTTP</th><th>{t.latencyMS}</th><th>{t.error}</th><th /></tr></thead><tbody>{deliveries.items.map(delivery => <tr key={delivery.id}><td><code>{delivery.event_type}</code></td><td>{delivery.attempt}</td><td>{delivery.status}</td><td>{delivery.http_status || '—'}</td><td>{delivery.latency_ms}</td><td><code>{delivery.error_code || '—'}</code></td><td>{delivery.status !== 'succeeded' && <button className="danger-link" type="button" aria-label={`${t.redeliver} ${delivery.event_type}`} onClick={() => mutate(`/v1/notification-destinations/${encodeURIComponent(deliveries.destination)}/deliveries/${delivery.id}/redeliver`)}>{t.redeliver}</button>}</td></tr>)}</tbody></table></div></section>}
    </ResourcePage>
  );
}

function AccessPage({ t }) {
  const [state, setState] = useState({ status: 'loading', items: [] });
  useEffect(() => {
    let live = true;
    request('/v1/access?limit=100').then(result => live && setState({ status: 'ready', items: result.items })).catch(() => live && setState({ status: 'failed', items: [] }));
    return () => { live = false; };
  }, []);
  const accessItems = state.status === 'ready' ? state.items : [];
  return (
    <ResourcePage className="access-page" eyebrow="ACS / ClickHouse · 90 day TTL" title={t.access} body={t.accessBody}>
      <div className="log-summary"><article><span>{t.observedResponses}</span><strong>{state.status === 'ready' ? state.items.length : '—'}</strong></article><article><span>mTLS</span><strong>{accessItems.filter(item => item.authentication === 'mtls').length}</strong></article><article><span>OIDC</span><strong>{accessItems.filter(item => item.authentication === 'oidc').length}</strong></article></div>
      <p className="scope-note">{t.accessIncomplete}</p>
      <CollectionState state={state} t={t}>
        <div className="table-frame log-table"><table><thead><tr><th>{t.time}</th><th>{t.principal}</th><th>{t.authentication}</th><th>{t.environments}</th><th>{t.resource}</th><th>{t.revision}</th><th>{t.vaultRevisions}</th></tr></thead><tbody>{state.items.map((record, index) => <tr key={`${record.time}-${record.principal}-${index}`}><td><time>{formatDate(record.time, t.locale)}</time></td><td><code title={record.principal}>{record.principal}</code></td><td>{record.authentication}</td><td><code>{record.environment || '—'}</code></td><td><span>{record.resource_type}</span><strong title={record.namespace ? `${record.namespace}/${record.resource}` : record.resource}>{record.namespace ? `${record.namespace}/${record.resource}` : record.resource}</strong></td><td>{record.config_revision ? `v${record.config_revision}` : '—'}</td><td><div className="environment-links">{Object.entries(record.vault_revisions).map(([item, revision]) => <code key={item}>{item} @ v{revision}</code>)}</div></td></tr>)}</tbody></table></div>
      </CollectionState>
    </ResourcePage>
  );
}

function AuditPage({ principal, t }) {
  const pageSize = 25;
  const [query, setQuery] = useState('');
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(0);
  const [state, setState] = useState({ status: 'loading', items: [], hasMore: false });
  useEffect(() => {
    let live = true;
    const parameters = new URLSearchParams({ limit: String(pageSize), offset: String(page * pageSize) });
    if (search) parameters.set('q', search);
    setState({ status: 'loading', items: [], hasMore: false });
    request(`/v1/audit?${parameters}`).then(result => live && setState({ status: 'ready', items: result.items, hasMore: Boolean(result.has_more) })).catch(() => live && setState({ status: 'failed', items: [], hasMore: false }));
    return () => { live = false; };
  }, [page, search]);
  const submit = event => {
    event.preventDefault();
    setPage(0);
    setSearch(query.trim());
  };
  const first = page * pageSize + 1;
  return (
    <ResourcePage className="audit-page" eyebrow="AUD / durable mutation metadata" title={t.audit} body={t.auditBody}>
      <form className="config-index-toolbar audit-toolbar" role="search" onSubmit={submit}>
        <label><span>{t.searchAudit}</span><input type="search" maxLength="256" value={query} placeholder={t.searchAuditPlaceholder} onChange={event => setQuery(event.target.value)} /></label>
        <button className="secondary-action compact-action" type="submit">{t.search}</button>
      </form>
      <p className="scope-note">{t.auditAtLeastOnce}</p>
      {state.status === 'loading' && <div className="collection-state"><span className="loading-line" /></div>}
      {state.status === 'failed' && <div className="collection-state"><p>{t.loadFailed}</p></div>}
      {state.status === 'ready' && state.items.length === 0 && <div className="collection-state"><p>{search ? t.noMatchingAudits : t.noResources}</p></div>}
      {state.status === 'ready' && state.items.length > 0 && <>
        <div className="table-frame log-table"><table><thead><tr><th>{t.time}</th><th>{t.operation}</th><th>{t.actor}</th><th>{t.action}</th><th>{t.outcome}</th><th>{t.resource}</th><th>{t.revision}</th><th>{t.delivery}</th></tr></thead><tbody>{state.items.map(record => <tr key={record.id}><td><time>{formatDate(record.time, t.locale)}</time></td><td><code title={record.operation_id}>{record.operation_id}</code></td><td><ActorIdentity actorID={record.actor_id} principal={principal} /></td><td><code>{record.action}</code></td><td><span className={record.outcome === 'success' ? 'status active' : 'status archived'}>{record.outcome}</span></td><td><span>{record.resource_type}</span><strong title={record.namespace ? `${record.namespace}/${record.resource}` : record.resource}>{record.namespace ? `${record.namespace}/${record.resource}` : record.resource}</strong></td><td>{record.revision ? `v${record.revision}` : '—'}</td><td>#{record.delivery_attempt}</td></tr>)}</tbody></table></div>
        <nav className="config-pagination" aria-label={t.auditPagination}><span>{first}–{first + state.items.length - 1}</span><div><button type="button" disabled={page === 0} onClick={() => setPage(value => value - 1)}>{t.previous}</button><code>{t.pageNumber.replace('{page}', page + 1)}</code><button type="button" disabled={!state.hasMore} onClick={() => setPage(value => value + 1)}>{t.next}</button></div></nav>
      </>}
    </ResourcePage>
  );
}

const configEditorTheme = EditorView.theme({
  '&': { height: '100%', backgroundColor: 'var(--editor-surface)', color: 'var(--ink)' },
  '.cm-scroller': { fontFamily: 'ui-monospace, "SFMono-Regular", Consolas, monospace', lineHeight: '1.65' },
  '.cm-content': { padding: '14px 0' },
  '.cm-gutters': { borderRight: '1px solid var(--line)', backgroundColor: 'var(--surface-muted)', color: 'var(--muted)' },
  '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: 'var(--surface-muted)' },
  '&.cm-focused': { outline: 'none' },
});

const configHighlightStyle = HighlightStyle.define([
  { tag: [tags.propertyName, tags.attributeName, tags.typeName], color: 'var(--syntax-key)', fontWeight: '650' },
  { tag: [tags.string, tags.character, tags.attributeValue], color: 'var(--syntax-string)' },
  { tag: [tags.number, tags.integer, tags.float], color: 'var(--syntax-number)' },
  { tag: [tags.keyword, tags.atom, tags.bool, tags.null], color: 'var(--syntax-keyword)', fontWeight: '650' },
  { tag: [tags.comment, tags.docComment], color: 'var(--syntax-comment)', fontStyle: 'italic' },
  { tag: [tags.punctuation, tags.bracket, tags.operator, tags.separator], color: 'var(--syntax-punctuation)' },
  { tag: tags.invalid, color: 'var(--danger)', textDecoration: 'underline' },
]);

const configEditorNonce = document.querySelector('meta[name="csp-nonce"]')?.content;
const configLanguage = format => format === 'json' ? json() : yaml();
const configEditorExtensions = (format, readOnly, label) => [
  basicSetup,
  configLanguage(format),
  configEditorTheme,
  syntaxHighlighting(configHighlightStyle),
  EditorState.tabSize.of(2),
  EditorState.readOnly.of(readOnly),
  EditorView.editable.of(!readOnly),
  EditorView.contentAttributes.of({ 'aria-label': label, 'aria-readonly': String(readOnly), spellcheck: 'false' }),
  ...(configEditorNonce ? [EditorView.cspNonce.of(configEditorNonce)] : []),
];

function ConfigCodeEditor({ value, format, label, readOnly = true, onChange, className = '' }) {
  const host = useRef(null);
  const view = useRef(null);
  const change = useRef(onChange);
  const syncing = useRef(false);
  change.current = onChange;

  useEffect(() => {
    const update = EditorView.updateListener.of(event => {
      if (event.docChanged && !syncing.current) change.current?.(event.state.doc.toString());
    });
    view.current = new EditorView({
      doc: value,
      extensions: [...configEditorExtensions(format, readOnly, label), update],
      parent: host.current,
    });
    return () => {
      view.current?.destroy();
      view.current = null;
    };
  }, [format, label, readOnly]);

  useEffect(() => {
    if (!view.current || view.current.state.doc.toString() === value) return;
    syncing.current = true;
    view.current.dispatch({ changes: { from: 0, to: view.current.state.doc.length, insert: value } });
    syncing.current = false;
  }, [value]);

  return <div className={`config-code-editor ${readOnly ? 'read-only ' : ''}${className}`} data-format={format} ref={host} />;
}

function ConfigDiffEditor({ source, target, sourceFormat, targetFormat, sourceLabel, targetLabel, t }) {
  const host = useRef(null);
  useEffect(() => {
    const merge = new MergeView({
      a: { doc: source, extensions: configEditorExtensions(sourceFormat, true, sourceLabel) },
      b: { doc: target, extensions: configEditorExtensions(targetFormat, true, targetLabel) },
      parent: host.current,
      highlightChanges: true,
      gutter: true,
      collapseUnchanged: { margin: 3, minSize: 8 },
    });
    return () => merge.destroy();
  }, [source, sourceFormat, sourceLabel, target, targetFormat, targetLabel]);

  return <section className="config-diff-panel">
    <header className="config-diff-heading">
      <strong>{sourceLabel}<code>{sourceFormat.toUpperCase()}</code></strong>
      <span className="diff-legend"><i className="removed" />{t.diffRemoved}<i className="added" />{t.diffAdded}</span>
      <strong>{targetLabel}<code>{targetFormat.toUpperCase()}</code></strong>
    </header>
    <div className="config-diff-editor" ref={host} />
  </section>;
}

function ConfigDetail({ principal, configKey, environmentKey, inventory, t }) {
  const [state, setState] = useState({ status: 'loading' });
  const [source, setSource] = useState('');
  const [tab, setTab] = useState('raw');
  const [transferMode, setTransferMode] = useState('merge');
  const [refresh, setRefresh] = useState(0);
  const [saveError, setSaveError] = useState(null);
  const [validation, setValidation] = useState({ status: 'idle', warnings: [] });
  const [compareSeed, setCompareSeed] = useState(null);
  const [viewedRevision, setViewedRevision] = useState(0);
  const [inspection, setInspection] = useState({ status: 'idle' });
  const [restoring, setRestoring] = useState(0);
  const path = `/v1/environments/${encodeURIComponent(environmentKey)}/configs/${encodeURIComponent(configKey)}`;
  const dirty = state.status === 'ready' && source !== state.config.content;
  const confirmDiscard = useUnsavedChanges(dirty, t.unsavedChanges);
  const environments = useMemo(() => {
    const contexts = inventory.status === 'ready'
      ? inventory.configs.find(config => config.key === configKey)?.environments.filter(environment => !environment.archived) || []
      : [];
    return contexts.some(environment => environment.key === environmentKey)
      ? contexts
      : [{ key: environmentKey }, ...contexts];
  }, [configKey, environmentKey, inventory]);
  useEffect(() => {
    let live = true;
    Promise.all([request(path), request(`${path}/revisions`)])
      .then(([config, revisions]) => {
        if (!live) return;
        setState({ status: 'ready', config, revisions: revisions.items });
        setSource(config.content);
        setViewedRevision(config.revision);
        setInspection({ status: 'idle' });
      })
      .catch(() => live && setState({ status: 'failed' }));
    return () => { live = false; };
  }, [path, refresh]);
  useEffect(() => {
    if (state.status !== 'ready' || !viewedRevision || viewedRevision === state.config.revision) return undefined;
    let live = true;
    setInspection({ status: 'loading', revision: viewedRevision });
    request(`${path}/revisions/${viewedRevision}`)
      .then(config => live && setInspection({ status: 'ready', revision: viewedRevision, config }))
      .catch(() => live && setInspection({ status: 'failed', revision: viewedRevision }));
    return () => { live = false; };
  }, [path, state, viewedRevision]);

  const save = async () => {
    setSaveError(null);
    setValidation(current => ({ ...current, status: 'saving' }));
    try {
      const result = await request(path, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({
          name: state.config.config_name,
          expected_revision: state.config.revision,
          format: state.config.format,
          content: source,
        }),
      });
      setValidation({ status: 'saved', revision: result.revision, warnings: result.warnings || [] });
      setRefresh(value => value + 1);
    } catch (error) {
      setValidation(current => ({ ...current, status: 'idle' }));
      setSaveError(managementError(error, t, t.saveFailed));
    }
  };
  const formatSource = async () => {
    setSaveError(null);
    setValidation({ status: 'loading', warnings: [] });
    try {
      const result = await request('/v1/configs/validate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ environment: environmentKey, format: state.config.format, content: source }),
      });
      setSource(result.content);
      setValidation({ status: 'ready', warnings: result.warnings || [] });
    } catch (error) {
      setValidation({ status: 'failed', warnings: [] });
      setSaveError(managementError(error, t, t.validationFailed));
    }
  };
  const compare = revision => {
    setCompareSeed({ environment: environmentKey, revision });
    setTab('compare');
  };
  const inspectRevision = revision => {
    setViewedRevision(revision);
    setInspection({ status: revision === state.config.revision ? 'idle' : 'loading', revision });
    setTab('raw');
  };
  const showCurrent = nextTab => {
    if (state.status === 'ready') setViewedRevision(state.config.revision);
    setInspection({ status: 'idle' });
    setTab(nextTab);
  };
  const openTransfer = mode => {
    setTransferMode(mode);
    setTab('transfer');
  };
  const restore = async revision => {
    setSaveError(null);
    try {
      const result = await request(`${path}/restore`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({ source_revision: revision, expected_revision: state.config.revision }),
      });
      setValidation({ status: 'saved', warnings: result.warnings || [] });
      setState(current => ({ ...current, config: { ...current.config, revision: result.revision } }));
      setCompareSeed(null);
      setRestoring(0);
      setRefresh(value => value + 1);
    } catch (error) {
      setSaveError(managementError(error, t, t.operationFailed));
    }
  };

  if (state.status !== 'ready') return <CollectionState state={{ status: state.status, items: [] }} t={t} />;
  const selectedRevision = state.revisions.find(revision => revision.revision === viewedRevision);
  const viewingCurrent = viewedRevision === state.config.revision;
  const viewedFormat = viewingCurrent ? state.config.format : inspection.config?.format || selectedRevision?.format || state.config.format;
  const viewedContent = viewingCurrent ? source : inspection.config?.content || '';
  const operationMode = tab === 'compare' || tab === 'transfer';
  const operationTitle = tab === 'compare'
    ? t.compareRevisions
    : tab === 'transfer'
      ? `${t[transferMode]} · ${t.source} → ${t.target}`
      : tab === 'clone'
        ? t.cloneConfig
        : '';
  return (
    <div className="page config-detail">
      <div className="config-detail-heading">
        <div>
          <p className="eyebrow"><a href="#/configs">{t.configs}</a> / <a href={`#/configs/${configKey}`}><code>{configKey}</code></a> / {environmentKey}</p>
          <div className="config-title-line"><h1 title={state.config.config_name}>{state.config.config_name}</h1><code title={configKey}>{configKey}</code><span>{t.current} v{state.config.revision}</span></div>
        </div>
        <div className="detail-heading-actions">
          <label className="config-environment-select"><span>{t.environment}</span><select aria-label={t.environment} value={environmentKey} onChange={event => { if (confirmDiscard()) window.location.hash = `/configs/${configKey}/${event.target.value}`; }}>{environments.map(environment => <option key={environment.key} value={environment.key}>{environment.key}</option>)}</select></label>
          <div className="detail-action-group">
            <div className="config-operation-switch">
              <button type="button" aria-pressed={tab === 'compare'} onClick={() => { setCompareSeed(null); setTab('compare'); }}>{t.compare}</button>
              {principal.role === 'admin' && <button type="button" aria-pressed={tab === 'transfer' && transferMode === 'merge'} onClick={() => openTransfer('merge')}>{t.merge}</button>}
              {principal.role === 'admin' && <button type="button" aria-pressed={tab === 'transfer' && transferMode === 'replace'} onClick={() => openTransfer('replace')}>{t.replace}</button>}
            </div>
            {principal.role === 'admin' && <button className="config-edit-action" type="button" aria-pressed={tab === 'raw' && viewingCurrent} onClick={() => showCurrent('raw')}>{t.editConfig}</button>}
            {principal.role === 'admin' && <details className="detail-more-actions"><summary aria-label={t.moreActions}>•••</summary><button type="button" onClick={event => { event.currentTarget.closest('details').removeAttribute('open'); setTab('clone'); }}>{t.clone}</button></details>}
          </div>
        </div>
      </div>
      {!operationMode && <div className="revision-track" aria-label={t.revisionHistory}>
        {state.revisions.slice(0, 8).reverse().map(revision => (
          <button type="button" className={`${revision.revision === state.config.revision ? 'current-revision ' : ''}${revision.revision === viewedRevision ? 'selected-revision' : ''}`} aria-pressed={revision.revision === viewedRevision} onClick={() => inspectRevision(revision.revision)} key={revision.revision}><b>v{revision.revision}</b><small>{revision.revision === state.config.revision ? t.current : formatDate(revision.created_at, t.locale).split(',')[0]}</small></button>
        ))}
      </div>}
      <div className={operationMode ? 'config-detail-content operation-mode' : 'config-detail-content'}>
        <div className="config-detail-main">
          {operationTitle ? <div className="config-operation-heading"><h2>{operationTitle}</h2><button type="button" onClick={() => showCurrent('raw')}>{t.cancel}</button></div> : <div className="tabbar" role="tablist" aria-label={t.configView}>
            <button type="button" role="tab" aria-selected={tab === 'raw'} onClick={() => showCurrent('raw')}>{t.raw}</button>
            {principal.role === 'admin' && <button type="button" role="tab" aria-selected={tab === 'resolved'} onClick={() => showCurrent('resolved')}>{t.resolvedPreview}</button>}
            <button type="button" role="tab" aria-selected={tab === 'history'} onClick={() => setTab('history')}>{t.history}</button>
          </div>}
          {tab === 'raw' ? (
            inspection.status === 'loading' ? <div className="collection-state"><span className="loading-line" /></div>
              : inspection.status === 'failed' ? <p className="inline-error" role="alert">{t.operationFailed}</p>
                : <section className="editor-panel">
                  <div className="editor-toolbar"><span>{viewedFormat.toUpperCase()}</span><code>{t.revision} {viewedRevision}</code></div>
                  <ConfigCodeEditor label={viewingCurrent ? t.configurationSource : t.revisionLabel.replace('{revision}', viewedRevision)} value={viewedContent} format={viewedFormat} readOnly={!viewingCurrent || principal.role !== 'admin'} onChange={value => { setSource(value); setSaveError(null); setValidation({ status: 'idle', warnings: [] }); }} />
                  {principal.role === 'admin' && viewingCurrent && <div className="editor-actions">
                    <div className="editor-feedback" role="status">
                      {validation.status === 'saved' ? t.savedRevision.replace('{revision}', validation.revision) : validation.status === 'ready' && validation.warnings.length === 0 ? t.formatComplete : ''}
                      <ActionError error={saveError} t={t} />
                      <ConfigWarningLines warnings={validation.warnings} t={t} />
                    </div>
                    <div className="row-actions"><button type="button" disabled={validation.status === 'loading' || validation.status === 'saving'} onClick={formatSource}>{t.formatSource}</button><button className="primary-action compact-action" type="button" disabled={!dirty || validation.status === 'saving'} onClick={save}>{t.saveChanges}</button></div>
                  </div>}
                </section>
          ) : tab === 'resolved' ? (
            <ResolvedPreview path={path} t={t} />
          ) : tab === 'history' ? (
            <section className="history-view">
              <div className="table-frame history-table"><table><thead><tr><th>{t.revision}</th><th>{t.format}</th><th>{t.actor}</th><th>{t.created}</th><th>{t.action}</th></tr></thead>
                <tbody>{state.revisions.map(revision => <tr className={revision.revision === viewedRevision ? 'selected-history-row' : ''} key={revision.revision}><td><button className="revision-link" type="button" aria-label={t.viewRevision.replace('{revision}', revision.revision)} onClick={() => inspectRevision(revision.revision)}>v{revision.revision}</button></td><td><code>{revision.format}</code></td><td><ActorIdentity actorID={revision.actor_id} principal={principal} /></td><td><time>{formatDate(revision.created_at, t.locale)}</time></td><td><div className="row-actions"><button type="button" aria-label={t.compareRevision.replace('{revision}', revision.revision)} onClick={() => compare(revision.revision)}>{t.compare}</button>{principal.role === 'admin' && revision.revision !== state.config.revision && (restoring === revision.revision
                  ? <button className="danger-link" type="button" aria-label={t.confirmRestore.replace('{revision}', revision.revision)} onClick={() => restore(revision.revision)}>{t.confirmRestore.replace('{revision}', revision.revision)}</button>
                  : <button type="button" aria-label={t.restoreRevision.replace('{revision}', revision.revision)} onClick={() => setRestoring(revision.revision)}>{t.restore}</button>)}</div></td></tr>)}</tbody>
              </table></div>
              <ActionError error={saveError} t={t} />
            </section>
          ) : tab === 'compare' ? (
            <ConfigRevisionWorkbench
              key={`${environmentKey}/${configKey}@${state.config.revision}/${compareSeed?.revision || 0}`}
              mode="compare"
              configKey={configKey}
              currentEnvironment={environmentKey}
              currentRevision={state.config.revision}
              currentRevisions={state.revisions}
              currentContent={state.config.content}
              currentFormat={state.config.format}
              environments={environments}
              initialSource={compareSeed}
              t={t}
            />
          ) : tab === 'transfer' ? (
            <ConfigRevisionWorkbench key={`${environmentKey}/${configKey}/${transferMode}`} mode={transferMode} configKey={configKey} currentEnvironment={environmentKey} currentRevision={state.config.revision} currentRevisions={state.revisions} currentContent={state.config.content} currentFormat={state.config.format} environments={environments} t={t}
              onCommitted={(result, targetEnvironment) => {
                setValidation({ status: 'saved', warnings: result.warnings || [] });
                showCurrent('raw');
                if (targetEnvironment === environmentKey) setRefresh(value => value + 1);
                else window.location.hash = `/configs/${configKey}/${targetEnvironment}`;
              }} />
          ) : (
            <ConfigClone path={path} t={t} />
          )}
        </div>
        {!operationMode && <aside className="config-meta-panel">
          <h2>{t.configIdentity}</h2>
          <dl>
            <div><dt>{t.resourceKey}</dt><dd><code>{configKey}</code></dd></div>
            <div><dt>{t.environment}</dt><dd><code>{environmentKey}</code></dd></div>
            <div><dt>{t.revision}</dt><dd><strong>v{viewedRevision}</strong>{viewingCurrent && ` · ${t.current}`}</dd></div>
            <div><dt>{t.format}</dt><dd>{(selectedRevision?.format || state.config.format).toUpperCase()}</dd></div>
            {selectedRevision && <><div><dt>{t.created}</dt><dd><time>{formatDate(selectedRevision.created_at, t.locale)}</time></dd></div><div><dt>{t.actor}</dt><dd><ActorIdentity actorID={selectedRevision.actor_id} principal={principal} /></dd></div></>}
          </dl>
        </aside>}
      </div>
    </div>
  );
}

function CompareRevisionLane({ side, environment, environments, currentRevision, pool, source, target, onEnvironmentChange, onChoose, t }) {
  return <section className="compare-revision-lane">
    <label><span>{side === 'left' ? t.leftEnvironment : t.rightEnvironment}</span><select value={environment} onChange={event => onEnvironmentChange(event.target.value)}>{environments.map(item => <option key={item.key} value={item.key}>{item.key}</option>)}</select></label>
    <small>{t.revisionPool}</small>
    <div className="compare-revision-list">
      {pool?.status === 'loading' && <span className="loading-line" />}
      {pool?.status === 'failed' && <p className="inline-error">{t.operationFailed}</p>}
      {pool?.items?.map(revision => {
        const selection = { environment, revision: revision.revision };
        const identity = `${environment}@v${revision.revision}`;
        const selectedAs = source?.environment === environment && source.revision === revision.revision
          ? t.source
          : target?.environment === environment && target.revision === revision.revision
            ? t.target
            : '';
        const isCurrent = revision.revision === currentRevision;
        return <article className={selectedAs ? 'compare-revision-card selected' : 'compare-revision-card'} draggable onDragStart={event => { event.dataTransfer.effectAllowed = 'copy'; event.dataTransfer.setData('application/x-configra-revision', JSON.stringify(selection)); }} data-testid={`revision-${environment}-${revision.revision}`} key={revision.revision}>
          <div><strong>v{revision.revision}</strong><code>{revision.format.toUpperCase()}</code>{isCurrent && <span>{t.current}</span>}{selectedAs && <span>{selectedAs}</span>}</div>
          <time>{formatDate(revision.created_at, t.locale)}</time>
          <div className="compare-card-actions"><button type="button" aria-label={t.useAsSource.replace('{revision}', identity)} onClick={() => onChoose('source', selection)}>{t.setSource}</button><button type="button" aria-label={t.useAsTarget.replace('{revision}', identity)} onClick={() => onChoose('target', selection)}>{t.setTarget}</button></div>
        </article>;
      })}
    </div>
  </section>;
}

function ConfigRevisionWorkbench({ mode, configKey, currentEnvironment, currentRevision, currentRevisions, currentContent, currentFormat, environments, initialSource, t, onCommitted }) {
  const otherEnvironment = environments.find(environment => environment.key !== currentEnvironment)?.key || currentEnvironment;
  const [leftEnvironment, setLeftEnvironment] = useState(currentEnvironment);
  const [rightEnvironment, setRightEnvironment] = useState(otherEnvironment);
  const [pools, setPools] = useState({});
  const [source, setSource] = useState(initialSource || null);
  const [target, setTarget] = useState(null);
  const [comparison, setComparison] = useState({ status: 'idle' });
  const [actionError, setActionError] = useState('');
  const environmentOptions = environments.length > 0 ? environments : [{ key: currentEnvironment }];
  const declaredCurrent = Object.fromEntries(environmentOptions.map(environment => [environment.key, environment.key === currentEnvironment ? currentRevision : environment.revision]));
  const currentFor = environment => declaredCurrent[environment] || Math.max(0, ...(pools[environment]?.items || []).map(revision => revision.revision));

  useEffect(() => {
    let live = true;
    const keys = [...new Set([leftEnvironment, rightEnvironment])];
    setPools(current => ({ ...current, ...Object.fromEntries(keys.map(key => [key, { status: 'loading', items: [] }])) }));
    Promise.all(keys.map(async environment => {
      if (environment === currentEnvironment) return [environment, currentRevisions];
      const result = await request(`/v1/environments/${encodeURIComponent(environment)}/configs/${encodeURIComponent(configKey)}/revisions`);
      return [environment, result.items];
    })).then(results => {
      if (!live) return;
      setPools(current => ({ ...current, ...Object.fromEntries(results.map(([key, items]) => [key, { status: 'ready', items }])) }));
    }).catch(() => live && setPools(current => ({ ...current, ...Object.fromEntries(keys.map(key => [key, { status: 'failed', items: [] }])) })));
    return () => { live = false; };
  }, [configKey, currentEnvironment, currentRevisions, leftEnvironment, rightEnvironment]);

  useEffect(() => {
    if (!source || !target) {
      setComparison({ status: 'idle' });
      return undefined;
    }
    let live = true;
    setComparison({ status: 'loading' });
    const read = selection => selection.environment === currentEnvironment && selection.revision === currentRevision
      ? Promise.resolve({ content: currentContent, format: currentFormat })
      : request(`/v1/environments/${encodeURIComponent(selection.environment)}/configs/${encodeURIComponent(configKey)}/revisions/${selection.revision}`);
    const operation = mode === 'compare'
      ? Promise.all([read(source), read(target)]).then(([sourceConfig, targetConfig]) => ({ source: sourceConfig.content, target: targetConfig.content, sourceFormat: sourceConfig.format, targetFormat: targetConfig.format }))
      : Promise.all([
        request(`/v1/environments/${encodeURIComponent(target.environment)}/configs/${encodeURIComponent(configKey)}/transfer-preview`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ mode, source_environment: source.environment, source_config: configKey, source_revision: source.revision, target_revision: target.revision }),
        }),
        read(target),
      ]).then(([preview, targetConfig]) => ({ target: targetConfig.content, result: preview.content, targetFormat: targetConfig.format, resultFormat: preview.format, expectedTargetRevision: preview.expected_target_revision }));
    operation
      .then(result => live && setComparison({ status: 'ready', ...result }))
      .catch(error => live && setComparison({ status: 'failed', code: error.code }));
    return () => { live = false; };
  }, [configKey, currentContent, currentEnvironment, currentFormat, currentRevision, mode, source, target]);

  const sameRevision = (left, right) => left && right && left.environment === right.environment && left.revision === right.revision;
  const choose = (slot, selection) => {
    if (!selection) {
      if (slot === 'source') setSource(null);
      else setTarget(null);
      setActionError('');
      return;
    }
    setActionError('');
    if (slot === 'source') {
      setSource(selection);
      if (sameRevision(selection, target)) setTarget(null);
      return;
    }
    setTarget(selection);
    if (sameRevision(selection, source)) setSource(null);
  };
  const drop = (event, slot) => {
    event.preventDefault();
    try {
      const selection = JSON.parse(event.dataTransfer.getData('application/x-configra-revision'));
      if (selection.environment && Number.isInteger(selection.revision)) choose(slot, selection);
    } catch {
      // Ignore drags from outside this workbench.
    }
  };
  const identity = selection => selection ? `${selection.environment}/${configKey}@v${selection.revision}` : '—';
  const selectedFormat = selection => pools[selection?.environment]?.items.find(revision => revision.revision === selection.revision)?.format || (selection?.environment === currentEnvironment && selection.revision === currentRevision ? currentFormat : '');
  const commit = async () => {
    setActionError('');
    setComparison(current => ({ ...current, saving: true }));
    try {
      const result = await request(`/v1/environments/${encodeURIComponent(target.environment)}/configs/${encodeURIComponent(configKey)}/${mode}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({ source_environment: source.environment, source_config: configKey, source_revision: source.revision, target_revision: target.revision, expected_target_revision: comparison.expectedTargetRevision }),
      });
      onCommitted(result, target.environment);
    } catch (error) {
      setComparison(current => ({ ...current, saving: false }));
      setActionError(error.status === 409 ? t.saveConflict : t.saveFailed);
    }
  };

  return <section className="config-compare-workbench">
    <p className="compare-drag-hint">{mode === 'compare' ? t.compareDragHint : t.transferDragHint}</p>
    <div className="compare-picker">
      <CompareRevisionLane side="left" environment={leftEnvironment} environments={environmentOptions} currentRevision={currentFor(leftEnvironment)} pool={pools[leftEnvironment]} source={source} target={target} onEnvironmentChange={setLeftEnvironment} onChoose={choose} t={t} />
      <div className="compare-drop-stack">
        {['source', 'target'].map(slot => {
          const selection = slot === 'source' ? source : target;
          return <div className={selection ? 'compare-drop-slot filled' : 'compare-drop-slot'} aria-label={t[slot]} onDragEnter={event => event.currentTarget.classList.add('drag-over')} onDragLeave={event => event.currentTarget.classList.remove('drag-over')} onDragOver={event => { event.preventDefault(); event.dataTransfer.dropEffect = 'copy'; }} onDrop={event => { event.currentTarget.classList.remove('drag-over'); drop(event, slot); }} data-testid={`compare-${slot}-slot`} key={slot}>
            <span>{t[slot]}</span><strong>{selection ? identity(selection) : t.dropRevisionHere}</strong>
            {selection && <button type="button" onClick={() => choose(slot, null)}>×</button>}
          </div>;
        })}
      </div>
      <CompareRevisionLane side="right" environment={rightEnvironment} environments={environmentOptions} currentRevision={currentFor(rightEnvironment)} pool={pools[rightEnvironment]} source={source} target={target} onEnvironmentChange={setRightEnvironment} onChoose={choose} t={t} />
    </div>
    {actionError && <p className="inline-error compare-action-error" role="alert">{actionError}</p>}
    {comparison.status === 'loading' && <div className="collection-state compare-loading"><span className="loading-line" /></div>}
    {comparison.status === 'ready' && (mode === 'compare' ? <ConfigDiffEditor source={comparison.source} target={comparison.target} sourceFormat={comparison.sourceFormat} targetFormat={comparison.targetFormat} sourceLabel={identity(source)} targetLabel={identity(target)} t={t} /> : <div className="transfer-result">
      <div className="transfer-direction"><code>{identity(source)}</code><span aria-hidden="true">→</span><code>{identity(target)}</code></div>
      <ConfigDiffEditor source={comparison.target} target={comparison.result} sourceFormat={comparison.targetFormat} targetFormat={comparison.resultFormat} sourceLabel={t.targetBefore} targetLabel={t.resultAfter} t={t} />
      <div className="editor-actions"><span className="transfer-conflict-hint">{t.targetConflictHint.replace('{revision}', comparison.expectedTargetRevision)}</span><button className="primary-action compact-action" type="button" disabled={comparison.saving} onClick={commit}>{(mode === 'merge' ? t.mergeInto : t.replaceInto).replace('{environment}', target.environment)}</button></div>
    </div>)}
    {comparison.status === 'failed' && <p className="inline-error" role="alert">{comparison.code === 'format_mismatch'
      ? t.mergeFormatMismatch.replace('{source}', selectedFormat(source).toUpperCase()).replace('{target}', selectedFormat(target).toUpperCase())
      : t.operationFailed}</p>}
  </section>;
}

function ResolvedPreview({ path, t }) {
  const [state, setState] = useState({ status: 'idle' });
  const reveal = async () => {
    setState({ status: 'loading' });
    try {
      setState({ status: 'ready', value: await request(`${path}/resolved-preview`) });
    } catch {
      setState({ status: 'failed' });
    }
  };
  if (state.status !== 'ready') {
    return (
      <section className="reveal-panel">
        <p>{state.status === 'failed' ? t.operationFailed : t.resolvedWarning}</p>
        <button className="primary-action compact-action" type="button" disabled={state.status === 'loading'} onClick={reveal}>{t.revealResolvedValues}</button>
      </section>
    );
  }
  return (
    <section className="transfer-result resolved-result">
      <div className="resolved-meta"><strong>{t.vaultRevisions}</strong>{Object.entries(state.value.vault_revisions).map(([item, revision]) => <code key={item}>{item} @ v{revision}</code>)}</div>
      <ConfigCodeEditor label={t.resolvedConfiguration} readOnly value={state.value.content} format={state.value.format} />
    </section>
  );
}

function ConfigClone({ path, t }) {
  const [state, setState] = useState({ status: 'idle', error: '' });
  const clone = async event => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    setState({ status: 'loading', error: '' });
    try {
      const result = await request(`${path}/clone`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({ target_environment: data.get('target_environment'), target_config: data.get('target_config'), target_name: data.get('target_name') }),
      });
      setState({ status: 'ready', error: '', warnings: result.warnings || [] });
    } catch {
      setState({ status: 'failed', error: t.operationFailed });
    }
  };
  return <section className="transfer-panel"><form className="clone-form" onSubmit={clone}>
    <label>{t.targetEnvironment}<input name="target_environment" required pattern="[a-z][a-z0-9_-]{0,62}" /></label>
    <label>{t.targetConfig}<input name="target_config" required pattern="[a-z][a-z0-9_-]{0,62}" /></label>
    <label>{t.targetName}<input name="target_name" required maxLength="255" /></label>
    <div className="form-actions"><span className={state.status === 'ready' ? 'success-note' : 'inline-error'} role="status">{state.status === 'ready' ? t.cloneComplete : state.error}</span><button className="primary-action compact-action" type="submit" disabled={state.status === 'loading'}>{t.cloneConfig}</button></div>
    {state.warnings?.length > 0 && <div className="config-warning-list" role="status"><ConfigWarningLines warnings={state.warnings} t={t} /></div>}
  </form></section>;
}

function ConfigWarningLines({ warnings, t }) {
  return warnings.map(warning => <span key={`${warning.code}-${warning.namespace_key}-${warning.item_key}-${warning.field_key}`}>{t[warning.code] || warning.code}: <code>{`{vault.${warning.namespace_key}.${warning.item_key}.${warning.field_key}}`}</code></span>);
}

function ResourcePage({ eyebrow, title, body, action, children, className = '' }) {
  return <div className={`page ${className}`.trim()}><div className="resource-heading"><div><p className="eyebrow">{eyebrow}</p><h1>{title}</h1><p>{body}</p></div>{action}</div>{children}</div>;
}

function CollectionState({ state, t, children }) {
  if (state.status === 'loading') return <div className="collection-state"><span className="loading-line" /></div>;
  if (state.status === 'failed') return <div className="collection-state"><p>{t.loadFailed}</p></div>;
  if (state.items.length === 0) return <div className="collection-state"><p>{t.noResources}</p></div>;
  return children;
}

function formatDate(value, locale) {
  if (!value) return '—';
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}

function ActorIdentity({ actorID, principal }) {
  if (!actorID) return '—';
  const separator = actorID.lastIndexOf('|');
  const issuer = separator > 0 ? actorID.slice(0, separator) : '';
  const subject = separator > 0 ? actorID.slice(separator + 1) : actorID;
  const currentActorID = principal?.issuer ? `${principal.issuer}|${principal.subject}` : principal?.subject;
  const compactSubject = subject.length > 18 ? `${subject.slice(0, 8)}…${subject.slice(-6)}` : subject;
  const isCurrent = actorID === currentActorID;
  let provider = '';
  if (issuer) {
    try {
      provider = new URL(issuer).host;
    } catch {
      provider = issuer;
    }
  }
  return <span className="actor-identity" title={actorID}>
    <strong>{isCurrent && principal?.email ? principal.email : issuer ? compactSubject : subject}</strong>
    {provider && <code>{isCurrent ? `${provider} · ${compactSubject}` : provider}</code>}
  </span>;
}

function Pending({ route, t }) {
  return <div className="page pending"><p className="eyebrow">Configra / {route}</p><h1>{t.notFoundTitle}</h1><p>{t.notFoundBody}</p><a className="secondary-action compact-action pending-action" href="#/overview">{t.overview}</a></div>;
}

function Forbidden({ t }) {
  return <div className="page pending"><p className="eyebrow">Configra / 403</p><h1>{t.forbiddenTitle}</h1><p>{t.forbiddenBody}</p><a className="secondary-action compact-action pending-action" href="#/overview">{t.overview}</a></div>;
}

function App() {
  const [language, setLanguageState] = useState(() => localStorage.getItem('configra-language') || (navigator.language.startsWith('zh') ? 'zh' : 'en'));
  const [theme, setThemeState] = useState(() => {
    const saved = localStorage.getItem('configra-theme');
    const initial = saved === 'light' || saved === 'dark' ? saved : window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    document.documentElement.dataset.theme = initial;
    return initial;
  });
  const [session, setSession] = useState({ status: 'loading' });
  const t = messages[language];
  const setLanguage = value => {
    localStorage.setItem('configra-language', value);
    document.documentElement.lang = value === 'zh' ? 'zh-CN' : 'en';
    setLanguageState(value);
  };
  const setTheme = value => {
    localStorage.setItem('configra-theme', value);
    document.documentElement.dataset.theme = value;
    setThemeState(value);
  };
  const load = () => {
    setSession({ status: 'loading' });
    request('/v1/me')
      .then(principal => setSession({ status: 'ready', principal }))
      .catch(error => setSession({ status: error.status === 401 ? 'anonymous' : 'failed' }));
  };
  useEffect(load, []);
  useEffect(() => { document.documentElement.lang = language === 'zh' ? 'zh-CN' : 'en'; }, [language]);

  if (session.status === 'loading') return <Loading />;
  if (session.status === 'anonymous') return <Login language={language} setLanguage={setLanguage} theme={theme} setTheme={setTheme} t={t} />;
  if (session.status === 'failed') return <Failure retry={load} t={t} />;
  return <Shell principal={session.principal} language={language} setLanguage={setLanguage} theme={theme} setTheme={setTheme} t={t} />;
}

createRoot(document.getElementById('root')).render(<App />);
