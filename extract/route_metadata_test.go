package extract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fiberRouteSource = `package service
import "github.com/gofiber/fiber/v2"
type Handler struct{}
func (h *Handler) Register(app *fiber.App) {
 app.Get("/health", h.health)
 app.All("/index", h.handleIndex)
}
func (h *Handler) health(c *fiber.Ctx) error { return nil }
func (h *Handler) handleIndex(c *fiber.Ctx) error { return nil }
`

func routeDiagram(directives string) string {
	return "classDiagram\n class Handler\n <<struct>> Handler\n " + directives + "\n"
}

func TestFiberRouteMetadataPasses(t *testing.T) {
	diagram := routeDiagram("%% grepple:route GET /health Handler.health\n %% pi:route ALL /index Handler.handleIndex")
	diagnostics, err := CheckClassDiagram(diagram, []Source{{Path: "service/routes.go", Text: fiberRouteSource}})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("route metadata: %v %+v", err, diagnostics)
	}
}

func TestFiberRouteMetadataRequiresExactRoute(t *testing.T) {
	tests := []struct {
		name, directive string
	}{
		{"method", "%% grepple:route POST /health Handler.health"},
		{"path", "%% grepple:route GET /ready Handler.health"},
		{"handler", "%% grepple:route GET /health Handler.handleIndex"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostics, err := CheckClassDiagram(routeDiagram(test.directive), []Source{{Path: "service/routes.go", Text: fiberRouteSource}})
			if err != nil {
				t.Fatal(err)
			}
			if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "Expected Fiber route") {
				t.Fatalf("got %+v", diagnostics)
			}
		})
	}
}

func TestFiberRouteMetadataRejectsMalformedAndDuplicate(t *testing.T) {
	for _, directive := range []string{
		"%% grepple:route get /health Handler.health",
		"%% grepple:route GET Handler.health",
		"%% grepple:route GET /health Handler",
		"%% grepple:route GET /health Handler.health extra",
	} {
		_, err := ParseClassDiagram(routeDiagram(directive))
		if err == nil || !strings.Contains(err.Error(), "malformed route directive") {
			t.Fatalf("%q: got %v", directive, err)
		}
	}

	duplicate := routeDiagram("%% grepple:route GET /health Handler.health\n %% pi:route GET /health Handler.health")
	_, err := ParseClassDiagram(duplicate)
	if err == nil || !strings.Contains(err.Error(), "duplicate route directive") {
		t.Fatalf("duplicate: got %v", err)
	}
}

func TestUnrelatedGetCallsAreNotFiberRoutes(t *testing.T) {
	source := `package service
import "github.com/gofiber/fiber/v2"
type Handler struct{}
type Client struct{}
func (c *Client) Get(path string, handler any) {}
func (h *Handler) Register(client *Client) { client.Get("/health", h.health) }
func (h *Handler) Local() { app := &Client{}; app.Get("/health", h.health) }
func (h *Handler) health(c *fiber.Ctx) error { return nil }
`
	diagnostics, err := CheckClassDiagram(routeDiagram("%% grepple:route GET /health Handler.health"), []Source{{Path: "service/routes.go", Text: source}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "Expected Fiber route") {
		t.Fatalf("got %+v", diagnostics)
	}
}

func TestFiberRouteMetadataUsesDeclaredTypePackageScope(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/routes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sources := []Source{
		{Path: filepath.Join(root, "a", "routes.go"), Text: "package service\ntype Handler struct{}\n"},
		{Path: filepath.Join(root, "b", "routes.go"), Text: fiberRouteSource},
	}
	diagram := "classDiagram\n class Handler\n <<struct>> Handler\n %% grepple:package Handler example.com/routes/a\n %% grepple:route GET /health Handler.health\n"
	diagnostics, err := CheckClassDiagram(diagram, sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "Expected Fiber route") {
		t.Fatalf("got %+v", diagnostics)
	}
}

func TestFiberRouteExtractionResolvesSupportedImportAliases(t *testing.T) {
	for _, module := range []string{"github.com/gofiber/fiber/v2", "github.com/gofiber/fiber/v3"} {
		source := `package service
		import web "` + module + `"
		type Handler struct{}
		func (h *Handler) Register(app *web.App) { app.Get("/health", h.health) }
		func (h *Handler) health(c *web.Ctx) error { return nil }`
		diagnostics, err := CheckClassDiagram(routeDiagram("%% grepple:route GET /health Handler.health"), []Source{{Path: "service/routes.go", Text: source}})
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("module %s: %v %+v", module, err, diagnostics)
		}
	}
}

