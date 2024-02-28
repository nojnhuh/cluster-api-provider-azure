{{- define "capaso.commonLabels" -}}
app.kubernetes.io/name: capaso-workload
helm.sh/chart: {{ $.Chart.Name }}-{{ $.Chart.Version | replace "+" "_" }}
app.kubernetes.io/managed-by: {{ $.Release.Service }}
app.kubernetes.io/instance: {{ $.Release.Name }}
{{- end }}

{{- define "capaso.clusterName" -}}
{{ default $.Release.Name $.Values.clusterName }}
{{- end }}

{{- define "capaso.asoResourceAnnotations" -}}
serviceoperator.azure.com/credential-from: {{ $.Values.credentialSecretName }}
{{- end }}

{{- define "capaso.asoManagedClusterSpec" -}}
{{- $ := index . 0 -}}
{{- $clusterName := index . 1 -}}
resources:
- apiVersion: resources.azure.com/v1api20200601
  kind: ResourceGroup
  metadata:
    name: {{ quote $clusterName }}
    annotations:
      {{- include "capaso.asoResourceAnnotations" $ | nindent 6 }}
  spec:
    location: {{ $.Values.location }}
{{- end }}

{{- define "capaso.asoManagedControlPlaneSpec" -}}
{{- $ := index . 0 -}}
{{- $clusterName := index . 1 -}}
version: {{ $.Values.kubernetesVersion | quote  }}
resources:
- apiVersion: "containerservice.azure.com/{{ $.Values.managedClusterAPIVersion }}"
  kind: ManagedCluster
  metadata:
    name: {{ $clusterName | quote }}
    annotations:
      {{- include "capaso.asoResourceAnnotations" $ | nindent 6 }}
  spec:
    owner:
      name: {{ quote $clusterName }}
    dnsPrefix: {{ quote $clusterName }}
    location: {{ default $.Values.location $.Values.managedClusterSpec.location | quote }}
    {{- toYaml (unset $.Values.managedClusterSpec "location") | nindent 4 }}
    agentPoolProfiles:
      # TODO: This agent pool only exists to facilitate managing agent pools
      # separately from managed clusters:
      # https://github.com/Azure/azure-service-operator/issues/2791
      - vmSize: Standard_DS2_v2
        name: stub
        count: 1
        mode: System
    operatorSpec:
      secrets:
        adminCredentials:
          name: {{ printf "%s-kubeconfig" $clusterName | quote }}
          key: value
{{- end }}

{{- define "capaso.asoManagedMachinePoolSpec" -}}
{{- $ := index . 0 -}}
{{- $clusterName := index . 1 -}}
{{- $mpName := index . 2 -}}
{{- $mp := index . 3 -}}
resources:
- apiVersion: "containerservice.azure.com/{{ $.Values.managedMachinePoolAPIVersion }}"
  kind: ManagedClustersAgentPool
  metadata:
    name: {{ printf "%s-%s" $clusterName $mpName | quote }}
    annotations:
      {{- include "capaso.asoResourceAnnotations" $ | nindent 6 }}
  spec:
    azureName: {{ $mpName | quote }}
    {{- if $mp.owner }}
      {{- fail (printf ".Values.managedMachinePoolSpecs.%s.owner is not allowed to be set." $mpName) }}
    {{- end }}
    owner:
      name: {{ quote $clusterName }}
    {{- toYaml $mp | nindent 4 }}
{{- end }}
