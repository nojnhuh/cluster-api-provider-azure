{{- define "capaso.commonLabels" -}}
app.kubernetes.io/name: capaso-workload
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "capaso.clusterName" -}}
{{ default .Release.Name .Values.clusterName }}
{{- end }}

{{- define "capaso.asoResourceAnnotations" -}}
serviceoperator.azure.com/credential-from: {{ .Values.credentialSecretName }}
{{- end }}
