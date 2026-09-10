# Contributing to Agent SDK Go

Thank you for your interest in contributing to Agent SDK Go! This document provides guidelines and instructions for contributing to this project.

## Code of Conduct

Please be respectful and considerate of others when contributing to this project. We aim to foster an inclusive and welcoming community.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR-USERNAME/agent-sdk-go.git`
3. Build the project with `go build ./...` and run the checks with `./scripts/check_all.sh`
4. Create a new branch for your changes: `git checkout -b feature/your-feature-name`

## Development Workflow

1. Make your changes
2. Run the checks to ensure your changes meet the project standards:
   ```bash
   ./scripts/check_all.sh
   ```
3. Commit your changes with a descriptive commit message
4. Push your changes to your fork
5. Submit a pull request

## Pull Request Process

1. Ensure your code passes all checks (linting, security, tests)
2. Update documentation if necessary
3. Include a clear description of the changes in your pull request
4. Link any related issues in your pull request description

## Coding Standards

- Follow Go best practices and idiomatic Go
- Use meaningful variable and function names
- Write clear comments and documentation
- Include tests for new functionality
- Keep functions and methods focused and small

## Testing

All new features and bug fixes should include tests. Run the tests with:

```bash
cd test && make test
```

## Documentation

Update documentation to reflect any changes you make. This includes:

- Code comments
- README.md updates
- Examples if applicable

## Versioning

We use semantic versioning. The version is managed in the `version.txt` file and can be bumped using:

```bash
./scripts/version.sh bump
```

## License

By contributing to this project, you agree that your contributions will be licensed under the project's [MIT License](LICENSE).

## Questions?

If you have any questions or need help, please open an issue or reach out to the maintainers. 