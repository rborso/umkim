import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Form,
  Modal,
  ModalVariant,
  PageSection,
  PageSectionVariants,
  TextInput,
  TextArea,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  Switch,
  Label,
  Spinner,
  EmptyState,
  EmptyStateBody,
  FormSelect,
  FormSelectOption,
  Alert,
  AlertVariant,
} from '@patternfly/react-core';
import {
  Table,
  Thead,
  Tr,
  Th,
  Tbody,
  Td,
} from '@patternfly/react-table';
import { PlusIcon, SyncIcon } from '@patternfly/react-icons';
import {
  createPolicy,
  deletePolicy,
  getPolicy,
  listPolicies,
  policyDisplayName,
  updatePolicy,
} from '../api/client';
import type { FailoverMode, PlacementStrategy, Policy, PolicyCreate } from '../api/types';
import { ErrorAlert } from '../components/ErrorAlert';
import { PolicyHelpPanel } from '../components/PolicyHelpPanel';
import { LabeledField } from '../components/forms/LabeledField';
import { getFieldHelp } from '../help/fieldHelp';
import { DEFAULT_REGO } from '../policies/templates';
import {
  FAILOVER_MODE_OPTIONS,
  PLACEMENT_STRATEGY_OPTIONS,
  PREVIEW_SERVICE_TYPE_OPTIONS,
  matchLabelsToText,
  textToMatchLabels,
} from '../policies/placementForm';
import { validatePolicyForm } from '../policies/policyFormValidation';
import { PlacementImpactPanel } from '../components/catalog/PlacementImpactPanel';
import { usePlacementPreview } from '../hooks/usePlacementPreview';
import type { PlacementPreviewRequest } from '../api/types';
import { useAuth } from '../auth/AuthContext';
import { isOrgAdmin, orgPrefixedName, primaryOrg } from '../auth/orgUtils';

type PolicyFormState = PolicyCreate & {
  _matchLabelsText: string;
  _labelSelectorText: string;
  _previewServiceType: string;
  _policyIdSuffix: string;
};

const emptyForm = (): PolicyFormState => ({
  display_name: '',
  policy_type: 'GLOBAL',
  rego_code: DEFAULT_REGO,
  description: '',
  enabled: true,
  priority: 100,
  _matchLabelsText: '',
  _labelSelectorText: '',
  placement: { cluster_count: 1, strategy: 'single' },
  failover: { mode: 'manual', hub_health_path: '/', failure_threshold: 3, success_threshold: 2, cooldown_seconds: 300 },
  _previewServiceType: 'container',
  _policyIdSuffix: '',
});

function orgDedicatedDefaults(orgSlug: string): Pick<PolicyFormState, '_matchLabelsText' | '_labelSelectorText'> {
  return {
    _matchLabelsText: `fleet.org=${orgSlug}\nfleet.pool=dedicated`,
    _labelSelectorText: `fleet.org=${orgSlug}\nfleet.placement/pool=dedicated`,
  };
}

function policyToForm(p: Policy): PolicyFormState {
  return {
    display_name: p.display_name || '',
    policy_type: p.policy_type || 'GLOBAL',
    rego_code: p.rego_code || DEFAULT_REGO,
    description: p.description || '',
    enabled: p.enabled ?? true,
    priority: p.priority ?? 100,
    label_selector: p.label_selector,
    cluster_requirements: p.cluster_requirements,
    placement: p.placement ?? { cluster_count: 1, strategy: 'single' },
    failover: p.failover ?? { mode: 'manual', hub_health_path: '/', failure_threshold: 3, success_threshold: 2, cooldown_seconds: 300 },
    _matchLabelsText: matchLabelsToText(p.cluster_requirements?.matchLabels),
    _labelSelectorText: matchLabelsToText(p.label_selector),
    _previewServiceType: 'container',
    _policyIdSuffix: '',
  };
}

