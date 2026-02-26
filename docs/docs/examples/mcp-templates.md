---
sidebar_position: 2
---

# SLOP Script Patterns

SLOP scripts let you orchestrate multiple MCPs in a single `run_slop` call — no agent round-trips for intermediate results. This page shows practical patterns you can adapt for your own workflows.

See the [SLOP Language Reference](/docs/reference/slop-language) for full syntax.

## Pattern A: Codebase Intelligence → Infographic

Analyze your codebase with LCI, persist the results, and generate an architecture chart.

```python
# Scan all Go structs and interfaces across the codebase
symbols = lci.search(query: "type struct interface", language: "go")

# Group by package
by_package = group_by(symbols, |s| s["package"])

# Build a summary per package
summary = []
for pkg in keys(by_package) {
    items = by_package[pkg]
    structs = filter(items, |s| contains(s["kind"], "struct"))
    interfaces = filter(items, |s| contains(s["kind"], "interface"))
    summary = append(summary, {
        "package": pkg,
        "structs": len(structs),
        "interfaces": len(interfaces)
    })
}

# Persist for future sessions
mem_save("codebase", "architecture", summary,
    description: "Package-level struct/interface counts")

# Generate architecture chart
labels = map(summary, |s| s["package"])
struct_counts = map(summary, |s| s["structs"])

banana.generate_chart(
    type: "bar",
    title: "Codebase Architecture",
    labels: labels,
    data: struct_counts
)

emit(packages: len(summary), total_symbols: len(symbols))
```

**What this demonstrates:** Pipes + `group_by` for data shaping, `mem_save` for persistence across sessions, and MCP tool calls for visualization — all in one script with zero agent round-trips.

## Pattern B: Design System Sync

Extract a Figma component library, write design tokens to your repo, open a PR, and notify the team.

```python
# Extract components from Figma
file = figma.get_file(file_key: "abc123XYZ")
components = file["document"]["children"]

# Transform into design tokens
colors = components
    | filter(|c| startswith(c["name"], "Colors/"))
    | map(|c| {
        "name": replace(c["name"], "Colors/", ""),
        "hex": c["fills"][0]["color"]
    })

typography = components
    | filter(|c| startswith(c["name"], "Typography/"))
    | map(|c| {
        "name": replace(c["name"], "Typography/", ""),
        "size": c["style"]["fontSize"],
        "family": c["style"]["fontFamily"]
    })

tokens = {"colors": colors, "typography": typography}

# Write tokens to project
filesystem.write_file(
    path: "src/design-tokens.json",
    content: json_stringify(tokens)
)

# Create a PR
pr = github.create_pull_request(
    repo: "org/frontend",
    title: "Sync design tokens from Figma",
    head: "design-token-sync",
    body: format("Updated {} colors, {} typography tokens",
        len(colors), len(typography))
)

# Notify the design channel
slack.post_message(
    channel: "#design",
    text: format("Design tokens synced from Figma → PR: {}", pr["url"])
)

emit(colors: len(colors), typography: len(typography), pr: pr["url"])
```

**What this demonstrates:** End-to-end design handoff — Figma extraction, data transformation with pipes, filesystem writes, GitHub PR creation, and Slack notification in a single script.

## Pattern C: Sprint Digest Report

Aggregate data from GitHub and Linear, generate charts, and email a weekly digest.

```python
# Fetch merged PRs this week from GitHub
prs = github.list_pull_requests(
    repo: "org/app",
    state: "closed",
    sort: "updated",
    direction: "desc"
)
merged = prs | filter(|p| p["merged_at"] != none)

# Fetch sprint data from Linear
sprint = linear.get_active_cycle(team: "engineering")
completed = sprint["issues"]
    | filter(|i| i["state"]["name"] == "Done")

# Build summary
pr_authors = merged
    | map(|p| p["user"]["login"])
    | unique()

summary = {
    "prs_merged": len(merged),
    "contributors": pr_authors,
    "stories_completed": len(completed),
    "velocity": sprint["completedScopeHistory"]
}

# Generate velocity chart
banana.generate_chart(
    type: "line",
    title: "Sprint Velocity",
    labels: map(summary["velocity"], |v| v["day"]),
    data: map(summary["velocity"], |v| v["scope"])
)

# Email the digest
email.send(
    to: "team@company.com",
    subject: format("Weekly Digest: {} PRs, {} stories", len(merged), len(completed)),
    body: format(
        "PRs merged: {}\nContributors: {}\nStories completed: {}\nSee #releases for charts.",
        len(merged), join(pr_authors, ", "), len(completed)
    )
)

emit(prs: len(merged), stories: len(completed))
```

**What this demonstrates:** Multi-source data aggregation (GitHub + Linear), functional transforms to extract insights, chart generation, and email delivery — replacing a manual weekly process.

## Pattern D: Batch Operations with Checkpointing

Process items in bulk with progress checkpointing and error collection.

```python
# Load previous progress (if resuming)
checkpoint = mem_load("batch", "repo-labels")
processed = []
errors = []
start = 0

if checkpoint != none {
    processed = checkpoint["processed"]
    errors = checkpoint["errors"]
    start = len(processed)
}

# Repos to process
repos = ["org/api", "org/frontend", "org/docs", "org/infra", "org/mobile"]
labels = ["priority:high", "priority:medium", "priority:low"]

for i in range(start, len(repos)) {
    repo = repos[i]

    for label in labels {
        result = github.create_label(
            repo: repo,
            name: label,
            color: "ededed"
        )

        if has_key(result, "error") {
            errors = append(errors, {"repo": repo, "label": label, "error": result["error"]})
        }
    }

    processed = append(processed, repo)

    # Checkpoint after each repo
    mem_save("batch", "repo-labels", {
        "processed": processed,
        "errors": errors
    }, description: "Label sync progress")
}

emit(
    processed: len(processed),
    errors: len(errors),
    failed: errors
)
```

**What this demonstrates:** Resilient batch processing — `mem_save` checkpoints progress so a failed script can resume where it left off, errors are collected without stopping the loop, and the final `emit` gives a clean success/failure summary.

## When to Use SLOP Scripts vs execute_tool

| Scenario | Use |
|----------|-----|
| One-off tool call | `execute_tool` — simpler, no script overhead |
| 2-3 sequential calls | Either works — agent can chain `execute_tool` calls |
| Data transformation between calls | `run_slop` — pipes and transforms avoid round-trips |
| Loops over items | `run_slop` — batch processing stays out of agent context |
| Resumable/checkpointed work | `run_slop` — `mem_save`/`mem_load` for persistence |
| Reusable workflow | `run_slop` — save as a `.slop` file, invoke by path |

## Next Steps

- [Cross-MCP Workflows](/docs/examples/math-calculations) — Agent-driven `execute_tool` orchestration
- [Multi-MCP Orchestration](/docs/examples/multi-mcp-orchestration) — Scale to 17+ MCPs
- [SLOP Language Reference](/docs/reference/slop-language) — Full syntax and built-in functions
