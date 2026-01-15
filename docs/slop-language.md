# SLOP Language Reference

SLOP (Simple Language for Orchestrating Pipelines) is a lightweight scripting language designed for orchestrating MCP tools and data transformations.

## Quick Start

```python
# Variables (use = for assignment)
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

# Output results
emit(doubled)
```

## Language Basics

### Variables

```python
# Assignment uses =
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
# Map: apply function to each element
map([1, 2, 3], |x| x * 2)  # [2, 4, 6]

# Filter: keep elements matching predicate
filter([1, 2, 3, 4], |x| x % 2 == 0)  # [2, 4]

# Reduce: combine elements into single value
reduce([1, 2, 3], 0, |acc, x| acc + x)  # 6

# All/Any
all([2, 4, 6], |x| x % 2 == 0)  # true
any([1, 2, 3], |x| x > 2)       # true

# Group by
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

### Crypto Functions (slop-mcp)

```python
# Password generation
crypto_password(32)        # Secure 32-char password
crypto_passphrase(6)       # 6-word passphrase

# Key generation
crypto_ed25519_keygen()    # Ed25519 keypair
crypto_rsa_keygen(4096)    # RSA keypair
crypto_x25519_keygen()     # X25519 key

# Hashing
crypto_sha256("data")      # SHA-256 hash
crypto_sha512("data")      # SHA-512 hash
crypto_random_bytes(32)    # 32 random bytes (hex)
crypto_random_base64(32)   # 32 random bytes (base64)

# Encoding
crypto_base64_encode("hello")
crypto_hex_encode("hello")
```

### SLOP Reference Functions (slop-mcp)

```python
# Search for functions
slop_search("string")      # Search all functions
slop_search("split", 5)    # Limit to 5 results

# List categories
slop_categories()          # All function categories

# Get help for specific function
slop_help("map")           # Details about map()
```

## MCP Service Calls

SLOP can call MCP tools directly:

```python
# Call MCP tool
result = mcp_name.tool_name(arg1, arg2)

# Example: filesystem operations
files = filesystem.list_directory("/path")

# Example: GitHub operations
issues = github.list_issues(repo="owner/repo")
```

## CLI Tool Calls (slop-mcp)

When running through slop-mcp, CLI tools are available:

```python
# Call CLI tool
result = cli.tool_name(args...)

# Example: jq
parsed = cli.jq(filter=".data", stdin=json_data)
```

## Output

Use `emit()` to output results:

```python
# Single value
emit("Hello, World!")

# Multiple emits
for item in items {
    emit(item)
}

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
# Fetch data from MCP
data = api.get_users()

# Transform
active_users = filter(data, |u| u["status"] == "active")
names = map(active_users, |u| u["name"])
sorted_names = sorted(names)

# Generate report
report = {
    "total": len(data),
    "active": len(active_users),
    "names": sorted_names,
    "generated_at": now()
}

# Output
emit(report)
```
