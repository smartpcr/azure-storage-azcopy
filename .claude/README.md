# Claude Code Configuration for AzCopy

This directory contains Claude Code configuration, custom agents, slash commands, and persistent memory/context for the AzCopy project.

## Structure

```
.claude/
├── README.md (this file)
├── settings.local.json          # Claude Code permissions and settings
├── agents/                      # Custom AI agents
│   ├── azcopy-test-expert.md
│   ├── azcopy-transfer-architect.md
│   └── azcopy-code-reviewer.md
├── commands/                    # Custom slash commands
│   ├── test.md
│   ├── build.md
│   ├── debug-transfer.md
│   ├── review-pr.md
│   ├── explain-arch.md
│   └── capture-context.md
├── context/                     # Persistent context across sessions
│   ├── README.md
│   ├── architecture-decisions.md
│   ├── known-issues.md
│   ├── optimization-opportunities.md
│   └── development-notes.md
└── memory/                      # Session memory and historical context
    ├── README.md
    ├── index.md
    └── YYYY-MM-DD/             # Dated memory entries
```

## Custom Agents

Specialized AI agents with deep AzCopy knowledge:

### AzCopy Test Expert
Expert in writing and debugging tests for AzCopy. Use for:
- Creating unit and E2E tests
- Debugging test failures
- Analyzing test coverage
- Understanding test frameworks

### AzCopy Transfer Architect
Expert in transfer architecture and Storage Transfer Engine. Use for:
- Understanding transfer flow
- Debugging transfer issues
- Performance optimization
- Implementing transfer features

### AzCopy Code Reviewer
Senior code reviewer for AzCopy. Use for:
- Code review with security focus
- Go best practices
- Platform compatibility checks
- Azure SDK usage validation

## Custom Slash Commands

Quick commands for common tasks:

### `/test`
Run AzCopy tests (unit, e2e, specific patterns, coverage)

### `/build`
Build AzCopy binary (standard, coverage, static, platform-specific)

### `/debug-transfer`
Debug transfer issues with expert analysis

### `/review-pr`
Comprehensive code review of changes

### `/explain-arch`
Explain AzCopy architecture and components

### `/capture-context`
Save current conversation context to memory

## Context Files

Persistent knowledge maintained across sessions:

- **architecture-decisions.md** - Key architectural choices and rationale
- **known-issues.md** - Current bugs, limitations, workarounds
- **optimization-opportunities.md** - Performance and quality improvements
- **development-notes.md** - Developer tips and best practices

## Memory System

Historical context from Claude Code sessions:

- Organized by date (YYYY-MM-DD)
- Categorized (feature, bugfix, investigation, refactor)
- Indexed for searchability
- Linked to related issues/PRs

Use `/capture-context` to save important discussions and decisions.

## Usage Examples

### Running tests
```
/test unit
/test e2e
/test specific TestHTTPDownload
/test coverage
```

### Building
```
/build standard
/build coverage
/build platform darwin_arm64
```

### Getting help
```
/debug-transfer
/explain-arch transfer-flow
/review-pr
```

### Capturing context
```
/capture-context
```

## Permissions

The `settings.local.json` file grants Claude Code permissions to:
- Run Go build and test commands
- Execute AzCopy binaries for testing
- Use git commands
- Access temporary directories

Permissions are scoped to safe, common development operations.

## Maintenance

### Agents
Update agents when:
- New patterns emerge
- Architecture changes
- Best practices evolve

### Commands
Add new commands for:
- Frequent tasks
- Complex workflows
- Team standards

### Context
Update context files when:
- Architectural decisions are made
- Issues are discovered or resolved
- Optimizations are identified
- New patterns are learned

### Memory
Use `/capture-context` to save:
- Important discussions
- Design decisions
- Investigation findings
- Feature implementations

## Benefits

1. **Consistency**: Agents maintain consistent knowledge across sessions
2. **Efficiency**: Commands automate common tasks
3. **Context**: Persistent memory preserves decisions and learnings
4. **Onboarding**: New contributors get instant access to project knowledge
5. **Quality**: Expert review and testing patterns built-in

## Getting Started

1. **Use agents**: Reference agents in your prompts for specialized help
2. **Try commands**: Use `/` to see available commands, tab for autocomplete
3. **Read context**: Review context files to understand the project
4. **Capture memory**: Use `/capture-context` for important discussions

---

*This configuration enables Claude Code to provide expert-level assistance for AzCopy development*
