import { useEffect, useState } from 'react';
import {
  Button,
  Form,
  FormGroup,
  Modal,
  ModalVariant,
  Spinner,
} from '@patternfly/react-core';
import {
  createInstanceBackup,
  getInstanceBackup,
  getInstanceWorkload,
} from '../../api/client';
import type { CatalogItemInstance, InstanceWorkloadResponse } from '../../api/types';
import { ErrorAlert } from '../ErrorAlert';

type Props = {
  instance: CatalogItemInstance | null;
  isOpen: boolean;
  onClose: () => void;
  onStarted?: () => void;
};

async function pollBackup(name: string, timeoutMs = 600000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const st = await getInstanceBackup(name);
    if (st.phase === 'Complete' || st.phase === 'Failed') {
      if (st.phase === 'Failed') {
        throw new Error(st.message || 'backup failed');
      }
      return;
    }
    await new Promise((r) => setTimeout(r, 5000));
  }
  throw new Error('backup timed out');
}

export function InstanceBackupModal({ instance, isOpen, onClose, onStarted }: Props) {
  const [workload, setWorkload] = useState<InstanceWorkloadResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const instanceId = instance?.id || instance?.uid || '';

  useEffect(() => {
    if (!isOpen || !instanceId) return;
    setLoading(true);
    setError(null);
    getInstanceWorkload(instanceId)
      .then(setWorkload)
      .catch(setError)
      .finally(() => setLoading(false));
  }, [isOpen, instanceId]);

  const onBackup = async () => {
    if (!instanceId) return;
    setSubmitting(true);
    setError(null);
    try {
      const accepted = await createInstanceBackup({
        instance_id: instanceId,
        cluster: workload?.cluster,
        release: workload?.release,
        namespace: workload?.namespace,
        wait: true,
      });
      onStarted?.();
      await pollBackup(accepted.name);
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
      title="Backup instance"
      isOpen={isOpen}
      onClose={onClose}
      actions={[
        <Button key="cancel" variant="link" onClick={onClose} isDisabled={submitting}>
          Cancel
        </Button>,
        <Button
          key="backup"
          variant="primary"
          onClick={onBackup}
          isDisabled={loading || submitting || !workload?.deployed}
          isLoading={submitting}
        >
          Start backup
        </Button>,
      ]}
    >
      {error ? <ErrorAlert error={error} /> : null}
      {loading ? (
        <Spinner size="md" />
      ) : workload ? (
        <Form>
          <FormGroup label="Instance">{instance?.display_name || instanceId}</FormGroup>
          <FormGroup label="Cluster">{workload.cluster}</FormGroup>
          <FormGroup label="Release">{workload.release}</FormGroup>
          <FormGroup label="Namespace">{workload.namespace || 'default'}</FormGroup>
          {!workload.deployed ? (
            <p>Workload is not deployed; backup is unavailable.</p>
          ) : (
            <p>Creates a Velero backup on the spoke and exports the DCM instance manifest to the hub.</p>
          )}
        </Form>
      ) : null}
    </Modal>
  );
}
