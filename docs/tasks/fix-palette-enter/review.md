# Command palette Enter behavior

The code review found no blocking issues. Enter runs the selected local result, including the first commit returned by a delayed search. Explicit selection follows the result ID as the list changes. The input ignores composition keystrokes, and the Ask AI button owns the AI request.

Regression coverage lives in [CommandPalette.test.tsx](../../../src/components/CommandPalette.test.tsx). The tests exercise first-match selection, IME confirmation, arrow-key navigation across incoming results, Enter with an unmatched query, and explicit Ask AI activation.
