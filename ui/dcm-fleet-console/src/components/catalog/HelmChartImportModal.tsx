import { useEffect, useState } from 'react';
import {
  Button,
  Form,
  Modal,
  ModalVariant,
  TextArea,
  TextInput,
} from '@patternfly/react-core';
import type { CatalogItemCreate } from '../../api/types';
import {
  buildHelmImportCatalogItem,
  DEFAULT_HELM_OCI_PREFIX_HINTS,
  isDnsLabel,
  suggestCatalogId,
  validateOciRepository,
} from '../../catalog/helmImportFields';
import { getFieldHelp } from '../../help/fieldHelp';
import { ErrorAlert } from '../ErrorAlert';
import { LabeledField } from '../forms/LabeledField';

type Props = {
  isOpen: boolean;
  onClose: () => void;
  onImport: (body: CatalogItemCreate & { id: string }) => Promise<void>;
};

export function HelmChartImportModal({ isOpen, onClose, onImport }: Props) {
  const [catalogId, setCatalogId] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [description, setDescription] = useState('');
  const [chartRepository, setChartRepository] = useState('oci://registry-1.docker.io/bitnamicharts');
  const [chartName, setChartName] = useState('');
  const [chartVersion, setChartVersion] = useState('');
  const [valuesYaml, setValuesYaml] = useState('');
  const [idTouched, setIdTouched] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    if (!isOpen) return;
    setCatalogId('');
    setDisplayName('');
    setDescription('');
    setChartRepository('oci://registry-1.docker.io/bitnamicharts');
    setChartName('');
    setChartVersion('');
    setValuesYaml('');
    setIdTouched(false);
    setError(null);
    setSaving(false);
  }, [isOpen]);

  useEffect(() => {
    if (!idTouched && chartName.trim()) {
      setCatalogId(suggestCatalogId(chartName));
    }
  }, [chartName, idTouched]);

  const submit = async () => {
    setError(null);
    const ociErr = validateOciRepository(chartRepository);
    if (ociErr) {
      setError(new Error(ociErr));
      return;
    }
    if (!chartName.trim() || !chartVersion.trim()) {
      setError(new Error('Chart name and version are required'));
      return;
    }
    if (!displayName.trim()) {
      setError(new Error('Display name is required'));
      return;
    }
    if (!isDnsLabel(catalogId.trim())) {
      setError(new Error('Catalog ID must be a DNS label (lowercase alphanumeric and hyphens)'));
      return;
    }
    setSaving(true);
    try {
      const body = buildHelmImportCatalogItem({
        catalogId,
        displayName,
        description,
        chartRepository,
        chartName,
        chartVersion,
        valuesYaml,
      });
      await onImport(body);
      onClose();
    } catch (e) {
      setError(e);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      variant={ModalVariant.medium}
      title="Import Helm chart"
      isOpen={isOpen}
      onClose={onClose}
      actions={[
        <Button
          key="import"
          variant="primary"
          onClick={submit}
          isLoading={saving}
          data-testid="helm-import-submit"
        >
          Import
        </Button>,
        <Button key="cancel" variant="link" onClick={onClose}>
          Cancel
        </Button>,
      ]}
    >
      <p style={{ marginBottom: '1rem' }}>
        Register an OCI Helm chart as a DCM catalog item. Deploy still uses placement and the helm
        service provider. Classic HTTPS chart repos are not supported — use <code>oci://</code> only.
        Private charts: push to Harbor first, then import with the Harbor OCI prefix.
      </p>
      <p style={{ marginBottom: '1rem', fontSize: '0.875rem', color: 'var(--pf-v5-global--Color--200)' }}>
        Allowed prefixes (lab default): {DEFAULT_HELM_OCI_PREFIX_HINTS.join(', ')}
      </p>
      <ErrorAlert error={error} />
      <Form>
        <LabeledField
          label="Chart name"
          isRequired
          fieldId="hci-chart-name"
          help={getFieldHelp('helmImport.chartName')}
        >
          <TextInput
            id="hci-chart-name"
            value={chartName}
            onChange={(_e, v) => setChartName(v)}
            data-testid="helm-import-chart-name"
          />
        </LabeledField>
        <LabeledField
          label="OCI repository"
          isRequired
          fieldId="hci-repo"
          help={getFieldHelp('helmImport.repository')}
        >
          <TextInput
            id="hci-repo"
            value={chartRepository}
            onChange={(_e, v) => setChartRepository(v)}
            data-testid="helm-import-repository"
          />
        </LabeledField>
        <LabeledField
          label="Chart version"
          isRequired
          fieldId="hci-ver"
          help={getFieldHelp('helmImport.version')}
        >
          <TextInput
            id="hci-ver"
            value={chartVersion}
            onChange={(_e, v) => setChartVersion(v)}
            data-testid="helm-import-version"
          />
        </LabeledField>
        <LabeledField
          label="Catalog ID"
          isRequired
          fieldId="hci-id"
          help={getFieldHelp('helmImport.catalogId')}
        >
          <TextInput
            id="hci-id"
            value={catalogId}
            onChange={(_e, v) => {
              setIdTouched(true);
              setCatalogId(v);
            }}
            data-testid="helm-import-catalog-id"
          />
        </LabeledField>
        <LabeledField
          label="Display name"
          isRequired
          fieldId="hci-dn"
          help={getFieldHelp('catalog.displayName')}
        >
          <TextInput
            id="hci-dn"
            value={displayName}
            onChange={(_e, v) => setDisplayName(v)}
            data-testid="helm-import-display-name"
          />
        </LabeledField>
        <LabeledField label="Description" fieldId="hci-desc" help={getFieldHelp('catalog.description')}>
          <TextArea
            id="hci-desc"
            value={description}
            onChange={(_e, v) => setDescription(v)}
            rows={2}
          />
        </LabeledField>
        <LabeledField label="Default values (YAML)" fieldId="hci-values" help={getFieldHelp('helmImport.values')}>
          <TextArea
            id="hci-values"
            value={valuesYaml}
            onChange={(_e, v) => setValuesYaml(v)}
            rows={8}
            data-testid="helm-import-values"
          />
        </LabeledField>
      </Form>
    </Modal>
  );
}
