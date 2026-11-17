#!/usr/bin/env bash
set -euo pipefail

cd "$(cd "$(dirname "$0")" && pwd)"

if [ -n "$(git status --porcelain)" ]; then
  echo "工作区有未提交改动，请先提交或暂存后再同步上游。" >&2
  exit 1
fi

if ! git remote get-url upstream >/dev/null 2>&1; then
  git remote add upstream https://github.com/RuoJi6/ARTEX.git
fi

git switch main
git fetch upstream --tags --prune
git merge upstream/main

echo "上游已合并到本地 main。检查和测试通过后执行：git push origin main"
