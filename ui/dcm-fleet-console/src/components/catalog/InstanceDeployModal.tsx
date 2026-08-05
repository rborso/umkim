import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Form,
  FormSelect,
  FormSelectOption,
  Modal,
  ModalVariant,
  ProgressStep,
  ProgressStepper,
  TextInput,
  TextArea,
  Spinner,
  Title,
  Alert,
  AlertVariant,
} from '@patternfly/react-core';
import type { CatalogItem, PlacementPreviewRequest } from '../../api/types';
import {
  allDeployFormFields,
  buildUserValues,
  defaultFieldValues,
  deployFieldsForServiceType,
  fieldsFromCatalogItem,
  partitionByTier,
  readonlyFieldsFromCatalog,
  resolveFieldTier,
} from '../../catalog/fieldUtils';
import {
  filterVisibleFields,
  orderFieldsByDependency,
  pruneHiddenFieldValues,
} from '../../catalog/fieldVisibility';
import { coerceEnumSelection } from '../../catalog/dependsOnOptions';
import { collectSecretRequests, redactUserValues } from '../../catalog/fieldSecrets';
import { validateDeployForm } from '../../catalog/fieldValidation';
import {
  validateRequireAuthAsync,
  validateRequireAuthSync,
  primaryClusterFromPreview,
} from '../../catalog/routingAuthValidation';
import { applyInstancePresets } from '../../catalog/valuePresets';
import {
  buildSiteDomainContext,
  fqdnFromPrefix,
  siteIdForDomainOption,
  siteZoneList,
  type EdgeDomainRegistry,
} from '../../catalog/siteDomains';
import {
  deployHasSiteDomainField,
  deploySupportsDomainRouting,
} from '../../catalog/fleetEdgeRoutingFields';
import { valuesToDisplayString } from '../../catalog/valuesMerge';
import { defaultLabelSelectorForCatalog, demoInstanceLabelSelector, isDemoCatalogItem, type DeployPlacementOption } from '../../catalog/demoDefaults';
import { DeployConfigureStep } from './DeployConfigureStep';
import { ErrorAlert } from '../ErrorAlert';
import { LabeledField } from '../forms/LabeledField';
import { getFieldHelp } from '../../help/fieldHelp';
import { textToMatchLabels } from '../../policies/placementForm';
import { usePlacementPreview } from '../../hooks/usePlacementPreview';
import { PlacementImpactPanel } from './PlacementImpactPanel';
import { useAuth } from '../../auth/AuthContext';
import { isCustomerUser, primaryOrg } from '../../auth/orgUtils';
import {
  catalogItemDisplayName,
  catalogItemId,
  catalogItemServiceType,
  createCatalogItemInstance,
  fleetSiteEffectiveId,
  fleetSiteDisplayName,
  fleetSiteIsManagement,
  fleetSiteIsRegistry,
  getCatalogItem,
  getEdgeDomains,
  listFleetSites,
  putCatalogInstanceSecrets,
} from '../../api/client';

type Props = {
  isOpen: boolean;
  catalogItem: CatalogItem | null;
  catalogItems?: CatalogItem[];
  initialLabelSelector?: Record<string, string>;
  lockCatalogItem?: boolean;
  onClose: () => void;
  onDeployed: () => void;
};

const STEPS = [
  { id: 'template', name: 'Template' },
  { id: 'configure', name: 'Configure' },
  { id: 'placement', name: 'Placement' },
  { id: 'review', name: 'Review' },
] as const;

const MAX_STEP = STEPS.length - 1;

function labelSelectorToText(sel?: Record<string, string>): string {
  if (!sel) return '';
  return Object.entries(sel)
    .map(([k, v]) => `${k}=${v}`)
    .join('\n');
}

function mergeFleetSiteLabel(text: string, siteId: string): string {
  const lines = text
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith('fleet.site='));
  if (siteId) lines.unshift(`fleet.site=${siteId}`);
  return lines.join('\n');
}

function orgPlacementLabels(
  orgSlug: string | undefined,
  pool: 'shared' | 'dedicated' = 'shared',
): Record<string, string> {
  if (!orgSlug) return {};
  return {
    'fleet.org': orgSlug,
    'fleet.placement/pool': pool,
  };
}

