import type { FieldHelpContent } from './fieldHelp';

export const APPS_FIELD_HELP: Record<string, FieldHelpContent> = {
  'apps.clusterName': {
    description:
      'Ready workload cluster that will run the Helm release. You must select it; the hub does not pick a cluster.',
    example: 'lab-single-0',
  },
  'apps.chart': {
    description:
      'OCI chart URL, or a short name under the hub Harbor charts project (for example whoami).',
    example: 'oci://10.10.0.1:30443/charts/whoami',
  },
  'apps.version': {
    description: 'Chart version from Harbor. Leave empty only if the registry has a default.',
    example: '0.1.0',
  },
  'apps.releaseName': {
    description: 'Helm release name on the selected cluster. Also used as the default Service name for hub routes.',
    example: 'whoami',
  },
  'apps.namespace': {
    description: 'Spoke Kubernetes namespace. Created if missing.',
    example: 'default',
  },
  'apps.routeMode': {
    description:
      'none leaves ingress to the chart. hub publishes through the hub Traefik gateway to this cluster. site uses the cluster’s own ingress.',
    examples: ['none', 'hub', 'site'],
  },
  'apps.hubHost': {
    description:
      'Public hostname on the hub gateway. Leave empty to use <release>.<hub edge domain>. Must resolve to the hub WAN for ACME.',
    example: 'whoami.demo.example.test',
  },
  'apps.siteHost': {
    description:
      'Public hostname on the cluster site ingress (port-forward or reverse proxy). Leave empty to use <release>.<site edge domain>.',
    example: 'whoami.homelab.example.com',
  },
  'apps.valuesYaml': {
    description: 'Optional Helm values YAML merged at install time.',
    example: 'replicaCount: 1',
  },
  'apps.llamaProfile': {
    description:
      'Built-in llama-cpp-helm values profile from the hub API (runtime + model + registry image). You can edit the generated YAML before install. Open the help on Values (YAML) for extraArgs, top-p, and temp examples.',
    example: 'qwen35-2b-ud-4g',
  },
  'apps.llmBackend': {
    description:
      'When enabled, a Ready install is discovered as a hub LiteLLM backend (spoke WireGuard upstream, not the FleetApp route). Requires Infrastructure → LLM platform enabled. Uncheck for private or test installs.',
    example: 'enabled after qwen35 profile on lab-gpu-0',
  },
  'apps.llamaRoute': {
    description:
      'Fleet Llama profiles use hostNetwork on the GPU node. Hub/site routes target the chart Service and usually do not reach llama-server; hub LiteLLM uses the spoke upstream address instead. Use route only if you customized networking.',
  },
  'apps.editInstall': {
    description:
      'Patch chart version and Helm values on an existing FleetApp. Triggers helm upgrade --install on the spoke when the spec changes.',
    example: 'ctxSize: 8192',
  },
  'apps.rockyCpu': {
    description: 'vCPU cores for the Rocky Linux 10 guest (writes cpu in values).',
    example: '2',
  },
  'apps.rockyMemory': {
    description: 'Guest memory request (Kubernetes quantity).',
    example: '2Gi',
  },
  'apps.rockyDisk': {
    description: 'Root disk PVC size for the CDI import. Must cover the Rocky cloud image (~10Gi+).',
    example: '20Gi',
  },
  'apps.libraryChart': {
    description:
      'Ready library chart on the hub. Short names still work; they expand to Harbor charts.',
    example: 'whoami',
  },
  'charts.origin': {
    description: 'Upstream oci:// URL or an https:// link to a .tgz. Private charts should be uploaded instead.',
    example: 'oci://ghcr.io/example/app',
  },
  'charts.imagePolicy': {
    description:
      'external leaves image pulls on upstream registries. proxy uses Harbor pull-through. copy mirrors discovered images into Harbor.',
    examples: ['external', 'proxy', 'copy'],
  },
  'charts.upload': {
    description: 'Helm package (.tgz). Chart.yaml name and version become the library identity.',
    example: 'whoami-0.1.0.tgz',
  },
};
