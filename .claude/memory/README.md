# AzCopy Memory / Context System

This directory stores captured context from Claude Code sessions to maintain continuity across conversations.

## Structure

```
memory/
├── README.md (this file)
├── index.md (searchable index of all memory entries)
├── YYYY-MM-DD/
│   ├── feature-{name}.md
│   ├── bugfix-{name}.md
│   ├── investigation-{name}.md
│   └── refactor-{name}.md
└── context/
    ├── architecture-decisions.md
    ├── known-issues.md
    └── optimization-opportunities.md
```

## Memory Entry Format

Each memory entry should contain:

```markdown
# [Category] Title

**Date**: YYYY-MM-DD
**Component**: cmd|ste|common|e2etest|traverser
**Status**: active|resolved|blocked|completed

## Summary
Brief description of the topic/issue/feature

## Context
Detailed context and background

## Key Decisions
1. Decision with rationale
2. Another decision

## Code References
- `file/path.go:123` - Description
- `another/file.go:456` - Description

## Action Items
- [ ] Task 1
- [ ] Task 2
- [x] Completed task

## Related
- Issue #123
- PR #456
- Related memory: YYYY-MM-DD/feature-xyz.md

## Outcome
Final resolution or current state
```

## Usage

Use `/capture-context` command to automatically save current conversation context.

Or manually create entries for important discussions, decisions, or investigations.

Update `index.md` when adding new entries for searchability.
