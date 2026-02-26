---
sidebar_position: 3
---

# SLOP Language Reference

SLOP (Simple Language for Orchestrating Pipelines) is a lightweight scripting language embedded in slop-mcp for orchestrating MCP tools and data transformations.

## Why SLOP?

As described in Anthropic's [Code Execution with MCP](https://www.anthropic.com/engineering/code-execution-with-mcp) article, executing code within the MCP layer eliminates the round-trip overhead of shuttling intermediate results through the agent's context window. SLOP is purpose-built for this pattern:

- **Sandboxed by design** — each `run_slop` call creates an isolated runtime with no filesystem, network, or shell access. Scripts interact with the outside world *only* through MCP tool calls.
- **Native MCP piping** — the `|` operator chains MCP tool outputs through transforms without intermediate variables or agent round-trips.
- **Zero-dependency** — SLOP is compiled into the slop-mcp binary. No npm install, no pip, no runtime to manage.
- **Agent-friendly errors** — structured error responses with line, column, and source line so agents can self-correct.

## Quick Start

```python
# Variables
name = "world"
count = 42

# String interpolation
greeting = format("Hello, {}!", name)

# Lists and maps
numbers = [1, 2, 3, 4, 5]
config = {"host": "localhost", "port": 8080}

# Functions
doubled = map(numbers, |x| x * 2)
evens = filter(numbers, |x| x % 2 == 0)

# Pipe operator
result = [1, 2, 3, 4, 5] | filter(|x| x > 2) | map(|x| x * 10)
# Result: [30, 40, 50]

# Output results
emit(doubled)
```

## Language Basics

### Variables

```python
x = 10
name = "Alice"
items = [1, 2, 3]
```

### Data Types

| Type | Example | Description |
|------|---------|-------------|
| `int` | `42` | Integer numbers |
| `float` | `3.14` | Floating-point numbers |
| `string` | `"hello"` | Text strings |
| `bool` | `true`, `false` | Boolean values |
| `list` | `[1, 2, 3]` | Ordered collections |
| `map` | `{"a": 1}` | Key-value pairs |
| `set` | `{1, 2, 3}` | Unique values |
| `none` | `none` | Null/nil value |

### Operators

```python
# Arithmetic
x + y    # Addition
x - y    # Subtraction
x * y    # Multiplication
x / y    # Division
x % y    # Modulo
x ** y   # Power

# Comparison
x == y   # Equal
x != y   # Not equal
x < y    # Less than
x <= y   # Less or equal
x > y    # Greater than
x >= y   # Greater or equal

# Logical
x and y  # Logical AND
x or y   # Logical OR
not x    # Logical NOT

# Membership
x in list     # Contains
x not in list # Not contains
```

### Control Flow

```python
# Conditionals
if x > 0 {
    emit("positive")
} else if x < 0 {
    emit("negative")
} else {
    emit("zero")
}

# For loops
for item in items {
    emit(item)
}

for i in range(0, 10) {
    emit(i)
}

# While loops
while x > 0 {
    x = x - 1
}
```

### Functions

```python
# Define a function
fn greet(name) {
    return format("Hello, {}!", name)
}

# Lambda expressions
double = |x| x * 2
add = |a, b| a + b

# Call functions
result = greet("World")
doubled = double(21)
```

## Pipe Operator

The `|` operator passes the result of the left expression as the first argument to the right function, enabling fluent data transformation chains:

```python
# Chain list transforms
[1, 2, 3, 4, 5] | filter(|x| x > 2) | map(|x| x * 10)
# Result: [30, 40, 50]

# String processing pipeline
"hello world foo bar"
  | split(" ")
  | filter(|w| len(w) > 3)
  | map(|w| upper(w))
  | join(", ")
# Result: "HELLO, WORLD"

# Pipe MCP results through transforms
github.list_repos(org: "anthropics")
  | map(|r| {"name": r["name"], "stars": r["stargazers_count"]})
  | filter(|r| r["stars"] > 100)
  | sorted(|a, b| b["stars"] - a["stars"])
```

## MCP Tool Calls

SLOP calls MCP tools directly using `mcp_name.tool_name(param: value)` syntax:

```python
# Basic call
result = math.calculate(expression: "2 + 2")

# Named parameters
github.create_issue(repo: "org/repo", title: "Bug", labels: ["bug"])

# Capture results
files = filesystem.list_directory(path: "/src")
content = filesystem.read_file(path: "config.json")

# Cross-MCP chaining with pipes
github.list_issues(repo: "org/app", state: "open")
  | filter(|i| "critical" in i["labels"])
  | map(|i| i["title"])
```

### Multi-MCP Orchestration

