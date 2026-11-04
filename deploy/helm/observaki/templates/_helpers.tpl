{{/*
Helpers for the Observaki chart.
*/}}

{{- define "observaki.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "observaki.fullname" -}}
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

{{- define "observaki.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "observaki.selectorLabels" -}}
app.kubernetes.io/name: {{ include "observaki.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "observaki.labels" -}}
helm.sh/chart: {{ include "observaki.chart" . }}
{{ include "observaki.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: observaki
{{- end -}}

{{/*
Component labels. Call with (dict "root" $ "component" "gateway").
*/}}
{{- define "observaki.componentSelectorLabels" -}}
{{ include "observaki.selectorLabels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "observaki.componentLabels" -}}
{{ include "observaki.labels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "observaki.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}

{{- define "observaki.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "observaki.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "observaki.clickhouseURL" -}}
{{- if eq .Values.dependencies.mode "bundled" -}}
{{- printf "http://%s-clickhouse:8123" (include "observaki.fullname" .) -}}
{{- else -}}
{{- required "external.clickhouse.url is required when dependencies.mode=external" .Values.external.clickhouse.url -}}
{{- end -}}
{{- end -}}

{{- define "observaki.clickhouseDatabase" -}}
{{- if eq .Values.dependencies.mode "bundled" -}}
observaki
{{- else -}}
{{- default "observaki" .Values.external.clickhouse.database -}}
{{- end -}}
{{- end -}}

{{- define "observaki.clickhouseCredentialSecret" -}}
{{- if eq .Values.dependencies.mode "bundled" -}}
{{- default (printf "%s-clickhouse" (include "observaki.fullname" .)) .Values.clickhouse.auth.existingSecret -}}
{{- else -}}
{{- .Values.external.clickhouse.credentialsSecret -}}
{{- end -}}
{{- end -}}

{{- define "observaki.clickhouseNativeHost" -}}
{{- if eq .Values.dependencies.mode "bundled" -}}
{{- printf "%s-clickhouse" (include "observaki.fullname" .) -}}
{{- else -}}
{{- $address := required "external.clickhouse.nativeAddress is required when migrations.enabled=true and dependencies.mode=external" .Values.external.clickhouse.nativeAddress -}}
{{- if contains ":" $address -}}
{{- first (splitList ":" $address) -}}
{{- else -}}
{{- $address -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "observaki.clickhouseNativePort" -}}
{{- if eq .Values.dependencies.mode "bundled" -}}
9000
{{- else -}}
{{- $address := .Values.external.clickhouse.nativeAddress -}}
{{- if contains ":" $address -}}
{{- last (splitList ":" $address) -}}
{{- else -}}
9000
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "observaki.victoriametricsURL" -}}
{{- if eq .Values.dependencies.mode "bundled" -}}
{{- printf "http://%s-victoriametrics:8428" (include "observaki.fullname" .) -}}
{{- else -}}
{{- required "external.victoriametrics.url is required when dependencies.mode=external" .Values.external.victoriametrics.url -}}
{{- end -}}
{{- end -}}

{{- define "observaki.postgresSecretName" -}}
{{- if eq .Values.dependencies.mode "bundled" -}}
{{- default (printf "%s-postgres" (include "observaki.fullname" .)) .Values.postgres.auth.existingSecret -}}
{{- else -}}
{{- required "external.postgres.urlSecret is required when dependencies.mode=external" .Values.external.postgres.urlSecret -}}
{{- end -}}
{{- end -}}

{{- define "observaki.postgresSecretKey" -}}
{{- if eq .Values.dependencies.mode "bundled" -}}
database-url
{{- else -}}
url
{{- end -}}
{{- end -}}

{{- define "observaki.natsURL" -}}
{{- if eq .Values.dependencies.mode "bundled" -}}
{{- printf "nats://%s-nats:4222" (include "observaki.fullname" .) -}}
{{- else -}}
{{- required "external.nats.url is required when dependencies.mode=external" .Values.external.nats.url -}}
{{- end -}}
{{- end -}}

{{/*
Fail-fast validation for combinations that would silently ship an unsafe or
broken deployment. Rendered by templates/validate.yaml.
*/}}
{{- define "observaki.validate" -}}
{{- $mode := .Values.dependencies.mode -}}
{{- if not (has $mode (list "bundled" "external")) -}}
{{- fail (printf "dependencies.mode must be \"bundled\" or \"external\", got %q" $mode) -}}
{{- end -}}
{{- if not (has .Values.environment (list "development" "test" "staging" "production")) -}}
{{- fail (printf "environment must be development, test, staging or production, got %q" .Values.environment) -}}
{{- end -}}
{{- if eq $mode "external" -}}
{{- if not .Values.external.clickhouse.url -}}
{{- fail "external.clickhouse.url is required when dependencies.mode=external" -}}
{{- end -}}
{{- if not .Values.external.victoriametrics.url -}}
{{- fail "external.victoriametrics.url is required when dependencies.mode=external" -}}
{{- end -}}
{{- if not .Values.external.postgres.urlSecret -}}
{{- fail "external.postgres.urlSecret is required when dependencies.mode=external" -}}
{{- end -}}
{{- if not .Values.external.nats.url -}}
{{- fail "external.nats.url is required when dependencies.mode=external" -}}
{{- end -}}
{{- end -}}

{{- if and (eq $mode "external") .Values.migrations.enabled (not .Values.external.clickhouse.nativeAddress) -}}
{{- fail "external.clickhouse.nativeAddress is required when migrations.enabled=true and dependencies.mode=external (or set migrations.enabled=false)" -}}
{{- end -}}

{{- if not .Values.gateway.auth.devSnapshot }}
{{- if not .Values.gateway.auth.existingSecret -}}
{{- fail "gateway.auth.existingSecret is required when gateway.auth.devSnapshot=false" -}}
{{- end -}}
{{- end -}}

{{- $signed := and (ne .Values.environment "development") (ne .Values.environment "test") -}}
{{- if $signed -}}
{{- if .Values.gateway.auth.devSnapshot -}}
{{- fail (printf "environment=%s forbids gateway.auth.devSnapshot: set gateway.auth.existingSecret and gateway.auth.publicKey (AD-19)" .Values.environment) -}}
{{- end -}}
{{- if not .Values.gateway.auth.existingSecret -}}
{{- fail (printf "environment=%s requires gateway.auth.existingSecret with a signed snapshot (AD-19)" .Values.environment) -}}
{{- end -}}
{{- if not .Values.gateway.auth.publicKey -}}
{{- fail (printf "environment=%s requires gateway.auth.publicKey to verify the signed snapshot (AD-19)" .Values.environment) -}}
{{- end -}}
{{- if not .Values.gateway.auth.auditFile -}}
{{- fail (printf "environment=%s requires gateway.auth.auditFile for the auth audit trail" .Values.environment) -}}
{{- end -}}
{{- if and (eq $mode "bundled") (not .Values.clickhouse.auth.existingSecret) -}}
{{- fail (printf "environment=%s forbids the bundled development ClickHouse credential: set clickhouse.auth.existingSecret" .Values.environment) -}}
{{- end -}}
{{- if and (eq $mode "bundled") (not .Values.postgres.auth.existingSecret) -}}
{{- fail (printf "environment=%s forbids the bundled development PostgreSQL credential: set postgres.auth.existingSecret" .Values.environment) -}}
{{- end -}}
{{- end -}}
{{- end -}}
