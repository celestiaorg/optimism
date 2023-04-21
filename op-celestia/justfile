import '../justfiles/go.just'

# Build ldflags string
_LDFLAGSSTRING := "'" + trim(
    "-X main.GitCommit=" + GITCOMMIT + " " + \
    "-X main.GitDate=" + GITDATE + " " + \
    "-X main.Version=" + VERSION + " " + \
    "") + "'"

BINARY := "./bin/op-celestia-indexer"

# Build op-celestia-indexer binary
op-celestia-indexer:
    mkdir -p bin
    go build -ldflags {{_LDFLAGSSTRING}} -o {{BINARY}} ./cmd

# Clean build artifacts
clean:
    rm -f {{BINARY}}
    rm -rf bin

# Run tests
test: (go_test "./...")

# Run fuzzing tests
fuzz:
    go test -fuzz=. -fuzztime=10s ./...
