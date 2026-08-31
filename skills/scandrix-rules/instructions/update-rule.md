# Updating ScanDrix Rules

To update an existing rule via CLI:

```bash
scandrix rules update <rule-id> \
  --title "<New Title>" \
  --severity <LOW|MEDIUM|HIGH|CRITICAL> \
  --pattern "<UpdatedRegex>"
```
