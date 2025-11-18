Review code changes for AzCopy. Use the AzCopy Code Reviewer agent context.

Perform a comprehensive code review focusing on:
- Security (credentials, path traversal, input validation)
- Error handling and retry logic
- Resource cleanup (files, connections, goroutines)
- Testing coverage
- Performance implications
- Platform compatibility
- Azure SDK usage

Provide:
1. Summary of changes
2. Issues found with file:line references
3. Recommendations for improvement
4. Verdict (Approve / Approve with comments / Request changes)

Ask user for:
- PR number, branch name, or commit range to review
