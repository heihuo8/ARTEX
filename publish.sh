#!/usr/bin/env bash
set -euo pipefail

cd "$(cd "$(dirname "$0")" && pwd)"
tag="${1:-}"

if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "用法：./publish.sh v0.3.30.1" >&2
  exit 1
fi
if [ -n "$(git status --porcelain)" ]; then
  echo "工作区有未提交改动，停止发布。" >&2
  exit 1
fi
if git rev-parse "$tag" >/dev/null 2>&1; then
  echo "标签已存在：$tag" >&2
  exit 1
fi

git push origin main
git tag -a "$tag" -m "Release $tag"
git push origin "$tag"
echo "已推送 $tag；GitHub Actions 正在构建 Release 和 ghcr.io/heihuo8/artex。"