func TestFiberRouteExtractionRejectsLookalikePackages(t *testing.T) {
	modules := []string{"example.com/fiber", "github.com/gofiber/fiber", "github.com/gofiber/fiber/v1", "github.com/gofiber/fiber/v4"}
	for _, module := range modules {
		source := `package service
		import fiber "` + module + `"
		type Handler struct{}
		func (h *Handler) Register(app *fiber.App) { app.Get("/health", h.health) }
		func (h *Handler) health(c *fiber.Ctx) error { return nil }`
		diagnostics, err := CheckClassDiagram(routeDiagram("%% grepple:route GET /health Handler.health"), []Source{{Path: "service/routes.go", Text: source}})
		if err != nil {
			t.Fatalf("module %s: %v", module, err)
		}
		if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "Expected Fiber route") {
			t.Fatalf("module %s: got %+v", module, diagnostics)
		}
	}
}

func TestFiberRouteImportsAreFileScoped(t *testing.T) {
	sources := []Source{
		{Path: "service/import.go", Text: `package service
		import fiber "github.com/gofiber/fiber/v2"
		func consume(*fiber.App) {}`},
		{Path: "service/routes.go", Text: `package service
		import fiber "example.com/fiber"
		type Handler struct{}
		func (h *Handler) Register(app *fiber.App) { app.Get("/health", h.health) }
		func (h *Handler) health(c *fiber.Ctx) error { return nil }`},
	}
	diagnostics, err := CheckClassDiagram(routeDiagram("%% grepple:route GET /health Handler.health"), sources)
	if err != nil || len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "Expected Fiber route") {
		t.Fatalf("got %v %+v", err, diagnostics)
	}
}

func TestFiberRouteExtractionTracksConservativeRouterProvenance(t *testing.T) {
	routesSource := `package service
	import (
		fiber2 "github.com/gofiber/fiber/v2"
		fiber3 "github.com/gofiber/fiber/v3"
		fake "example.com/fiber"
	)
	func publicRoutes(handler *Handler) {
		app := fiber2.New(fiber2.Config{})
		public := app.Group("/public", middleware)
		public.Get("/status", handler.Status)
		v2 := public.Group("/v2")
		v2copy := v2
		v2copy.Post("/items", handler.Create)
		v3 := v2copy.Group("/v3", middleware)
		v3.Patch("/items/:id", handler.Update)
		v3 = other
		v3.Delete("/ignored", handler.Delete)
		dynamic := app.Group(prefix)
		dynamic.Get("/ignored", handler.Status)
		public.Get(routePath, handler.Status)
		app.Use("/ignored", handler.Status)
		app.Mount("/ignored", app)
		app.Add("GET", "/ignored", handler.Status)
		custom := fake.New()
		custom.Get("/ignored", handler.Status)
		app.Get("/ignored", unresolved.Handle)
		app.Get("/ignored", standalone)
	}
	func v3Routes(handler *Handler) {
		app := fiber3.New()
		alias := app
		alias.Head("/public/v3", handler.Status)
	}
	func parameterRoutes(app *fiber2.App, handler *Handler) {
		copy := app
		copy.Put("/public/parameter", handler.Update)
	}
	`
	declarationsSource := `package service
	type Handler struct{}
	func (h *Handler) methodRoutes(app *fiber2.App) {
		group := app.Group("/public")
		group.Options("/method", h.Status)
	}
	func (h *Handler) Status() {}
	func (h *Handler) Create() {}
	func (h *Handler) Update() {}
	func (h *Handler) Delete() {}
	`
	// Imports are file-scoped, so the declaration file imports Fiber separately.
	declarationsSource = strings.Replace(declarationsSource, "package service", "package service\nimport fiber2 \"github.com/gofiber/fiber/v2\"", 1)
	sources := []Source{{Path: "service/routes.go", Text: routesSource}, {Path: "service/handler.go", Text: declarationsSource}}
	want := map[string]bool{
		"GET /public/status Handler.Status":            true,
		"POST /public/v2/items Handler.Create":         true,
		"PATCH /public/v2/v3/items/:id Handler.Update": true,
		"HEAD /public/v3 Handler.Status":               true,
		"PUT /public/parameter Handler.Update":         true,
		"OPTIONS /public/method Handler.Status":        true,
	}
	previousOrder := ""
	for _, ordered := range [][]Source{sources, {sources[1], sources[0]}} {
		analysis, err := Analyze(ordered)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		var order []string
		for _, route := range analysis.GoFiberRoutes {
			key := route.Method + " " + route.Path + " " + route.Handler
			got[key] = true
			order = append(order, key)
		}
		currentOrder := strings.Join(order, "\n")
		if previousOrder != "" && currentOrder != previousOrder {
			t.Fatalf("route order depends on source order:\n%s\n---\n%s", previousOrder, currentOrder)
		}
		previousOrder = currentOrder
		if len(got) != len(want) {
			t.Fatalf("routes = %+v, want %+v", got, want)
		}
		for route := range want {
			if !got[route] {
				t.Errorf("missing route %q from %+v", route, got)
			}
		}
	}
}
