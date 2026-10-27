#!/bin/bash
# QE-08/CMMI-4: analyze docs/quality/history.jsonl for trends and regressions.
# Prints per-metric: first, last, delta, direction. Exits 1 if a metric
# regressed beyond its budget (coverage down, catalog shrinks, specs shrink).
set -u
cd "$(dirname "${0}")/.." || exit 1
H=docs/quality/history.jsonl
[ -f "${H}" ] || { echo "no history yet"; exit 0; }
[ "$(wc -l < "${H}")" -lt 2 ] && { echo "need >=2 records for trend"; exit 0; }

python3 - "$H" <<'PY'
import json,sys
rows=[json.loads(l) for l in open(sys.argv[1]) if l.strip()]
def num(v):
    s=str(v).rstrip('%')
    return float(s) if s.replace('.','',1).replace('-','',1).isdigit() else None
metrics={}
for r in rows:
    for k,v in r.items():
        if k in ('timestamp','commit','branch','run'): continue
        n=num(v)
        if n is not None: metrics.setdefault(k,[]).append(n)
regress=0
for k,vs in sorted(metrics.items()):
    first,last=vs[0],vs[-1]
    delta=last-first
    d='+' if delta>0 else ('-' if delta<0 else '=')
    # regression = a metric that should grow went down
    if k in ('coverage_bash','catalog_os','spec_files','coverage_go') and delta<0: regress=1
    print(f'{d} {k}: {first:g} -> {last:g} ({delta:+g})')
sys.exit(regress)
PY
