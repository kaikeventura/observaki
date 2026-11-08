#!/usr/bin/env bash
# Story 5.4 AC1 — metadata backup/restore for a Helm release.
#
#   ./deploy/helm/backup.sh backup --file observaki-metadata.json
#   ./deploy/helm/backup.sh restore --file observaki-metadata.json --force
#
# The API image is distroless, so the backup is streamed through
# `kubectl exec` from the running API pod. The restore requires --force because
# it replaces the control-plane metadata atomically.
set -euo pipefail

NAMESPACE=observaki
RELEASE=observaki
ACTION=""
FILE="-"
FORCE=0

usage() {
	cat <<'EOF'
Usage: deploy/helm/backup.sh <backup|restore> [options]

Options:
  --namespace NAME   Kubernetes namespace (default: observaki)
  --release NAME     Helm release name (default: observaki)
  --file PATH        backup file; "-" uses stdout/stdin (default: -)
  --force            required by restore to replace existing metadata
  --help             show this help

Examples:
  ./deploy/helm/backup.sh backup --file metadata.json
  ./deploy/helm/backup.sh restore --file metadata.json --force
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	backup | restore)
		[ -z "$ACTION" ] || {
			echo "only one action is allowed" >&2
			exit 2
		}
		ACTION=$1
		shift
		;;
	--namespace)
		NAMESPACE=${2:?--namespace requires a value}
		shift 2
		;;
	--release)
		RELEASE=${2:?--release requires a value}
		shift 2
		;;
	--file)
		FILE=${2:?--file requires a value}
		shift 2
		;;
	--force)
		FORCE=1
		shift
		;;
	--help | -h)
		usage
		exit 0
		;;
	*)
		echo "unknown argument: $1" >&2
		usage >&2
		exit 2
		;;
	esac
done

[ -n "$ACTION" ] || {
	usage >&2
	exit 2
}
command -v kubectl >/dev/null || {
	echo "kubectl is required" >&2
	exit 1
}

POD=$(kubectl -n "$NAMESPACE" get pods \
	-l "app.kubernetes.io/instance=${RELEASE},app.kubernetes.io/component=api" \
	--field-selector=status.phase=Running \
	-o jsonpath='{.items[0].metadata.name}')
[ -n "$POD" ] || {
	echo "no running API pod for release ${RELEASE} in namespace ${NAMESPACE}" >&2
	exit 1
}

case "$ACTION" in
backup)
	if [ "$FILE" = "-" ]; then
		kubectl exec -n "$NAMESPACE" "$POD" -- /observaki backup --output -
	else
		kubectl exec -n "$NAMESPACE" "$POD" -- /observaki backup --output - >"$FILE"
		echo "backup written to ${FILE}" >&2
	fi
	;;
restore)
	[ "$FORCE" -eq 1 ] || {
		echo "restore requires --force (it replaces the control-plane metadata)" >&2
		exit 2
	}
	if [ "$FILE" = "-" ]; then
		kubectl exec -i -n "$NAMESPACE" "$POD" -- /observaki restore --input - --force
	else
		kubectl exec -i -n "$NAMESPACE" "$POD" -- /observaki restore --input - --force <"$FILE"
	fi
	;;
esac
