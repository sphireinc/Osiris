#!/usr/bin/env sh
set -eu

workflow_dir=${CI_WORKFLOW_DIR:-.github/workflows}
ci="$workflow_dir/ci.yml"
docs="$workflow_dir/docs.yml"

fail() { printf '%s\n' "$*" >&2; exit 1; }

[ -f "$ci" ] || fail "missing CI workflow: $ci"
[ -f "$docs" ] || fail "missing documentation workflow: $docs"
command -v ruby >/dev/null 2>&1 || fail 'Ruby is required to validate workflow YAML.'
ruby -e 'require "yaml"; ARGV.each { |file| YAML.load_file(file) }' "$ci" "$docs" || fail 'workflow YAML is invalid'

for job in fast race native docs platform; do
  grep -q "^  $job:" "$ci" || fail "CI workflow is missing $job job"
done
for check in 'go vet ./...' 'go test ./...' 'go test -race' 'cc: \[gcc, clang\]' 'npm run check-reference' 'npm run build'; do
  grep -q "$check" "$ci" || fail "CI workflow is missing required check: $check"
done
grep -q 'actions/deploy-pages@v4' "$docs" || fail 'documentation workflow does not deploy Pages'
grep -q 'docs/dist' "$docs" || fail 'documentation workflow does not publish the React build output'

printf '%s\n' 'CI workflow contract checks passed.'
