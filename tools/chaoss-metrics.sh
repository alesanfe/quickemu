#!/bin/bash
# CHAOSS metrics snapshot: activity, contributor concentration, review
# latency proxies — from git history (no API token needed).
set -u
cd "$(dirname "${0}")/.." || exit 1
echo "{"
echo '  "tool": "chaoss-metrics.sh",'
echo '  "timestamp": "'"$(date -u +%Y-%m-%dT%H:%M:%SZ)"'",'
echo '  "metrics": {'
commits30=$(git log --since="30 days ago" --oneline | wc -l)
commits90=$(git log --since="90 days ago" --oneline | wc -l)
authors=$(git log --since="90 days ago" --format='%an' | sort -u | wc -l)
top_author_pct=$(git log --since="90 days ago" --format='%an' | sort | uniq -c | sort -rn | head -1 | awk -v t="${commits90}" '{printf "%d", ($1/(t>0?t:1))*100}')
last_commit=$(git log -1 --format='%ci' | cut -d' ' -f1)
tag_count=$(git tag | wc -l)
echo "    \"commits_30d\": ${commits30},"
echo "    \"commits_90d\": ${commits90},"
echo "    \"active_authors_90d\": ${authors},"
echo "    \"top_author_share_pct\": ${top_author_pct},"
echo "    \"last_commit\": \"${last_commit}\","
echo "    \"release_tags\": ${tag_count}"
echo '  }'
echo '}'
