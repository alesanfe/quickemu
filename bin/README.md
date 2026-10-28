# bin/

The four public entry points are **compiled Go binaries**, not scripts:

| Entry        | Source module           |
|--------------|-------------------------|
| `quickemu`   | `tools/quickemu-cli`    |
| `quickget`   | `tools/quickget-cli`    |
| `quickgetc`  | `tools/quickgetc`       |
| `quickreport`| `tools/quickreport`     |

Build them with:

```sh
go run ./tools/dev build   # or: task build
```

Sibling tools (`quickemu-vm`, `quickget-catalog`, …) are resolved by
`qelib.FindTool`: same directory, `../<name>/<name>` dev-tree,
`../tools/<name>/<name>` and packaged `../libexec/quickemu/`.
