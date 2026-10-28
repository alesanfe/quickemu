cd /mnt/c/Users/alex0/Documents/fyp/quickemu
export PATH="$HOME/go/bin:$PATH"
rc=0
for m in quickemu-ops quickemu-ports quickemu-proc quickemu-toml quickemu-archive quickreport; do
  (cd tools/$m && go test ./... 2>&1 | tail -2) || rc=1
done
echo "--- qespec ---"
go run ./tools/dev build >/dev/null 2>&1
(cd tools/qespec && go test -timeout 15m ./... 2>&1 | tail -5)
echo "RC=$rc"
