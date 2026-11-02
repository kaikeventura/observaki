#!/usr/bin/env bash
# Manage the Observaki T1 reference benchmark cluster.
#
# The cluster shape is owned by bench/env/t1.json. This script generates the
# Terraform variables from that profile and keeps the paid operations behind an
# explicit spend guard.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_DIR="$ROOT/bench/env"
TF_DIR="$ENV_DIR/terraform"
PROFILE="${OBSERVAKI_BENCH_PROFILE:-$ENV_DIR/t1.json}"
PRICES="${OBSERVAKI_BENCH_PRICES:-$ENV_DIR/prices.aws-us-east-1.json}"
BENCHCTL="$ROOT/bin/benchctl"
TFVARS="$TF_DIR/profile.auto.tfvars"
PLAN_FILE="$TF_DIR/.observaki.tfplan"
APPLIED_AT_FILE="$TF_DIR/.observaki-applied-at"
HOURS="${OBSERVAKI_BENCH_HOURS:-4}"

usage() {
	cat <<'EOF'
cluster.sh — provision, inspect and destroy the Observaki T1 benchmark cluster

Usage:
  ./bench/env/cluster.sh check
  ./bench/env/cluster.sh cost [--hours N] [--json]
  ./bench/env/cluster.sh plan
  ./bench/env/cluster.sh apply [--yes]
  ./bench/env/cluster.sh status
  ./bench/env/cluster.sh output
  ./bench/env/cluster.sh destroy [--yes]
  ./bench/env/cluster.sh cleanup [--yes]

Safety:
  - apply refuses to run unless OBSERVAKI_BENCH_ALLOW_SPEND=1 is set.
  - apply refuses AWS root credentials unless OBSERVAKI_BENCH_ALLOW_ROOT=1.
  - terraform.tfvars is operator-owned and must contain admin_cidr and
    ssh_public_key. The cluster shape comes from bench/env/t1.json.
  - destroy is idempotent and reports the estimated run cost.
EOF
}

die() {
	echo "cluster: $*" >&2
	exit 1
}

need_benchctl() {
	[ -x "$BENCHCTL" ] || die "benchctl not found; run make build"
}

need_terraform() {
	command -v terraform >/dev/null 2>&1 || die "terraform is required"
}

need_tfvars() {
	[ -f "$TF_DIR/terraform.tfvars" ] || die "missing $TF_DIR/terraform.tfvars; copy terraform.tfvars.example and set admin_cidr and ssh_public_key"
}

sync_profile() {
	need_benchctl
	"$BENCHCTL" env check --profile "$PROFILE" --prices "$PRICES" >/dev/null
	"$BENCHCTL" env tfvars --profile "$PROFILE" --out "$TFVARS" >/dev/null
}

tf() {
	terraform -chdir="$TF_DIR" "$@"
}

init_tf() {
	tf init -input=false >/dev/null
}

print_cost() {
	local hours="${1:-$HOURS}"
	need_benchctl
	"$BENCHCTL" env cost --profile "$PROFILE" --prices "$PRICES" --hours "$hours"
}

guard_spend() {
	if [ "${OBSERVAKI_BENCH_ALLOW_SPEND:-0}" != "1" ]; then
		die "refusing to create paid infrastructure; review 'cluster.sh cost' and set OBSERVAKI_BENCH_ALLOW_SPEND=1"
	fi
}

guard_root() {
	if [ "${OBSERVAKI_BENCH_ALLOW_ROOT:-0}" = "1" ]; then
		echo "warning: OBSERVAKI_BENCH_ALLOW_ROOT=1 is set; root credentials are allowed" >&2
		return 0
	fi
	command -v aws >/dev/null 2>&1 || die "aws CLI is required to verify the caller identity"
	local arn
	arn="$(aws sts get-caller-identity --query Arn --output text 2>/dev/null)" || die "could not verify the AWS caller identity; configure a dedicated IAM user or role"
	case "$arn" in
	*:root)
		die "refusing to apply with AWS root credentials; use a dedicated IAM user/role or set OBSERVAKI_BENCH_ALLOW_ROOT=1 for an isolated sandbox"
		;;
	esac
}

