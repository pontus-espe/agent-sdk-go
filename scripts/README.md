# Script Utilities

This directory contains utility scripts for the Agent SDK Go project. They run
the same checks as the GitHub Actions workflows, so a green `check_all.sh` means
a green CI run.

## Scripts

| Script | What it does |
| --- | --- |
| `check_all.sh` | Runs every check below plus the test suite |
| `lint.sh` | gofmt, goimports (when installed), `go vet` and the build |
| `run_lint.sh` | Runs golangci-lint with the repository configuration |
| `security_check.sh` | Runs gosec, excluding the examples |
| `run_gosec.sh` | Runs gosec directly with custom arguments |
| `build.sh` | Builds the module |
| `check_go_version.sh` | Verifies the installed Go toolchain is new enough |
| `version.sh` | Versioning helper, run with `bump` to bump the version |
| `debug_lint.sh` | Verbose linting output for debugging lint failures |

## Required tools

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
go install golang.org/x/tools/cmd/goimports@latest
```

If the tools are not found, make sure `GOPATH/bin` is on your PATH:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

## Pre-commit Setup

Pre-commit hooks check code quality before every commit:

1. Install pre-commit:
   ```bash
   pip install pre-commit
   ```

2. Install the hooks:
   ```bash
   pre-commit install
   ```

3. Pre-commit now runs automatically on `git commit`