Chain operations across multiple MCPs in a single script:

```python
# Read config, find matching repos, post to Slack
config = filesystem.read_file(path: "deps.json")
deps = json_parse(config) | map(|d| d["name"])

for dep in deps {
    info = github.get_repo(owner: "npm", repo: dep)
    emit(name: dep, stars: info["stargazers_count"])
}
```

### Dynamic MCP Calls

For programmatic tool invocation, use `__mcp_call__`:

```python
mcp_name = "github"
tool_name = "get_repo"
params = {"owner": "anthropics", "repo": "claude-code"}

result = __mcp_call__(mcp_name, tool_name, params)
```

## What SLOP Does That TypeScript/Python Don't

| What you want | TypeScript/Python | SLOP |
|---|---|---|
| Call 3 MCPs in sequence | 3 round-trips through agent context | 1 `run_slop` call, 0 round-trips |
| Transform MCP output | Parse JSON, write transform, serialize | `mcp.tool() \| map(\|x\| ...) \| filter(...)` |
| Persist data across calls | Set up database/file I/O | `mem_save("bank", "key", data)` |
| Batch operations | Loop in agent = N tool calls in context | `for item in items: mcp.tool(item)` |
| Sandbox execution | Container/VM setup required | Built-in: no fs/network/shell access |
| Error feedback to agent | Stack traces, unstructured | Line/column/source in JSON |

### Concrete Example

```
Without SLOP: 6 agent round-trips, each adding to context
─────────────────────────────────────────────────────────
Agent → search_tools("github issue")       → result in context
Agent → execute_tool(github, list_issues)   → 50 issues in context
Agent → execute_tool(github, get_issue, 42) → issue in context
Agent → execute_tool(linear, create_issue)  → result in context
Agent → execute_tool(slack, post_message)   → result in context
Agent → (finally responds to user)

With SLOP: 1 round-trip, intermediate data stays out of context
─────────────────────────────────────────────────────────
run_slop script='
  issues = github.list_issues(repo: "org/app", state: "open")
  critical = issues | filter(|i| "critical" in i["labels"])
  for issue in critical {
      linear.create_issue(title: issue["title"], priority: 1)
      slack.post_message(channel: "#incidents", text: issue["title"])
  }
  emit(synced: len(critical))
'
# Agent sees only: {"synced": 3}
```

## Sandboxing

Each `run_slop` invocation creates a completely isolated runtime:

- **No filesystem access** — scripts cannot read or write files directly
- **No network access** — scripts cannot make HTTP requests
- **No shell execution** — scripts cannot run commands
- **Fresh state** — no state leaks between script invocations
- **MCP tools are the only I/O** — the MCP server controls what's accessible

### In-Session State

Use `store_set`/`store_get` for thread-safe in-memory state within a session:

```python
store_set("counter", 0)
store_set("counter", store_get("counter") + 1)
```

Session state is shared across `run_slop` calls within the same session but does not persist to disk.

### Persistent Memory

Use `mem_save`/`mem_load` for disk-backed persistence controlled by the server:

```python
# Save data with optional metadata
mem_save("project", "config", {"version": "2.0"}, description: "Project config")

# Load it back (even in a different session)
config = mem_load("project", "config")

# Search across banks
results = mem_search("config")

# List entries in a bank
entries = mem_list("project")
```

## Recipes

SLOP includes built-in recipe templates for common patterns:

```python
# List available recipes
run_slop recipe="list"

# Use a recipe
run_slop recipe="batch_collect" script='
store_set("mcp_name", "github")
store_set("tool_name", "get_repo")
store_set("inputs", [
    {"owner": "anthropics", "repo": "claude-code"},
    {"owner": "standardbeagle", "repo": "slop-mcp"}
])
'
```

## Built-in Functions by Category

### String Functions

```python
len("hello")           # 5
upper("hello")         # "HELLO"
lower("HELLO")         # "hello"
split("a,b,c", ",")    # ["a", "b", "c"]
join(["a", "b"], "-")  # "a-b"
replace("hello", "l", "x")  # "hexxo"
strip("  hi  ")        # "hi"
contains("hello", "ell")    # true
startswith("hello", "he")   # true
endswith("hello", "lo")     # true
format("Hi {}!", "World")   # "Hi World!"
```

### List Functions

```python
len([1, 2, 3])         # 3
first([1, 2, 3])       # 1
last([1, 2, 3])        # 3
append([1, 2], 3)      # [1, 2, 3]
slice([1, 2, 3, 4], 1, 3)  # [2, 3]
reverse([1, 2, 3])     # [3, 2, 1]
sorted([3, 1, 2])      # [1, 2, 3]
unique([1, 2, 1, 3])   # [1, 2, 3]
flatten([[1, 2], [3]]) # [1, 2, 3]
range(0, 5)            # [0, 1, 2, 3, 4]
zip([1, 2], ["a", "b"]) # [[1, "a"], [2, "b"]]
```

