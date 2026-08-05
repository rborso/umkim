import { useEffect, useState } from 'react';
import { Alert, Button, Form, Label, TextInput, Title } from '@patternfly/react-core';
import { CheckCircleIcon, TimesCircleIcon } from '@patternfly/react-icons';
import { configureFleetClusterStorage, validateFleetClusterStorage } from '../../api/client';
import type {
  FleetCluster,
  FleetClusterConfigureStorage,
  FleetClusterStorageValidateResult,
} from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';
import { LabeledField } from '../forms/LabeledField';
import { getFieldHelp } from '../../help/fieldHelp';
import { NfsStorageSetupHelp } from './NfsStorageSetupHelp';

type Props = {
  cluster: FleetCluster;
  onUpdated: () => void;
};

const CHECK_LABELS: Record<string, string> = {
  nfs_utils: 'nfs-utils package',
  nfs_reachable: 'NFS server reachable',
  export_visible: 'Export visible',
  cluster_path: 'Cluster path on delegate',
  mount_write: 'Mount root is directory',
  provisioner_subdir: 'Provisioner subdirectory',
  nfs_client_path: 'Client mount path',
  ansible: 'Ansible job',
};

function phaseColor(phase: string): 'green' | 'blue' | 'orange' | 'red' | 'grey' {
  switch (phase) {
    case 'Applied':
      return 'green';
    case 'Applying':
    case 'Pending':
      return 'orange';
    case 'Failed':
      return 'red';
    default:
      return 'grey';
  }
}

function nfsStorageBody(
  cluster: FleetCluster,
  clusterName: string,
  nfsServer: string,
  nfsExportBase: string,
  nfsDelegateHost: string,
  nfsMountPath: string,
): FleetClusterConfigureStorage {
  return {
    localPath: { enabled: cluster.storage?.localPath?.enabled ?? true },
    nfs: {
      enabled: true,
      server: nfsServer.trim(),
      exportBase: nfsExportBase.trim() || '/srv/fleet-nfs',
      clusterPath: cluster.storage?.nfs?.clusterPath || clusterName,
      storageClass: cluster.storage?.nfs?.storageClass || 'nfs-client',
      delegateHost: nfsDelegateHost.trim() || undefined,
      mountPath: nfsMountPath.trim() || undefined,
    },
  };
}

