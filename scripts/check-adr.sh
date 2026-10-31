#!/usr/bin/env bash
#
# Validate the ADR set: required sections, lifecycle status, review evidence
# for accepted records, open questions linked to Phase 0, and index coverage.
#
# Usage: scripts/check-adr.sh

set -euo pipefail

cd "$(dirname "$0")/.."

adr_dir="docs/adr"
template="${adr_dir}/template.md"
index="${adr_dir}/README.md"
required_sections=(
	"Contexto"
	"Decisão"
	"Alternativas consideradas"
	"Consequências"
	"Status"
	"Revisão"
	"Questões abertas"
	"Referências"
	"Histórico"
)
valid_statuses=("proposto" "em revisão" "aceito" "substituído" "rejeitado")
errors=0
checked=0

fail() {
	local file="$1" message="$2"
	echo "ADR check: ${file}: ${message}" >&2
	errors=$((errors + 1))
}

section() {
	local file="$1" heading="$2"
	awk -v h="## ${heading}" '
		$0 == h { inside = 1; next }
		inside && /^## / { exit }
		inside { print }
	' "$file"
}

field() {
	local file="$1" name="$2"
	sed -nE "s/^- \\*\\*${name}:\\*\\* *([^ <]+).*/\\1/p" "$file" | head -n 1
}

check_structure() {
	local file="$1" allow_placeholder="${2:-0}" heading status reviewers review_section questions open_count row
	for heading in "${required_sections[@]}"; do
		if ! grep -qxF "## ${heading}" "$file"; then
			fail "$file" "missing required section '## ${heading}'"
		fi
	done

	status="$(field "$file" "Status")"
	if [[ -z "$status" ]]; then
		fail "$file" "missing '- **Status:**' metadata"
	else
		local valid=""
		for candidate in "${valid_statuses[@]}"; do
			[[ "$status" == "$candidate" ]] && valid="yes"
		done
		[[ -n "$valid" ]] || fail "$file" "invalid status '${status}'"
	fi

	reviewers="$(field "$file" "Revisores")"
	if [[ -z "$reviewers" ]]; then
		fail "$file" "missing '- **Revisores:**' metadata"
	fi

	if [[ "$status" == "aceito" ]]; then
		if [[ -z "$reviewers" || "$reviewers" == "pendente" ]]; then
			fail "$file" "status 'aceito' requires a named reviewer"
		fi
		review_section="$(section "$file" "Revisão")"
		if ! grep -Eiq 'aprovado' <<<"$review_section"; then
			fail "$file" "status 'aceito' requires an 'aprovado' row in '## Revisão'"
		fi
	fi

	questions="$(section "$file" "Questões abertas")"
	open_count="$(grep -cE '^\| Q-[0-9]+' <<<"$questions" || true)"
	if ((open_count == 0)); then
		if ! grep -q "Nenhuma questão aberta" <<<"$questions"; then
			fail "$file" "no open-question rows and no 'Nenhuma questão aberta' note"
		fi
	else
		while IFS= read -r row; do
			[[ -n "$row" ]] || continue
			if [[ "$allow_placeholder" != "1" ]] && ! grep -Eq 'S0\.[0-9]|Story [0-9]+\.[0-9]+' <<<"$row"; then
				fail "$file" "open question is not linked to a Phase 0 spike/story: ${row}"
			fi
		done < <(grep -E '^\| Q-[0-9]+' <<<"$questions")
	fi
}

if [[ ! -f "$template" ]]; then
	fail "$template" "template not found"
else
	check_structure "$template" 1
fi

if [[ ! -f "$index" ]]; then
	fail "$index" "index not found"
fi

declare -A seen_numbers=()

for file in "$adr_dir"/*.md; do
	base="$(basename "$file")"
	[[ "$base" == "README.md" || "$base" == "template.md" ]] && continue
	checked=$((checked + 1))

	if [[ ! "$base" =~ ^[0-9]{4}-[a-z0-9-]+\.md$ ]]; then
		fail "$file" "file name must match NNNN-slug.md"
		continue
	fi

	number="${base:0:4}"
	if [[ -n "${seen_numbers[$number]:-}" ]]; then
		fail "$file" "duplicate ADR number ${number} (also used by ${seen_numbers[$number]})"
	else
		seen_numbers[$number]="$base"
	fi

	if [[ -f "$index" ]] && ! grep -qF "](${base})" "$index"; then
		fail "$file" "not linked from docs/adr/README.md"
	fi

	check_structure "$file"
done

if ((checked == 0)); then
	echo "ADR check: no ADRs found in ${adr_dir}" >&2
	exit 1
fi

if ((errors > 0)); then
	echo "ADR check: ${errors} error(s) across ${checked} ADR(s)." >&2
	exit 1
fi

echo "ADR check: ${checked} ADR(s) valid."
