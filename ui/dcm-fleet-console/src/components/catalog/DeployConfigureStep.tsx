import { useState } from 'react';
import {
  Alert,
  AlertVariant,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
  ExpandableSection,
  TextInput,
  Title,
} from '@patternfly/react-core';
import type { CatalogFieldDef } from '../../api/types';
import type { SiteDomainFieldContext } from '../../catalog/siteDomains';
import { getFieldHelp } from '../../help/fieldHelp';
import { LabeledField } from '../forms/LabeledField';
import { DynamicFieldForm } from './DynamicFieldForm';

type Props = {
  displayName: string;
  onDisplayNameChange: (value: string) => void;
  serviceType: string;
  requiredFields: CatalogFieldDef[];
  commonFields: CatalogFieldDef[];
  advancedFields: CatalogFieldDef[];
  readonlyFields: CatalogFieldDef[];
  fieldValues: Record<string, string>;
  onFieldChange: (path: string, value: string) => void;
  fieldErrors?: Record<string, string>;
  siteDomainContext?: SiteDomainFieldContext;
};

export function DeployConfigureStep({
  displayName,
  onDisplayNameChange,
  serviceType,
  requiredFields,
  commonFields,
  advancedFields,
  readonlyFields,
  fieldValues,
  onFieldChange,
  fieldErrors = {},
  siteDomainContext,
}: Props) {
  const [advancedOpen, setAdvancedOpen] = useState(advancedFields.length > 0);

  const hasStructuredFields =
    requiredFields.length > 0 || commonFields.length > 0 || advancedFields.length > 0;

  const serviceIntro = (() => {
    switch (serviceType) {
      case 'helm':
        return {
          title: 'Helm deployment',
          body: 'Set release name and namespace; routing mode controls fleet edge exposure. Click Validate before advancing to Placement.',
        };
      case 'operator':
        return {
          title: 'OLM operator subscription',
          body: 'Package and channel must match your catalog source. Target namespace is where the operator installs operands.',
        };
      case 'container':
        return {
          title: 'Container workload',
          body: 'DNS-label name becomes the container resource name. Policies select the target cluster unless routing.cluster is set.',
        };
      default:
        return null;
    }
  })();

  return (
    <>
      {serviceIntro ? (
        <Alert variant={AlertVariant.info} title={serviceIntro.title} isInline>
          {serviceIntro.body}{' '}
          <a href="https://github.com/rborso/umkim/blob/main/QUICKGUIDE.md" target="_blank" rel="noreferrer">
            QUICKGUIDE
          </a>
        </Alert>
      ) : null}

      <LabeledField
        label="Display name"
        isRequired
        fieldId="dep-display"
        help={getFieldHelp('deploy.displayName')}
      >
        <TextInput
          id="dep-display"
          value={displayName}
          onChange={(_e, v) => onDisplayNameChange(v)}
          data-testid="deploy-display-name"
        />
      </LabeledField>

      {requiredFields.length > 0 ? (
        <>
          <Title headingLevel="h3" size="md" style={{ marginTop: '1rem' }} data-testid="deploy-tier-required">
            Required parameters
          </Title>
          <DynamicFieldForm
            fields={requiredFields}
            values={fieldValues}
            onChange={onFieldChange}
            idPrefix="dep-required"
            errors={fieldErrors}
            siteDomainContext={siteDomainContext}
          />
        </>
      ) : null}

      {commonFields.length > 0 ? (
        <>
          <Title headingLevel="h3" size="md" style={{ marginTop: '1rem' }} data-testid="deploy-tier-common">
            Common options
          </Title>
          <p style={{ fontSize: '0.875rem', color: 'var(--pf-v5-global--Color--200)' }}>
            Other template values are applied from defaults and not shown.
          </p>
          <DynamicFieldForm
            fields={commonFields}
            values={fieldValues}
            onChange={onFieldChange}
            idPrefix="dep-common"
            errors={fieldErrors}
            siteDomainContext={siteDomainContext}
          />
        </>
      ) : null}

      {advancedFields.length > 0 ? (
        <ExpandableSection
          toggleText="Advanced options"
          isExpanded={advancedOpen}
          onToggle={(_e, expanded) => setAdvancedOpen(expanded)}
          style={{ marginTop: '1rem' }}
          data-testid="deploy-tier-advanced"
        >
          <DynamicFieldForm
            fields={advancedFields}
            values={fieldValues}
            onChange={onFieldChange}
            idPrefix="dep-advanced"
            errors={fieldErrors}
            siteDomainContext={siteDomainContext}
          />
        </ExpandableSection>
      ) : null}

      {!hasStructuredFields && serviceType !== 'cluster' ? (
        <p>No editable fields defined for this catalog item.</p>
      ) : null}

      {readonlyFields.length > 0 ? (
        <>
          <Title headingLevel="h3" size="md" style={{ marginTop: '1.5rem' }}>
            Template defaults (read-only)
          </Title>
          <DescriptionList isCompact isHorizontal>
            {readonlyFields.map((f) => (
              <DescriptionListGroup key={f.path}>
                <DescriptionListTerm>
                  {f.display_name || f.path}
                </DescriptionListTerm>
                <DescriptionListDescription>
                  <code>{f.path}</code>
                  {f.default !== undefined && f.default !== null ? (
                    <span style={{ marginLeft: '0.5rem' }}>
                      = {String(f.default)}
                    </span>
                  ) : null}
                </DescriptionListDescription>
              </DescriptionListGroup>
            ))}
          </DescriptionList>
        </>
      ) : null}
    </>
  );
}
