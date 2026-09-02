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
    description: 'Public hostname on the hub gateway. Defaults to <release>.<edge domain> when empty.',
    example: 'whoami.demo.example.test',
  },
  'apps.valuesYaml': {
    description: 'Optional Helm values YAML merged at install time.',
    example: 'replicaCount: 1',
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
