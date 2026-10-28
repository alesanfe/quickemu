cd /mnt/c/Users/alex0/Documents/fyp/quickemu/tools/quickemu-cli
export PATH="$HOME/go/bin:$PATH"
go test -count=1 ./... 2>&1 | tail -8
