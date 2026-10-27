#!/bin/bash
# Dump the quickget catalog to JSON: for every OS in os_support emit
# releases, editions per release, arch, and the resolved get_ URL pair.
# This becomes the Go catalog's data source (catalog/data/catalog.json).
set -u
cd "$(dirname "${0}")/.." || exit 1
source lib/quickget/catalog/data.sh 2>/dev/null
source lib/quickget/catalog/get.sh 2>/dev/null
source lib/quickget/catalog/trust.sh 2>/dev/null || true
OUT=catalog/data/catalog.json
mkdir -p "$(dirname "${OUT}")"
python3 - "$OUT" <<'PY'
import json,sys,subprocess
oses=subprocess.check_output(['bash','-c','source lib/quickget/catalog/data.sh; os_support'],text=True).split()
entries=[]
for os in oses:
    try:
        rel=subprocess.check_output(['bash','-c',f'source lib/quickget/catalog/data.sh; releases_{os}'],text=True,timeout=30).split()
    except: rel=[]
    try:
        ed=subprocess.check_output(['bash','-c',f'source lib/quickget/catalog/data.sh; type -t editions_{os}'],text=True,timeout=10).strip()=='function'
        editions=subprocess.check_output(['bash','-c',f'source lib/quickget/catalog/data.sh; editions_{os}'],text=True,timeout=30).split() if ed else []
    except: editions=[]
    try:
        arch=subprocess.check_output(['bash','-c',f'source lib/quickget/catalog/data.sh; type -t arch_{os} >/dev/null && arch_{os} || echo amd64'],text=True,timeout=10).split()
    except: arch=['amd64']
    entries.append({'os':os,'releases':rel,'editions':editions,'arch':arch})
json.dump({'generated':'bash-dump','entries':entries},open(sys.argv[1],'w'),indent=1)
print(f'{len(entries)} entries dumped')
PY
