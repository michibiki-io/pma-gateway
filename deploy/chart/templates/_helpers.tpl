{{/*
Expand the name of the chart.
*/}}
{{- define "pma-gateway.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "pma-gateway.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "pma-gateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels.
*/}}
{{- define "pma-gateway.labels" -}}
helm.sh/chart: {{ include "pma-gateway.chart" . }}
app.kubernetes.io/name: {{ include "pma-gateway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels.
*/}}
{{- define "pma-gateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "pma-gateway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Create the name of the service account to use.
*/}}
{{- define "pma-gateway.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "pma-gateway.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Create the ConfigMap name.
*/}}
{{- define "pma-gateway.configMapName" -}}
{{- printf "%s-config" (include "pma-gateway.fullname" .) -}}
{{- end -}}

{{/*
Create the Secret name.
*/}}
{{- define "pma-gateway.secretName" -}}
{{- default (printf "%s-secret" (include "pma-gateway.fullname" .)) .Values.existingSecret -}}
{{- end -}}

{{/*
Normalize the public base path for probes and notes.
*/}}
{{- define "pma-gateway.publicBasePath" -}}
{{- $base := default "/" (index .Values.config "PMA_GATEWAY_PUBLIC_BASE_PATH") -}}
{{- if eq $base "" -}}
/
{{- else if hasPrefix "/" $base -}}
{{- trimSuffix "/" $base | default "/" -}}
{{- else -}}
/{{ trimSuffix "/" $base }}
{{- end -}}
{{- end -}}

{{/*
Return the public health path.
*/}}
{{- define "pma-gateway.healthPath" -}}
{{- $base := include "pma-gateway.publicBasePath" . -}}
{{- if eq $base "/" -}}
/healthz
{{- else -}}
{{- printf "%s/healthz" $base -}}
{{- end -}}
{{- end -}}

{{/*
Return the public readiness path.
*/}}
{{- define "pma-gateway.readyPath" -}}
{{- $base := include "pma-gateway.publicBasePath" . -}}
{{- if eq $base "/" -}}
/readyz
{{- else -}}
{{- printf "%s/readyz" $base -}}
{{- end -}}
{{- end -}}

{{/*
Return whether a writable data volume is needed.
*/}}
{{- define "pma-gateway.dataVolumeEnabled" -}}
{{- $driver := lower (default "sqlite" (index .Values.config "PMA_GATEWAY_DATABASE_DRIVER")) -}}
{{- if or .Values.persistence.enabled (eq $driver "sqlite") -}}
true
{{- else -}}
false
{{- end -}}
{{- end -}}

{{/*
Return the data mount path.
*/}}
{{- define "pma-gateway.dataMountPath" -}}
{{- default "/var/lib/pma-gateway" (index .Values.config "PMA_GATEWAY_DATA_DIR") -}}
{{- end -}}
