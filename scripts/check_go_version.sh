#!/bin/bash

set -e

REQUIRED_GO_VERSION="1.24"

echo "Checking Go version..."
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
MAJOR_MINOR=$(echo $GO_VERSION | cut -d. -f1,2)

if [[ $(echo "$MAJOR_MINOR >= $REQUIRED_GO_VERSION" | bc -l) -eq 1 ]]; then
  echo "✅ Go version $GO_VERSION meets the requirement (>= $REQUIRED_GO_VERSION)"
else
  echo "❌ Go version $GO_VERSION does not meet the requirement (>= $REQUIRED_GO_VERSION)"
  echo "Please upgrade your Go installation."
  exit 1
fi 