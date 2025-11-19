# AzCopy Context Files

This directory contains persistent context about the AzCopy codebase that should be available to Claude Code across all sessions.

## Files

### Core Documentation
- **architecture-decisions.md** - Key architectural choices and their rationale
- **known-issues.md** - Current bugs, limitations, and workarounds
- **optimization-opportunities.md** - Performance and code quality improvements identified
- **development-notes.md** - Developer tips and gotchas

### Component Deep Dives
- **ste.md** - Complete Storage Transfer Engine (STE) documentation
  - 108 Go files analyzed
  - Job management architecture
  - Transfer abstractions (downloaders, uploaders, senders)
  - Source info providers
  - Performance & concurrency systems
  - Retry & error handling
  - Complete file reference guide

## Purpose

These files provide:
1. Historical context for design decisions
2. Quick reference for common issues
3. Onboarding information for new contributors
4. Memory across Claude Code sessions

## Maintenance

Update these files as you:
- Make architectural decisions
- Discover bugs or limitations
- Identify optimization opportunities
- Learn new patterns or best practices
- Add significant test coverage
