# NogoChain API Documentation Automation

This document explains how to automatically generate NogoChain API documentation.

## 📋 Table of Contents

1. [OpenAPI Specification](#openapi-specification)
2. [Generate HTML Documentation](#generate-html-documentation)
3. [Generate Markdown Documentation](#generate-markdown-documentation)
4. [Online Documentation Services](#online-documentation-services)
5. [Automation Workflow](#automation-workflow)

---

## OpenAPI Specification

NogoChain uses the **OpenAPI 3.0** specification to define all HTTP API endpoints.

**File Location**: `docs/api/openapi.yaml`

**Contents**:
- All API endpoint definitions
- Request/Response formats
- Authentication methods
- Error code descriptions

---

## Generate HTML Documentation

### Method 1: Using Swagger UI (Recommended)

**Run Locally**:

```bash
# Install Swagger UI
docker run -d \
  -p 8081:8080 \
  -e SWAGGER_JSON=/api/openapi.yaml \
  -v $(pwd)/docs/api:/api \
  swaggerapi/swagger-ui

# Access http://localhost:8081
```

**Deploy to Server**:

```bash
# Run on server
docker run -d \
  -p 80:8080 \
  -e SWAGGER_JSON=/api/openapi.yaml \
  -v /path/to/nogo/docs/api:/api \
  --name nogo-api-docs \
  swaggerapi/swagger-ui
```

Access: `http://your-server-ip/`

### Method 2: Using ReDoc

```bash
# Install ReDoc CLI
npm install -g @redocly/cli

# Generate static HTML
redocly build-docs docs/api/openapi.yaml --output=docs/api/index.html

# Use Python simple HTTP server to view
cd docs/api
python -m http.server 8081
# Access http://localhost:8081
```

---

## Generate Markdown Documentation

```bash
# Install widdershins
npm install -g widdershins

# Generate Markdown
widdershins docs/api/openapi.yaml -o docs/api/API-Auto-Generated.md

# View generated documentation
cat docs/api/API-Auto-Generated.md
```

---

## Online Documentation Services

### Option 1: SwaggerHub

1. Visit https://app.swaggerhub.com
2. Create an account
3. Upload `openapi.yaml`
4. Get sharing link

### Option 2: ReadMe.com

1. Visit https://readme.com
2. Create a project
3. Import OpenAPI specification
4. Customize documentation style

### Option 3: Self-Hosted (Recommended)

Deploy complete documentation service using Docker Compose:

```yaml
# docker-compose.docs.yml
version: '3.8'

services:
  swagger-ui:
    image: swaggerapi/swagger-ui
    ports:
      - "8081:8080"
    volumes:
      - ./docs/api:/api
    environment:
      - SWAGGER_JSON=/api/openapi.yaml
    restart: unless-stopped

  redoc:
    image: redocly/redoc
    ports:
      - "8082:80"
    volumes:
      - ./docs/api:/usr/share/nginx/html/api
    environment:
      - SPEC_URL=/api/openapi.yaml
    restart: unless-stopped
```

Run:
```bash
docker-compose -f docker-compose.docs.yml up -d

# Access:
# - Swagger UI: http://localhost:8081
# - ReDoc: http://localhost:8082
```

---

## Automation Workflow

### CI/CD Integration

Automatically generate documentation in GitHub Actions:

```yaml
# .github/workflows/api-docs.yml
name: Generate API Docs

on:
  push:
    branches: [main]
    paths:
      - 'docs/api/openapi.yaml'

jobs:
  generate-docs:
    runs-on: ubuntu-latest
    
    steps:
      - uses: actions/checkout@v3
      
      - name: Generate Markdown Docs
        run: |
          npm install -g widdershins
          widdershins docs/api/openapi.yaml -o docs/api/API-Auto-Generated.md
      
      - name: Deploy to GitHub Pages
        uses: peaceiris/actions-gh-pages@v3
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          publish_dir: ./docs/api
```

### Documentation Update Check

```bash
#!/bin/bash
# scripts/check-api-docs.sh

# Check if OpenAPI specification is valid
npm install -g @redocly/cli
redocly lint docs/api/openapi.yaml

# Exit if check fails
if [ $? -ne 0 ]; then
    echo "❌ OpenAPI specification validation failed"
    exit 1
fi

echo "✅ OpenAPI specification validation passed"
```

---

## Best Practices

### 1. Keep Synchronized

Update `openapi.yaml` whenever API code is modified:

```bash
# Add pre-commit hook
cat > .git/hooks/pre-commit << 'EOF'
#!/bin/bash
if git diff --cached --name-only | grep -q 'blockchain/http.go'; then
    echo "⚠️  HTTP route change detected, please update docs/api/openapi.yaml"
    exit 1
fi
EOF

chmod +x .git/hooks/pre-commit
```

### 2. Version Control

Use semantic versioning in `openapi.yaml`:

```yaml
info:
  version: 1.1.0  # MAJOR.MINOR.PATCH
```

### 3. Test Documentation

Test API examples using Postman or Insomnia:

```bash
# Export Postman Collection
# Use openapi-to-postman tool
npm install -g openapi-to-postmanv2
openapi-to-postmanv2 --input docs/api/openapi.yaml --output docs/api/postman-collection.json
```

---

## Troubleshooting

### Issue 1: Swagger UI Cannot Load

**Solution**:
```bash
# Check YAML syntax
python -c "import yaml; yaml.safe_load(open('docs/api/openapi.yaml'))"

# Use online validator
# https://editor.swagger.io/
```

### Issue 2: Documentation Inconsistent with Code

**Solution**:
1. Run automated tests to verify API
2. Manually test each endpoint
3. Update `openapi.yaml`
4. Regenerate documentation

---

## References

- **OpenAPI Specification**: https://swagger.io/specification/
- **Swagger UI**: https://swagger.io/tools/swagger-ui/
- **ReDoc**: https://redocly.com/
- **OpenAPI Tools**: https://openapi.tools/

---

**Last Updated**: 2026-04-01  
**Maintainer**: NogoChain Development Team
