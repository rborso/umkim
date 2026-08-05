import type { FieldHelpContent } from './fieldHelp';
import { DOCS } from './docsUrls';

export const DEPLOY_WIZARD_FIELD_HELP: Record<string, FieldHelpContent> = {
  'deploy.catalogTemplate': {
    description: 'Catalog item (template) to instantiate. Determines service type and editable fields on the next step.',
    example: 'whoami-helm',
    learnMoreUrl: DOCS.deployApp,
  },
  'deploy.serviceType': {
    description: 'Read-only service type from the selected catalog item (container, helm, operator, or cluster).',
    example: 'helm',
    learnMoreUrl: DOCS.dcm,
  },
  'deploy.displayName': {
    description: 'Human-readable name for this catalog instance (shown in the instances list).',
    example: 'whoami-lab',
  },
  'deploy.placementOption': {
    description:
      'Where the workload runs across the fleet. Pod replica count is set in the catalog configure step, not here.',
    example: 'single',
    examples: ['single', 'least_utilized', 'ha_standby'],
    whenToUse:
      'Single cluster for most apps; least utilized picks the spoke with lowest CPU/memory; HA across two sites enables hub standby failover.',
    relatedFields: ['deploy.labelSelector'],
    commonMistakes: [
      'Expecting HA option to create two pod replicas on one cluster — it places on two clusters.',
    ],
    learnMoreUrl: DOCS.failover,
  },
  'deploy.placementSite': {
    description:
      'Fleet site for placement (sets fleet.site label) and default DNS zone when exposing via site or chart ingress.',
    example: 'homelab-ext',
    relatedFields: ['deploy.labelSelector', 'values.fleet.ingress.baseDomain'],
    learnMoreUrl: DOCS.deployApp,
  },
  'deploy.labelSelector': {
    description:
      'Key=value labels (one per line) matched against policy label_selector. Container/Helm demos use profile=demo; Rocky/VM items use profile=kubevirt-test. For VMs also set routing.cluster to the target KVM cluster so the site kubevirt policy can select the provider.',
    example: 'fleet.workload/profile=kubevirt-test\nfleet.placement/scope=single',
    whenToUse:
      'scope=single → demo-*-single / demo-vm-single. VMs need routing.cluster=<cluster> (e.g. lab-int04) in addition to kubevirt-test labels.',
    commonMistakes: [
      'Using fleet.workload/profile=demo for rocky9-vm (matches container policies, not demo-vm-single).',
      'Omitting routing.cluster for VMs (site kubevirt policy never selects kubevirt-<cluster>).',
      'Confusing fleet.placement/scope with catalog workload.replicas (pod count).',
    ],
    learnMoreUrl: DOCS.deployApp,
  },
  'deploy.placementPool': {
    description:
      'Org users: shared places on platform shared clusters (counts against sharedQuota); dedicated places on clusters labeled fleet.org for your org.',
    example: 'shared',
    learnMoreUrl: `${DOCS.multiTenancy}#labels`,
  },
};
