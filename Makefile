.PHONY: fmt check-fmt metrics lint complexity loc dupl test all

all: check-fmt lint metrics test

## Formatting ----------------------------------------------------------------

# Format all Go files (writes changes)
fmt:
	gofmt -w .

# Check formatting without modifying (exits non-zero if unformatted)
check-fmt:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "Unformatted files:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	@echo "All files formatted correctly"

## Metrics -------------------------------------------------------------------

# Run all metrics
metrics: loc complexity dupl

# Lines of code (excluding blanks), comments, complexity, DRYness
loc:
	@echo "=== Lines of Code ==="
	scc --no-cocomo .

# Cyclomatic complexity (top 15 most complex functions)
complexity:
	@echo "=== Cyclomatic Complexity (top 15) ==="
	-gocyclo -top 15 .
	@echo ""
	@echo "=== Cyclomatic Complexity (average) ==="
	-gocyclo -avg .

# Code duplication
dupl:
	@echo "=== Code Duplication (threshold: 100 tokens) ==="
	-golangci-lint run --enable dupl --disable-all 2>/dev/null || true

## Linting -------------------------------------------------------------------

# Run golangci-lint with all metric linters
lint:
	golangci-lint run

## Testing -------------------------------------------------------------------

test:
	go test -v ./...
