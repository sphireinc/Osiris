#!/bin/sh
# Fails when generated artifacts or local dependencies are tracked. This is a
# repository policy check, not a replacement for package-manager integrity.
set -eu

fail=0

for path in osiris internal/osiris/.osiris.lock; do
  if git ls-files --error-unmatch "$path" >/dev/null 2>&1; then
    printf '%s\n' "generated artifact must not be tracked: $path" >&2
    fail=1
  fi
done

if git ls-files | grep -E '(^|/)(node_modules|dist|\.osiris)(/|$)|(^|/)\.DS_Store$' >/dev/null; then
  printf '%s\n' 'generated dependency, output, cache, or OS metadata is tracked.' >&2
  git ls-files | grep -E '(^|/)(node_modules|dist|\.osiris)(/|$)|(^|/)\.DS_Store$' >&2
  fail=1
fi

for path in \
  osiris \
  internal/osiris/.osiris.lock \
  internal/osiris/.osiris/cache/probe \
  docs/node_modules/probe \
  docs/dist/probe \
  .DS_Store; do
  if ! git check-ignore -q "$path"; then
    printf '%s\n' "expected ignored path is not ignored: $path" >&2
    fail=1
  fi
done

if ! git diff --check; then
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi

printf '%s\n' 'Repository hygiene checks passed.'
