cd /mnt/c/Users/alex0/Documents/fyp/quickemu
export PATH="$HOME/go/bin:$PATH"
for d in tools/*/; do
  m=$(basename $d)
  [ -f "$d/go.mod" ] || continue
  f=$(cd $d && GOWORK=off go test -count=1 -race ./... 2>&1 | grep -E 'FAIL|DATA RACE' | head -3)
  [ -n "$f" ] && echo "RACE-OR-FAIL: $m -> $f"
done
echo RACE_SWEEP_DONE
