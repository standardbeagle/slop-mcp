// Package builtins provides additional built-in functions for SLOP scripts.
package builtins

import (
	"fmt"
	"sort"
	"strings"

	"github.com/standardbeagle/slop/pkg/slop"
)

// SlopFunction describes a SLOP built-in function.
type SlopFunction struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Signature   string   `json:"signature"`
	Description string   `json:"description"`
	Example     string   `json:"example,omitempty"`
	Returns     string   `json:"returns,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// SlopReference contains the complete SLOP language reference.
var SlopReference = []SlopFunction{
	// Math functions
	{Name: "abs", Category: "math", Signature: "abs(x)", Description: "Returns the absolute value of a number", Example: "abs(-5) // 5", Returns: "number"},
	{Name: "ceil", Category: "math", Signature: "ceil(x)", Description: "Rounds up to the nearest integer", Example: "ceil(4.2) // 5", Returns: "int"},
	{Name: "floor", Category: "math", Signature: "floor(x)", Description: "Rounds down to the nearest integer", Example: "floor(4.8) // 4", Returns: "int"},
	{Name: "round", Category: "math", Signature: "round(x)", Description: "Rounds to the nearest integer", Example: "round(4.5) // 5", Returns: "int"},
	{Name: "min", Category: "math", Signature: "min(a, b, ...)", Description: "Returns the minimum value", Example: "min(3, 1, 4) // 1", Returns: "number"},
	{Name: "max", Category: "math", Signature: "max(a, b, ...)", Description: "Returns the maximum value", Example: "max(3, 1, 4) // 4", Returns: "number"},
	{Name: "sum", Category: "math", Signature: "sum(list)", Description: "Returns the sum of all elements", Example: "sum([1, 2, 3]) // 6", Returns: "number"},
	{Name: "avg", Category: "math", Signature: "avg(list)", Description: "Returns the average of all elements", Example: "avg([1, 2, 3]) // 2", Returns: "float"},
	{Name: "pow", Category: "math", Signature: "pow(base, exp)", Description: "Returns base raised to exponent", Example: "pow(2, 3) // 8", Returns: "number"},
	{Name: "sqrt", Category: "math", Signature: "sqrt(x)", Description: "Returns the square root", Example: "sqrt(16) // 4", Returns: "float"},
	{Name: "log", Category: "math", Signature: "log(x)", Description: "Returns the natural logarithm", Example: "log(2.718) // ~1", Returns: "float"},
	{Name: "log10", Category: "math", Signature: "log10(x)", Description: "Returns the base-10 logarithm", Example: "log10(100) // 2", Returns: "float"},
	{Name: "log2", Category: "math", Signature: "log2(x)", Description: "Returns the base-2 logarithm", Example: "log2(8) // 3", Returns: "float"},
	{Name: "exp", Category: "math", Signature: "exp(x)", Description: "Returns e raised to x", Example: "exp(1) // 2.718...", Returns: "float"},
	{Name: "sin", Category: "math", Signature: "sin(x)", Description: "Returns the sine (radians)", Example: "sin(0) // 0", Returns: "float", Tags: []string{"trig"}},
	{Name: "cos", Category: "math", Signature: "cos(x)", Description: "Returns the cosine (radians)", Example: "cos(0) // 1", Returns: "float", Tags: []string{"trig"}},
	{Name: "tan", Category: "math", Signature: "tan(x)", Description: "Returns the tangent (radians)", Example: "tan(0) // 0", Returns: "float", Tags: []string{"trig"}},
	{Name: "asin", Category: "math", Signature: "asin(x)", Description: "Returns the arc sine", Example: "asin(0) // 0", Returns: "float", Tags: []string{"trig"}},
	{Name: "acos", Category: "math", Signature: "acos(x)", Description: "Returns the arc cosine", Example: "acos(1) // 0", Returns: "float", Tags: []string{"trig"}},
	{Name: "atan", Category: "math", Signature: "atan(x)", Description: "Returns the arc tangent", Example: "atan(0) // 0", Returns: "float", Tags: []string{"trig"}},
	{Name: "atan2", Category: "math", Signature: "atan2(y, x)", Description: "Returns the arc tangent of y/x", Example: "atan2(1, 1) // 0.785...", Returns: "float", Tags: []string{"trig"}},

	// String functions
	{Name: "len", Category: "string", Signature: "len(s)", Description: "Returns the length of a string or list", Example: `len("hello") // 5`, Returns: "int"},
	{Name: "upper", Category: "string", Signature: "upper(s)", Description: "Converts string to uppercase", Example: `upper("hello") // "HELLO"`, Returns: "string"},
	{Name: "lower", Category: "string", Signature: "lower(s)", Description: "Converts string to lowercase", Example: `lower("HELLO") // "hello"`, Returns: "string"},
	{Name: "title", Category: "string", Signature: "title(s)", Description: "Converts string to title case", Example: `title("hello world") // "Hello World"`, Returns: "string"},
	{Name: "capitalize", Category: "string", Signature: "capitalize(s)", Description: "Capitalizes first character", Example: `capitalize("hello") // "Hello"`, Returns: "string"},
	{Name: "strip", Category: "string", Signature: "strip(s)", Description: "Removes leading/trailing whitespace", Example: `strip("  hi  ") // "hi"`, Returns: "string"},
	{Name: "lstrip", Category: "string", Signature: "lstrip(s)", Description: "Removes leading whitespace", Example: `lstrip("  hi") // "hi"`, Returns: "string"},
	{Name: "rstrip", Category: "string", Signature: "rstrip(s)", Description: "Removes trailing whitespace", Example: `rstrip("hi  ") // "hi"`, Returns: "string"},
	{Name: "split", Category: "string", Signature: "split(s, sep)", Description: "Splits string by separator", Example: `split("a,b,c", ",") // ["a", "b", "c"]`, Returns: "list"},
	{Name: "join", Category: "string", Signature: "join(list, sep)", Description: "Joins list elements with separator", Example: `join(["a", "b"], "-") // "a-b"`, Returns: "string"},
	{Name: "replace", Category: "string", Signature: "replace(s, old, new)", Description: "Replaces all occurrences", Example: `replace("hello", "l", "x") // "hexxo"`, Returns: "string"},
	{Name: "contains", Category: "string", Signature: "contains(s, sub)", Description: "Checks if string contains substring", Example: `contains("hello", "ell") // true`, Returns: "bool"},
	{Name: "startswith", Category: "string", Signature: "startswith(s, prefix)", Description: "Checks if string starts with prefix", Example: `startswith("hello", "he") // true`, Returns: "bool"},
	{Name: "endswith", Category: "string", Signature: "endswith(s, suffix)", Description: "Checks if string ends with suffix", Example: `endswith("hello", "lo") // true`, Returns: "bool"},
	{Name: "find", Category: "string", Signature: "find(s, sub)", Description: "Returns index of substring or -1", Example: `find("hello", "l") // 2`, Returns: "int"},
	{Name: "index", Category: "string", Signature: "index(s, sub)", Description: "Returns index of substring (errors if not found)", Example: `index("hello", "l") // 2`, Returns: "int"},
	{Name: "count", Category: "string", Signature: "count(s, sub)", Description: "Counts occurrences of substring", Example: `count("hello", "l") // 2`, Returns: "int"},
	{Name: "repeat", Category: "string", Signature: "repeat(s, n)", Description: "Repeats string n times", Example: `repeat("ab", 3) // "ababab"`, Returns: "string"},
	{Name: "pad_left", Category: "string", Signature: "pad_left(s, width, char)", Description: "Pads string on left", Example: `pad_left("5", 3, "0") // "005"`, Returns: "string"},
	{Name: "pad_right", Category: "string", Signature: "pad_right(s, width, char)", Description: "Pads string on right", Example: `pad_right("5", 3, "0") // "500"`, Returns: "string"},
	{Name: "lines", Category: "string", Signature: "lines(s)", Description: "Splits string into lines", Example: `lines("a\nb") // ["a", "b"]`, Returns: "list"},
	{Name: "words", Category: "string", Signature: "words(s)", Description: "Splits string into words", Example: `words("hello world") // ["hello", "world"]`, Returns: "list"},
	{Name: "chr", Category: "string", Signature: "chr(n)", Description: "Returns character for Unicode code point", Example: `chr(65) // "A"`, Returns: "string"},
	{Name: "ord", Category: "string", Signature: "ord(c)", Description: "Returns Unicode code point for character", Example: `ord("A") // 65`, Returns: "int"},
	{Name: "format", Category: "string", Signature: "format(template, ...args)", Description: "Formats string with placeholders", Example: `format("Hello {}", "World") // "Hello World"`, Returns: "string"},
	{Name: "partition", Category: "string", Signature: "partition(s, sep)", Description: "Splits into (before, sep, after)", Example: `partition("a:b:c", ":") // ["a", ":", "b:c"]`, Returns: "list"},

	// String validation
	{Name: "isalpha", Category: "string", Signature: "isalpha(s)", Description: "Checks if all chars are alphabetic", Example: `isalpha("abc") // true`, Returns: "bool", Tags: []string{"validation"}},
	{Name: "isdigit", Category: "string", Signature: "isdigit(s)", Description: "Checks if all chars are digits", Example: `isdigit("123") // true`, Returns: "bool", Tags: []string{"validation"}},
	{Name: "isalnum", Category: "string", Signature: "isalnum(s)", Description: "Checks if all chars are alphanumeric", Example: `isalnum("abc123") // true`, Returns: "bool", Tags: []string{"validation"}},
	{Name: "isspace", Category: "string", Signature: "isspace(s)", Description: "Checks if all chars are whitespace", Example: `isspace("  ") // true`, Returns: "bool", Tags: []string{"validation"}},

	// List functions
	{Name: "list", Category: "list", Signature: "list(x)", Description: "Converts to list", Example: `list("abc") // ["a", "b", "c"]`, Returns: "list"},
	{Name: "append", Category: "list", Signature: "append(list, item)", Description: "Adds item to end of list", Example: `append([1, 2], 3) // [1, 2, 3]`, Returns: "list"},
	{Name: "extend", Category: "list", Signature: "extend(list1, list2)", Description: "Adds all items from list2", Example: `extend([1], [2, 3]) // [1, 2, 3]`, Returns: "list"},
	{Name: "insert", Category: "list", Signature: "insert(list, idx, item)", Description: "Inserts item at index", Example: `insert([1, 3], 1, 2) // [1, 2, 3]`, Returns: "list"},
	{Name: "remove", Category: "list", Signature: "remove(list, item)", Description: "Removes first occurrence of item", Example: `remove([1, 2, 1], 1) // [2, 1]`, Returns: "list"},
	{Name: "pop", Category: "list", Signature: "pop(list)", Description: "Removes and returns last item", Example: `pop([1, 2, 3]) // 3`, Returns: "any"},
	{Name: "clear", Category: "list", Signature: "clear(list)", Description: "Removes all items", Example: `clear([1, 2, 3]) // []`, Returns: "list"},
	{Name: "copy", Category: "list", Signature: "copy(list)", Description: "Creates shallow copy", Example: `copy([1, 2]) // [1, 2]`, Returns: "list"},
	{Name: "reverse", Category: "list", Signature: "reverse(list)", Description: "Reverses list in place", Example: `reverse([1, 2, 3]) // [3, 2, 1]`, Returns: "list"},
	{Name: "reversed", Category: "list", Signature: "reversed(list)", Description: "Returns reversed copy", Example: `reversed([1, 2, 3]) // [3, 2, 1]`, Returns: "list"},
	{Name: "sorted", Category: "list", Signature: "sorted(list)", Description: "Returns sorted copy", Example: `sorted([3, 1, 2]) // [1, 2, 3]`, Returns: "list"},
	{Name: "first", Category: "list", Signature: "first(list)", Description: "Returns first element", Example: `first([1, 2, 3]) // 1`, Returns: "any"},
	{Name: "last", Category: "list", Signature: "last(list)", Description: "Returns last element", Example: `last([1, 2, 3]) // 3`, Returns: "any"},
	{Name: "nth", Category: "list", Signature: "nth(list, n)", Description: "Returns nth element (0-indexed)", Example: `nth([1, 2, 3], 1) // 2`, Returns: "any"},
	{Name: "slice", Category: "list", Signature: "slice(list, start, end)", Description: "Returns sublist", Example: `slice([1, 2, 3, 4], 1, 3) // [2, 3]`, Returns: "list"},
	{Name: "take", Category: "list", Signature: "take(list, n)", Description: "Returns first n elements", Example: `take([1, 2, 3], 2) // [1, 2]`, Returns: "list"},
	{Name: "drop", Category: "list", Signature: "drop(list, n)", Description: "Returns list without first n elements", Example: `drop([1, 2, 3], 1) // [2, 3]`, Returns: "list"},
	{Name: "take_while", Category: "list", Signature: "take_while(list, fn)", Description: "Takes while predicate is true", Example: `take_while([1, 2, 3], |x| x < 3) // [1, 2]`, Returns: "list"},
	{Name: "drop_while", Category: "list", Signature: "drop_while(list, fn)", Description: "Drops while predicate is true", Example: `drop_while([1, 2, 3], |x| x < 2) // [2, 3]`, Returns: "list"},
	{Name: "chunk", Category: "list", Signature: "chunk(list, size)", Description: "Splits into chunks of size", Example: `chunk([1, 2, 3, 4], 2) // [[1, 2], [3, 4]]`, Returns: "list"},
	{Name: "window", Category: "list", Signature: "window(list, size)", Description: "Returns sliding windows", Example: `window([1, 2, 3], 2) // [[1, 2], [2, 3]]`, Returns: "list"},
	{Name: "flatten", Category: "list", Signature: "flatten(list)", Description: "Flattens nested lists", Example: `flatten([[1, 2], [3]]) // [1, 2, 3]`, Returns: "list"},
	{Name: "unique", Category: "list", Signature: "unique(list)", Description: "Returns list with duplicates removed", Example: `unique([1, 2, 1]) // [1, 2]`, Returns: "list"},
	{Name: "dedup", Category: "list", Signature: "dedup(list)", Description: "Removes consecutive duplicates", Example: `dedup([1, 1, 2, 2, 1]) // [1, 2, 1]`, Returns: "list"},
	{Name: "compact", Category: "list", Signature: "compact(list)", Description: "Removes none values", Example: `compact([1, none, 2]) // [1, 2]`, Returns: "list"},
	{Name: "zip", Category: "list", Signature: "zip(list1, list2)", Description: "Pairs elements from two lists", Example: `zip([1, 2], ["a", "b"]) // [[1, "a"], [2, "b"]]`, Returns: "list"},
	{Name: "zip_with", Category: "list", Signature: "zip_with(list1, list2, fn)", Description: "Zips with custom function", Example: `zip_with([1, 2], [3, 4], |a, b| a + b) // [4, 6]`, Returns: "list"},
	{Name: "enumerate", Category: "list", Signature: "enumerate(list)", Description: "Returns (index, value) pairs", Example: `enumerate(["a", "b"]) // [[0, "a"], [1, "b"]]`, Returns: "list"},
	{Name: "interleave", Category: "list", Signature: "interleave(list1, list2)", Description: "Interleaves two lists", Example: `interleave([1, 3], [2, 4]) // [1, 2, 3, 4]`, Returns: "list"},
	{Name: "range", Category: "list", Signature: "range(start, end, step)", Description: "Generates number sequence", Example: `range(0, 5) // [0, 1, 2, 3, 4]`, Returns: "list"},
	{Name: "find_index", Category: "list", Signature: "find_index(list, fn)", Description: "Returns index where predicate is true", Example: `find_index([1, 2, 3], |x| x > 1) // 1`, Returns: "int"},

	// Higher-order functions
	{Name: "map", Category: "functional", Signature: "map(list, fn)", Description: "Applies function to each element", Example: `map([1, 2], |x| x * 2) // [2, 4]`, Returns: "list"},
	{Name: "filter", Category: "functional", Signature: "filter(list, fn)", Description: "Keeps elements where predicate is true", Example: `filter([1, 2, 3], |x| x > 1) // [2, 3]`, Returns: "list"},
	{Name: "reject", Category: "functional", Signature: "reject(list, fn)", Description: "Removes elements where predicate is true", Example: `reject([1, 2, 3], |x| x > 1) // [1]`, Returns: "list"},
	{Name: "reduce", Category: "functional", Signature: "reduce(list, init, fn)", Description: "Reduces list to single value", Example: `reduce([1, 2, 3], 0, |a, b| a + b) // 6`, Returns: "any"},
	{Name: "flat_map", Category: "functional", Signature: "flat_map(list, fn)", Description: "Maps and flattens result", Example: `flat_map([1, 2], |x| [x, x]) // [1, 1, 2, 2]`, Returns: "list"},
	{Name: "group_by", Category: "functional", Signature: "group_by(list, fn)", Description: "Groups by function result", Example: `group_by([1, 2, 3], |x| x % 2)`, Returns: "map"},
	{Name: "all", Category: "functional", Signature: "all(list, fn)", Description: "True if all elements match predicate", Example: `all([2, 4, 6], |x| x % 2 == 0) // true`, Returns: "bool"},
	{Name: "any", Category: "functional", Signature: "any(list, fn)", Description: "True if any element matches predicate", Example: `any([1, 2, 3], |x| x > 2) // true`, Returns: "bool"},

	// Map/Dict functions
	{Name: "dict", Category: "map", Signature: "dict(pairs)", Description: "Creates map from key-value pairs", Example: `dict([["a", 1], ["b", 2]]) // {"a": 1, "b": 2}`, Returns: "map"},
	{Name: "keys", Category: "map", Signature: "keys(map)", Description: "Returns list of keys", Example: `keys({"a": 1, "b": 2}) // ["a", "b"]`, Returns: "list"},
	{Name: "values", Category: "map", Signature: "values(map)", Description: "Returns list of values", Example: `values({"a": 1, "b": 2}) // [1, 2]`, Returns: "list"},
	{Name: "items", Category: "map", Signature: "items(map)", Description: "Returns list of [key, value] pairs", Example: `items({"a": 1}) // [["a", 1]]`, Returns: "list"},
	{Name: "get", Category: "map", Signature: "get(map, key, default)", Description: "Gets value with optional default", Example: `get({"a": 1}, "b", 0) // 0`, Returns: "any"},
	{Name: "has_key", Category: "map", Signature: "has_key(map, key)", Description: "Checks if key exists", Example: `has_key({"a": 1}, "a") // true`, Returns: "bool"},
	{Name: "merge", Category: "map", Signature: "merge(map1, map2)", Description: "Merges two maps", Example: `merge({"a": 1}, {"b": 2}) // {"a": 1, "b": 2}`, Returns: "map"},

	// Set functions
	{Name: "set", Category: "set", Signature: "set(list)", Description: "Creates set from list", Example: `set([1, 2, 1]) // {1, 2}`, Returns: "set"},
	{Name: "add", Category: "set", Signature: "add(set, item)", Description: "Adds item to set", Example: `add({1, 2}, 3) // {1, 2, 3}`, Returns: "set"},
	{Name: "discard", Category: "set", Signature: "discard(set, item)", Description: "Removes item from set", Example: `discard({1, 2}, 1) // {2}`, Returns: "set"},
	{Name: "union", Category: "set", Signature: "union(set1, set2)", Description: "Returns union of sets", Example: `union({1, 2}, {2, 3}) // {1, 2, 3}`, Returns: "set"},
	{Name: "intersection", Category: "set", Signature: "intersection(set1, set2)", Description: "Returns intersection", Example: `intersection({1, 2}, {2, 3}) // {2}`, Returns: "set"},
	{Name: "difference", Category: "set", Signature: "difference(set1, set2)", Description: "Returns set1 - set2", Example: `difference({1, 2}, {2}) // {1}`, Returns: "set"},
	{Name: "symmetric_difference", Category: "set", Signature: "symmetric_difference(set1, set2)", Description: "Returns elements in either but not both", Example: `symmetric_difference({1, 2}, {2, 3}) // {1, 3}`, Returns: "set"},
	{Name: "issubset", Category: "set", Signature: "issubset(set1, set2)", Description: "Checks if set1 is subset of set2", Example: `issubset({1}, {1, 2}) // true`, Returns: "bool"},
	{Name: "issuperset", Category: "set", Signature: "issuperset(set1, set2)", Description: "Checks if set1 is superset of set2", Example: `issuperset({1, 2}, {1}) // true`, Returns: "bool"},

	// Type functions
	{Name: "type", Category: "type", Signature: "type(x)", Description: "Returns type name as string", Example: `type(42) // "int"`, Returns: "string"},
	{Name: "str", Category: "type", Signature: "str(x)", Description: "Converts to string", Example: `str(42) // "42"`, Returns: "string"},
	{Name: "int", Category: "type", Signature: "int(x)", Description: "Converts to integer", Example: `int("42") // 42`, Returns: "int"},
	{Name: "float", Category: "type", Signature: "float(x)", Description: "Converts to float", Example: `float("3.14") // 3.14`, Returns: "float"},
	{Name: "bool", Category: "type", Signature: "bool(x)", Description: "Converts to boolean", Example: `bool(1) // true`, Returns: "bool"},
	{Name: "is_string", Category: "type", Signature: "is_string(x)", Description: "Checks if value is string", Example: `is_string("hi") // true`, Returns: "bool"},
	{Name: "is_int", Category: "type", Signature: "is_int(x)", Description: "Checks if value is integer", Example: `is_int(42) // true`, Returns: "bool"},
	{Name: "is_float", Category: "type", Signature: "is_float(x)", Description: "Checks if value is float", Example: `is_float(3.14) // true`, Returns: "bool"},
	{Name: "is_number", Category: "type", Signature: "is_number(x)", Description: "Checks if value is numeric", Example: `is_number(42) // true`, Returns: "bool"},
	{Name: "is_bool", Category: "type", Signature: "is_bool(x)", Description: "Checks if value is boolean", Example: `is_bool(true) // true`, Returns: "bool"},
	{Name: "is_list", Category: "type", Signature: "is_list(x)", Description: "Checks if value is list", Example: `is_list([1, 2]) // true`, Returns: "bool"},
	{Name: "is_map", Category: "type", Signature: "is_map(x)", Description: "Checks if value is map", Example: `is_map({"a": 1}) // true`, Returns: "bool"},
	{Name: "is_set", Category: "type", Signature: "is_set(x)", Description: "Checks if value is set", Example: `is_set({1, 2}) // true`, Returns: "bool"},
	{Name: "is_none", Category: "type", Signature: "is_none(x)", Description: "Checks if value is none", Example: `is_none(none) // true`, Returns: "bool"},
	{Name: "is_callable", Category: "type", Signature: "is_callable(x)", Description: "Checks if value is callable", Example: `is_callable(|x| x) // true`, Returns: "bool"},
	{Name: "none", Category: "type", Signature: "none", Description: "The none/null value", Example: `x = none`, Returns: "none"},

	// Random functions
	{Name: "random_int", Category: "random", Signature: "random_int(min, max)", Description: "Random integer in range [min, max]", Example: `random_int(1, 100)`, Returns: "int"},
	{Name: "random_float", Category: "random", Signature: "random_float(min, max)", Description: "Random float in range [min, max)", Example: `random_float(0.0, 1.0)`, Returns: "float"},
	{Name: "random_choice", Category: "random", Signature: "random_choice(list)", Description: "Random element from list", Example: `random_choice(["a", "b", "c"])`, Returns: "any"},
	{Name: "random_choices", Category: "random", Signature: "random_choices(list, n)", Description: "N random elements with replacement", Example: `random_choices([1, 2, 3], 2)`, Returns: "list"},
	{Name: "random_shuffle", Category: "random", Signature: "random_shuffle(list)", Description: "Returns shuffled copy", Example: `random_shuffle([1, 2, 3])`, Returns: "list"},
	{Name: "random_chance", Category: "random", Signature: "random_chance(p)", Description: "Returns true with probability p", Example: `random_chance(0.5)`, Returns: "bool"},
	{Name: "random_weighted", Category: "random", Signature: "random_weighted(weights)", Description: "Weighted random selection", Example: `random_weighted({"a": 0.7, "b": 0.3})`, Returns: "string"},
	{Name: "random_seed", Category: "random", Signature: "random_seed(n)", Description: "Sets random seed for reproducibility", Example: `random_seed(42)`, Returns: "none"},
	{Name: "random_uuid", Category: "random", Signature: "random_uuid()", Description: "Generates UUID v4", Example: `random_uuid()`, Returns: "string"},
	{Name: "random_hex", Category: "random", Signature: "random_hex(n)", Description: "Generates n hex characters", Example: `random_hex(16)`, Returns: "string"},

	// JSON functions
	{Name: "json_parse", Category: "json", Signature: "json_parse(s)", Description: "Parses JSON string", Example: `json_parse('{"a": 1}') // {"a": 1}`, Returns: "any"},
	{Name: "json_stringify", Category: "json", Signature: "json_stringify(x)", Description: "Converts to JSON string", Example: `json_stringify({"a": 1}) // '{"a":1}'`, Returns: "string"},

	// Regex functions
	{Name: "regex_match", Category: "regex", Signature: "regex_match(pattern, s)", Description: "Returns first match or none", Example: `regex_match("\\d+", "abc123") // "123"`, Returns: "string"},
	{Name: "regex_find_all", Category: "regex", Signature: "regex_find_all(pattern, s)", Description: "Returns all matches", Example: `regex_find_all("\\d+", "a1b2") // ["1", "2"]`, Returns: "list"},
	{Name: "regex_test", Category: "regex", Signature: "regex_test(pattern, s)", Description: "Tests if pattern matches", Example: `regex_test("\\d+", "abc123") // true`, Returns: "bool"},
	{Name: "regex_replace", Category: "regex", Signature: "regex_replace(pattern, s, repl)", Description: "Replaces matches", Example: `regex_replace("\\d", "a1b2", "X") // "aXbX"`, Returns: "string"},
	{Name: "regex_split", Category: "regex", Signature: "regex_split(pattern, s)", Description: "Splits by pattern", Example: `regex_split("\\s+", "a b  c") // ["a", "b", "c"]`, Returns: "list"},
	{Name: "match", Category: "regex", Signature: "match(pattern, s)", Description: "Alias for regex_match", Example: `match("\\d+", "abc123")`, Returns: "string"},

	// Date/Time functions
	{Name: "now", Category: "time", Signature: "now()", Description: "Returns current timestamp", Example: `now()`, Returns: "string"},
	{Name: "today", Category: "time", Signature: "today()", Description: "Returns current date", Example: `today() // "2024-01-15"`, Returns: "string"},
	{Name: "time_format", Category: "time", Signature: "time_format(t, fmt)", Description: "Formats timestamp", Example: `time_format(now(), "2006-01-02")`, Returns: "string"},
	{Name: "time_parse", Category: "time", Signature: "time_parse(s, fmt)", Description: "Parses time string", Example: `time_parse("2024-01-15", "2006-01-02")`, Returns: "string"},
	{Name: "time_add", Category: "time", Signature: "time_add(t, duration)", Description: "Adds duration to time", Example: `time_add(now(), "1h")`, Returns: "string"},
	{Name: "time_diff", Category: "time", Signature: "time_diff(t1, t2)", Description: "Returns difference between times", Example: `time_diff(t1, t2)`, Returns: "string"},

	// Encoding functions
	{Name: "base64_encode", Category: "encoding", Signature: "base64_encode(s)", Description: "Encodes to base64", Example: `base64_encode("hello") // "aGVsbG8="`, Returns: "string"},
	{Name: "base64_decode", Category: "encoding", Signature: "base64_decode(s)", Description: "Decodes from base64", Example: `base64_decode("aGVsbG8=") // "hello"`, Returns: "string"},
	{Name: "url_encode", Category: "encoding", Signature: "url_encode(s)", Description: "URL encodes string", Example: `url_encode("a b") // "a%20b"`, Returns: "string"},
	{Name: "url_decode", Category: "encoding", Signature: "url_decode(s)", Description: "URL decodes string", Example: `url_decode("a%20b") // "a b"`, Returns: "string"},
	{Name: "html_escape", Category: "encoding", Signature: "html_escape(s)", Description: "Escapes HTML entities", Example: `html_escape("<div>") // "&lt;div&gt;"`, Returns: "string"},
	{Name: "html_unescape", Category: "encoding", Signature: "html_unescape(s)", Description: "Unescapes HTML entities", Example: `html_unescape("&lt;") // "<"`, Returns: "string"},

	// Hash functions
	{Name: "hash_md5", Category: "hash", Signature: "hash_md5(s)", Description: "Returns MD5 hash (hex)", Example: `hash_md5("hello")`, Returns: "string"},
	{Name: "hash_sha256", Category: "hash", Signature: "hash_sha256(s)", Description: "Returns SHA-256 hash (hex)", Example: `hash_sha256("hello")`, Returns: "string"},
	{Name: "hash_sha512", Category: "hash", Signature: "hash_sha512(s)", Description: "Returns SHA-512 hash (hex)", Example: `hash_sha512("hello")`, Returns: "string"},
	{Name: "hash_hmac", Category: "hash", Signature: "hash_hmac(key, msg, algo)", Description: "Returns HMAC", Example: `hash_hmac("key", "msg", "sha256")`, Returns: "string"},

	// Validation functions
	{Name: "validate_email", Category: "validation", Signature: "validate_email(s)", Description: "Validates email format", Example: `validate_email("a@b.com") // true`, Returns: "bool"},
	{Name: "validate_url", Category: "validation", Signature: "validate_url(s)", Description: "Validates URL format", Example: `validate_url("https://example.com") // true`, Returns: "bool"},
	{Name: "validate_uuid", Category: "validation", Signature: "validate_uuid(s)", Description: "Validates UUID format", Example: `validate_uuid("550e8400-...") // true`, Returns: "bool"},
	{Name: "validate_json", Category: "validation", Signature: "validate_json(s)", Description: "Validates JSON syntax", Example: `validate_json('{"a":1}') // true`, Returns: "bool"},
	{Name: "uuid", Category: "validation", Signature: "uuid()", Description: "Generates UUID v4", Example: `uuid()`, Returns: "string"},

	// Data generators
	{Name: "gen_name", Category: "generator", Signature: "gen_name()", Description: "Generates random full name", Example: `gen_name() // "John Smith"`, Returns: "string"},
	{Name: "gen_first_name", Category: "generator", Signature: "gen_first_name()", Description: "Generates random first name", Example: `gen_first_name() // "John"`, Returns: "string"},
	{Name: "gen_last_name", Category: "generator", Signature: "gen_last_name()", Description: "Generates random last name", Example: `gen_last_name() // "Smith"`, Returns: "string"},
	{Name: "gen_email", Category: "generator", Signature: "gen_email()", Description: "Generates random email", Example: `gen_email() // "john.smith@example.com"`, Returns: "string"},
	{Name: "gen_phone", Category: "generator", Signature: "gen_phone()", Description: "Generates random phone number", Example: `gen_phone() // "(555) 123-4567"`, Returns: "string"},
	{Name: "gen_word", Category: "generator", Signature: "gen_word()", Description: "Generates random lorem word", Example: `gen_word() // "lorem"`, Returns: "string"},
	{Name: "gen_words", Category: "generator", Signature: "gen_words(n)", Description: "Generates n random words", Example: `gen_words(5)`, Returns: "string"},
	{Name: "gen_sentence", Category: "generator", Signature: "gen_sentence()", Description: "Generates random sentence", Example: `gen_sentence()`, Returns: "string"},
	{Name: "gen_paragraph", Category: "generator", Signature: "gen_paragraph()", Description: "Generates random paragraph", Example: `gen_paragraph()`, Returns: "string"},
	{Name: "gen_lorem", Category: "generator", Signature: "gen_lorem()", Description: "Generates lorem ipsum text", Example: `gen_lorem()`, Returns: "string"},
	{Name: "gen_color", Category: "generator", Signature: "gen_color()", Description: "Generates random hex color", Example: `gen_color() // "#a1b2c3"`, Returns: "string"},
	{Name: "gen_rgb", Category: "generator", Signature: "gen_rgb()", Description: "Generates random RGB values", Example: `gen_rgb() // [128, 64, 255]`, Returns: "list"},

	// Control/Debug
	{Name: "print", Category: "control", Signature: "print(...args)", Description: "Prints values to stdout", Example: `print("hello", 42)`, Returns: "none"},
	{Name: "assert", Category: "control", Signature: "assert(condition, msg)", Description: "Asserts condition is true", Example: `assert(x > 0, "must be positive")`, Returns: "none"},
	{Name: "assert_eq", Category: "control", Signature: "assert_eq(a, b)", Description: "Asserts a equals b", Example: `assert_eq(1 + 1, 2)`, Returns: "none"},
	{Name: "assert_ne", Category: "control", Signature: "assert_ne(a, b)", Description: "Asserts a not equals b", Example: `assert_ne(1, 2)`, Returns: "none"},
	{Name: "assert_true", Category: "control", Signature: "assert_true(x)", Description: "Asserts x is true", Example: `assert_true(ok)`, Returns: "none"},
	{Name: "assert_false", Category: "control", Signature: "assert_false(x)", Description: "Asserts x is false", Example: `assert_false(failed)`, Returns: "none"},
	{Name: "assert_none", Category: "control", Signature: "assert_none(x)", Description: "Asserts x is none", Example: `assert_none(result)`, Returns: "none"},
	{Name: "assert_not_none", Category: "control", Signature: "assert_not_none(x)", Description: "Asserts x is not none", Example: `assert_not_none(value)`, Returns: "none"},
	{Name: "error", Category: "control", Signature: "error(msg)", Description: "Raises an error", Example: `error("something went wrong")`, Returns: "none"},
	{Name: "sleep", Category: "control", Signature: "sleep(ms)", Description: "Sleeps for milliseconds", Example: `sleep(1000)`, Returns: "none"},
	{Name: "log_debug", Category: "control", Signature: "log_debug(msg)", Description: "Logs debug message", Example: `log_debug("starting")`, Returns: "none"},
	{Name: "log_info", Category: "control", Signature: "log_info(msg)", Description: "Logs info message", Example: `log_info("processing")`, Returns: "none"},
	{Name: "log_warn", Category: "control", Signature: "log_warn(msg)", Description: "Logs warning message", Example: `log_warn("deprecated")`, Returns: "none"},
	{Name: "log_error", Category: "control", Signature: "log_error(msg)", Description: "Logs error message", Example: `log_error("failed")`, Returns: "none"},

	// Environment
	{Name: "env_get", Category: "env", Signature: "env_get(name)", Description: "Gets environment variable", Example: `env_get("HOME")`, Returns: "string"},
	{Name: "env_mode", Category: "env", Signature: "env_mode()", Description: "Returns execution mode", Example: `env_mode() // "production"`, Returns: "string"},
	{Name: "env_debug", Category: "env", Signature: "env_debug()", Description: "Returns true if debug mode", Example: `env_debug()`, Returns: "bool"},

	// Store (key-value)
	{Name: "store_get", Category: "store", Signature: "store_get(key)", Description: "Gets value from store", Example: `store_get("mykey")`, Returns: "any"},
	{Name: "store_set", Category: "store", Signature: "store_set(key, value)", Description: "Sets value in store", Example: `store_set("mykey", 42)`, Returns: "none"},
	{Name: "store_delete", Category: "store", Signature: "store_delete(key)", Description: "Deletes key from store", Example: `store_delete("mykey")`, Returns: "none"},
	{Name: "store_exists", Category: "store", Signature: "store_exists(key)", Description: "Checks if key exists", Example: `store_exists("mykey")`, Returns: "bool"},
	{Name: "store_keys", Category: "store", Signature: "store_keys()", Description: "Returns all keys in store", Example: `store_keys()`, Returns: "list"},

	// Crypto (slop-mcp additions)
	{Name: "crypto_password", Category: "crypto", Signature: "crypto_password(length)", Description: "Generates secure password", Example: `crypto_password(32)`, Returns: "string", Tags: []string{"slop-mcp"}},
	{Name: "crypto_passphrase", Category: "crypto", Signature: "crypto_passphrase(words)", Description: "Generates passphrase from EFF wordlist", Example: `crypto_passphrase(6)`, Returns: "string", Tags: []string{"slop-mcp"}},
	{Name: "crypto_ed25519_keygen", Category: "crypto", Signature: "crypto_ed25519_keygen()", Description: "Generates Ed25519 keypair", Example: `crypto_ed25519_keygen()`, Returns: "map", Tags: []string{"slop-mcp"}},
	{Name: "crypto_rsa_keygen", Category: "crypto", Signature: "crypto_rsa_keygen(bits)", Description: "Generates RSA keypair", Example: `crypto_rsa_keygen(4096)`, Returns: "map", Tags: []string{"slop-mcp"}},
	{Name: "crypto_x25519_keygen", Category: "crypto", Signature: "crypto_x25519_keygen()", Description: "Generates X25519 key", Example: `crypto_x25519_keygen()`, Returns: "map", Tags: []string{"slop-mcp"}},
	{Name: "crypto_sha256", Category: "crypto", Signature: "crypto_sha256(data)", Description: "SHA-256 hash (hex)", Example: `crypto_sha256("hello")`, Returns: "string", Tags: []string{"slop-mcp"}},
	{Name: "crypto_sha512", Category: "crypto", Signature: "crypto_sha512(data)", Description: "SHA-512 hash (hex)", Example: `crypto_sha512("hello")`, Returns: "string", Tags: []string{"slop-mcp"}},
	{Name: "crypto_hash", Category: "crypto", Signature: "crypto_hash(data)", Description: "Hash with algorithm option", Example: `crypto_hash("hello")`, Returns: "string", Tags: []string{"slop-mcp"}},
	{Name: "crypto_random_bytes", Category: "crypto", Signature: "crypto_random_bytes(n)", Description: "Cryptographic random bytes (hex)", Example: `crypto_random_bytes(32)`, Returns: "string", Tags: []string{"slop-mcp"}},
	{Name: "crypto_random_base64", Category: "crypto", Signature: "crypto_random_base64(n)", Description: "Cryptographic random (base64)", Example: `crypto_random_base64(32)`, Returns: "string", Tags: []string{"slop-mcp"}},
	{Name: "crypto_base64_encode", Category: "crypto", Signature: "crypto_base64_encode(data)", Description: "Base64 encode", Example: `crypto_base64_encode("hello")`, Returns: "string", Tags: []string{"slop-mcp"}},
	{Name: "crypto_base64_decode", Category: "crypto", Signature: "crypto_base64_decode(data)", Description: "Base64 decode", Example: `crypto_base64_decode("aGVsbG8=")`, Returns: "string", Tags: []string{"slop-mcp"}},
	{Name: "crypto_hex_encode", Category: "crypto", Signature: "crypto_hex_encode(data)", Description: "Hex encode", Example: `crypto_hex_encode("hello")`, Returns: "string", Tags: []string{"slop-mcp"}},
	{Name: "crypto_hex_decode", Category: "crypto", Signature: "crypto_hex_decode(data)", Description: "Hex decode", Example: `crypto_hex_decode("68656c6c6f")`, Returns: "string", Tags: []string{"slop-mcp"}},

	// JWT functions (slop-mcp additions)
	{Name: "jwt_decode", Category: "jwt", Signature: "jwt_decode(token)", Description: "Decodes JWT without verification", Example: `jwt_decode("eyJ...")`, Returns: "map", Tags: []string{"slop-mcp", "auth"}},
	{Name: "jwt_verify", Category: "jwt", Signature: "jwt_verify(token, key, alg)", Description: "Verifies JWT signature", Example: `jwt_verify(token, secret, "HS256")`, Returns: "map", Tags: []string{"slop-mcp", "auth"}},
	{Name: "jwt_sign", Category: "jwt", Signature: "jwt_sign(claims, key, alg)", Description: "Creates signed JWT", Example: `jwt_sign({"sub": "1234"}, secret, "HS256")`, Returns: "string", Tags: []string{"slop-mcp", "auth"}},
	{Name: "jwt_expired", Category: "jwt", Signature: "jwt_expired(token)", Description: "Checks if JWT exp claim has passed", Example: `jwt_expired(token)`, Returns: "bool", Tags: []string{"slop-mcp", "auth"}},

	// Template functions (slop-mcp additions)
	{Name: "template_render", Category: "template", Signature: "template_render(tmpl, data)", Description: "Renders Go template with data", Example: `template_render("Hello {{.name}}", {"name": "World"})`, Returns: "string", Tags: []string{"slop-mcp"}},
	{Name: "template_render_file", Category: "template", Signature: "template_render_file(path, data)", Description: "Renders Go template from file", Example: `template_render_file("template.tmpl", data)`, Returns: "string", Tags: []string{"slop-mcp"}},
	{Name: "indent", Category: "template", Signature: "indent(text, spaces)", Description: "Indents all lines by spaces", Example: `indent("hello\nworld", 4)`, Returns: "string", Tags: []string{"slop-mcp"}},
	{Name: "dedent", Category: "template", Signature: "dedent(text)", Description: "Removes common leading whitespace", Example: `dedent("  hello\n  world")`, Returns: "string", Tags: []string{"slop-mcp"}},
	{Name: "wrap", Category: "template", Signature: "wrap(text, width)", Description: "Wraps text at specified width", Example: `wrap("long text here", 40)`, Returns: "string", Tags: []string{"slop-mcp"}},
}