warn_root() {
	command -v aws >/dev/null 2>&1 || return 0
	local arn
	arn="$(aws sts get-caller-identity --query Arn --output text 2>/dev/null || true)"
	case "$arn" in
	*:root) echo "warning: AWS root credentials detected; apply will refuse unless OBSERVAKI_BENCH_ALLOW_ROOT=1" >&2 ;;
	esac
}

elapsed_hours() {
	if [ ! -f "$APPLIED_AT_FILE" ]; then
		printf '%s' "$HOURS"
		return 0
	fi
	local applied now
	applied="$(cat "$APPLIED_AT_FILE")"
	now="$(date +%s)"
	awk -v applied="$applied" -v now="$now" 'BEGIN { hours = (now - applied) / 3600; if (hours < 0) hours = 0; printf "%.2f", hours }'
}

command="${1:-help}"
shift || true

case "$command" in
help | -h | --help)
	usage
	;;
check)
	need_benchctl
	"$BENCHCTL" env check --profile "$PROFILE" --prices "$PRICES"
	;;
cost)
	need_benchctl
	args=(--profile "$PROFILE" --prices "$PRICES" --hours "$HOURS")
	while [ "$#" -gt 0 ]; do
		case "$1" in
		--hours)
			[ "$#" -ge 2 ] || die "--hours requires a value"
			args+=(--hours "$2")
			shift 2
			;;
		--json)
			args+=(--json)
			shift
			;;
		*)
			die "unknown cost flag: $1"
			;;
		esac
	done
	"$BENCHCTL" env cost "${args[@]}"
	;;
plan)
	need_benchctl
	need_terraform
	need_tfvars
	warn_root
	sync_profile
	init_tf
	print_cost
	tf plan -input=false -out=.observaki.tfplan
	;;
apply)
	guard_spend
	guard_root
	need_tfvars
	sync_profile
	init_tf
	print_cost
	if [ "${1:-}" != "--yes" ]; then
		printf "Type 'apply' to create the T1 cluster: " >&2
		read -r answer || die "aborted"
		[ "$answer" = "apply" ] || die "aborted"
	fi
	tf plan -input=false -out=.observaki.tfplan
	tf apply -input=false -auto-approve .observaki.tfplan
	[ -f "$APPLIED_AT_FILE" ] || date +%s >"$APPLIED_AT_FILE"
	rm -f "$PLAN_FILE"
	echo "cluster created; keep the result of 'terraform output' with the benchmark report"
	tf output
	;;
status)
	need_terraform
	init_tf
	tf output || die "no cluster state; run cluster.sh plan/apply first"
	hours="$(elapsed_hours)"
	print_cost "$hours"
	;;
output)
	need_terraform
	init_tf
	tf output -json cluster
	;;
destroy | cleanup)
	need_benchctl
	need_terraform
	need_tfvars
	sync_profile
	init_tf
	hours="$(elapsed_hours)"
	echo "elapsed since apply: ${hours}h"
	print_cost "$hours"
	if [ "${1:-}" != "--yes" ]; then
		printf "Type 'destroy' to delete the T1 cluster: " >&2
		read -r answer || die "aborted"
		[ "$answer" = "destroy" ] || die "aborted"
	fi
	tf destroy -input=false -auto-approve
	remaining="$(tf state list 2>/dev/null | wc -l | tr -d ' ')" || remaining=0
	[ "$remaining" = "0" ] || die "destroy left $remaining resource(s) in state"
	rm -f "$APPLIED_AT_FILE" "$PLAN_FILE" "$TFVARS"
	echo "cleanup: 0 resources remaining"
	;;
*)
	usage
	die "unknown command: $command"
	;;
esac
