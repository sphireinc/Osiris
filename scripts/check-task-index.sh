#!/usr/bin/env sh
set -eu

index='tasks/index.tsv'
[ -f "$index" ] || { printf '%s\n' "missing $index" >&2; exit 1; }

awk -F '|' '
  /^#/ { next }
  NF != 6 { printf "invalid index record at line %d\n", NR > "/dev/stderr"; bad = 1; next }
  $1 !~ /^[0-9][0-9][0-9]$/ { printf "invalid task id %s\n", $1 > "/dev/stderr"; bad = 1 }
  $3 !~ /^(active|ready|blocked|complete|superseded|historical)$/ { printf "invalid status for %s: %s\n", $1, $3 > "/dev/stderr"; bad = 1 }
  $4 == "" || $5 == "" || $6 == "" { printf "empty metadata for %s\n", $1 > "/dev/stderr"; bad = 1 }
  seen[$1]++ { printf "duplicate task id %s\n", $1 > "/dev/stderr"; bad = 1 }
  { print $1 "|" $2 "|" $3 "|" $5 }
  END { exit bad }
' "$index" > /tmp/osiris-task-index.$$ 

while IFS='|' read -r id path status dependencies; do
  [ -f "$path" ] || { printf '%s\n' "index path missing for $id: $path" >&2; exit 1; }
  if [ "$dependencies" != '-' ]; then
    old_ifs=$IFS
    IFS=','
    for dependency in $dependencies; do
      grep -q "^$dependency|" "$index" || { printf '%s\n' "unknown dependency $dependency for $id" >&2; exit 1; }
    done
    IFS=$old_ifs
  fi
done < /tmp/osiris-task-index.$$
rm -f /tmp/osiris-task-index.$$

for task in tasks/[0-9][0-9][0-9]-*.md tasks/tasks/[0-9][0-9][0-9]-*.md tasks/completed/[0-9][0-9][0-9]-*.md; do
  [ -e "$task" ] || continue
  id=${task##*/}
  id=${id%%-*}
  grep -q "^$id|" "$index" || { printf '%s\n' "task missing from index: $task" >&2; exit 1; }
done

printf '%s\n' 'Task index checks passed.'
