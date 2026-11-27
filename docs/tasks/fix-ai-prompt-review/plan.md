# AI prompt review fixes

Keep the AI request behavior and contact redaction defined in [F019](../../pm/features/F019-ai-redaction-preview.md).

- Clear the active explanation when the command palette opens. Verify that Ask AI → Run → Ctrl/Cmd+K → Escape results in one explanation request.
- Narrow the Git remote exception. Verify that colon-delimited contacts are masked while SCP-style Git remotes remain readable.
- Match an international phone followed by a complete parenthesized North American number before matching a standalone international phone. Verify separate placeholders and preserve existing international formats with parenthesized groups.