// RegisterSlopSearch registers the slop_search built-in function.
func RegisterSlopSearch(rt *slop.Runtime) {
	rt.RegisterBuiltin("slop_search", builtinSlopSearch)
	rt.RegisterBuiltin("slop_categories", builtinSlopCategories)
	rt.RegisterBuiltin("slop_help", builtinSlopHelp)
}

func builtinSlopSearch(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	query := ""
	if len(args) > 0 {
		if sv, ok := args[0].(*slop.StringValue); ok {
			query = strings.ToLower(sv.Value)
		}
	}

	category := ""
	if v, ok := kwargs["category"]; ok {
		if sv, ok := v.(*slop.StringValue); ok {
			category = sv.Value
		}
	}

	limit := 20
	if len(args) > 1 {
		if iv, ok := args[1].(*slop.IntValue); ok {
			limit = int(iv.Value)
		}
	}

	var results []any
	for _, fn := range SlopReference {
		// Category filter
		if category != "" && fn.Category != category {
			continue
		}

		// Search in name, description, signature
		searchText := strings.ToLower(fn.Name + " " + fn.Description + " " + fn.Signature)
		if query == "" || strings.Contains(searchText, query) {
			results = append(results, map[string]any{
				"name":        fn.Name,
				"category":    fn.Category,
				"signature":   fn.Signature,
				"description": fn.Description,
				"example":     fn.Example,
				"returns":     fn.Returns,
			})
		}

		if len(results) >= limit {
			break
		}
	}

	return slop.GoToValue(results), nil
}

