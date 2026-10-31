#!/usr/bin/env bash
#
# Verify the Developer Certificate of Origin on every non-merge commit in a
# range. A commit passes when it has a well-formed Signed-off-by trailer whose
# e-mail matches the commit author or committer.
#
# Usage: scripts/check-dco.sh <base-ref> [head-ref]

set -euo pipefail

usage() {
	cat <<'EOF'
Usage: scripts/check-dco.sh <base-ref> [head-ref]

Checks every non-merge commit in <base-ref>..<head-ref> for a Signed-off-by
trailer matching the commit author or committer. Defaults to HEAD.
EOF
}

if [[ $# -lt 1 || $# -gt 2 ]]; then
	usage >&2
	exit 2
fi

base="$1"
head="${2:-HEAD}"

if ! git rev-parse --verify --quiet "${base}^{commit}" >/dev/null; then
	echo "DCO: base ref not found: ${base}" >&2
	exit 2
fi

if ! git rev-parse --verify --quiet "${head}^{commit}" >/dev/null; then
	echo "DCO: head ref not found: ${head}" >&2
	exit 2
fi

if ! merge_base="$(git merge-base "$base" "$head")"; then
	echo "DCO: cannot find a merge base between ${base} and ${head}" >&2
	exit 2
fi

checked=0
failed=0
summary=""

while IFS= read -r sha; do
	[[ -n "$sha" ]] || continue
	checked=$((checked + 1))

	subject="$(git log -1 --format=%s "$sha")"
	author_email="$(git log -1 --format=%ae "$sha" | tr '[:upper:]' '[:lower:]')"
	committer_email="$(git log -1 --format=%ce "$sha" | tr '[:upper:]' '[:lower:]')"
	message="$(git log -1 --format=%B "$sha")"
	trailers="$(printf '%s\n' "$message" | git interpret-trailers --parse)"

	reason=""
	if ! printf '%s\n' "$trailers" | grep -Eiq '^Signed-off-by: .+ <[^<>[:space:]]+@[^<>[:space:]]+>$'; then
		reason="missing Signed-off-by trailer"
	else
		signed_emails="$(printf '%s\n' "$trailers" |
			sed -nE 's/^Signed-off-by: .*<([^<>]+)>[[:space:]]*$/\1/p' |
			tr '[:upper:]' '[:lower:]')"
		if ! printf '%s\n' "$signed_emails" | grep -Fqx "$author_email" &&
			! printf '%s\n' "$signed_emails" | grep -Fqx "$committer_email"; then
			reason="Signed-off-by does not match the author <${author_email}> or committer <${committer_email}>"
		fi
	fi

	if [[ -n "$reason" ]]; then
		failed=$((failed + 1))
		short="${sha:0:7}"
		echo "DCO: ${short} (${subject}): ${reason}" >&2
		summary+="- \`${short}\` ${subject}: ${reason}"$'\n'
		if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
			echo "::error title=DCO sign-off::${short} ${subject}: ${reason}"
		fi
	else
		echo "DCO ok: ${sha:0:7} ${subject}"
	fi
done < <(git rev-list --no-merges --reverse "${merge_base}..${head}")

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
	{
		echo "## DCO sign-off"
		echo
		if ((failed > 0)); then
			echo "**${failed} de ${checked} commit(s) sem sign-off válido.**"
			echo
			printf '%s' "$summary"
		elif ((checked == 0)); then
			echo "Nenhum commit não-merge para verificar."
		else
			echo "Todos os ${checked} commit(s) têm sign-off válido."
		fi
	} >>"$GITHUB_STEP_SUMMARY"
fi

if ((failed > 0)); then
	cat >&2 <<'EOF'

DCO check failed. Every commit must certify the Developer Certificate of
Origin with a Signed-off-by trailer that matches the commit author or
committer.

Fix the commits and push again:

  # sign off every commit since the base branch
  git rebase --signoff origin/main
  git push --force-with-lease

  # or fix only the latest commit
  git commit --amend --signoff --no-edit

Configure git with the name and e-mail you have the right to use:

  git config user.name "Your Name"
  git config user.email "you@example.com"

See CONTRIBUTING.md#developer-certificate-of-origin-dco.
EOF
	exit 1
fi

if ((checked == 0)); then
	echo "DCO: no non-merge commits to check in ${merge_base:0:7}..${head:0:7}."
else
	echo "DCO: all ${checked} commit(s) passed."
fi
