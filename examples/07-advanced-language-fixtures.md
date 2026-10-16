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
  RevisionIndex @class:15-34
    insert @method:17-19
    lookup @method:22-30
```

## Retrieve a complete documented declaration

The match is inside `lookup`, but the result retains its Doxygen comment, attribute, enclosing generic class, and namespace:

```bash
grepple -F "entries_.find" advanced-files/index.cpp
```

```text
advanced-files/index.cpp


// … 11 lines collapsed …

mfJ│12│namespace advanced {
2lC│13│
_FB│14│template <Revisioned T>
msU│15│class RevisionIndex {
18z│16│public:
oNW│17│    void insert(std::string key, T value) {
VYE│18│        entries_.insert_or_assign(std::move(key), std::move(value));
cOK│19│    }
miP│20│
EQm│21│    /// ADVANCED_DOC: return a value only when its revision satisfies the caller.
40x│22│    [[nodiscard]] std::optional<T> lookup(const std::string &key, unsigned long minimum) const {
459│23│        const auto found = entries_.find(key);
9f-│24│        if (found == entries_.end() || found->second.revision() < minimum) {
j5m│25│            return std::nullopt;
k1_│26│        }
1N5│27│        const char *marker = "ADVANCED_END";
_dA│28│        (void)marker;
SFF│29│        return found->second;
xxs│30│    }
r4Y│31│
6cQ│32│private:
Tcg│33│    std::unordered_map<std::string, T> entries_;
viz│34│};
BwT│35│
fBy│36│} // namespace advanced
```

## Exercise Python decorators and async code

```bash
grepple -F "await self._execute" advanced-files/worker.py
```

The result attaches the comment and `@traced` decorator to the complete async `process` method rather than returning only the matching line.

## Try Go call navigation

The Go fixture contains an interface method and an injected function field. `--related` expands the matched `Deliver` method, reports both resolved calls under “Next points,” and shows its related type definition; it does not expand the callees' bodies. The same flags navigate syntax-backed declarations and calls in other supported code fixtures:

```bash
grepple --related -F "g.auditor.Record" advanced-files
```

```text
advanced-files/gateway.go


// … 25 lines collapsed …

HH1│26│// Deliver validates and forwards one item.
Fkb│27│// ADVANCED_DOC: failures include enough metadata for an audit trail.
FDV│28│func (g *Gateway[T]) Deliver(ctx context.Context, id string, item T) error {
Rsl│29│	if id == "" {
t-g│30│		return fmt.Errorf("id is required")
sQ6│31│	}
YwI│32│	if err := g.auditor.Record(ctx, "delivery.started", map[string]string{"id": id}); err != nil {
Iib│33│		return fmt.Errorf("record audit event: %w", err)
TTo│34│	}
0hT│35│	if err := g.deliver(ctx, item); err != nil {
yaf│36│		return fmt.Errorf("deliver %s: %w", id, err)
bVk│37│	}
4Me│38│	_ = "ADVANCED_END"
EC5│39│	return nil
H9-│40│}

Next points (code navigation):
  → Gateway  advanced-files/gateway.go:16-19  receiver-type:28
  → g.auditor.Record → Auditor.Record  advanced-files/gateway.go:12-12  call:32
  → g.deliver → Gateway.deliver  advanced-files/gateway.go:18-18  call:35

Related type definitions:

advanced-files/gateway.go:16-19  Gateway
1GJ│16│type Gateway[T any] struct {
c0w│17│	auditor Auditor
SKC│18│	deliver func(context.Context, T) error
hI6│19│}
```

The paths passed to the command define the declaration index. Search a directory or source glob when you want cross-file navigation; searching one file intentionally limits related points to that file. Use `grepple --at advanced-files/gateway.go:12-12` to retrieve a listed declaration exactly, or add `--follow-related 1` to inline up to two unique callees and show their own next points. Matching a function declaration also emits compact `←` caller locations.

## Run the fixtures as regression tests

From the repository root:

```bash
go test ./internal/parser -run AdvancedExamples
```

The tests verify language detection, useful nested outline symbols, leading-document attachment, and structural coverage through a match near the end of each declaration. TSX is allowed to compact setup statements while retaining the component documentation and matched JSX region.
