#!/bin/bash

# Script for running golangci-lint with consistent flags in CI environments
# This ensures proper handling of VCS stamping issues

# Set default environment variables
export GOFLAGS="-buildvcs=false"

# Output debug information
echo "Current directory: $(pwd)"
echo "Go files in current directory:"
find . -name "*.go" -type f | head -n 5

# Try to find the golangci-lint binary
GOLANGCI_LINT=$(which golangci-lint)
if [ -z "$GOLANGCI_LINT" ]; then
  # Try common installation paths
  if [ -f "$HOME/go/bin/golangci-lint" ]; then
    GOLANGCI_LINT="$HOME/go/bin/golangci-lint"
  elif [ -f "/usr/local/bin/golangci-lint" ]; then
    GOLANGCI_LINT="/usr/local/bin/golangci-lint"
  else
    echo "Error: golangci-lint not found in PATH"
    exit 1
  fi
fi

# Parse additional arguments
ARGS=""
for arg in "$@"; do
  ARGS="$ARGS $arg"
done

# If no args provided, default to scanning the entire project
if [ -z "$ARGS" ]; then
  # Check for Go files in the current directory
  GO_FILES_COUNT=$(find . -name "*.go" -type f | wc -l)
  
  if [ "$GO_FILES_COUNT" -eq 0 ]; then
    echo "No Go files found in current directory. Checking parent directory..."
    if [ -d "../pkg" ] && [ -f "../go.mod" ]; then
      echo "Found Go files in parent directory, switching to it."
      cd ..
      GO_FILES_COUNT=$(find . -name "*.go" -type f | wc -l)
      echo "Found $GO_FILES_COUNT Go files in parent directory."
    fi
  else
    echo "Found $GO_FILES_COUNT Go files in current directory."
  fi

  # Now set the argument
  ARGS="./..."
fi

# Use configuration file if it exists
CONFIG_FLAG=""
if [ -f ".golangci.yml" ]; then
  CONFIG_FLAG="--config=.golangci.yml"
fi

# First attempt - with both flags
echo "Running: $GOLANGCI_LINT run $CONFIG_FLAG $ARGS"
$GOLANGCI_LINT run $CONFIG_FLAG $ARGS
exit_code=$?

# If first attempt fails, try with environment variable only
if [ $exit_code -ne 0 ]; then
  echo "Retrying linting with environment variable approach..."
  GOFLAGS="-buildvcs=false" $GOLANGCI_LINT run $CONFIG_FLAG $ARGS
  exit_code=$?
fi

# If second attempt fails, try with build tags
if [ $exit_code -ne 0 ]; then
  echo "Retrying linting with build tags approach..."
  $GOLANGCI_LINT run $CONFIG_FLAG --build-tags="buildvcs=false" $ARGS
  exit_code=$?
fi

# Final fallback - basic run
if [ $exit_code -ne 0 ]; then
  echo "Retrying linting with minimal options..."
  $GOLANGCI_LINT run $ARGS
  exit_code=$?
fi

# Report scan status
if [ $exit_code -eq 0 ]; then
  echo "Linting completed successfully!"
else
  echo "Linting failed with exit code $exit_code"
fi

exit $exit_code 