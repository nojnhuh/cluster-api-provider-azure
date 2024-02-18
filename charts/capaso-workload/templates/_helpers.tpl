{{- define "capaso.asoResourceAnnotations" -}}
serviceoperator.azure.com/credential-from: {{ .Values.credentialSecretName }}
{{- end }}
