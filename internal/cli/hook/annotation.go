// Package hook runs repository-owned, read-only GritQL checks.
package hook

import (
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"io"
	"os"
	"strings"
)

// grepple: infrastructure
// annotationConfig adds exact attached Go declaration comments to GritQL selection.
type annotationConfig struct {
	Prefix string   `yaml:"prefix"`
	Values []string `yaml:"values"`
}

// grepple: entity
// annotationPosition identifies a selected syntax node, not a lexical type name.
type annotationPosition struct{ line, column int }

// grepple: entity
// declarationAnnotation retains only named, directly declared literal type evidence.
type declarationAnnotation struct{ comments *ast.CommentGroup }

func validateAnnotationConfig(r rule) error {
	a := r.Annotation
	if a == nil {
		return nil
	}
	if r.Engine != "gritql-v1" {
		return fmt.Errorf("annotation requires the file-local gritql-v1 engine")
	}
	if !strings.HasPrefix(a.Prefix, "//") || len(a.Prefix) < 3 || len(a.Prefix) > 128 || strings.ContainsAny(a.Prefix, "\r\n") {
		return fmt.Errorf("annotation requires a short line-comment prefix")
	}
	if len(a.Values) == 0 || len(a.Values) > 32 {
		return fmt.Errorf("annotation requires 1-32 allowed values")
	}
	seen := map[string]bool{}
	for _, v := range a.Values {
		if !hookID.MatchString(v) || seen[v] {
			return fmt.Errorf("annotation values must be unique lowercase identifiers")
		}
		seen[v] = true
	}
	return nil
}

func filterAnnotatedFindings(root string, findings []Finding, rules []compiledRule) ([]Finding, error) {
	configs := annotationRules(rules)
	if len(configs) == 0 || len(findings) == 0 {
		return findings, nil
	}
	repository, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer repository.Close()
	byPath := map[string]map[annotationPosition]declarationAnnotation{}
	filtered := make([]Finding, 0, len(findings))
	for _, f := range findings {
		a := configs[f.ID]
		if a == nil {
			filtered = append(filtered, f)
			continue
		}
		declarations, ok := byPath[f.Path]
		if !ok {
			declarations, err = readDeclarationAnnotations(repository, f.Path)
			if err != nil {
				return nil, fmt.Errorf("hook %s: incomplete annotation scan: %w", f.ID, err)
			}
			byPath[f.Path] = declarations
		}
		d, named := declarations[annotationPosition{f.Line, f.Column}]
		if !named || !validDeclarationAnnotation(d.comments, a) {
			filtered = append(filtered, f)
		}
	}
	return filtered, nil
}

func readDeclarationAnnotations(root *os.Root, path string) (map[annotationPosition]declarationAnnotation, error) {
	file, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxCachedSourceBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(data) > maxCachedSourceBytes {
		return nil, fmt.Errorf("annotation source %s exceeds byte limit", path)
	}
	positions := token.NewFileSet()
	tree, err := goparser.ParseFile(positions, path, data, goparser.ParseComments|goparser.AllErrors|goparser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	return collectDeclarationAnnotations(positions, tree), nil
}

func collectDeclarationAnnotations(positions *token.FileSet, tree *ast.File) map[annotationPosition]declarationAnnotation {
	out := map[annotationPosition]declarationAnnotation{}
	ast.Inspect(tree, func(n ast.Node) bool {
		decl, ok := n.(*ast.GenDecl)
		if !ok || decl.Tok != token.TYPE {
			return true
		}
		for _, spec := range decl.Specs {
			t, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			literal := classifiedLiteral(t.Type)
			if literal == nil {
				continue
			}
			comments := t.Doc
			// A group-level marker must not accidentally classify every group member.
			if comments == nil && !decl.Lparen.IsValid() && len(decl.Specs) == 1 {
				comments = decl.Doc
			}
			p := positions.PositionFor(literal.Pos(), false)
			out[annotationPosition{p.Line, p.Column}] = declarationAnnotation{comments: comments}
		}
		return true
	})
	return out
}
func classifiedLiteral(expr ast.Expr) ast.Expr {
	switch e := expr.(type) {
	case *ast.StructType, *ast.InterfaceType:
		return e
	case *ast.ParenExpr:
		return classifiedLiteral(e.X)
	}
	return nil
}
func validDeclarationAnnotation(group *ast.CommentGroup, a *annotationConfig) bool {
	if group == nil {
		return false
	}
	count := 0
	valid := false
	prefix := annotationCommentText(a.Prefix)
	for _, comment := range group.List {
		text := annotationCommentText(comment.Text)
		if !strings.HasPrefix(text, prefix) {
			continue
		}
		count++
		value := strings.TrimSpace(strings.TrimPrefix(text, prefix))
		valid = false
		for _, allowed := range a.Values {
			if value == allowed {
				valid = true
			}
		}
	}
	return count == 1 && valid
}

// gofmt inserts whitespace after // for prose-like directives with a space
// after the colon. Normalize only that opening whitespace, not the marker body.
func annotationCommentText(text string) string {
	if !strings.HasPrefix(text, "//") {
		return text
	}
	return "//" + strings.TrimLeft(text[2:], " \t")
}
