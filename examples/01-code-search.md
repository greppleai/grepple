# Code search (structural context)

For parser-backed code, a match appears in its containing declaration with anchored `HASH│LINE│content` rows. When available, search also shows related calls and type evidence. Unrelated lines collapse to `// … N lines collapsed …` rather than printing the whole file.

## Go

Searching for a type used across several definitions:

```bash
grepple "http.HandlerFunc" sample-files/server.go
```

```text
sample-files/server.go


// … 13 lines collapsed …

zpE│14│// Server wraps the standard library HTTP server.
FQS│15│type Server struct {
Zhj│16│	cfg    Config
Xka│17│	routes map[string]http.HandlerFunc
hI6│18│}

XHd│20│// NewServer builds a Server from the given config.
ah3│21│func NewServer(cfg Config) *Server {
m5S│22│	return &Server{cfg: cfg, routes: map[string]http.HandlerFunc{}}
DVO│23│}

dcR│25│// Handle registers a handler for a path.
t8r│26│func (s *Server) Handle(path string, fn http.HandlerFunc) {
PCM│27│	s.routes[path] = fn
H9-│28│}

Next points (code navigation):
  → Config  sample-files/server.go:9-12  parameter-type:21
  → Server  sample-files/server.go:15-18  result-type:20
  → Server  sample-files/server.go:15-18  receiver-type:26

Related type definitions:

sample-files/server.go:9-12  Config
Rj1│9│type Config struct {
F3r│10│	Addr    string
swh│11│	Timeout int
AUS│12│}

sample-files/server.go:15-18  Server
FQS│15│type Server struct {
Zhj│16│	cfg    Config
Xka│17│	routes map[string]http.HandlerFunc
hI6│18│}
```

Only the matching declarations expand here; `Config` appears separately under related type definitions, with anchored source lines.

## TypeScript / TSX

The enclosing component function and its JSX are kept together:

```bash
grepple "useState" sample-files/Button.tsx
```

```text
sample-files/Button.tsx

lLp│1│import { useState } from "react";

// … 6 lines collapsed …

B_R│8│export function Button({ label, onPress }: ButtonProps) {
W2q│9│  const [pressed, setPressed] = useState(false);
2lC│10│
qtG│11│  function handleClick() {
M_S│12│    setPressed(true);
Rnu│13│    onPress?.();
IgR│14│  }
miP│15│
CyQ│16│  return (
KBE│17│    <button aria-pressed={pressed} onClick={handleClick}>
Fme│18│      {label}
lbu│19│    </button>
vRI│20│  );
AUS│21│}

Next points (code navigation):
  → ButtonProps  sample-files/Button.tsx:3-6  parameter-type:8
  → useState  dependency:react  call:9 [dependency-unresolved; react]

Related type definitions:

sample-files/Button.tsx:3-6  ButtonProps
tzZ│3│type ButtonProps = {
eYb│4│  label: string;
MMm│5│  onPress?: () => void;
Y3a│6│};
```

## Java

The matching method appears inside its enclosing class; unrelated members are omitted as collapsed spans:

```bash
grepple "mapToDouble" sample-files/Order.java
```

```text
sample-files/Order.java


// … 4 lines collapsed …

jbB│5│public class Order {

// … 10 lines collapsed …

qfN│16│    public double total() {
3ym│17│        return items.stream()
RrF│18│            .mapToDouble(item -> item.quantity() * item.price())
Bub│19│            .sum();
xxs│20│    }

// … 4 lines collapsed …

AUS│25│}
```

## Kotlin

The matching `restock` function is expanded inside its class; unrelated functions collapse away:

```bash
grepple "copy\(stock" sample-files/Inventory.kt
```

```text
sample-files/Inventory.kt


// … 4 lines collapsed …

8Se│5│class Inventory(private val products: MutableList<Product>) {

// … 3 lines collapsed …

7Lr│9│    fun restock(sku: String, amount: Int) {
cgd│10│        val index = products.indexOfFirst { it.sku == sku }
KYT│11│        if (index >= 0) {
eXC│12│            val current = products[index]
TSY│13│            products[index] = current.copy(stock = current.stock + amount)
k1_│14│        }
cOK│15│    }

// … 3 lines collapsed …

AUS│19│}
```
