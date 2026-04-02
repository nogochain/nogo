# NogoChain API 文档自动化

本文档说明如何自动生成 NogoChain 的 API 文档。

## 📋 目录

1. [OpenAPI 规范](#openapi-规范)
2. [生成 HTML 文档](#生成-html-文档)
3. [生成 Markdown 文档](#生成-markdown-文档)
4. [在线文档服务](#在线文档服务)
5. [自动化流程](#自动化流程)

---

## OpenAPI 规范

NogoChain 使用 **OpenAPI 3.0** 规范定义所有 HTTP API 接口。

**文件位置**: `docs/api/openapi.yaml`

**包含内容**:
- 所有 API 端点定义
- 请求/响应格式
- 认证方式
- 错误码说明

---

## 生成 HTML 文档

### 方法 1: 使用 Swagger UI（推荐）

**本地运行**:

```bash
# 安装 Swagger UI
docker run -d \
  -p 8081:8080 \
  -e SWAGGER_JSON=/api/openapi.yaml \
  -v $(pwd)/docs/api:/api \
  swaggerapi/swagger-ui

# 访问 http://localhost:8081
```

**部署到服务器**:

```bash
# 在服务器上运行
docker run -d \
  -p 80:8080 \
  -e SWAGGER_JSON=/api/openapi.yaml \
  -v /path/to/nogo/docs/api:/api \
  --name nogo-api-docs \
  swaggerapi/swagger-ui
```

访问：`http://your-server-ip/`

### 方法 2: 使用 Redoc

```bash
# 安装 Redoc CLI
npm install -g @redocly/cli

# 生成静态 HTML
redocly build-docs docs/api/openapi.yaml --output=docs/api/index.html

# 使用 Python 简单 HTTP 服务器查看
cd docs/api
python -m http.server 8081
# 访问 http://localhost:8081
```

---

## 生成 Markdown 文档

```bash
# 安装 widdershins
npm install -g widdershins

# 生成 Markdown
widdershins docs/api/openapi.yaml -o docs/api/API-Auto-Generated.md

# 查看生成的文档
cat docs/api/API-Auto-Generated.md
```

---

## 在线文档服务

### 方案 1: SwaggerHub

1. 访问 https://app.swaggerhub.com
2. 创建账户
3. 上传 `openapi.yaml`
4. 获取分享链接

### 方案 2: ReadMe.com

1. 访问 https://readme.com
2. 创建项目
3. 导入 OpenAPI 规范
4. 自定义文档样式

### 方案 3: 自托管（推荐）

使用 Docker Compose 部署完整的文档服务：

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

运行：
```bash
docker-compose -f docker-compose.docs.yml up -d

# 访问：
# - Swagger UI: http://localhost:8081
# - ReDoc: http://localhost:8082
```

---

## 自动化流程

### CI/CD 集成

在 GitHub Actions 中自动生成文档：

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

### 文档更新检查

```bash
#!/bin/bash
# scripts/check-api-docs.sh

# 检查 OpenAPI 规范是否有效
npm install -g @redocly/cli
redocly lint docs/api/openapi.yaml

# 如果检查失败，退出
if [ $? -ne 0 ]; then
    echo "❌ OpenAPI 规范验证失败"
    exit 1
fi

echo "✅ OpenAPI 规范验证通过"
```

---

## 最佳实践

### 1. 保持同步

每次修改 API 代码时，同步更新 `openapi.yaml`：

```bash
# 添加 pre-commit hook
cat > .git/hooks/pre-commit << 'EOF'
#!/bin/bash
if git diff --cached --name-only | grep -q 'blockchain/http.go'; then
    echo "⚠️  检测到 HTTP 路由变更，请更新 docs/api/openapi.yaml"
    exit 1
fi
EOF

chmod +x .git/hooks/pre-commit
```

### 2. 版本控制

在 `openapi.yaml` 中使用语义化版本：

```yaml
info:
  version: 1.1.0  # MAJOR.MINOR.PATCH
```

### 3. 测试文档

使用 Postman 或 Insomnia 测试 API 文档中的示例：

```bash
# 导出 Postman Collection
# 使用 openapi-to-postman 工具
npm install -g openapi-to-postmanv2
openapi-to-postmanv2 --input docs/api/openapi.yaml --output docs/api/postman-collection.json
```

---

## 故障排查

### 问题 1: Swagger UI 无法加载

**解决**:
```bash
# 检查 YAML 语法
python -c "import yaml; yaml.safe_load(open('docs/api/openapi.yaml'))"

# 使用在线验证器
# https://editor.swagger.io/
```

### 问题 2: 文档与代码不一致

**解决**:
1. 运行自动化测试验证 API
2. 手动测试每个端点
3. 更新 `openapi.yaml`
4. 重新生成文档

---

## 参考资源

- **OpenAPI 规范**: https://swagger.io/specification/
- **Swagger UI**: https://swagger.io/tools/swagger-ui/
- **ReDoc**: https://redocly.com/
- **OpenAPI 工具**: https://openapi.tools/

---

**最后更新**: 2026-04-01  
**维护者**: NogoChain 开发团队