function mergeOrgLabels(
  base: Record<string, string> | undefined,
  orgSlug: string | undefined,
  injectOrg: boolean,
  pool: 'shared' | 'dedicated' = 'shared',
): Record<string, string> | undefined {
  if (!injectOrg || !orgSlug) return base;
  return { ...base, ...orgPlacementLabels(orgSlug, pool) };
}

function stepVariant(
  index: number,
  current: number,
): 'success' | 'info' | 'pending' {
  if (index < current) return 'success';
  if (index === current) return 'info';
  return 'pending';
}

export function InstanceDeployModal({
  isOpen,
  catalogItem,
  catalogItems = [],
  initialLabelSelector,
  lockCatalogItem = false,
  onClose,
  onDeployed,
}: Props) {
  const { user } = useAuth();
  const orgSlug = primaryOrg(user);
  const injectOrgLabels = isCustomerUser(user);
  const [placementPool, setPlacementPool] = useState<'shared' | 'dedicated'>('shared');
  const [step, setStep] = useState(0);
  const [loading, setLoading] = useState(false);
  const [selectedItem, setSelectedItem] = useState<CatalogItem | null>(catalogItem);
  const [displayName, setDisplayName] = useState('');
  const [fieldValues, setFieldValues] = useState<Record<string, string>>({});
  const [deployFields, setDeployFields] = useState(
    fieldsFromCatalogItem(catalogItem || {}),
  );
  const [readonlyFields, setReadonlyFields] = useState(
    readonlyFieldsFromCatalog(fieldsFromCatalogItem(catalogItem || {})),
  );
  const [serviceType, setServiceType] = useState('');
  const [labelSelectorText, setLabelSelectorText] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [configureValidationOk, setConfigureValidationOk] = useState<boolean | null>(null);
  const [placementValidationOk, setPlacementValidationOk] = useState<boolean | null>(null);
  const [placementOption, setPlacementOption] = useState<DeployPlacementOption>('single');
  const [fleetSites, setFleetSites] = useState<import('../../api/types').FleetSite[]>([]);
  const [edgeRegistry, setEdgeRegistry] = useState<EdgeDomainRegistry | null>(null);
  const [domainOptionByField, setDomainOptionByField] = useState<Record<string, string>>({});
  const [placementSiteId, setPlacementSiteId] = useState('');

  const supportsDomainRouting = deploySupportsDomainRouting(serviceType);
  const hasSiteDomainField = deployHasSiteDomainField(deployFields);

  const placementSites = useMemo(
    () => fleetSites.filter((s) => fleetSiteIsRegistry(s) && !fleetSiteIsManagement(s)),
    [fleetSites],
  );

  const handleDomainZoneChange = useCallback(
    (fieldPath: string, optionId: string, zone: string) => {
      setDomainOptionByField((prev) => ({ ...prev, [fieldPath]: optionId }));
      setFieldValues((prev) => {
        let next = { ...prev, [fieldPath]: zone };
        const siteId = siteIdForDomainOption(optionId, buildSiteDomainContext(
          placementSites,
          edgeRegistry,
          prev,
          domainOptionByField,
          () => {},
        ).options);
        if (siteId) {
          setPlacementSiteId(siteId);
          setLabelSelectorText((labels) => mergeFleetSiteLabel(labels, siteId));
        }
        next = applyInstancePresets(deployFields, next);
        return pruneHiddenFieldValues(deployFields, next);
      });
    },
    [deployFields, placementSites, edgeRegistry, domainOptionByField],
  );

  const siteDomainContext = useMemo(
    () =>
      hasSiteDomainField
        ? buildSiteDomainContext(
            placementSites,
            edgeRegistry,
            fieldValues,
            domainOptionByField,
            handleDomainZoneChange,
          )
        : undefined,
    [
      hasSiteDomainField,
      placementSites,
      edgeRegistry,
      fieldValues,
      domainOptionByField,
      handleDomainZoneChange,
    ],
  );

  const activeItem = selectedItem || catalogItem;
  const activeId = activeItem ? catalogItemId(activeItem) : '—';

  const visibleDeployFields = useMemo(
    () => orderFieldsByDependency(filterVisibleFields(deployFields, fieldValues)),
    [deployFields, fieldValues],
  );

  const { required: requiredFields, common: commonFields, advanced: advancedFields } =
    useMemo(() => partitionByTier(visibleDeployFields), [visibleDeployFields]);

  const previewUserValues = useMemo(
    () => buildUserValues(deployFields, fieldValues),
    [deployFields, fieldValues],
  );

  const reviewUserValues = useMemo(
    () => redactUserValues(previewUserValues, deployFields),
    [previewUserValues, deployFields],
  );

  const previewRequest = useMemo((): PlacementPreviewRequest | null => {
    if (!isOpen || activeId === '—') return null;
    return {
      mode: 'deploy',
      verbose: true,
      catalog_item_id: activeId,
      service_type: serviceType || undefined,
      label_selector: textToMatchLabels(labelSelectorText) || undefined,
      user_values: previewUserValues.length > 0 ? previewUserValues : undefined,
    };
  }, [isOpen, activeId, serviceType, labelSelectorText, previewUserValues]);

  const { preview, loading: previewLoading, error: previewError, refetch: refetchPlacement } =
    usePlacementPreview(previewRequest);

  useEffect(() => {
    if (!isOpen) return;
    let cancelled = false;
    (async () => {
      try {
        const [sites, edge] = await Promise.all([listFleetSites(), getEdgeDomains()]);
        if (cancelled) return;
        setFleetSites(sites);
        setEdgeRegistry((edge.registry as EdgeDomainRegistry) || null);
      } catch {
        if (!cancelled) {
          setFleetSites([]);
          setEdgeRegistry(null);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen) {
      setStep(0);
      setError(null);
      setDomainOptionByField({});
      setPlacementSiteId('');
      setPlacementPool('shared');
      return;
    }
    setSelectedItem(catalogItem);
    const defaultLabels = mergeOrgLabels(
      catalogItem
        ? defaultLabelSelectorForCatalog(catalogItemId(catalogItem), initialLabelSelector)
        : initialLabelSelector,
      orgSlug,
      injectOrgLabels,
      placementPool,
    );
    setLabelSelectorText(labelSelectorToText(defaultLabels));
    setPlacementOption('single');
    if (lockCatalogItem && catalogItem) {
      setStep(1);
    } else {
      setStep(0);
    }
  }, [isOpen, catalogItem, initialLabelSelector, lockCatalogItem, placementPool]);

  useEffect(() => {
    if (!isOpen || !activeItem) return;
    const id = catalogItemId(activeItem);
    if (id === '—') return;

    let cancelled = false;
    (async () => {
      setLoading(true);
      setError(null);
      try {
        const full = await getCatalogItem(id);
        if (cancelled) return;
        const st = catalogItemServiceType(full);
        const catalogFields = fieldsFromCatalogItem(full);
        const deploy = deployFieldsForServiceType(st, catalogFields);
        setServiceType(st);
        setDeployFields(deploy);
        setReadonlyFields(readonlyFieldsFromCatalog(catalogFields));
        setFieldValues(applyInstancePresets(deploy, defaultFieldValues(deploy)));
        setDomainOptionByField({});
        setDisplayName(
          `${catalogItemDisplayName(full)}-${Date.now().toString(36).slice(-4)}`,
        );
        if (!initialLabelSelector || Object.keys(initialLabelSelector).length === 0) {
          const demoLabels = mergeOrgLabels(
            defaultLabelSelectorForCatalog(id),
            orgSlug,
            injectOrgLabels,
            placementPool,
          );
          if (demoLabels) {
            setLabelSelectorText(labelSelectorToText(demoLabels));
          }
        }
        setSelectedItem(full);
      } catch (e) {
        if (!cancelled) setError(e);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [isOpen, activeItem?.id, activeItem?.uid, initialLabelSelector]);

  useEffect(() => {
    setFieldValues((prev) => pruneHiddenFieldValues(deployFields, prev));
  }, [deployFields, fieldValues['routing.mode'], fieldValues['values.__preset.exposure'], fieldValues['values.__preset.database']]);

  const handleFieldChange = (path: string, value: string) => {
    setFieldErrors((prev) => {
      if (!prev[path]) return prev;
      const next = { ...prev };
      delete next[path];
      return next;
    });
    setFieldValues((prev) => {
      let next = { ...prev, [path]: value };
      next = pruneHiddenFieldValues(deployFields, next);
      for (const field of deployFields) {
        if (field.depends_on?.path === path) {
          const coerced = coerceEnumSelection(field, next[field.path] ?? '', next);
          next[field.path] = coerced;
        }
      }
      next = applyInstancePresets(deployFields, next);
      return next;
    });
  };

  const validateConfigure = (): string | null => {
    if (!displayName.trim()) return 'Display name is required';
    const formFields = allDeployFormFields(deployFields, fieldValues);
    const { errors, message } = validateDeployForm(
      formFields,
      fieldValues,
      resolveFieldTier,
    );
    setFieldErrors(errors);
    if (message) return message;
    const authSync = validateRequireAuthSync(fieldValues);
    if (authSync) {
      setFieldErrors({ ...errors, 'routing.require_auth': authSync });
      return authSync;
    }
    return null;
  };

  const runValidateConfigure = async () => {
    const configureErr = validateConfigure();
    if (configureErr) {
      setConfigureValidationOk(false);
      setError(new Error(configureErr));
      return;
    }
    if (fieldValues['routing.require_auth'] === 'true') {
      const authErr = await validateRequireAuthAsync(fieldValues, preview);
      if (authErr) {
        setFieldErrors((prev) => ({ ...prev, 'routing.require_auth': authErr }));
        setConfigureValidationOk(false);
        setError(new Error(authErr));
        return;
      }
    }
    if (deployFields.length > 0 && previewUserValues.length === 0) {
      setConfigureValidationOk(false);
      setError(new Error('Set at least one deployment field'));
      return;
    }
    setConfigureValidationOk(true);
    setError(null);
  };

  const runValidatePlacement = async () => {
    setPlacementValidationOk(null);
    setError(null);
    await refetchPlacement();
  };

  useEffect(() => {
    if (preview && !previewLoading && !previewError && step === 2) {
      setPlacementValidationOk(preview.deployable !== false);
    }
  }, [preview, previewLoading, previewError, step]);

  const goToStep = (target: number) => {
    if (target < 0 || target > MAX_STEP) return;
    if (target > step) return;
    setError(null);
    setStep(target);
  };

  const goNext = async () => {
    if (step === 0 && activeId === '—') {
      setError(new Error('Select a catalog template'));
      return;
    }
    if (step === 1) {
      const configureErr = validateConfigure();
      if (configureErr) {
        setError(new Error(configureErr));
        return;
      }
      if (deployFields.length > 0 && previewUserValues.length === 0) {
        setError(new Error('Set at least one deployment field'));
        return;
      }
      if (fieldValues['routing.require_auth'] === 'true') {
        const authErr = await validateRequireAuthAsync(fieldValues, preview);
        if (authErr) {
          setFieldErrors((prev) => ({ ...prev, 'routing.require_auth': authErr }));
          setError(new Error(authErr));
          return;
        }
      }
    }
    if (step === 2 && preview?.deployable === false) {
      setError(new Error('No eligible clusters for this placement'));
      return;
    }
    setError(null);
    setStep(Math.min(step + 1, MAX_STEP));
  };

  const goBack = () => {
    setError(null);
    setStep(Math.max(step - 1, 0));
  };

  const deploy = async () => {
    if (!activeItem || activeId === '—') return;
    const configureErr = validateConfigure();
    if (configureErr) {
      setError(new Error(configureErr));
      return;
    }
    if (preview?.deployable === false) {
      setError(new Error('No eligible clusters — fix placement or policies first'));
      return;
    }
    const authErr = await validateRequireAuthAsync(fieldValues, preview);
    if (authErr) {
      setFieldErrors((prev) => ({ ...prev, 'routing.require_auth': authErr }));
      setError(new Error(authErr));
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const releaseName = fieldValues['metadata.name']?.trim() || '';
      const namespace = fieldValues['release.namespace']?.trim() || 'default';
      const cluster =
        fieldValues['routing.cluster']?.trim() || primaryClusterFromPreview(preview) || '';
      const secretRequests = collectSecretRequests(
        deployFields,
        fieldValues,
        releaseName,
        cluster,
        namespace,
      );
      if (secretRequests.length > 0 && !cluster) {
        throw new Error(
          'Target cluster is required to store deployment secrets — set routing.cluster or complete placement preview',
        );
      }
      for (const req of secretRequests) {
        await putCatalogInstanceSecrets(req);
      }
      const userValues = buildUserValues(deployFields, fieldValues);
      if (userValues.length === 0) {
        throw new Error('Set at least one deployment field');
      }
      const label_selector = textToMatchLabels(labelSelectorText);
      await createCatalogItemInstance({
        display_name: displayName.trim(),
        spec: {
          catalog_item_id: activeId,
          user_values: userValues,
          ...(label_selector ? { label_selector } : {}),
        },
      });
      onDeployed();
      onClose();
    } catch (e) {
      setError(e);
    } finally {
      setSaving(false);
    }
  };

  const title = activeItem
    ? `Deploy: ${catalogItemDisplayName(activeItem)}`
    : 'Deploy catalog instance';

  return (
    <Modal
      variant={ModalVariant.large}
      title={title}
      isOpen={isOpen}
      onClose={onClose}
      actions={[
        step > 0 ? (
          <Button key="back" variant="secondary" onClick={goBack}>
            Back
          </Button>
        ) : null,
        step === 1 ? (
          <Button
            key="validate-configure"
            variant="secondary"
            onClick={() => void runValidateConfigure()}
            data-testid="deploy-validate-configure"
          >
            Validate
          </Button>
        ) : null,
        step === 2 ? (
          <Button
            key="validate-placement"
            variant="secondary"
            onClick={() => void runValidatePlacement()}
            isLoading={previewLoading}
            data-testid="deploy-validate-placement"
          >
            Validate placement
          </Button>
        ) : null,
        step < MAX_STEP ? (
          <Button
            key="next"
            variant="primary"
            onClick={() => void goNext()}
            isDisabled={loading || (step === 2 && preview?.deployable === false)}
            data-testid="deploy-wizard-next"
          >
            Next
          </Button>
        ) : (
          <Button
            key="deploy"
            variant="primary"
            onClick={deploy}
            isLoading={saving}
            isDisabled={loading || preview?.deployable === false}
            data-testid="instance-deploy-submit"
          >
            Deploy
          </Button>
        ),
        <Button key="cancel" variant="link" onClick={onClose}>
          Cancel
        </Button>,
      ]}
    >
      <ErrorAlert error={error} />
      {configureValidationOk === true && step === 1 ? (
        <Alert
          variant={AlertVariant.success}
          title="Configuration validation passed"
          isInline
          style={{ marginBottom: '0.75rem' }}
          data-testid="deploy-validate-configure-success"
        />
      ) : null}
      {placementValidationOk === true && step === 2 && preview ? (
        <Alert
          variant={AlertVariant.success}
          title="Placement validation passed"
          isInline
          style={{ marginBottom: '0.75rem' }}
          data-testid="deploy-validate-placement-success"
        />
      ) : null}
      {placementValidationOk === false && step === 2 && preview ? (
        <Alert
          variant={AlertVariant.warning}
          title="Placement validation failed"
          isInline
          style={{ marginBottom: '0.75rem' }}
          data-testid="deploy-validate-placement-failed"
        >
          No eligible clusters for this placement.
        </Alert>
      ) : null}

      <ProgressStepper
        aria-label="Deploy wizard steps"
        data-testid="deploy-wizard-stepper"
        style={{ marginBottom: '1.5rem' }}
      >
        {STEPS.map((s, index) => (
          <ProgressStep
            key={s.id}
            id={`deploy-step-${s.id}`}
            titleId={`deploy-step-title-${s.id}`}
            variant={stepVariant(index, step)}
            isCurrent={index === step}
            aria-label={`${s.name}${index === step ? ', current step' : index < step ? ', completed' : ''}`}
            onClick={index < step ? () => goToStep(index) : undefined}
            style={index < step ? { cursor: 'pointer' } : undefined}
          >
            {s.name}
          </ProgressStep>
        ))}
      </ProgressStepper>

      {loading ? (
        <Spinner size="lg" />
      ) : (
        <Form>
          {step === 0 && (
            <>
              <LabeledField label="Catalog template" isRequired fieldId="dep-catalog" help={getFieldHelp('deploy.catalogTemplate')}>
                {lockCatalogItem ? (
                  <TextInput
                    id="dep-catalog"
                    value={activeItem ? catalogItemDisplayName(activeItem) : ''}
                    isDisabled
                  />
                ) : (
                  <FormSelect
                    id="dep-catalog"
                    value={activeId}
                    onChange={(_e, v) => {
                      const found = catalogItems.find((c) => catalogItemId(c) === v);
                      setSelectedItem(found || null);
                    }}
                    data-testid="deploy-catalog-select"
                  >
                    {catalogItems.map((c) => {
                      const id = catalogItemId(c);
                      return (
                        <FormSelectOption
                          key={id}
                          value={id}
                          label={`${catalogItemDisplayName(c)} (${catalogItemServiceType(c)})`}
                        />
                      );
                    })}
                  </FormSelect>
                )}
              </LabeledField>
              <LabeledField label="Service type" fieldId="dep-stype" help={getFieldHelp('deploy.serviceType')}>
                <TextInput id="dep-stype" value={serviceType} isDisabled />
              </LabeledField>
            </>
          )}

          {step === 1 && (
            <DeployConfigureStep
              displayName={displayName}
              onDisplayNameChange={setDisplayName}
              serviceType={serviceType}
              requiredFields={requiredFields}
              commonFields={commonFields}
              advancedFields={advancedFields}
              readonlyFields={readonlyFields}
              fieldValues={fieldValues}
              fieldErrors={fieldErrors}
              onFieldChange={handleFieldChange}
              siteDomainContext={siteDomainContext}
            />
          )}

          {step === 2 && (
            <>
              {supportsDomainRouting && placementSites.length > 0 ? (
                <LabeledField
                  label="Target site"
                  fieldId="dep-placement-site"
                  help={getFieldHelp('deploy.placementSite')}
                >
                  <FormSelect
                    id="dep-placement-site"
                    value={placementSiteId}
                    onChange={(_e, v) => {
                      const siteId = v || '';
                      setPlacementSiteId(siteId);
                      setLabelSelectorText((labels) => mergeFleetSiteLabel(labels, siteId));
                      const site = placementSites.find((s) => fleetSiteEffectiveId(s) === siteId);
                      if (!site) return;
                      const zone = siteZoneList(site)[0] ?? '';
                      setFieldValues((prev) => {
                        let next = { ...prev };
                        const mode = prev['routing.mode'] || 'hub';
                        if (mode === 'none' && zone) {
                          next['values.fleet.ingress.baseDomain'] = zone;
                        }
                        if (mode === 'site' && zone) {
                          const prefix =
                            prev['values.fleet.ingress.host'] ||
                            prev['metadata.name'] ||
                            '';
                          if (!prev['routing.site_host']?.trim()) {
                            next['routing.site_host'] = fqdnFromPrefix(prefix, zone);
                          }
                        }
                        return applyInstancePresets(deployFields, next);
                      });
                      if (zone) {
                        setDomainOptionByField((prev) => ({
                          ...prev,
                          'values.fleet.ingress.baseDomain': `site:${siteId}:${zone}`,
                        }));
                      }
                    }}
                    data-testid="deploy-site-select"
                  >
                    <FormSelectOption value="" label="(select site)" />
                    {placementSites.map((s) => {
                      const id = fleetSiteEffectiveId(s);
                      return (
                        <FormSelectOption
                          key={id}
                          value={id}
                          label={fleetSiteDisplayName(s)}
                        />
                      );
                    })}
                  </FormSelect>
                </LabeledField>
              ) : null}
              {isDemoCatalogItem(activeId) ? (
                <LabeledField
                  label="Placement"
                  fieldId="dep-placement-option"
                  help={getFieldHelp('deploy.placementOption')}
                >
                  <FormSelect
                    id="dep-placement-option"
                    value={placementOption}
                    onChange={(_e, v) => {
                      const option = (v || 'single') as DeployPlacementOption;
                      setPlacementOption(option);
                      setLabelSelectorText(labelSelectorToText(demoInstanceLabelSelector(option)));
                    }}
                    data-testid="deploy-placement-option"
                  >
                    <FormSelectOption value="single" label="One cluster (default)" />
                    <FormSelectOption value="least_utilized" label="Least utilized cluster" />
                    <FormSelectOption value="ha_standby" label="HA across two sites (hub failover)" />
                  </FormSelect>
                </LabeledField>
              ) : null}
              {injectOrgLabels ? (
                <LabeledField
                  label="Placement pool"
                  fieldId="dep-pool"
                  help={getFieldHelp('deploy.placementPool')}
                >
                  <FormSelect
                    id="dep-pool"
                    value={placementPool}
                    onChange={(_e, v) =>
                      setPlacementPool((v || 'shared') as 'shared' | 'dedicated')
                    }
                    data-testid="deploy-placement-pool"
                  >
                    <FormSelectOption value="shared" label="Shared pool (quota applies)" />
                    <FormSelectOption value="dedicated" label="Dedicated org clusters" />
                  </FormSelect>
                </LabeledField>
              ) : null}
              <LabeledField
                label="Instance label selector (optional)"
                fieldId="dep-labels"
                help={getFieldHelp('deploy.labelSelector')}
                helperText={
                  <>
                    Optional key=value labels (one per line) matched against policy{' '}
                    <code>label_selector</code>. Leave empty for global policies; use capability
                    keys such as <code>fleet.capability/storage.rwo=true</code> only when scoping
                    a custom policy.
                  </>
                }
              >
                <TextArea
                  id="dep-labels"
                  value={labelSelectorText}
                  onChange={(_e, v) => setLabelSelectorText(v)}
                  rows={3}
                  placeholder="fleet.capability/storage.rwo=true"
                  data-testid="deploy-label-selector"
                  readOnly={injectOrgLabels}
                />
                {injectOrgLabels && orgSlug ? (
                  <small>
                    <code>fleet.org={orgSlug}</code> and{' '}
                    <code>fleet.placement/pool={placementPool}</code> are set automatically.
                  </small>
                ) : null}
              </LabeledField>
              <Title headingLevel="h3" size="md" style={{ marginTop: '1rem' }}>
                Placement impact
              </Title>
              <PlacementImpactPanel
                preview={preview}
                loading={previewLoading}
                error={previewError}
              />
            </>
          )}

          {step === 3 && (
            <>
              <DescriptionSummary
                displayName={displayName}
                serviceType={serviceType}
                catalogName={activeItem ? catalogItemDisplayName(activeItem) : '—'}
                labelSelectorText={labelSelectorText}
                userValues={reviewUserValues}
              />
              <Title headingLevel="h3" size="md" style={{ marginTop: '1rem' }}>
                Placement
              </Title>
              <PlacementImpactPanel
                preview={preview}
                loading={previewLoading}
                error={previewError}
              />
            </>
          )}
        </Form>
      )}
    </Modal>
  );
}

function DescriptionSummary({
  displayName,
  serviceType,
  catalogName,
  labelSelectorText,
  userValues,
}: {
  displayName: string;
  serviceType: string;
  catalogName: string;
  labelSelectorText: string;
  userValues: Array<{ path: string; value: unknown }>;
}) {
  const renderValue = (uv: { path: string; value: unknown }) => {
    if (uv.path === 'values' && typeof uv.value === 'object' && uv.value !== null) {
      return (
        <pre
          style={{
            margin: 0,
            maxHeight: '12rem',
            overflow: 'auto',
            fontSize: '0.75rem',
            whiteSpace: 'pre-wrap',
          }}
        >
          {valuesToDisplayString(uv.value as Record<string, unknown>)}
        </pre>
      );
    }
    if (typeof uv.value === 'object' && uv.value !== null) {
      return (
        <pre
          style={{
            margin: 0,
            maxHeight: '12rem',
            overflow: 'auto',
            fontSize: '0.75rem',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
          }}
        >
          {JSON.stringify(uv.value, null, 2)}
        </pre>
      );
    }
    return (
      <span style={{ wordBreak: 'break-word', whiteSpace: 'pre-wrap' }}>{String(uv.value)}</span>
    );
  };

  return (
    <>
      <DescriptionList isCompact>
        <DescriptionListGroup>
          <DescriptionListTerm>Template</DescriptionListTerm>
          <DescriptionListDescription>{catalogName}</DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>Service type</DescriptionListTerm>
          <DescriptionListDescription>{serviceType}</DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>Display name</DescriptionListTerm>
          <DescriptionListDescription>{displayName || '—'}</DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>Label selector</DescriptionListTerm>
          <DescriptionListDescription>
            <span style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
              {labelSelectorText || '(none)'}
            </span>
          </DescriptionListDescription>
        </DescriptionListGroup>
      </DescriptionList>
      {userValues.length > 0 ? (
        <>
          <Title headingLevel="h3" size="md" style={{ marginTop: '1rem' }}>
            Deployment values
          </Title>
          <DescriptionList isCompact data-testid="deploy-review-values">
            {userValues.map((uv) => (
              <DescriptionListGroup key={uv.path}>
                <DescriptionListTerm>
                  <code style={{ wordBreak: 'break-all' }}>{uv.path}</code>
                </DescriptionListTerm>
                <DescriptionListDescription>{renderValue(uv)}</DescriptionListDescription>
              </DescriptionListGroup>
            ))}
          </DescriptionList>
        </>
      ) : null}
    </>
  );
}
