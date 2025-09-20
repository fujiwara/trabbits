#!/bin/bash

# Generate coverage summary report from Go test coverage data
# Usage: ./scripts/coverage-summary.sh [coverage.out]

set -e

COVERAGE_FILE="${1:-coverage.out}"
OUTPUT_FILE="${2:-coverage_summary.md}"

# Check if coverage file exists
if [ ! -f "$COVERAGE_FILE" ]; then
    echo "Coverage file not found: $COVERAGE_FILE"
    echo "Please run 'make test-coverage' first"
    exit 1
fi

# Generate markdown summary
{
    echo "## Test Coverage Report"
    echo ""
    echo "**Total Coverage:** $(go tool cover -func="$COVERAGE_FILE" | tail -1 | awk '{print $NF}')"
    echo ""
    echo "### Package Summary"
    echo ""
    echo "| Package | Coverage |"
    echo "|---------|----------|"

    # Main package (root)
    coverage=$(go tool cover -func="$COVERAGE_FILE" | grep -E "^github.com/fujiwara/trabbits/[^/]+\.go:" | awk '{print $NF}' | sed 's/%//' | awk '{sum+=$1; count++} END {if(count>0) printf "%.1f%%", sum/count; else print "0.0%"}')
    echo "| github.com/fujiwara/trabbits | $coverage |"

    # Sub-packages (dynamically detected)
    subpackages=$(go tool cover -func="$COVERAGE_FILE" | grep -E "^github.com/fujiwara/trabbits/[^/]+/" | sed 's|^github.com/fujiwara/trabbits/||' | sed 's|/.*||' | sort -u)
    for pkg in $subpackages; do
        coverage=$(go tool cover -func="$COVERAGE_FILE" | grep -E "^github.com/fujiwara/trabbits/$pkg/[^/]+\.go:" | awk '{print $NF}' | sed 's/%//' | awk '{sum+=$1; count++} END {if(count>0) printf "%.1f%%", sum/count; else print "0.0%"}')
        echo "| github.com/fujiwara/trabbits/$pkg | $coverage |"
    done

    echo ""
    echo "<details>"
    echo "<summary>Detailed Coverage by File</summary>"
    echo ""
    echo "\`\`\`"
    go tool cover -func="$COVERAGE_FILE" | head -n -1
    echo "\`\`\`"
    echo "</details>"
} > "$OUTPUT_FILE"

echo "Coverage summary generated: $OUTPUT_FILE"

# Also display to stdout if not in CI
if [ -z "$CI" ]; then
    cat "$OUTPUT_FILE"
fi