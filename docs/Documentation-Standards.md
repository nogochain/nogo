# NogoChain Documentation Standards

> **Version**: 1.1.0
> **Last Updated**: 2026-04-26
> **Status**: ✅ Production Ready
> **Language**: English (Primary)
> **Scope**: All NogoChain Technical Documentation

This document defines the standard format and structure requirements for NogoChain project documentation, ensuring consistency, professionalism, and maintainability.

---

## Table of Contents

1. [Document Structure Standards](#document-structure-standards)
2. [Formatting Standards](#formatting-standards)
3. [Terminology Standards](#terminology-standards)
4. [Version Management](#version-management)
5. [Change History](#change-history)
6. [Code Reference Standards](#code-reference-standards)
7. [Example Code Standards](#example-code-standards)
8. [Document Templates](#document-templates)

---

## Document Structure Standards

### 1. Document Header

Every document must include standard header information:

```markdown
# Document Title

> **Version**: 1.0.0
> **Last Updated**: 2026-04-26
> **Applicable Version**: NogoChain v1.0.0+
> **Status**: ✅ Production Ready | 🚧 In Development | ⚠️ Deprecated
```

**Requirements**:
- Use Semantic Versioning for version numbers
- Date format: YYYY-MM-DD
- Use Emoji to indicate status

### 2. Introduction Section

Document should begin with a brief introduction:

```markdown
This document provides complete instructions for..., covering... All content is based on the latest code implementation to ensure accuracy and executability.

**Code References**:
- Module 1: [`File Path`](file:///path/to/file)
- Module 2: [`File Path`](file:///path/to/file)
```

### 3. Table of Contents

Documents with more than 5 sections must include a table of contents:

```markdown
## Table of Contents

1. [Section 1](#section-1)
2. [Section 2](#section-2)
3. [Section 3](#section-3)
```

### 4. Main Content

#### 4.1 Section Numbering

Use multi-level numbering:
```markdown
## 1. Level 1 Heading

### 1.1 Level 2 Heading

#### 1.1.1 Level 3 Heading
```

#### 4.2 Table Usage

Tables must have headers and alignment:

```markdown
| Column 1 | Column 2 | Column 3 |
|----------|----------|----------|
| Left     | Center   | Right    |
| Content  | Content  | Content  |
```

#### 4.3 Code Blocks

All code blocks must specify language:

````markdown
```bash
# Shell command
echo "Hello"
```

```go
// Go code
func main() {
    fmt.Println("Hello")
}
```

```json
{
  "key": "value"
}
```
````

### 5. Document Footer

Each document should include a standard footer:

```markdown
---

**Last Updated**: 2026-04-26
**Version**: 1.0.0
**Maintainer**: NogoChain Development Team
**Related Documents**:
- [Document 1](./doc1.md)
- [Document 2](./doc2.md)
```

---

## Formatting Standards

### 1. Heading Format

- Level 1: `# Title` (document title, only one)
- Level 2: `## 1. Title` (main sections)
- Level 3: `### 1.1 Title` (subsections)
- Level 4: `#### 1.1.1 Title` (details)

### 2. List Format

#### Ordered Lists
```markdown
1. Step 1
2. Step 2
3. Step 3
```

#### Unordered Lists
```markdown
- Item 1
- Item 2
- Item 3
```

#### Nested Lists
```markdown
1. Main item
   - Subitem 1
   - Subitem 2
2. Main item
   - Subitem 1
   - Subitem 2
```

### 3. Emphasis Format

```markdown
**Bold**: Important content
*Italic*: Emphasized content
***Bold Italic***: Very important
~~Strikethrough~~: Deprecated content
```

### 4. Quote Format

```markdown
> This is quoted content
> Can have multiple lines
```

### 5. Link Format

#### Internal Links
```markdown
[Section Name](#section-name)
```

#### External Links
```markdown
[Website Name](https://example.com)
```

#### Code References
```markdown
[`Filename`](file:///path/to/file)
[`Function Name`](file:///path/to/file#L10-L20)
```

### 6. Image Format

```markdown
![Description](./images/picture.png "Hover Title")
```

---

## Terminology Standards

### 1. Terminology Consistency

- Must explain on first occurrence
- Use consistently throughout the document
- Accurate English-Chinese correspondence

**Example**:
```markdown
Proof of Work (PoW) is a consensus mechanism...
```

### 2. Glossary Reference

Complex terms should reference the glossary:
```markdown
See [Glossary](./Glossary-EN.md#NogoPow)
```

### 3. Naming Conventions

- **Code elements**: Use `code format` (e.g., `Config`, `LoadConfig()`)
- **Filenames**: Use `code format` (e.g., `config.go`)
- **API endpoints**: Use `code format` (e.g., `/chain/info`)

---

## Version Management

### 1. Document Version Numbers

Follow Semantic Versioning:
- **Major**: Breaking changes (incompatible)
- **Minor**: New features (compatible)
- **Patch**: Small modifications (compatible)

**Format**: `Major.Minor.Patch` (e.g., `1.2.3`)

### 2. Version Records

Each document should record version history:

```markdown
## Changelog

| Version | Date       | Changes          | Author      |
|---------|------------|------------------|-------------|
| 1.0.0   | 2026-04-26 | Initial version  | Dev Team    |
| 1.1.0   | 2026-04-27 | Added API docs    | Dev Team    |
```

### 3. Compatibility Notes

Major changes should note compatibility:

```markdown
**Compatibility Warning**: v2.0.0 is incompatible with v1.x configs, manual migration required.
```

---

## Change History

### 1. Change Record Location

Documents should include changelog at the end:

```markdown
---

## Changelog

### v1.0.0 (2026-04-26)
- ✅ Initial version
- ✅ Completed core feature documentation
- ✅ Added code examples
```

### 2. Change Type Identifiers

Use Emoji to identify change types:
- ✅ New feature
- 🔧 Modification
- ❌ Deletion
- 📝 Documentation
- 🐛 Bug fix
- ⚡ Performance

### 3. Change Descriptions

Each change should be detailed:
```markdown
### v1.1.0 (2026-04-27)
- ✅ Added WebSocket API documentation
  - Added connection example
  - Added subscription example
  - Added error handling
- 🔧 Modified configuration documentation
  - Updated environment variable list
  - Corrected default values
```

---

## Code Reference Standards

### 1. Reference Format

Must use precise links:
```markdown
[`Filename`](file:///absolute/path)
[`Function Name`](file:///absolute/path#LStart-LEnd)
```

**Example**:
```markdown
Configuration loading function [`LoadConfig`](file:///d:/NogoChain/nogo/blockchain/config/config.go#L215-L217)
```

### 2. Reference Accuracy

- Links must be clickable
- Paths must be correct
- Line numbers must be precise

### 3. Reference Density

Important functions and structures must have references:
- Each API endpoint
- Each configuration item
- Each algorithm implementation

---

## Example Code Standards

### 1. Executability

All example code must be executable:
- Complete context
- Correct syntax
- Actual parameters

### 2. Comments

Example code should include comments:
```bash
# 1. Download Go 1.21.5
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz

# 2. Extract
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
```

### 3. Security Warnings

Security-related examples must be labeled:
```bash
# ⚠️ WARNING: This is an example token, use strong random token in production
export ADMIN_TOKEN="example_token"
```

---

## Document Templates

### 1. API Documentation Template

```markdown
# API Name

> **Version**: 1.0.0
> **Last Updated**: 2026-04-26
> **Status**: ✅ Stable | 🚧 Testing | ⚠️ Deprecated

## Endpoint

```
HTTP METHOD /path/to/endpoint
```

## Description

Brief description of API functionality.

## Request Parameters

| Parameter | Type   | Required | Description |
|-----------|--------|----------|-------------|
| param     | string | Yes      | Parameter description |

## Response Format

```json
{
  "key": "value"
}
```

## Error Codes

| Error Code | Description |
|------------|-------------|
| 400        | Bad request  |
| 401        | Unauthorized |

## Example

```bash
curl http://localhost:8080/endpoint
```

## Code Reference

[`handler function`](file:///path/to/file)

## Related APIs

- [Related API 1](./api1.md)
- [Related API 2](./api2.md)
```

### 2. Configuration Documentation Template

```markdown
# Configuration Item Name

> **Version**: 1.0.0
> **Last Updated**: 2026-04-26

## Description

Detailed description of configuration item.

## Environment Variables

| Variable   | Default | Required | Description |
|------------|---------|----------|-------------|
| VAR_NAME   | default | Yes      | Description |

## Configuration File Example

```yaml
key: value
```

## Code Reference

[`Configuration struct`](file:///path/to/file)
```

### 3. Algorithm Documentation Template

```markdown
# Algorithm Name

> **Version**: 1.0.0
> **Last Updated**: 2026-04-26

## Overview

Algorithm overview description.

## Mathematical Principles

$$ Formula $$

## Implementation Steps

1. Step 1
2. Step 2
3. Step 3

## Complexity Analysis

- Time complexity: O(n)
- Space complexity: O(n)

## Code Implementation

[`Implementation file`](file:///path/to/file)

## Test Cases

[`Test file`](file:///path/to/test)
```

---

## Quality Checklist

Check the following items before publishing documents:

- [ ] Document header information complete
- [ ] Clear table of contents structure
- [ ] Consistent terminology usage
- [ ] Accurate code references
- [ ] Executable example code
- [ ] All links valid
- [ ] Format meets standards
- [ ] Correct version information
- [ ] Recorded changelog
- [ ] Related document links

---

## Language Requirements

### Primary Language

**All documentation MUST be in English as the primary language**

- Document titles: English
- Section headings: English
- Body text: English
- Code comments: English
- Error messages: English

### Chinese Translation (Optional)

Chinese translations may be provided as supplementary material:

- Place in separate files with `_cn.md` suffix
- Translation files should reference the English original
- Maintain synchronization with English version

**Example File Structure**:
```
docs/
├── README.md                    # English (primary)
├── Algorithm-Manual.md          # English (primary)
├── FAQ-EN.md                    # English (primary)
├── FAQ-CN.md                    # Chinese translation
└── API/
    ├── API_Complete_Reference.md # English (primary)
    └── API 完整参考_cn.md         # Chinese translation
```

---

**Last Updated**: 2026-04-26
**Version**: 1.1.0
**Maintainer**: NogoChain Development Team

## Changelog

### v1.1.0 (2026-04-26)
- 🌐 Converted to English as primary language (was Chinese)
- 📝 Added "Language Requirements" section
- ✏️ Updated all examples to use English
- 📅 Updated date to 2026-04-26
- 🔧 Aligned with project requirement: "English primary, Chinese translation follows"

### v1.0.0 (2026-04-09)
- Initial version (Chinese)
