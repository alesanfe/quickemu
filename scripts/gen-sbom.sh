#!/bin/bash
# QE-10: generate a minimal SBOM (CycloneDX 1.5 JSON) for a release.
# Usage: scripts/gen-sbom.sh <version> > sbom.json
set -euo pipefail
V="${1:-unknown}"
echo "{"
echo '  "bomFormat": "CycloneDX",'
echo '  "specVersion": "1.5",'
echo '  "version": 1,'
echo '  "metadata": {'
echo '    "timestamp": "'"$(date -u +%Y-%m-%dT%H:%M:%SZ)"'",'
echo '    "tools": { "vendor": "quickemu", "name": "gen-sbom.sh" },'
echo '    "component": { "type": "application", "name": "quickemu", "version": "'"${V}"'" }'
echo '  },'
echo '  "components": ['
find bin lib -name '*.sh' -o -name 'quickemu' -o -name 'quickget' -o -name 'quickgetc' -o -name 'quickreport' 2>/dev/null | sort | awk 'NR>1{printf ",\n"} {printf "    { \"type\": \"file\", \"name\": \"%s\" }", $0}'
echo ''
echo '  ]'
echo '}'
