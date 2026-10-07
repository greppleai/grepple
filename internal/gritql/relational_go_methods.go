// Package gritql evaluates bounded, source-authored structural queries.
package gritql

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	goparser "go/parser"
	"go/token"
	"path"
	"strings"
)

func goImportName(file goInterfaceFile, qualifier string, packages map[string]string) string {
	for imported, alias := range file.imports {
		if alias == "" {
			alias = packages[imported]
			if alias == "" {
				alias = path.Base(imported)
				if len(alias) > 1 && alias[0] == 'v' && alias[1] >= '0' && alias[1] <= '9' {
					alias = path.Base(path.Dir(imported))
				}
			}
		}
		if alias == qualifier {
			return imported
		}
	}
	return ""
}

func normalizedGoType(expr ast.Expr, file goInterfaceFile, packagePath string, packages map[string]string) (string, error) {
	if ellipsis, ok := expr.(*ast.Ellipsis); ok {
		value, err := normalizedGoType(ellipsis.Elt, file, packagePath, packages)
		return "..." + value, err
	}
	var raw bytes.Buffer
	if err := format.Node(&raw, token.NewFileSet(), expr); err != nil {
		return "", err
	}
	copyExpr, err := goparser.ParseExpr(raw.String())
	if err != nil {
		return "", err
	}
	ast.Inspect(copyExpr, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SelectorExpr:
			id, ok := node.X.(*ast.Ident)
			if !ok {
				return true
			}
			imported := goImportName(file, id.Name, packages)
			if imported == "" {
				imported = "unresolved:" + packagePath + ":" + id.Name
			}
			node.X = &ast.Ident{Name: imported}
			return false
		case *ast.Ident:
			if !isGoPredeclaredType(node.Name) {
				node.Name = packagePath + "." + node.Name
			}
		}
		return true
	})
	raw.Reset()
	err = format.Node(&raw, token.NewFileSet(), copyExpr)
	return raw.String(), err
}

func isGoPredeclaredType(name string) bool {
	switch name {
	case "bool", "string", "byte", "rune", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "float32", "float64", "complex64", "complex128", "error", "any":
		return true
	}
	return false
}

func normalizedGoFields(fields *ast.FieldList, file goInterfaceFile, packagePath string, packages map[string]string) (string, error) {
	if fields == nil {
		return "", nil
	}
	values := []string{}
	for _, field := range fields.List {
		typ, err := normalizedGoType(field.Type, file, packagePath, packages)
		if err != nil {
			return "", err
		}
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for range count {
			values = append(values, typ)
		}
	}
	return strings.Join(values, ","), nil
}

func normalizedGoMethod(name string, typ *ast.FuncType, file goInterfaceFile, packagePath string, packages map[string]string) (string, error) {
	params, err := normalizedGoFields(typ.Params, file, packagePath, packages)
	if err != nil {
		return "", err
	}
	results, err := normalizedGoFields(typ.Results, file, packagePath, packages)
	if err != nil {
		return "", err
	}
	return name + "(" + params + ")(" + results + ")", nil
}

func projectedGoMethod(finding Finding, files map[string]goInterfaceFile, packages map[string]string, module string, interfaceSide bool) (string, error) {
	file, ok := files[finding.Path()]
	if !ok {
		return "", fmt.Errorf("missing method source %s", finding.Path())
	}
	packagePath := module + "/" + path.Dir(finding.Path())
	if !interfaceSide {
		function, ok := file.functions[finding.StartByte()]
		if !ok || function.Recv == nil {
			return "", fmt.Errorf("%s: expected method declaration", finding.Path())
		}
		return normalizedGoMethod(function.Name.Name, function.Type, file, packagePath, packages)
	}
	// The grammar-selected method element is reparsed in a minimal interface.
	parsed, err := goparser.ParseFile(token.NewFileSet(), finding.Path(), "package p\ntype contract interface {\n"+finding.Text()+"\n}", 0)
	if err != nil {
		return "", err
	}
	typ := parsed.Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Type.(*ast.InterfaceType)
	if len(typ.Methods.List) != 1 || len(typ.Methods.List[0].Names) != 1 {
		return "", fmt.Errorf("expected one named interface method")
	}
	method := typ.Methods.List[0]
	signature, ok := method.Type.(*ast.FuncType)
	if !ok {
		return "", fmt.Errorf("expected interface method signature")
	}
	return normalizedGoMethod(method.Names[0].Name, signature, file, packagePath, packages)
}

func joinGoMethodSignatures(left, right, sources []Finding, spec RelationSpec) ([]RelationHit, error) {
	files, packages, err := goInterfaceFiles(sources, spec.GoModule)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{"Error()(string)": true} // The predeclared Go error interface.
	for _, finding := range right {
		key, err := projectedGoMethod(finding, files, packages, spec.GoModule, true)
		if err != nil {
			return nil, err
		}
		known[key] = true
	}
	hits := []RelationHit{}
	for _, finding := range left {
		key, err := projectedGoMethod(finding, files, packages, spec.GoModule, false)
		if err != nil {
			return nil, err
		}
		if !known[key] {
			hits = append(hits, RelationHit{Left: finding})
		}
	}
	return hits, nil
}
