# quickemu-qmp

ADR-0001 phase 1: QMP client extracted from the Bash socat path.

## Build

    cd tools/quickemu-qmp && go build -o quickemu-qmp .

## Usage

    quickemu-qmp execute --socket <path> --command '{"execute":"query-status"}' [--timeout 5s] [--raw]
    quickemu-qmp watch   --socket <path>          # stream QMP async events as JSONL

- Default output is the versioned JSON envelope
  (`{schemaVersion:1,command:"qmp.execute",data,warnings,errors,meta}`).
- `--raw` prints the bare `{"return":...}`/`{"error":...}` reply, which is
  what `qmp_command` callers in `monitor.sh` consume.
- The Bash backend calls this binary when found (PATH or
  `tools/quickemu-qmp/quickemu-qmp`) and falls back to socat otherwise.
