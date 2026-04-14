# NogoChain Contribution Guidelines

## Development Workflow

### 1. Fork and Clone
```bash
git clone https://github.com/YOUR_USERNAME/nogo.git
cd nogo
git remote add upstream https://github.com/nogochain/nogo.git
```

### 2. Create Branch
```bash
git checkout -b feature/your-feature-name
```

### 3. Make Changes
- Follow Go coding standards
- Run tests locally: `make test`
- Run linter: `make lint`
- Run security scan: `make vuln`

### 4. Commit Changes
```bash
git add .
git commit -m "feat: description of your changes"
```

### 5. Push and Create PR
```bash
git push origin feature/your-feature-name
```

## Code Standards

### Go Version
- Minimum Go version: 1.25

### Code Style
- Run `go fmt ./...` before committing
- Follow [Effective Go](https://golang.org/doc/effective_go)

### Testing
- All new code must have tests
- Minimum test coverage: 80%
- Run: `make test`

### Security
- No hardcoded secrets
- Use `crypto/rand` for random numbers
- Validate all user input

## CI/CD Pipeline

All pull requests must pass:
1. **Lint** - golangci-lint
2. **Vet** - go vet
3. **Test** - go test with race detector
4. **Security** - gosec and trivy scans

## Branch Naming

- `feature/` - New features
- `fix/` - Bug fixes
- `docs/` - Documentation
- `refactor/` - Code refactoring
- `test/` - Test improvements

## Commit Message Format

```
<type>: <subject>

<body>

<footer>
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `style`: Formatting
- `refactor`: Code refactoring
- `test`: Tests
- `chore`: Maintenance

## License

By contributing, you agree that your contributions will be licensed under the GNU Lesser General Public License v3.0.
