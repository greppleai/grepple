# Code search (structural context)

For tree-sitter languages (Go, TypeScript/TSX, JavaScript/JSX, Java, Kotlin), a match
is rendered inside its enclosing definition. Unrelated lines are replaced with a
`// … N lines collapsed …` marker so you see the match *in context* without the whole
file.

## Go

Searching for a type used across several definitions:

```bash
grepple "http.HandlerFunc" sample-files/server.go
```

```text
sample-files/server.go


// … 8 lines collapsed …

 9   type Config struct { … }

// … 2 lines collapsed …

15   type Server struct {
16   	cfg    Config
17   	routes map[string]http.HandlerFunc
18   }

// … 2 lines collapsed …

21   func NewServer(cfg Config) *Server {
22   	return &Server{cfg: cfg, routes: map[string]http.HandlerFunc{}}
23   }

// … 2 lines collapsed …

26   func (s *Server) Handle(path string, fn http.HandlerFunc) {
27   	s.routes[path] = fn
28   }

// … 2 lines collapsed …

31   func (s *Server) ListenAndServe() error { … }
```

Definitions that do not contain the match (like `Config`) collapse to a one-line
summary (`type Config struct { … }`), while the ones that do are expanded.

## TypeScript / TSX

The enclosing component function and its JSX are kept together:

```bash
grepple "useState" sample-files/Button.tsx
```

```text
sample-files/Button.tsx

 1   import { useState } from "react";

// … 1 line collapsed …

 3   type ButtonProps = { … }

// … 1 line collapsed …

 8   export function Button({ label, onPress }: ButtonProps) {
 9     const [pressed, setPressed] = useState(false);
10
11     function handleClick() {
12       setPressed(true);
13       onPress?.();
14     }
15
16     return (
17       <button aria-pressed={pressed} onClick={handleClick}>
18         {label}
19       </button>
20     );
21   }
```

## Java

The enclosing class and method are kept; sibling members that don't match
collapse to one-line summaries (`public Order(...) { … }`):

```bash
grepple "mapToDouble" sample-files/Order.java
```

```text
sample-files/Order.java


// … 2 lines collapsed …

 3   import java.util.List;

// … 1 line collapsed …

 5   public class Order {
 6       public record LineItem(String sku, int quantity, double price) {}

// … 1 line collapsed …

 8       private final String id;
 9       private final List<LineItem> items;

// … 1 line collapsed …

11       public Order(String id, List<LineItem> items) { … }

// … 1 line collapsed …

16       public double total() {
17           return items.stream()
18               .mapToDouble(item -> item.quantity() * item.price())
19               .sum();
20       }

// … 1 line collapsed …

22       public String describe() { … }
25   }
```

## Kotlin

Expression-bodied functions that don't match collapse to `fun name(...) = …`,
while the matched function is expanded in full:

```bash
grepple "copy\(stock" sample-files/Inventory.kt
```

```text
sample-files/Inventory.kt


// … 2 lines collapsed …

 3   data class Product(val sku: String, val name: String, val stock: Int)

// … 1 line collapsed …

 5   class Inventory(private val products: MutableList<Product>) {
 6       fun find(sku: String): Product? = …

// … 1 line collapsed …

 9       fun restock(sku: String, amount: Int) {
10           val index = products.indexOfFirst { it.sku == sku }
11           if (index >= 0) {
12               val current = products[index]
13               products[index] = current.copy(stock = current.stock + amount)
14           }
15       }

// … 1 line collapsed …

17       fun lowStock(threshold: Int): List<Product> = …
19   }
```
