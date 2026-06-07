#!/bin/bash

# Script to ensure no fmt.Print* functions are used for logging, as per GEMINI.md mandates.
# fmt.Sprintf and fmt.Errorf are allowed as they don't print to stdout.

echo "🚀 Checking for forbidden fmt.Print usage..."

# Search for fmt.Print, fmt.Printf, fmt.Println
# We use -P for perl-regex to use negative lookahead if needed, 
# but a simple grep -E is enough here.
FORBIDDEN=$(grep -rE "fmt\.Print(f|ln)?" . --include="*.go" --exclude-dir=vendor --exclude-dir=ui)

if [ -n "$FORBIDDEN" ]; then
    echo "❌ Forbidden fmt.Print usage found:"
    echo "$FORBIDDEN"
    echo ""
    echo "Please use log/slog instead for logging."
    exit 1
fi

echo "✅ No forbidden fmt.Print usage found."
exit 0
