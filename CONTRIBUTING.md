# Contributing

Thanks for your interest in contributing to `cli`.

## Setup

```bash
git clone https://github.com/bjaus/cli.git
cd cli
```

Requires Go 1.24+ and [golangci-lint](https://golangci-lint.run/welcome/install/).

## Development

```bash
make check   # lint + test (the full CI check)
make lint    # linters only
make test    # tests with race detector + coverage report
make cover   # coverage summary
```

## Guidelines

- Every exported symbol needs a doc comment.
- Tests go in `_test.go` files using [testify](https://github.com/stretchr/testify). Use `testpackage` (package `cli_test`) for public API tests, `internal_test.go` (package `cli`) for internals.
- All tests must call `t.Parallel()`.
- Maintain 100% statement coverage. Run `make test` to verify.
- Linter must pass with zero issues. Run `make lint` to verify.
- Keep the interface-per-file philosophy: small interfaces, each doing one thing.

## Pull Requests

1. Fork the repo and create a branch from `main`.
2. Add tests for any new functionality.
3. Run `make check` and ensure it passes.
4. Open a pull request with a clear description of the change.
