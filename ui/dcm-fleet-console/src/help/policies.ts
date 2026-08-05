import type { FieldHelpContent } from './fieldHelp';
import { DOCS } from './docsUrls';

export const POLICIES_FIELD_HELP: Record<string, FieldHelpContent> = {
  'policy.id': {
    description: 'Org-admin policy IDs are prefixed with {orgSlug}-. Enter only the suffix in the console.',
    example: 'dedicated-container',
  },
  'policy.displayName': {
    description: 'Human-readable policy title shown in the policies table and placement preview.',
    example: 'Fleet helm requires target cluster',
    whenToUse: 'Choose a name that describes the placement rule, not the app being deployed.',
  },
  'policy.policyType': {
    description: 'GLOBAL policies apply fleet-wide; USER policies are scoped to the creating user.',
    example: 'GLOBAL',
  },
  'policy.description': {
    description: 'Short summary of what this policy enforces for operators browsing the policies list.',
    example: 'Default container placement on clusters with RWO storage',
  },
  'policy.priority': {
    description:
      'Lower numbers evaluate first when multiple policies match the same instance. Tie-breaker when Rego alone does not reject.',
    example: '100',
    whenToUse: 'Use 10–50 for hard requirements; 100+ for defaults and catch-all policies.',
    commonMistakes: ['Setting every policy to priority 1 — ordering becomes unpredictable.'],
  },
  'policy.enabled': {
    description: 'Disabled policies are skipped during placement and routing evaluation.',
    example: 'true',
  },
  'policy.matchLabels': {
    description:
      'Cluster labels required for placement (one per line, key=value). Every listed label must be present on the OCM ManagedCluster. This scopes which clusters are eligible before Rego runs.',
    example: 'fleet.capability/storage.rwx=true',
    examples: ['fleet.site=homelab', 'fleet.capability/kvm=true'],
    whenToUse:
      'Use capability labels to require storage, KVM, or site membership. Leave empty to allow any registered cluster.',
    relatedFields: ['policy.strategy', 'policy.clusterCount', 'policy.labelSelector'],
    commonMistakes: [
      'Using instance labels here — use Instance label selector instead.',
      'Omitting fleet.site when you intend site-scoped placement.',
    ],
  },
  'policy.clusterCount': {
    description:
      'How many clusters receive the workload when this policy wins. Works with placement strategy and failover mode.',
    example: '1',
    whenToUse: 'Use 1 for single-cluster apps; 2+ with spread for HA or geo distribution.',
    relatedFields: ['policy.strategy', 'policy.failoverMode', 'policy.siteDiversity'],
  },
  'policy.strategy': {
    description:
      'How eligible clusters are chosen: single picks one; spread distributes across N clusters; all_matching places on every matching cluster; least_utilized picks the cluster with lowest CPU/memory utilization.',
    example: 'single',
    examples: ['single', 'spread', 'all_matching', 'least_utilized'],
    whenToUse:
      'single for most apps; spread when cluster_count > 1; least_utilized for load-aware single placement; all_matching for fleet-wide daemons.',
    relatedFields: ['policy.clusterCount', 'policy.siteDiversity'],
  },
  'policy.siteDiversity': {
    description:
      'When enabled with spread and cluster_count ≥ 2, placement prefers clusters in different fleet.site values.',
    example: 'false',
    whenToUse: 'Enable for multi-datacenter HA when you have clusters in more than one site.',
    relatedFields: ['policy.strategy', 'policy.clusterCount'],
  },
  'policy.labelSelector': {
    description:
      'Instance labels that must match for this policy to apply (key=value, one per line). Matched against catalog instance label_selector set at deploy time.',
    example: 'fleet.workload/profile=demo\nfleet.placement/scope=single',
    whenToUse:
      'Scope policies to instances deployed with specific tags. Pair with deploy wizard label selector.',
    relatedFields: ['policy.matchLabels', 'deploy.labelSelector'],
    commonMistakes: [
      'Confusing with cluster match labels — label_selector filters instances, not clusters.',
    ],
  },
  'policy.regoCode': {
    description:
      'OPA Rego policy body evaluated by policy-manager. Can reject placement or influence provider selection. Use import rego.v1 for OPA 1.x syntax.',
    example: 'package dcm\n\nmain := {"rejected": false} if input.spec.service_type == "container"',
    whenToUse:
      'Use Rego for logic that cannot be expressed with labels alone (service type checks, resource limits, custom rules).',
    learnMoreUrl: 'https://www.openpolicyagent.org/docs/policy-language',
    commonMistakes: [
      'Forgetting import rego.v1 on OPA 1.x hubs.',
      'Expecting Rego to set cluster_count — use placement fields instead.',
    ],
  },
  'policy.failoverMode': {
    description:
      'How the fleet reacts when hub health checks fail: manual observes only; relocatable cold-migrates (requires cluster_count 1); standby hot-switches (requires cluster_count 2).',
    example: 'manual',
    examples: ['manual', 'relocatable', 'standby'],
    whenToUse: 'Start with manual; use relocatable/standby only when routing and storage support failover.',
    relatedFields: ['policy.clusterCount', 'policy.hubHealthPath'],
    learnMoreUrl: DOCS.failover,
  },
  'policy.hubHealthPath': {
    description:
      'HTTP path on hub edge routes used to detect hub-side failures for failover policies.',
    example: '/',
    relatedFields: ['policy.failoverMode', 'routing.health_path'],
  },
  'policy.failureThreshold': {
    description: 'Consecutive failed health checks before failover action is considered.',
    example: '3',
  },
  'policy.previewServiceType': {
    description:
      'Service type used when previewing which providers and clusters this policy would match. Does not change the saved policy.',
    example: 'helm',
    examples: ['container', 'operator', 'helm', 'cluster'],
    whenToUse: 'Pick the service type you are testing placement for before clicking Preview placement.',
  },
};
