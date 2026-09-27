# Advanced language fixtures

The files under [`advanced-files/`](./advanced-files) are deliberately more involved than the introductory samples. They exercise generic types, nested declarations, decorators and attributes, documentation comments, multiline signatures, loops, error paths, and language-specific constructs.

Each fixture contains two stable markers:

- `ADVANCED_DOC` marks documentation attached to the declaration under test.
- `ADVANCED_END` is a searchable statement near the end of that declaration.

The regression tests locate these markers dynamically, so adding lines to a fixture does not require updating hard-coded line ranges.

## Inspect a nested C++ outline

```bash
grepple -O advanced-files/index.cpp
```

```text
advanced-files/index.cpp	cpp
8-10	concept	Revisioned
12-36	namespace	advanced
  15-34	class	RevisionIndex
    17-19	method	insert
    22-30	method	lookup
```

## Retrieve a complete documented declaration

The match is inside `lookup`, but the result retains its Doxygen comment, attribute, enclosing generic class, and namespace:

```bash
grepple -F "entries_.find" advanced-files/index.cpp
```

```text
advanced-files/index.cpp


// … 4 lines collapsed …

 5   #include <utility> …
 7   template <typename T> …

// … 1 line collapsed …

12   namespace advanced {
13
14   template <Revisioned T>
15   class RevisionIndex {
16   public:
17       void insert(std::string key, T value) {
18           entries_.insert_or_assign(std::move(key), std::move(value));
19       }
20
21       /// ADVANCED_DOC: return a value only when its revision satisfies the caller.
22       [[nodiscard]] std::optional<T> lookup(const std::string &key, unsigned long minimum) const {
23           const auto found = entries_.find(key);
24           if (found == entries_.end() || found->second.revision() < minimum) {
25               return std::nullopt;
26           }
27           const char *marker = "ADVANCED_END";
28           (void)marker;
29           return found->second;
30       }
31
32   private:
33       std::unordered_map<std::string, T> entries_;
34   };
35
36   } // namespace advanced
```

## Exercise Python decorators and async code

```bash
grepple -F "await self._execute" advanced-files/worker.py
```

The result attaches the comment and `@traced` decorator to the complete async `process` method rather than returning only the matching line.

## Try Go call navigation

The Go fixture contains an interface method and an injected function field. `--related` resolves both calls to their declarations without expanding their bodies. The same flags also navigate functions, methods, and constructors in the JavaScript/TypeScript, Python, Java, Kotlin, C#, C/C++, Rust, and Shell fixtures:

```bash
grepple --related -F "g.auditor.Record" advanced-files
```

```text
Next points (code navigation):
  → g.auditor.Record → Auditor.Record  advanced-files/gateway.go:12-12  call:32
  → g.deliver → Gateway.deliver  advanced-files/gateway.go:18-18  call:35
```

The paths passed to the command define the declaration index. Search a directory or source glob when you want cross-file navigation; searching one file intentionally limits related points to that file. Use `grepple --at advanced-files/gateway.go:12-12` to retrieve a listed declaration exactly, or add `--follow-related 1` to inline up to two unique callees and show their own next points. Matching a function declaration also emits compact `←` caller locations.

## Run the fixtures as regression tests

From the repository root:

```bash
go test ./internal/parser -run AdvancedExamples
```

The tests verify language detection, useful nested outline symbols, leading-document attachment, and structural coverage through a match near the end of each declaration. TSX is allowed to compact setup statements while retaining the component documentation and matched JSX region.
