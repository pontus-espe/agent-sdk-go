# Go Code Quality Guidelines

This document outlines the guidelines that keep the Agent SDK Go code base
consistent and maintainable.

## Quality Checks

The `Code Quality` workflow runs golangci-lint (errcheck, govet, staticcheck),
gosec and the test suite with coverage on every push and pull request. The `CI`
workflow additionally enforces formatting and runs `go vet` and the build.

The checks cover:

### 1. Code Formatting (`gofmt`)

All code must be formatted according to Go standards using `gofmt`.

- Run `gofmt -s -w .` before committing code
- Configure your editor to run `gofmt` on save
- The CI workflow will auto-format code that doesn't meet standards

### 2. Code Style (`staticcheck`)

Follow Go style guidelines:

- Package names should be lowercase, single-word identifiers
- Use camelCase for variable names, PascalCase for exported names
- Include documentation comments for all exported items
- Use meaningful, clear, and concise names

### 3. Code Correctness (`go vet`)

`go vet` examines code for potential bugs:

- Ensure proper error handling
- Avoid unreachable code
- Correct use of format verbs in printf-like functions
- Proper struct tag usage

### 4. Ineffectual Assignments (`ineffassign`)

Avoid ineffectual assignments where a variable is assigned a value that is never used:

```go
// Bad
x := 5
x = 10 // Previous assignment to x is ineffectual
```

### 5. Spelling (`misspell`)

Maintain correct spelling in comments and strings:

- Use American English spelling
- Run `misspell -w .` to auto-fix common spelling errors

### 6. Cyclomatic Complexity (`gocyclo`)

Keep functions simple and maintainable:

- Break complex functions into smaller, focused functions
- Aim for cyclomatic complexity under 15
- Use early returns to avoid deep nesting

### 7. License Headers

Ensure files include the appropriate license header.

## Maintaining Quality

We use several tools to maintain code quality:

1. **Local Development**:
   - Run `gofmt -s -w .` and `go vet ./...` before committing
   - `./scripts/check_all.sh` runs the same checks the CI pipeline runs
   - Configure editor integrations with these tools

2. **CI Pipeline**:
   - The `ci.yml` workflow checks formatting, vet, build and tests
   - The `code-quality.yml` workflow runs golangci-lint, gosec and coverage

3. **Pre-commit Hooks** (recommended):
   - Install [pre-commit](https://pre-commit.com/)
   - Use the provided `.pre-commit-config.yaml` file

## Common Issues and Fixes

### Documentation

All exported functions, methods, and types should be documented:

```go
// Bad
func ExportedFunction() {}

// Good
// ExportedFunction does something useful and returns nothing.
func ExportedFunction() {}
```

### Error Handling

Always check error returns:

```go
// Bad
fd, _ := os.Open("file.txt")

// Good
fd, err := os.Open("file.txt")
if err != nil {
    return fmt.Errorf("opening file: %w", err)
}
```

### Return Values

Be consistent with named returns:

```go
// Either use named returns consistently:
func divide(a, b int) (result int, err error) {
    if b == 0 {
        err = errors.New("division by zero")
        return
    }
    result = a / b
    return
}

// Or don't use them at all:
func divide(a, b int) (int, error) {
    if b == 0 {
        return 0, errors.New("division by zero")
    }
    return a / b, nil
}
```

## Resources

- [Effective Go](https://golang.org/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [golangci-lint](https://golangci-lint.run/)
- [gosec](https://github.com/securego/gosec)
- [Go Proverbs](https://go-proverbs.github.io/) 