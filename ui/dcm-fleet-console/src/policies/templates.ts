/** Rego templates validated against policy-manager (OPA Rego v1). */

export interface PolicyTemplate {
  id: string;
  label: string;
  description: string;
  rego_code: string;
}

export const POLICY_TEMPLATES: PolicyTemplate[] = [
  {
    id: 'demo-container-single',
    label: 'Demo containers · one cluster',
    description:
      'Accept container service_type. Pair with label_selector fleet.workload/profile=demo and fleet.placement/scope=single, cluster_requirements fleet.capability/storage.rwo=true, placement.cluster_count=1.',
    rego_code: `package dcm.fleet.demo.container

main := {
  "rejected": false
} if input.spec.service_type == "container"
`,
  },
  {
    id: 'demo-container-ha-standby',
    label: 'Demo containers · HA across two sites',
    description:
      'Accept container service_type. Pair with label_selector fleet.workload/profile=demo and fleet.placement/scope=ha-standby, cluster_requirements fleet.capability/storage.rwo=true, placement.cluster_count=2, site_diversity, failover.mode=standby.',
    rego_code: `package dcm.fleet.demo.container.ha

main := {
  "rejected": false
} if input.spec.service_type == "container"
`,
  },
  {
    id: 'demo-helm-single',
    label: 'Demo Helm · one cluster',
    description:
      'Accept helm service_type. Pair with label_selector fleet.workload/profile=demo and fleet.placement/scope=single, cluster_requirements fleet.capability/storage.rwo=true, placement.cluster_count=1.',
    rego_code: `package dcm.fleet.demo.helm

main := {
  "rejected": false
} if input.spec.service_type == "helm"
`,
  },
  {
    id: 'demo-helm-ha-standby',
    label: 'Demo Helm · HA across two sites',
    description:
      'Accept helm service_type. Pair with label_selector fleet.workload/profile=demo and fleet.placement/scope=ha-standby, cluster_requirements fleet.capability/storage.rwo=true, placement.cluster_count=2, site_diversity, failover.mode=standby.',
    rego_code: `package dcm.fleet.demo.helm.ha

main := {
  "rejected": false
} if input.spec.service_type == "helm"
`,
  },
  {
    id: 'deny',
    label: 'Reject all',
    description: 'Reject placement requests — for testing enforcement.',
    rego_code: `package dcm.fleet.deny

main := {
  "rejected": true,
  "rejection_reason": "Denied by policy"
}
`,
  },
];

export const DEFAULT_REGO = POLICY_TEMPLATES[0].rego_code;
