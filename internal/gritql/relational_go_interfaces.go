// Package gritql evaluates bounded, source-authored structural programs and relations.
package gritql

import (
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"path"
	"strconv"
)

// goInterfaceFile indexes only source-written declarations and import names.
type goInterfaceFile struct {
	packageName string
	imports     map[string]string
	functions   map[int]*ast.FuncDecl
	interfaces  map[string]bool
}

func parseGoInterfaceFile(finding Finding) (goInterfaceFile, error) {
	set := token.NewFileSet()
	file, err := goparser.ParseFile(set, finding.Path(), finding.Text(), 0)
	if err != nil {
		return goInterfaceFile{}, fmt.Errorf("interface source %s: %w", finding.Path(), err)
	}
	out := goInterfaceFile{packageName: file.Name.Name, imports: map[string]string{}, functions: map[int]*ast.FuncDecl{}, interfaces: map[string]bool{}}
	if err := indexGoImports(file, &out, finding.Path()); err != nil {
		return out, err
	}
	indexGoDeclarations(file, &out, set, finding.StartByte())
	return out, nil
}

func goInterfaceFiles(sources []Finding, module string) (map[string]goInterfaceFile, map[string]string, error) {
	files := map[string]goInterfaceFile{}
	packages := map[string]string{}
	for _, source := range sources {
		file, err := parseGoInterfaceFile(source)
		if err != nil {
			return nil, nil, err
		}
		packagePath := module + "/" + path.Dir(source.Path())
		if name, ok := packages[packagePath]; ok && name != file.packageName {
			return nil, nil, fmt.Errorf("ambiguous package in %s", source.Path())
		}
		files[source.Path()] = file
		packages[packagePath] = file.packageName
	}
	return files, packages, nil
}

func goInterfaceDeclarations(right []Finding, files map[string]goInterfaceFile, module string, binding string) (map[string]bool, error) {
	known := map[string]bool{}
	for _, finding := range right {
		file, ok := files[finding.Path()]
		if !ok {
			return nil, fmt.Errorf("missing interface source %s", finding.Path())
		}
		name, err := goBindingLexeme(finding, binding)
		if err != nil {
			return nil, err
		}
		if _, present := file.interfaces[name]; !present {
			return nil, fmt.Errorf("%s: %q is not a literal interface declaration", finding.Path(), name)
		}
		known[module+"/"+path.Dir(finding.Path())+"."+name] = file.interfaces[name]
	}
	return known, nil
}

func goBindingLexeme(finding Finding, name string) (string, error) {
	for _, binding := range finding.Bindings() {
		if binding.Name() != name {
			continue
		}
		node, ok := binding.Node()
		if !ok || node.Lexeme() == "" {
			return "", fmt.Errorf("%s: %s must bind one identifier", finding.Path(), name)
		}
		return node.Lexeme(), nil
	}
	return "", fmt.Errorf("%s: missing binding %s", finding.Path(), name)
}

func goReturnedInterface(typ ast.Expr, file goInterfaceFile, packagePath string, known map[string]bool, packages map[string]string) bool {
	switch node := typ.(type) {
	case *ast.ParenExpr:
		return goReturnedInterface(node.X, file, packagePath, known, packages)
	case *ast.IndexExpr:
		return goReturnedInterface(node.X, file, packagePath, known, packages)
	case *ast.IndexListExpr:
		return goReturnedInterface(node.X, file, packagePath, known, packages)
	case *ast.InterfaceType:
		return len(node.Methods.List) > 0
	case *ast.Ident:
		return known[packagePath+"."+node.Name]
	case *ast.SelectorExpr:
		qualifier, ok := node.X.(*ast.Ident)
		if !ok {
			return false
		}
		for imported, alias := range file.imports {
			if alias == "" {
				alias = packages[imported]
			}
			if alias == qualifier.Name && known[imported+"."+node.Sel.Name] {
				return true
			}
		}
	}
	return false
}

func functionReturnsInterface(function *ast.FuncDecl, file goInterfaceFile, packagePath string, known map[string]bool, packages map[string]string) bool {
	if function.Type.Results == nil {
		return false
	}
	for _, field := range function.Type.Results.List {
		if !goResultTypeParameter(function, field.Type) && goReturnedInterface(field.Type, file, packagePath, known, packages) {
			return true
		}
	}
	return false
}

func joinGoInterfaceReturns(left, right, sources []Finding, spec RelationSpec) ([]RelationHit, error) {
	files, packages, err := goInterfaceFiles(sources, spec.GoModule)
	if err != nil {
		return nil, err
	}
	known, err := goInterfaceDeclarations(right, files, spec.GoModule, spec.RightKey.Binding)
	if err != nil {
		return nil, err
	}
	hits := []RelationHit{}
	for _, finding := range left {
		file, ok := files[finding.Path()]
		if !ok {
			return nil, fmt.Errorf("missing constructor source %s", finding.Path())
		}
		function, ok := file.functions[finding.StartByte()]
		if !ok || function.Recv != nil {
			return nil, fmt.Errorf("%s: interface result projection requires a top-level function", finding.Path())
		}
		if !functionReturnsInterface(function, file, spec.GoModule+"/"+path.Dir(finding.Path()), known, packages) {
			hits = append(hits, RelationHit{Left: finding})
		}
	}
	return hits, nil
}

func validateGoInterfaceReturns(spec RelationSpec) error {
	expectedMode := "unmatched_left_any"
	if spec.LeftKey.Projection == "go-method-signatures" {
		expectedMode = "unmatched_left"
	}
	if spec.GoModule == "" || spec.Scope != "repository" || spec.Mode != expectedMode || spec.Partition == nil || spec.Left.Language() != "go" || spec.Right.Language() != "go" || spec.Partition.Language() != "go" {
		return fmt.Errorf("go-interface-returns requires a Go module, repository unmatched_left_any, and complete Go source-file collector")
	}
	if spec.LeftKey.DescendantKind != "" || spec.RightKey.Projection != "" || spec.PartitionKey.Projection != "" {
		return fmt.Errorf("go-interface-returns does not support other key projections")
	}
	return nil
}

func indexGoImports(file *ast.File, out *goInterfaceFile, sourcePath string) error {
	for _, imp := range file.Imports {
		name := ""
		if imp.Name != nil {
			name = imp.Name.Name
		}
		value, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return err
		}
		if name == "." {
			return fmt.Errorf("interface projection cannot resolve dot imports in %s", sourcePath)
		}
		out.imports[value] = name
	}
	return nil
}

func indexGoDeclarations(file *ast.File, out *goInterfaceFile, set *token.FileSet, offset int) {
	for _, declaration := range file.Decls {
		switch node := declaration.(type) {
		case *ast.FuncDecl:
			out.functions[set.Position(node.Pos()).Offset+offset] = node
		case *ast.GenDecl:
			for _, spec := range node.Specs {
				typ, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if iface, ok := typ.Type.(*ast.InterfaceType); ok {
					out.interfaces[typ.Name.Name] = len(iface.Methods.List) > 0
				}
			}
		}
	}
}

func goResultTypeParameter(function *ast.FuncDecl, typ ast.Expr) bool {
	id, ok := typ.(*ast.Ident)
	if !ok || function.Type.TypeParams == nil {
		return false
	}
	for _, field := range function.Type.TypeParams.List {
		for _, name := range field.Names {
			if name.Name == id.Name {
				return true
			}
		}
	}
	return false
}
