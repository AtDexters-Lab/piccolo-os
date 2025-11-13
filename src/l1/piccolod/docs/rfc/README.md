# Piccolo RFCs

This directory houses request-for-comment documents that describe notable backend or runtime changes. Each RFC should:

1. Live in its own Markdown file named `YYYYMMDD-short-title.md`.
2. Capture the problem statement, proposed solution, affected modules/APIs, and roll-out considerations.
3. Include a final section titled `Implementation Notes & Status` summarizing what landed (commit/PR references, follow-ups, or reasons if the idea was abandoned).

Workflow:
- Open an RFC file in this folder when there is a non-trivial change (new API, security model updates, etc.).
- Reference the RFC from relevant PR descriptions.
- Update the `Implementation Notes & Status` section once the work is done so future readers know the outcome.

This keeps design history close to the code and gives the UI/backend teams a single place to track decisions and resulting actions.
