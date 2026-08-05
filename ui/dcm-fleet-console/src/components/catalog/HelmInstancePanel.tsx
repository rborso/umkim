import { useCallback, useEffect, useState } from 'react';
import {
  Button,
  Form,
  NumberInput,
  Spinner,
  TextArea,
} from '@patternfly/react-core';
import {
  getHelmInstanceHistory,
  rollbackHelmInstance,
  upgradeCatalogItemInstance,
  type HelmHistoryEntry,
} from '../../api/client';
import type { CatalogItemInstance, UserValue } from '../../api/types';
import { getFieldHelp } from '../../help/fieldHelp';
import { ErrorAlert } from '../ErrorAlert';
import { LabeledField } from '../forms/LabeledField';

type Props = {
  instance: CatalogItemInstance;
  onUpdated?: () => void;
};

export function HelmInstancePanel({ instance, onUpdated }: Props) {
  const id = instance.id || '';
  const [history, setHistory] = useState<HelmHistoryEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [rollbackRev, setRollbackRev] = useState(1);
  const [valuesYaml, setValuesYaml] = useState('');

  const loadHistory = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    setError(null);
    try {
      const h = await getHelmInstanceHistory(id);
      setHistory(h.releases || []);
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    void loadHistory();
  }, [loadHistory]);

  const onRollback = async () => {
    setError(null);
    try {
      await rollbackHelmInstance(id, rollbackRev);
      await loadHistory();
      onUpdated?.();
    } catch (e) {
      setError(e);
    }
  };

  const onUpgrade = async () => {
    const uv: UserValue[] = instance.spec?.user_values ? [...instance.spec.user_values] : [];
    const idx = uv.findIndex((u) => u.path === 'values');
    if (idx >= 0) uv[idx] = { path: 'values', value: valuesYaml };
    else uv.push({ path: 'values', value: valuesYaml });
    setError(null);
    try {
      await upgradeCatalogItemInstance(id, uv);
      onUpdated?.();
    } catch (e) {
      setError(e);
    }
  };

  return (
    <div style={{ marginTop: '1rem' }}>
      <h3 style={{ fontSize: '1rem' }}>Helm release</h3>
      <ErrorAlert error={error} />
      <Button variant="secondary" onClick={() => void loadHistory()} isDisabled={loading}>
        Refresh history
      </Button>
      {loading ? <Spinner size="md" /> : null}
      {history.length > 0 ? (
        <table style={{ width: '100%', fontSize: '0.875rem', marginTop: '0.5rem' }}>
          <thead>
            <tr>
              <th>Rev</th>
              <th>Status</th>
              <th>Chart</th>
              <th>Description</th>
            </tr>
          </thead>
          <tbody>
            {history.map((row, i) => (
              <tr key={row.revision ?? i}>
                <td>{row.revision ?? '—'}</td>
                <td>{row.status ?? '—'}</td>
                <td>{row.chart ?? '—'}</td>
                <td>{row.description ?? '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
      <Form style={{ marginTop: '1rem' }}>
        <LabeledField
          label="Rollback revision"
          fieldId="instance-rollback-revision"
          help={getFieldHelp('instance.rollbackRevision')}
        >
          <NumberInput
            value={rollbackRev}
            onMinus={() => setRollbackRev((r) => Math.max(1, r - 1))}
            onChange={(e) => setRollbackRev(parseInt(e.currentTarget.value, 10) || 1)}
            onPlus={() => setRollbackRev((r) => r + 1)}
            inputName="revision"
            inputAriaLabel="revision"
            minusBtnAriaLabel="minus"
            plusBtnAriaLabel="plus"
          />
        </LabeledField>
        <Button variant="warning" onClick={() => void onRollback()}>
          Rollback
        </Button>
        <LabeledField
          label="Upgrade values (YAML)"
          fieldId="instance-upgrade-values"
          help={getFieldHelp('instance.upgradeValues')}
          helperText="Merged into the current Helm release on upgrade."
        >
          <TextArea
            value={valuesYaml}
            onChange={(_e, v) => setValuesYaml(v)}
            rows={6}
            resizeOrientation="vertical"
            aria-label="values yaml"
          />
        </LabeledField>
        <Button variant="primary" onClick={() => void onUpgrade()}>
          Upgrade release
        </Button>
      </Form>
    </div>
  );
}