### Functional Programming

```python
map([1, 2, 3], |x| x * 2)  # [2, 4, 6]
filter([1, 2, 3, 4], |x| x % 2 == 0)  # [2, 4]
reduce([1, 2, 3], 0, |acc, x| acc + x)  # 6
all([2, 4, 6], |x| x % 2 == 0)  # true
any([1, 2, 3], |x| x > 2)       # true
group_by(["apple", "banana", "apricot"], |s| first(s))
# {"a": ["apple", "apricot"], "b": ["banana"]}
```

### Map Functions

```python
keys({"a": 1, "b": 2})     # ["a", "b"]
values({"a": 1, "b": 2})   # [1, 2]
items({"a": 1})            # [["a", 1]]
get({"a": 1}, "b", 0)      # 0 (default)
has_key({"a": 1}, "a")     # true
merge({"a": 1}, {"b": 2})  # {"a": 1, "b": 2}
```

### Math Functions

```python
abs(-5)        # 5
min(3, 1, 4)   # 1
max(3, 1, 4)   # 4
sum([1, 2, 3]) # 6
avg([1, 2, 3]) # 2
round(3.7)     # 4
floor(3.7)     # 3
ceil(3.2)      # 4
pow(2, 3)      # 8
sqrt(16)       # 4
```

### Random Functions

```python
random_int(1, 100)          # Random int in [1, 100]
random_float(0.0, 1.0)      # Random float in [0, 1)
random_choice(["a", "b"])   # Random element
random_shuffle([1, 2, 3])   # Shuffled copy
random_uuid()               # UUID v4
random_hex(16)              # 16 hex chars
```

### Type Functions

```python
type(42)          # "int"
str(42)           # "42"
int("42")         # 42
float("3.14")     # 3.14
bool(1)           # true
is_string("hi")   # true
is_list([1, 2])   # true
is_none(none)     # true
```

### JSON Functions

```python
json_parse('{"a": 1}')     # {"a": 1}
json_stringify({"a": 1})   # '{"a":1}'
```

### Regex Functions

```python
regex_match("\\d+", "abc123")      # "123"
regex_find_all("\\d+", "a1b2")     # ["1", "2"]
regex_test("\\d+", "abc123")       # true
regex_replace("\\d", "a1b2", "X")  # "aXbX"
regex_split("\\s+", "a b  c")      # ["a", "b", "c"]
```

### Date/Time Functions

```python
now()                              # Current timestamp
today()                            # "2024-01-15"
time_format(now(), "2006-01-02")   # Formatted date
time_add(now(), "1h")              # Add duration
```

### Encoding Functions

```python
base64_encode("hello")   # "aGVsbG8="
base64_decode("aGVsbG8=") # "hello"
url_encode("a b")        # "a%20b"
url_decode("a%20b")      # "a b"
html_escape("<div>")     # "&lt;div&gt;"
```

### Crypto Functions

```python
crypto_password(32)        # Secure 32-char password
crypto_passphrase(6)       # 6-word passphrase
crypto_ed25519_keygen()    # Ed25519 keypair
crypto_sha256("data")      # SHA-256 hash
crypto_random_bytes(32)    # 32 random bytes (hex)
```

### SLOP Reference Functions

```python
slop_search("string")      # Search all functions
slop_categories()          # All function categories
slop_help("map")           # Details about map()
```

## Output

Use `emit()` to return results to the agent:

```python
# Single value
emit("Hello, World!")

# Named outputs (returned as a map)
emit(status: "success", count: 42)

# Structured data
emit({
    "status": "success",
    "data": results
})
```

## Error Handling

```python
# Raise error
error("Something went wrong")

# Assertions
assert(x > 0, "x must be positive")
assert_eq(result, expected)
assert_none(optional_value)
```

## Complete Example

```python
# Fetch issues from GitHub, sync critical ones to Linear, notify Slack
issues = github.list_issues(repo: "org/app", state: "open")

critical = issues
  | filter(|i| "critical" in i["labels"])
  | sorted(|a, b| b["created_at"] > a["created_at"])

for issue in critical {
    linear.create_issue(
        title: issue["title"],
        description: issue["body"],
        priority: 1
    )
    slack.post_message(
        channel: "#incidents",
        text: format("Synced: {}", issue["title"])
    )
}

emit(synced: len(critical), total: len(issues))
```