func builtinSlopCategories(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	categories := make(map[string]int)
	for _, fn := range SlopReference {
		categories[fn.Category]++
	}

	var result []any
	for cat, count := range categories {
		result = append(result, map[string]any{
			"category": cat,
			"count":    count,
		})
	}

	// Sort by category name
	sort.Slice(result, func(i, j int) bool {
		return result[i].(map[string]any)["category"].(string) < result[j].(map[string]any)["category"].(string)
	})

	return slop.GoToValue(result), nil
}

func builtinSlopHelp(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("slop_help: requires function name")
	}

	name := ""
	if sv, ok := args[0].(*slop.StringValue); ok {
		name = sv.Value
	}

	for _, fn := range SlopReference {
		if fn.Name == name {
			return slop.GoToValue(map[string]any{
				"name":        fn.Name,
				"category":    fn.Category,
				"signature":   fn.Signature,
				"description": fn.Description,
				"example":     fn.Example,
				"returns":     fn.Returns,
				"tags":        fn.Tags,
			}), nil
		}
	}

	return slop.GoToValue(map[string]any{
		"error": fmt.Sprintf("function '%s' not found", name),
	}), nil
}

// SearchSlopFunctions searches the SLOP reference for matching functions.
// This is exported for use by MCP tools.
func SearchSlopFunctions(query string, category string, limit int) []SlopFunction {
	query = strings.ToLower(query)
	var results []SlopFunction

	for _, fn := range SlopReference {
		if category != "" && fn.Category != category {
			continue
		}

		searchText := strings.ToLower(fn.Name + " " + fn.Description + " " + fn.Signature)
		if query == "" || strings.Contains(searchText, query) {
			results = append(results, fn)
		}

		if limit > 0 && len(results) >= limit {
			break
		}
	}

	return results
}

// GetCategories returns all SLOP function categories with counts.
func GetCategories() map[string]int {
	categories := make(map[string]int)
	for _, fn := range SlopReference {
		categories[fn.Category]++
	}
	return categories
}
