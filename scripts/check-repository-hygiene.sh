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

# Implementation and public documentation names describe their behavior, not
# the internal work item that introduced them. Keep the historical task-number
# namespace confined to AGENT/tasks and other internal planning records. The
# two-digit threshold leaves ordinary runtime IDs such as task-1 untouched.
task_pattern='(^|[^[:alnum:]_])[Tt]ask[0-9]+[[:alnum:]_-]*|(^|[^[:alnum:]_])[Tt]ask[- ][0-9]{2,}([[:alnum:]_-]*)?'
task_refs=$(git grep -n -I -E "$task_pattern" -- cmd internal docs scripts .github 2>/dev/null || true)
if command -v rg >/dev/null 2>&1; then
  # Also inspect untracked source and documentation. Exclude this checker,
  # whose explanatory regular expression necessarily contains the pattern.
  worktree_refs=$(rg -n -I --hidden \
    --glob '!scripts/check-repository-hygiene.sh' \
    --glob '!docs/node_modules/**' \
    --glob '!docs/dist/**' \
    -e "$task_pattern" cmd internal docs scripts .github 2>/dev/null || true)
  if [ -n "$worktree_refs" ]; then
    task_refs=$(printf '%s\n%s' "$task_refs" "$worktree_refs")
  fi
fi
if [ -n "$task_refs" ]; then
  printf '%s\n' 'historical task-number identifiers must not appear in implementation or public documentation:' >&2
  printf '%s\n' "$task_refs" >&2
  fail=1
fi

task_paths=$(git ls-files -- cmd internal docs scripts .github | grep -E '(^|/)(lsp_)?task[0-9]' || true)
if command -v rg >/dev/null 2>&1; then
  worktree_paths=$(rg --files --hidden \
    --glob '!docs/node_modules/**' \
    --glob '!docs/dist/**' \
    cmd internal docs scripts .github 2>/dev/null | \
    grep -E '(^|/)(lsp_)?task[0-9]|(^|/)task[-_][0-9]{2,}' || true)
  if [ -n "$worktree_paths" ]; then
    task_paths=$(printf '%s\n%s' "$task_paths" "$worktree_paths")
  fi
fi
if [ -n "$task_paths" ]; then
  printf '%s\n' 'historical task-number paths must not appear in implementation or public documentation:' >&2
  printf '%s\n' "$task_paths" >&2
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi

printf '%s\n' 'Repository hygiene checks passed.'
