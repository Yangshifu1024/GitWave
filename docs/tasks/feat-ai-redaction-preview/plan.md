# AI prompt redaction and local preview

**Proposal** · [F019](../../pm/features/F019-ai-redaction-preview.md)

## Request flow

1. Assemble the system and user messages from the selected task, repository, Workspace rules, and language. Keep the existing private-key screening of complete Git blobs before hunk selection.
2. Scrub sensitive lines first. Count those lines once, then replace individual email and phone matches in the remaining text. Aggregate both message roles' counts for display.
3. Let each of the seven AI entry points prepare a preview locally or generate directly. Hold the sanitized preview in memory, scoped to the originating Workspace, repository, task, and object.
4. On confirmation, send the prepared messages through the existing provider chain. Preserve their text across attempts. Closing a preview or changing its scope ends the preview. Keep the existing offline policy and result handling.

## Verification

| Scenario | Expected behavior |
|---|---|
| Existing credential and private-key fixtures | Secret lines remain masked and full-blob prechecks continue to exclude key material |
| Email with a plus tag, international, mainland Chinese and formatted North American phone numbers | The surrounding text survives and the counts match replacements across both messages |
| Git SSH remote, SHA, timestamp and version text | Remain readable |
| Open and dismiss a preview | The local test provider receives zero requests |
| Send from preview after the source changes | The provider receives the system and user text shown in the preview |
| Primary provider fails, backup succeeds | Both providers receive identical sanitized message text |
| Generate directly | One user action starts generation with the same redaction rules |
| Switch Workspace, repo or selected object | The previous preview cannot be submitted for the new selection |
