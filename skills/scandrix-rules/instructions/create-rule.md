# Creating ScanDrix Rules

To create a new rule via CLI:

```bash
scandrix rules create "<Title>" \
  --pattern "<RegexPattern>" \
  --severity <LOW|MEDIUM|HIGH|CRITICAL> \
  --category <security|quality|architecture|performance> \
  --path "<GlobPath>"
```

### Examples:

1. **Secret Scanning**:
```bash
scandrix rules create "Prohibit Hardcoded JWT Secrets" \
  --pattern "jwt\.sign\([^,]+,\s*['\"][a-zA-Z0-9_\-]{8,}['\"]" \
  --severity CRITICAL \
  --category security
```

2. **Console Log Guard**:
```bash
scandrix rules create "Avoid console.log in Production Backend" \
  --pattern "console\.(log|debug|info)\(" \
  --severity MEDIUM \
  --category quality \
  --path "internal/**/*.go"
```
