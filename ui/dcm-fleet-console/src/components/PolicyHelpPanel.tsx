import {
  Button,
  ExpandableSection,
  List,
  ListItem,
  Text,
} from '@patternfly/react-core';
import { POLICY_TEMPLATES } from '../policies/templates';

export function PolicyHelpPanel({
  onInsertTemplate,
}: {
  onInsertTemplate?: (rego: string) => void;
}) {
  return (
    <ExpandableSection toggleText="Policy help and Rego examples" isIndented>
      <Text component="p">
        Policies are <strong>OPA Rego</strong> rules evaluated by policy-manager during
        placement and routing. The API validates Rego syntax on create and update — use{' '}
        <strong>Validate form</strong> before Save, then fix any Rego errors returned by the API.
      </Text>
      <Text component="h4">How policy fields work together</Text>
      <List>
        <ListItem>
          <code>label_selector</code> — which <em>catalog instances</em> this policy applies to
        </ListItem>
        <ListItem>
          <code>cluster_requirements.matchLabels</code> — which <em>clusters</em> are eligible
        </ListItem>
        <ListItem>
          <code>placement</code> — how many clusters and spread strategy
        </ListItem>
        <ListItem>
          <code>rego_code</code> — allow/reject and advanced logic
        </ListItem>
        <ListItem>
          <code>failover</code> — health-based relocation (pairs with cluster_count)
        </ListItem>
      </List>
      <Text component="p">
        Use <strong>Preview placement</strong> with a service type to see eligible clusters before
        saving. See{' '}
        <a href="https://github.com/rborso/umkim/blob/main/docs/dcm-placement-contract.md" target="_blank" rel="noreferrer">
          placement contract
        </a>{' '}
        for API details.
      </Text>
      <Text component="h4">Failover mode vs cluster count</Text>
      <List>
        <ListItem>
          <code>manual</code> — any cluster_count; observe health only
        </ListItem>
        <ListItem>
          <code>relocatable</code> — cluster_count must be 1; cold migrate on failure
        </ListItem>
        <ListItem>
          <code>standby</code> — cluster_count must be ≥ 2; hot standby switch
        </ListItem>
      </List>
      <Text component="h4">Required fields (snake_case in API)</Text>
      <List>
        <ListItem>
          <code>display_name</code> — human-readable title
        </ListItem>
        <ListItem>
          <code>policy_type</code> — <code>GLOBAL</code> or <code>USER</code> (UI defaults to
          GLOBAL)
        </ListItem>
        <ListItem>
          <code>rego_code</code> — valid Rego (use <code>import rego.v1</code> for OPA 1.x)
        </ListItem>
      </List>
      <Text component="p">
        Unlike providers and catalog items, policy write bodies use <strong>snake_case</strong>,
        not camelCase.
      </Text>
      <Text component="h4">Templates</Text>
      {POLICY_TEMPLATES.map((t) => (
        <div key={t.id} style={{ marginBottom: '0.75rem' }}>
          <Text component="small">{t.description}</Text>
          {onInsertTemplate ? (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => onInsertTemplate(t.rego_code)}
              data-testid={`policy-template-${t.id}`}
            >
              Insert: {t.label}
            </Button>
          ) : null}
        </div>
      ))}
      <Text component="small">
        Rego language reference:{' '}
        <a
          href="https://www.openpolicyagent.org/docs/policy-language"
          target="_blank"
          rel="noreferrer"
        >
          openpolicyagent.org/docs/policy-language
        </a>
      </Text>
    </ExpandableSection>
  );
}
