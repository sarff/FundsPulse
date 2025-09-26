# Contributing to FundsPulse

Thank you for your interest in contributing to FundsPulse! This document provides guidelines for contributing to the project.

## Getting Started

1. Fork the repository
2. Clone your fork locally
3. Create a new branch for your feature/fix
4. Make your changes
5. Test your changes
6. Submit a pull request

## Development Setup

1. Ensure you have Go 1.24+ installed
2. Clone the repository:
   ```bash
   git clone https://github.com/sarff/multiapibalancechecker.git
   cd multiapibalancechecker
   ```
3. Copy configuration files:
   ```bash
   cp .env.example .env
   cp config.example.yaml config.yaml
   ```
4. Fill in your API credentials in `.env`
5. Build and run:
   ```bash
   go build ./...
   go run ./cmd/balance-checker -config config.yaml -run-once
   ```

## Code Style

- Follow standard Go formatting (`gofmt -w .`)
- Use meaningful variable and function names
- Add comments for exported functions and complex logic
- Keep functions focused and small
- Use structured logging via `iSlogger`

## Testing

- Write unit tests for new functionality
- Ensure all tests pass: `go test ./...`
- Test with real API responses when possible (use `-run-once` flag)
- Mock external API calls in unit tests

## Commit Guidelines

- Use clear, descriptive commit messages
- Start with a verb in present tense (e.g., "Add", "Fix", "Update")
- Keep the first line under 72 characters
- Include context in the body if needed

Examples:
- `Add support for new payment provider`
- `Fix balance calculation for multiple currencies`
- `Update Telegram notification format`

## Pull Request Process

1. Ensure your code follows the style guidelines
2. Add tests for new functionality
3. Update documentation if needed
4. Ensure all tests pass
5. Fill out the pull request template with:
   - Description of changes
   - Testing performed
   - Any breaking changes

## Adding New Payment Providers

When adding support for a new payment provider:

1. Add example configuration to `presets_services/`
2. Update `config.example.yaml` with the new service
3. Update `.env.example` with required environment variables
4. Test with real API credentials
5. Document any special requirements in the PR

## Reporting Issues

When reporting issues:

- Use a clear, descriptive title
- Provide steps to reproduce
- Include relevant configuration (without sensitive data)
- Add logs or error messages
- Specify Go version and OS

## Security

- Never commit real API keys, tokens, or credentials
- Use environment variables for sensitive data
- Report security vulnerabilities privately to the maintainers

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

## Questions?

Feel free to open an issue for questions about contributing or reach out to the maintainers.