function formToPolicyCreate(form: PolicyFormState, isPatch = false): PolicyCreate {
  const { _matchLabelsText, _labelSelectorText, _previewServiceType: _pst, _policyIdSuffix: _pid, ...rest } = form;
  void _pst;
  void _pid;
  const matchLabels = textToMatchLabels(_matchLabelsText);
  const labelSelector = textToMatchLabels(_labelSelectorText);
  const body: PolicyCreate = {
    display_name: rest.display_name,
    policy_type: rest.policy_type,
    rego_code: rest.rego_code,
    description: rest.description,
    enabled: rest.enabled,
    priority: rest.priority,
    placement: rest.placement,
    failover: rest.failover,
  };
  if (isPatch) {
    body.label_selector = labelSelector || {};
    body.cluster_requirements = matchLabels
      ? { matchLabels, matchExpressions: [] }
      : { matchLabels: {}, matchExpressions: [] };
  } else {
    if (labelSelector) {
      body.label_selector = labelSelector;
    }
    if (matchLabels) {
      body.cluster_requirements = { matchLabels, matchExpressions: [] };
    }
  }
  return body;
}

export function PoliciesPage() {
  const { user } = useAuth();
  const orgSlug = primaryOrg(user);
  const orgAdminMode = !user?.dcAdmin && isOrgAdmin(user);
  const [rows, setRows] = useState<Policy[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [viewOpen, setViewOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [viewPolicy, setViewPolicy] = useState<Policy | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Policy | null>(null);
  const [form, setForm] = useState<PolicyFormState>(emptyForm());
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<unknown>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<unknown>(null);
  const [formValidationOk, setFormValidationOk] = useState<boolean | null>(null);
  const [formValidationErrors, setFormValidationErrors] = useState<string[]>([]);

  const policyPreviewRequest = useMemo((): PlacementPreviewRequest | null => {
    if (!formOpen) return null;
    const body = formToPolicyCreate(form);
    const probeLabels = textToMatchLabels(form._labelSelectorText);
    return {
      mode: 'policy',
      service_type: form._previewServiceType,
      probe_label_selector: probeLabels,
      policy: { ...body, ...(editId ? { id: editId } : {}) },
    };
  }, [formOpen, form, editId]);

  const {
    preview: policyPreview,
    loading: policyPreviewLoading,
    error: policyPreviewError,
    refetch: refetchPolicyPreview,
  } = usePlacementPreview(policyPreviewRequest);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setRows(await listPolicies());
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const openCreate = () => {
    setEditId(null);
    const base = emptyForm();
    if (orgAdminMode && orgSlug) {
      Object.assign(base, orgDedicatedDefaults(orgSlug));
    }
    setForm(base);
    setSaveError(null);
    setFormValidationOk(null);
    setFormValidationErrors([]);
    setFormOpen(true);
  };

  const openEdit = async (p: Policy) => {
    setEditId(p.id || null);
    setSaveError(null);
    try {
      const full = p.id ? await getPolicy(p.id) : p;
      setForm(policyToForm(full));
      setFormOpen(true);
    } catch (e) {
      setError(e);
    }
  };

  const openView = async (p: Policy) => {
    setSaveError(null);
    try {
      const full = p.id ? await getPolicy(p.id) : p;
      setViewPolicy(full);
      setViewOpen(true);
    } catch (e) {
      setError(e);
    }
  };

  const openDelete = (p: Policy) => {
    setDeleteTarget(p);
    setDeleteError(null);
    setDeleteOpen(true);
  };

  const save = async () => {
    const body = formToPolicyCreate(form, Boolean(editId));
    if (orgAdminMode && orgSlug) {
      // Re-assert dedicated org constraints even if the form was edited.
      body.label_selector = {
        ...(body.label_selector || {}),
        'fleet.org': orgSlug,
        'fleet.placement/pool': 'dedicated',
      };
      body.cluster_requirements = {
        matchLabels: {
          ...(body.cluster_requirements?.matchLabels || {}),
          'fleet.org': orgSlug,
          'fleet.pool': 'dedicated',
        },
        matchExpressions: body.cluster_requirements?.matchExpressions || [],
      };
    }
    const validation = validatePolicyForm(body, form._matchLabelsText);
    if (!validation.ok) {
      setFormValidationOk(false);
      setFormValidationErrors(validation.errors);
      setSaveError(new Error(validation.errors[0]));
      return;
    }
    setSaving(true);
    setSaveError(null);
    try {
      if (editId) {
        await updatePolicy(editId, body);
      } else {
        const createId =
          orgAdminMode && orgSlug
            ? orgPrefixedName(orgSlug, form._policyIdSuffix || 'policy')
            : undefined;
        await createPolicy(body, createId ? { id: createId } : undefined);
      }
      setFormOpen(false);
      await load();
    } catch (e) {
      setSaveError(e);
    } finally {
      setSaving(false);
    }
  };

  const runValidateForm = () => {
    const body = formToPolicyCreate(form);
    const validation = validatePolicyForm(body, form._matchLabelsText);
    setFormValidationOk(validation.ok);
    setFormValidationErrors(validation.errors);
    setSaveError(null);
  };

  const runPreviewPlacement = () => {
    void refetchPolicyPreview();
  };

  const confirmDelete = async () => {
    if (!deleteTarget?.id) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await deletePolicy(deleteTarget.id);
      setDeleteOpen(false);
      setDeleteTarget(null);
      await load();
    } catch (e) {
      setDeleteError(e);
    } finally {
      setDeleting(false);
    }
  };

  return (
    <>
      <PageSection variant={PageSectionVariants.light}>
        <Title headingLevel="h1" size="2xl">
          Policies
        </Title>
        {orgAdminMode ? (
          <p style={{ marginTop: '0.5rem' }}>
            Org-admin policies must target your dedicated clusters (
            <code>fleet.org</code> + <code>fleet.pool=dedicated</code>). Policy IDs are prefixed with{' '}
            <code>{orgSlug}-</code>.
          </p>
        ) : null}
        <PolicyHelpPanel
          onInsertTemplate={(rego) => {
            setEditId(null);
            const base = { ...emptyForm(), rego_code: rego };
            if (orgAdminMode && orgSlug) {
              Object.assign(base, orgDedicatedDefaults(orgSlug));
            }
            setForm(base);
            setFormOpen(true);
          }}
        />
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem>
              <Button
                variant="primary"
                icon={<PlusIcon />}
                onClick={openCreate}
                data-testid="policies-create-btn"
              >
                Create policy
              </Button>
            </ToolbarItem>
            <ToolbarItem>
              <Button variant="secondary" icon={<SyncIcon />} onClick={load}>
                Refresh
              </Button>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      </PageSection>
      <PageSection>
        <ErrorAlert error={error} />
        {loading ? (
          <Spinner aria-label="Loading policies" />
        ) : rows.length === 0 ? (
          <EmptyState>
            <EmptyStateBody>No policies defined. Create one to control placement routing.</EmptyStateBody>
            <Button variant="primary" onClick={openCreate}>
              Create policy
            </Button>
          </EmptyState>
        ) : (
          <Table aria-label="Policies" variant="compact">
            <Thead>
              <Tr>
                <Th>Display name</Th>
                <Th>Type</Th>
                <Th>Priority</Th>
                <Th>Enabled</Th>
                <Th>Description</Th>
                <Th>ID</Th>
                <Th>Actions</Th>
              </Tr>
            </Thead>
            <Tbody>
              {rows.map((p) => (
                <Tr key={p.id} data-testid={p.id ? `policy-row-${p.id}` : undefined}>
                  <Td data-testid="policy-display-name-cell">{policyDisplayName(p)}</Td>
                  <Td>{p.policy_type || '—'}</Td>
                  <Td>{p.priority ?? '—'}</Td>
                  <Td>
                    <Label color={p.enabled !== false ? 'green' : 'grey'}>
                      {p.enabled !== false ? 'yes' : 'no'}
                    </Label>
                  </Td>
                  <Td>{p.description || '—'}</Td>
                  <Td>
                    <code style={{ fontSize: '0.75rem' }}>{p.id || '—'}</code>
                  </Td>
                  <Td>
                    <Button
                      variant="link"
                      isInline
                      onClick={() => openView(p)}
                      data-testid={p.id ? `policy-view-${p.id}` : undefined}
                    >
                      View
                    </Button>
                    <Button
                      variant="link"
                      isInline
                      onClick={() => openEdit(p)}
                      data-testid={p.id ? `policy-edit-${p.id}` : undefined}
                    >
                      Edit
                    </Button>
                    <Button
                      variant="link"
                      isInline
                      isDanger
                      onClick={() => openDelete(p)}
                      data-testid={p.id ? `policy-delete-${p.id}` : undefined}
                    >
                      Delete
                    </Button>
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        )}
      </PageSection>

      <Modal
        variant={ModalVariant.large}
        title={editId ? 'Edit policy' : 'Create policy'}
        isOpen={formOpen}
        onClose={() => setFormOpen(false)}
        actions={[
          <Button
            key="validate-form"
            variant="secondary"
            onClick={runValidateForm}
            data-testid="policy-validate-form"
          >
            Validate form
          </Button>,
          <Button
            key="validate-placement"
            variant="secondary"
            onClick={runPreviewPlacement}
            isLoading={policyPreviewLoading}
            data-testid="policy-validate-placement"
          >
            Preview placement
          </Button>,
          <Button
            key="save"
            variant="primary"
            onClick={save}
            isLoading={saving}
            data-testid="policy-save-btn"
          >
            Save
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setFormOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <ErrorAlert error={saveError} />
        {formValidationOk === true ? (
          <Alert
            variant={AlertVariant.success}
            title="Form validation passed"
            isInline
            style={{ marginBottom: '0.75rem' }}
            data-testid="policy-validate-form-success"
          />
        ) : null}
        {formValidationOk === false && formValidationErrors.length > 0 ? (
          <Alert
            variant={AlertVariant.danger}
            title="Form validation failed"
            isInline
            style={{ marginBottom: '0.75rem' }}
            data-testid="policy-validate-form-errors"
          >
            <ul style={{ margin: 0, paddingLeft: '1.25rem' }}>
              {formValidationErrors.map((err) => (
                <li key={err}>{err}</li>
              ))}
            </ul>
          </Alert>
        ) : null}
        <PolicyHelpPanel onInsertTemplate={(rego) => setForm((f) => ({ ...f, rego_code: rego }))} />
        <Form>
          {orgAdminMode && !editId ? (
            <LabeledField label="Policy ID suffix" isRequired fieldId="pol-id" help={getFieldHelp('policy.id')}>
              <TextInput
                id="pol-id"
                value={form._policyIdSuffix}
                placeholder="dedicated-container"
                onChange={(_e, v) => setForm({ ...form, _policyIdSuffix: v })}
                data-testid="policy-id-suffix"
              />
              <small>
                Full id: <code>{orgPrefixedName(orgSlug, form._policyIdSuffix || '…')}</code>
              </small>
            </LabeledField>
          ) : null}
          <LabeledField label="Display name" isRequired fieldId="pol-display" help={getFieldHelp('policy.displayName')}>
            <TextInput
              id="pol-display"
              value={form.display_name}
              onChange={(_e, v) => setForm({ ...form, display_name: v })}
              data-testid="policy-display-name"
            />
          </LabeledField>
          <LabeledField label="Policy type" fieldId="pol-type" help={getFieldHelp('policy.policyType')}>
            <TextInput id="pol-type" value={form.policy_type} isDisabled />
          </LabeledField>
          <LabeledField label="Description" fieldId="pol-desc" help={getFieldHelp('policy.description')}>
            <TextInput
              id="pol-desc"
              value={form.description || ''}
              onChange={(_e, v) => setForm({ ...form, description: v })}
              data-testid="policy-description"
            />
          </LabeledField>
          <LabeledField label="Priority" fieldId="pol-priority" help={getFieldHelp('policy.priority')}>
            <TextInput
              id="pol-priority"
              type="number"
              value={String(form.priority ?? 100)}
              onChange={(_e, v) =>
                setForm({ ...form, priority: v === '' ? undefined : Number(v) })
              }
              data-testid="policy-priority"
            />
          </LabeledField>
          <LabeledField label="Enabled" fieldId="pol-enabled" help={getFieldHelp('policy.enabled')}>
            <Switch
              id="pol-enabled"
              isChecked={form.enabled !== false}
              onChange={(_e, checked) => setForm({ ...form, enabled: checked })}
              data-testid="policy-enabled"
            />
          </LabeledField>
          <LabeledField
            label="Instance label selector"
            fieldId="pol-label-selector"
            help={getFieldHelp('policy.labelSelector')}
            helperText="Labels on catalog instances that must match for this policy to apply (one per line, key=value)"
          >
            <TextArea
              id="pol-label-selector"
              value={form._labelSelectorText}
              onChange={(_e, v) => setForm({ ...form, _labelSelectorText: v })}
              rows={3}
              resizeOrientation="vertical"
              placeholder="fleet.site=homelab"
              data-testid="policy-label-selector"
              readOnly={orgAdminMode}
            />
          </LabeledField>
          <LabeledField
            label="Cluster match labels"
            fieldId="pol-match-labels"
            help={getFieldHelp('policy.matchLabels')}
            helperText="One per line: fleet.capability/storage.rwx=true or fleet.site=homelab (scope only)"
          >
            <TextArea
              id="pol-match-labels"
              value={form._matchLabelsText}
              onChange={(_e, v) => setForm({ ...form, _matchLabelsText: v })}
              rows={4}
              resizeOrientation="vertical"
              placeholder={'fleet.capability/storage.rwx=true'}
              data-testid="policy-match-labels"
              readOnly={orgAdminMode}
            />
          </LabeledField>
          <LabeledField label="Placement cluster count" fieldId="pol-cluster-count" help={getFieldHelp('policy.clusterCount')}>
            <TextInput
              id="pol-cluster-count"
              type="number"
              value={String(form.placement?.cluster_count ?? 1)}
              onChange={(_e, v) =>
                setForm({
                  ...form,
                  placement: {
                    ...form.placement,
                    cluster_count: v === '' ? 1 : Number(v),
                    strategy: form.placement?.strategy ?? 'single',
                  },
                })
              }
              data-testid="policy-cluster-count"
            />
          </LabeledField>
          <LabeledField label="Placement strategy" fieldId="pol-strategy" help={getFieldHelp('policy.strategy')}>
            <FormSelect
              id="pol-strategy"
              value={form.placement?.strategy ?? 'single'}
              onChange={(_e, v) =>
                setForm({
                  ...form,
                  placement: {
                    cluster_count: form.placement?.cluster_count ?? 1,
                    strategy: (v || 'single') as PlacementStrategy,
                  },
                })
              }
              data-testid="policy-placement-strategy"
            >
              {PLACEMENT_STRATEGY_OPTIONS.map((opt) => (
                <FormSelectOption key={opt.value} value={opt.value} label={opt.label} />
              ))}
            </FormSelect>
          </LabeledField>
          <LabeledField
            label="Site diversity (geo spread)"
            fieldId="pol-site-diversity"
            help={getFieldHelp('policy.siteDiversity')}
            helperText="When spread with cluster count ≥ 2, require distinct fleet.site values per target"
          >
            <Switch
              id="pol-site-diversity"
              isChecked={!!form.placement?.site_diversity}
              isDisabled={(form.placement?.cluster_count ?? 1) < 2}
              onChange={(_e, checked) =>
                setForm({
                  ...form,
                  placement: {
                    cluster_count: form.placement?.cluster_count ?? 1,
                    strategy: form.placement?.strategy ?? 'spread',
                    site_diversity: checked,
                  },
                })
              }
              data-testid="policy-site-diversity"
            />
          </LabeledField>
          <Title headingLevel="h3" size="md" style={{ marginTop: '1rem' }}>
            Failover
          </Title>
          <LabeledField
            label="Failover mode"
            fieldId="pol-failover-mode"
            help={getFieldHelp('policy.failoverMode')}
            helperText="manual = observe only; relocatable = cold relocate (cluster_count 1); standby = hot switch (cluster_count 2)."
          >
            <FormSelect
              id="pol-failover-mode"
              value={form.failover?.mode ?? 'manual'}
              onChange={(_e, v) =>
                setForm({
                  ...form,
                  failover: { ...form.failover, mode: v as FailoverMode },
                })
              }
              data-testid="policy-failover-mode"
            >
              {FAILOVER_MODE_OPTIONS.map((opt) => (
                <FormSelectOption key={opt.value} value={opt.value} label={opt.label} />
              ))}
            </FormSelect>
          </LabeledField>
          <LabeledField label="Hub health path" fieldId="pol-hub-health-path" help={getFieldHelp('policy.hubHealthPath')}>
            <TextInput
              id="pol-hub-health-path"
              value={form.failover?.hub_health_path ?? '/'}
              onChange={(_e, v) =>
                setForm({
                  ...form,
                  failover: { ...form.failover, hub_health_path: v },
                })
              }
              data-testid="policy-hub-health-path"
            />
          </LabeledField>
          <LabeledField label="Failure threshold" fieldId="pol-failure-threshold" help={getFieldHelp('policy.failureThreshold')}>
            <TextInput
              id="pol-failure-threshold"
              type="number"
              value={String(form.failover?.failure_threshold ?? 3)}
              onChange={(_e, v) =>
                setForm({
                  ...form,
                  failover: {
                    ...form.failover,
                    failure_threshold: v === '' ? 3 : Number(v),
                  },
                })
              }
              data-testid="policy-failure-threshold"
            />
          </LabeledField>
          <LabeledField
            label="Preview service type"
            fieldId="pol-preview-stype"
            help={getFieldHelp('policy.previewServiceType')}
            helperText="Used to match registered providers when previewing this policy."
          >
            <FormSelect
              id="pol-preview-stype"
              value={form._previewServiceType}
              onChange={(_e, v) => setForm({ ...form, _previewServiceType: v })}
              data-testid="policy-preview-service-type"
            >
              {PREVIEW_SERVICE_TYPE_OPTIONS.map((opt) => (
                <FormSelectOption key={opt.value} value={opt.value} label={opt.label} />
              ))}
            </FormSelect>
          </LabeledField>
          <Title headingLevel="h3" size="md" style={{ marginTop: '1rem' }}>
            Placement impact
          </Title>
          <PlacementImpactPanel
            preview={policyPreview}
            loading={policyPreviewLoading}
            error={policyPreviewError}
          />
          <LabeledField label="Rego code" isRequired fieldId="pol-rego" help={getFieldHelp('policy.regoCode')}>
            <TextArea
              id="pol-rego"
              value={form.rego_code}
              onChange={(_e, v) => setForm({ ...form, rego_code: v })}
              rows={14}
              resizeOrientation="vertical"
              style={{ fontFamily: 'monospace', fontSize: '0.85rem' }}
              data-testid="policy-rego-code"
            />
          </LabeledField>
        </Form>
      </Modal>

      <Modal
        variant={ModalVariant.large}
        title={viewPolicy ? policyDisplayName(viewPolicy) : 'Policy'}
        isOpen={viewOpen}
        onClose={() => setViewOpen(false)}
        actions={[
          <Button
            key="close"
            variant="primary"
            onClick={() => setViewOpen(false)}
            data-testid="policy-view-close"
          >
            Close
          </Button>,
        ]}
      >
        {viewPolicy ? (
          <>
            <p>
              <strong>Type:</strong> {viewPolicy.policy_type} · <strong>Priority:</strong>{' '}
              {viewPolicy.priority ?? '—'} · <strong>Enabled:</strong>{' '}
              {viewPolicy.enabled !== false ? 'yes' : 'no'}
            </p>
            <p>
              <strong>Description:</strong> {viewPolicy.description || '—'}
            </p>
            {viewPolicy.cluster_requirements?.matchLabels ? (
              <p>
                <strong>Cluster requirements:</strong>{' '}
                <code>{JSON.stringify(viewPolicy.cluster_requirements.matchLabels)}</code>
              </p>
            ) : null}
            {viewPolicy.placement ? (
              <p>
                <strong>Placement:</strong> count={viewPolicy.placement.cluster_count ?? 1},
                strategy={viewPolicy.placement.strategy ?? 'single'}
              </p>
            ) : null}
            <p>
              <strong>ID:</strong> <code>{viewPolicy.id}</code>
            </p>
            <pre
              data-testid="policy-view-rego"
              style={{
                fontSize: '0.8rem',
                padding: '0.75rem',
                background: 'var(--pf-v5-global--BackgroundColor--200)',
                overflow: 'auto',
                maxHeight: '24rem',
              }}
            >
              {viewPolicy.rego_code || '—'}
            </pre>
          </>
        ) : null}
      </Modal>

      <Modal
        variant={ModalVariant.small}
        title="Delete policy"
        isOpen={deleteOpen}
        onClose={() => setDeleteOpen(false)}
        actions={[
          <Button
            key="del"
            variant="danger"
            onClick={confirmDelete}
            isLoading={deleting}
            data-testid="policy-delete-confirm"
          >
            Delete
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setDeleteOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <ErrorAlert error={deleteError} />
        <p>
          Delete policy <strong>{deleteTarget ? policyDisplayName(deleteTarget) : ''}</strong>?
          This cannot be undone.
        </p>
      </Modal>
    </>
  );
}
