#!/usr/bin/env bash
# Checks that a pull request title follows Conventional Commits.
# The title becomes the squash commit on main, so it is the project history.
#
#   scripts/check-pr-title.sh "feat(index): add the HNSW graph"
set -euo pipefail

title="${1:-}"
types='feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert'
pattern="^(${types})(\([a-z0-9._/-]+\))?!?: [a-z0-9].*[^. ]$"
max=100

if [[ -z "$title" ]]; then
  echo "usage: $0 \"<pull request title>\"" >&2
  exit 2
fi

if [[ ! "$title" =~ $pattern ]]; then
  echo "invalid title: $title" >&2
  echo "expected: <type>(<optional scope>): <lowercase summary, no final period>" >&2
  echo "types: ${types//|/, }" >&2
  exit 1
fi

if ((${#title} > max)); then
  echo "invalid title: ${#title} characters, the limit is $max" >&2
  exit 1
fi

echo "valid title: $title"
