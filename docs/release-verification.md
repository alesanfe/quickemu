# Verifying release artifacts

Every release ships `SHA256SUMS` + `sbom.json` alongside the tarball.

```bash
# 1. Download tarball + checksums
# 2. Verify integrity
sha256sum -c SHA256SUMS
# 3. Verify provenance: the SBOM lists every shipped file
python3 -c "import json; print(len(json.load(open('sbom.json'))['components']), 'files')"
# 4. Verify tag corresponds to CI workflow run (see release notes link)
```

Tags are annotated; verify with `git tag -v <tag>` when signed.
