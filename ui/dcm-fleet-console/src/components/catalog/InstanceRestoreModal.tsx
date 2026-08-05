import { useEffect, useState } from 'react';
import {
  Button,
  Form,
  FormGroup,
  FormSelect,
  FormSelectOption,
  Modal,
  ModalVariant,
  Spinner,
  Switch,
} from '@patternfly/react-core';
import {
  createInstanceRestore,
  getInstanceRestore,
  listFleetClusters,
  listInstanceBackups,
} from '../../api/client';
import type {
  CatalogItemInstance,
  FleetCluster,
  InstanceBackupResponse,
} from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';

type Props = {
  instance: CatalogItemInstance | null;
  isOpen: boolean;
  onClose: () => void;
  onStarted?: () => void;
};

async function pollRestore(name: string, timeoutMs = 900000): Promise<string | undefined> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const st = await getInstanceRestore(name);
    if (st.phase === 'Complete' || st.phase === 'Failed') {
      if (st.phase === 'Failed') {
        throw new Error(st.message || 'restore failed');
      }
      return st.new_instance_id;
    }
    await new Promise((r) => setTimeout(r, 5000));
  }
  throw new Error('restore timed out');
}

export function InstanceRestoreModal({ instance, isOpen, onClose, onStarted }: Props) {
  const instanceId = instance?.id || instance?.uid || '';
  const [backups, setBackups] = useState<InstanceBackupResponse[]>([]);
  const [clusters, setClusters] = useState<FleetCluster[]>([]);
  const [backupName, setBackupName] = useState('');
  const [targetCluster, setTargetCluster] = useState('');
  const [rebind, setRebind] = useState(true);
  const [cloneMode, setCloneMode] = useState(false);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    if (!isOpen) return;
    setLoading(true);
    setError(null);
    Promise.all([
      listInstanceBackups(instanceId ? { instance_id: instanceId } : undefined),
      listFleetClusters(),
    ])
      .then(([b, c]) => {
        const complete = b.filter((x) => x.phase === 'Complete');
        setBackups(complete);
        setClusters(c);
        if (complete.length > 0) {
          setBackupName(complete[0].name);
        }
        if (c.length > 0 && !targetCluster) {
          setTargetCluster(c[0].name || '');
        }
      })
      .catch(setError)
      .finally(() => setLoading(false));
  }, [isOpen, instanceId]);

  const selectedBackup = backups.find((b) => b.name === backupName);
  const sourceCluster = selectedBackup?.cluster || '';

  const onRestore = async () => {
    if (!backupName || !targetCluster) return;
    setSubmitting(true);
    setError(null);
    try {
      const patches =
        sourceCluster && targetCluster !== sourceCluster
          ? [{ path: 'routing.cluster', value: targetCluster }]
          : undefined;
      const accepted = await createInstanceRestore({
        backup_name: backupName,
        target_cluster: targetCluster,
        source_cluster: sourceCluster || undefined,
        rebind: {
          enabled: rebind,
          mode: cloneMode ? 'clone' : 'supersede',
          ...(cloneMode ? { display_name_suffix: '-restored' } : {}),
          user_value_patches: patches,
        },
      });
      onStarted?.();
      await pollRestore(accepted.name);
      onClose();
    } catch (e) {
      setError(e);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      variant={ModalVariant.medium}
      title="Restore instance"
      isOpen={isOpen}
      onClose={onClose}
      actions={[
        <Button key="cancel" variant="link" onClick={onClose} isDisabled={submitting}>
          Cancel
        </Button>,
        <Button
          key="restore"
          variant="primary"
          onClick={onRestore}
          isDisabled={loading || submitting || !backupName || !targetCluster}
          isLoading={submitting}
        >
          Start restore
        </Button>,
      ]}
    >
      {error ? <ErrorAlert error={error} /> : null}
      {loading ? (
        <Spinner size="md" />
      ) : (
        <Form>
          <FormGroup label="Backup" fieldId="restore-backup">
            <FormSelect
              id="restore-backup"
              value={backupName}
              onChange={(_e, v) => setBackupName(v)}
              isDisabled={backups.length === 0}
            >
              {backups.length === 0 ? (
                <FormSelectOption value="" label="No completed backups" />
              ) : (
                backups.map((b) => (
                  <FormSelectOption
                    key={b.name}
                    value={b.name}
                    label={`${b.name} (${b.cluster}, ${b.completed_at || b.phase})`}
                  />
                ))
              )}
            </FormSelect>
          </FormGroup>
          <FormGroup label="Target cluster" fieldId="restore-target">
            <FormSelect
              id="restore-target"
              value={targetCluster}
              onChange={(_e, v) => setTargetCluster(v)}
            >
              {clusters.map((c) => (
                <FormSelectOption key={c.name} value={c.name} label={c.name} />
              ))}
            </FormSelect>
          </FormGroup>
          <FormGroup label="Rebind DCM instance" fieldId="restore-rebind">
            <Switch
              id="restore-rebind"
              isChecked={rebind}
              onChange={(_e, checked) => setRebind(checked)}
            />
          </FormGroup>
          {rebind ? (
            <FormGroup label="Clone (keep old instance)" fieldId="restore-clone">
              <Switch
                id="restore-clone"
                isChecked={cloneMode}
                onChange={(_e, checked) => setCloneMode(checked)}
              />
            </FormGroup>
          ) : null}
          {sourceCluster && targetCluster && sourceCluster !== targetCluster ? (
            <p>
              Cross-cluster restore: patches <code>routing.cluster</code> to{' '}
              <code>{targetCluster}</code>.
            </p>
          ) : null}
        </Form>
      )}
    </Modal>
  );
}