export function ClusterStoragePanel({ cluster, onUpdated }: Props) {
  const clusterName = cluster.name;
  const phase = cluster.provisioning_phase;
  const clusterReady = phase === 'Ready' || !!cluster.available;
  const storagePhase = cluster.storageStatus?.phase || '';
  const nfsEnabled = cluster.storage?.nfs?.enabled || cluster.storage?.backend === 'nfs_external' || cluster.storage?.backend === 'nfs_local';
  const localPathOn = cluster.storage?.localPath?.enabled ?? true;
  const longhornOn = !!cluster.storage?.longhorn?.enabled || cluster.storage?.backend === 'longhorn';
  const nfsApplied = nfsEnabled && storagePhase === 'Applied';
  const storageFailed = storagePhase === 'Failed';
  const canConfigure =
    clusterReady &&
    !cluster.read_only &&
    storagePhase !== 'Applying' &&
    storagePhase !== 'Pending';
  const needsValidateBeforeEnable = !nfsApplied && !storageFailed;

  const [nfsServer, setNfsServer] = useState('');
  const [nfsExportBase, setNfsExportBase] = useState('/srv/fleet-nfs');
  const [nfsDelegateHost, setNfsDelegateHost] = useState('');
  const [nfsMountPath, setNfsMountPath] = useState('');
  const [nfsFormDirty, setNfsFormDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [validating, setValidating] = useState(false);
  const [actionError, setActionError] = useState<unknown>(null);
  const [validateResult, setValidateResult] = useState<FleetClusterStorageValidateResult | null>(
    null,
  );

  // Seed from API when switching clusters or after NFS is applied — not on background polls.
  useEffect(() => {
    setNfsFormDirty(false);
    setValidateResult(null);
    setNfsServer(cluster.storage?.nfs?.server || '');
    setNfsExportBase(cluster.storage?.nfs?.exportBase || '/srv/fleet-nfs');
    setNfsDelegateHost(cluster.storage?.nfs?.delegateHost || '');
    setNfsMountPath(cluster.storage?.nfs?.mountPath || '');
  }, [clusterName]);

  useEffect(() => {
    if (nfsFormDirty || nfsApplied) return;
    const nfs = cluster.storage?.nfs;
    if (!nfs?.server) return;
    setNfsServer(nfs.server);
    setNfsExportBase(nfs.exportBase || '/srv/fleet-nfs');
    setNfsDelegateHost(nfs.delegateHost || '');
    setNfsMountPath(nfs.mountPath || '');
  }, [cluster.storage?.nfs, nfsApplied, nfsFormDirty]);

  useEffect(() => {
    if (storagePhase === 'Applying' || storagePhase === 'Pending') {
      const interval = setInterval(() => onUpdated(), 8000);
      return () => clearInterval(interval);
    }
    return undefined;
  }, [storagePhase, onUpdated]);

  const defaultMountPath = `${nfsExportBase.replace(/\/$/, '')}/${cluster.storage?.nfs?.clusterPath || clusterName}`;

  const applyNfs = async () => {
    if (!nfsServer.trim()) return;
    setSaving(true);
    setActionError(null);
    const body: FleetClusterConfigureStorage = {
      ...nfsStorageBody(cluster, clusterName, nfsServer, nfsExportBase, nfsDelegateHost, nfsMountPath),
      force: storageFailed || nfsApplied || undefined,
    };
    try {
      await configureFleetClusterStorage(clusterName, body);
      setNfsFormDirty(false);
      onUpdated();
    } catch (e) {
      setActionError(e);
    } finally {
      setSaving(false);
    }
  };

  const validateNfs = async () => {
    if (!nfsServer.trim()) return;
    setValidating(true);
    setActionError(null);
    setValidateResult(null);
    const body = nfsStorageBody(cluster, clusterName, nfsServer, nfsExportBase, nfsDelegateHost, nfsMountPath);
    try {
      const result = await validateFleetClusterStorage(clusterName, body);
      setValidateResult(result);
    } catch (e) {
      setActionError(e);
    } finally {
      setValidating(false);
    }
  };

  if (!clusterReady) {
    return null;
  }

  return (
    <div style={{ marginTop: '1.5rem', marginBottom: '1.5rem' }}>
      <Title headingLevel="h2" size="lg">
        Storage
      </Title>
      <p style={{ marginTop: '0.5rem', maxWidth: '42rem' }}>
        Default RWO is K3s local-path (or Longhorn on HA when enabled at create). Enable external NFS
        here for RWX via nfs-subdir-external-provisioner.
      </p>
      <div style={{ marginTop: '0.75rem' }}>
        <Label color={localPathOn ? 'green' : 'grey'}>
          Local path: {localPathOn ? 'enabled' : 'disabled'}
        </Label>
        <Label color={longhornOn ? 'green' : 'grey'} style={{ marginLeft: '0.5rem' }}>
          Longhorn: {longhornOn ? 'enabled' : 'disabled'}
        </Label>
        <Label color={nfsEnabled ? 'green' : 'grey'} style={{ marginLeft: '0.5rem' }}>
          NFS: {nfsEnabled ? 'enabled' : 'disabled'}
        </Label>
        {storagePhase && (
          <Label color={phaseColor(storagePhase)} style={{ marginLeft: '0.5rem' }}>
            {storagePhase}
          </Label>
        )}
      </div>
      {storageFailed && cluster.storageStatus?.message && (
        <Alert
          variant="danger"
          title="NFS storage configure failed"
          isInline
          style={{ marginTop: '0.75rem', maxWidth: '42rem' }}
        >
          {cluster.storageStatus.message}
        </Alert>
      )}
      {!storageFailed && cluster.storageStatus?.message && (
        <p style={{ marginTop: '0.5rem', color: 'var(--pf-v6-global--Color--200)' }}>
          {cluster.storageStatus.message}
        </p>
      )}
      {nfsEnabled && cluster.storage?.nfs && nfsApplied && (
        <dl style={{ marginTop: '0.75rem', maxWidth: '36rem' }}>
          <dt>NFS server</dt>
          <dd>{cluster.storage.nfs.server || '—'}</dd>
          <dt>Export base</dt>
          <dd>{cluster.storage.nfs.exportBase || '/srv/fleet-nfs'}</dd>
          <dt>Cluster path</dt>
          <dd>{cluster.storage.nfs.clusterPath || clusterName}</dd>
          {cluster.storage.nfs.mountPath && (
            <>
              <dt>Client mount path</dt>
              <dd>
                <code>{cluster.storage.nfs.mountPath}</code>
              </dd>
            </>
          )}
          {cluster.storage.nfs.delegateHost && (
            <>
              <dt>Delegate host</dt>
              <dd>{cluster.storage.nfs.delegateHost}</dd>
            </>
          )}
        </dl>
      )}
      {canConfigure && nfsApplied && (
        <Alert
          variant="info"
          title="NFS marked applied but you can re-run configure if the provisioner is missing on the cluster"
          isInline
          style={{ marginTop: '0.75rem', maxWidth: '42rem' }}
        />
      )}
      {canConfigure && (
        <Form style={{ maxWidth: '36rem', marginTop: '1rem' }}>
          <NfsStorageSetupHelp clusterName={clusterName} exportBase={nfsExportBase} />
          <ErrorAlert error={actionError} />
          {needsValidateBeforeEnable && !validateResult?.ok && (
            <Alert
              variant="warning"
              isInline
              title="Validate NFS before enabling"
              style={{ marginBottom: '1rem' }}
            >
              Run <strong>Validate NFS</strong> to confirm the server export, mount path, and
              provisioner subdirectory creation.
            </Alert>
          )}
          {validateResult && (
            <Alert
              variant={validateResult.ok ? 'success' : 'warning'}
              title={validateResult.ok ? 'NFS validation passed' : 'NFS validation failed'}
              isInline
              style={{ marginBottom: '1rem' }}
            >
              <ul style={{ margin: 0, paddingLeft: '1.25rem' }}>
                {validateResult.checks.map((check) => (
                  <li key={check.name} style={{ marginBottom: '0.35rem' }}>
                    {check.ok ? (
                      <CheckCircleIcon color="var(--pf-v6-global--success-color--100)" />
                    ) : (
                      <TimesCircleIcon color="var(--pf-v6-global--danger-color--100)" />
                    )}{' '}
                    <strong>{CHECK_LABELS[check.name] || check.name}:</strong> {check.message}
                  </li>
                ))}
              </ul>
              {(validateResult.mountPath || validateResult.nfsPath) && (
                <p style={{ marginTop: '0.5rem', marginBottom: 0 }}>
                  {validateResult.nfsPath && (
                    <>
                      Export directory: <code>{validateResult.nfsPath}</code>
                    </>
                  )}
                  {validateResult.mountPath && (
                    <>
                      {validateResult.nfsPath ? ' · ' : ''}
                      Client mount: <code>{validateResult.mountPath}</code>
                    </>
                  )}
                  {validateResult.node ? ` (from ${validateResult.node})` : ''}
                </p>
              )}
            </Alert>
          )}
          <LabeledField label="NFS server" fieldId="storageNfsServer" isRequired help={getFieldHelp('clusterStorage.nfsServer')}>
            <TextInput
              id="storageNfsServer"
              value={nfsServer}
              onChange={(_e, v) => {
                setNfsFormDirty(true);
                setValidateResult(null);
                setNfsServer(v);
              }}
              placeholder="198.51.100.40"
            />
          </LabeledField>
          <LabeledField label="NFS export base" fieldId="storageNfsExportBase" help={getFieldHelp('clusterStorage.nfsExportBase')}>
            <TextInput
              id="storageNfsExportBase"
              value={nfsExportBase}
              onChange={(_e, v) => {
                setNfsFormDirty(true);
                setValidateResult(null);
                setNfsExportBase(v);
              }}
              placeholder="/srv/fleet-nfs"
            />
          </LabeledField>
          <LabeledField
            label="Client mount path"
            fieldId="storageNfsMountPath"
            help={getFieldHelp('clusterStorage.nfsMountPath')}
            helperText="Path mounted on cluster nodes for nfs-subdir-external-provisioner."
          >
            <TextInput
              id="storageNfsMountPath"
              value={nfsMountPath}
              onChange={(_e, v) => {
                setNfsFormDirty(true);
                setValidateResult(null);
                setNfsMountPath(v);
              }}
              placeholder={defaultMountPath}
            />
          </LabeledField>
          <LabeledField label="Delegate host (SSH mkdir on NFS server)" fieldId="storageNfsDelegate" help={getFieldHelp('clusterStorage.nfsDelegateHost')}>
            <TextInput
              id="storageNfsDelegate"
              value={nfsDelegateHost}
              onChange={(_e, v) => {
                setNfsFormDirty(true);
                setValidateResult(null);
                setNfsDelegateHost(v);
              }}
              placeholder="NFS server IP when not a cluster node"
            />
          </LabeledField>
          <LabeledField label="Cluster subdirectory" fieldId="storageNfsClusterPath" help={getFieldHelp('clusterStorage.nfsClusterPath')}>
            <TextInput
              id="storageNfsClusterPath"
              value={cluster.storage?.nfs?.clusterPath || clusterName}
              readOnly
            />
          </LabeledField>
          <div style={{ display: 'flex', gap: '0.75rem', flexWrap: 'wrap' }}>
            <Button
              variant="secondary"
              isLoading={validating}
              isDisabled={!nfsServer.trim() || saving}
              onClick={() => void validateNfs()}
              data-testid="validate-cluster-storage"
            >
              Validate NFS
            </Button>
            <Button
              variant="primary"
              isLoading={saving}
              isDisabled={
                !nfsServer.trim() ||
                validating ||
                (needsValidateBeforeEnable && !validateResult?.ok)
              }
              onClick={() => void applyNfs()}
            data-testid="configure-cluster-storage"
          >
            {storageFailed ? 'Retry NFS storage' : nfsApplied ? 'Re-apply NFS storage' : 'Enable NFS storage'}
          </Button>
          </div>
        </Form>
      )}
    </div>
  );
}
