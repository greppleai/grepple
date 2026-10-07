//go:build duckdb

package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

// Every fact field is a native typed DuckDB column, not JSON or an encoded blob.
// _ord preserves original order and _key is the relevant indexed lookup key.
type columnDuckStore struct {
	*duckStore
	counts map[string]int64
}

var columnQueries = map[string]string{
	"types":    `SELECT * EXCLUDE (_ord,_key) FROM types WHERE _key IN (SELECT unnest(?::VARCHAR[])) ORDER BY _ord`,
	"imports":  `SELECT * EXCLUDE (_ord,_key) FROM imports WHERE _key IN (SELECT unnest(?::VARCHAR[])) ORDER BY _ord`,
	"exports":  `SELECT * EXCLUDE (_ord,_key) FROM exports WHERE _key IN (SELECT unnest(?::VARCHAR[])) ORDER BY _ord`,
	"usages":   `SELECT * EXCLUDE (_ord,_key) FROM usages WHERE _key IN (SELECT unnest(?::VARCHAR[])) ORDER BY _ord`,
	"members":  `SELECT * EXCLUDE (_ord,_key) FROM members WHERE _key IN (SELECT unnest(?::VARCHAR[])) ORDER BY _ord`,
	"existing": `SELECT DISTINCT _key FROM declarations WHERE _key IN (SELECT unnest(?::VARCHAR[]))`,
	"defs":     `SELECT * EXCLUDE (_ord,_key) FROM declarations WHERE _key IN (SELECT unnest(?::VARCHAR[])) ORDER BY _ord`,
	"calls":    `SELECT * EXCLUDE (_ord,_key) FROM calls WHERE _ord IN (SELECT unnest(?::BIGINT[])) ORDER BY _ord`,
	"out":      `SELECT _ord,_key,"TargetID","CandidateTargetIDs" FROM calls WHERE _key IN (SELECT unnest(?::VARCHAR[])) ORDER BY _ord`,
	"in":       `SELECT c._ord,c._key,c."TargetID",c."CandidateTargetIDs" FROM calls c JOIN (SELECT DISTINCT call_ord FROM targets WHERE target IN (SELECT unnest(?::VARCHAR[]))) t ON c._ord=t.call_ord ORDER BY c._ord`,
	"fields":   `SELECT * EXCLUDE (_ord,_key) FROM fields WHERE _key IN (SELECT unnest(?::VARCHAR[])) ORDER BY _ord`,
}

func openColumnDuck(path string) (queryIndex, error) {
	db, err := sql.Open("duckdb", path+"?access_mode=read_only&threads=1&memory_limit=512MB")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	base := &duckStore{db: db, stmts: map[string]*sql.Stmt{}, queries: columnQueries}
	if err = db.Ping(); err != nil {
		base.Close()
		return nil, err
	}
	rows, err := db.Query("SELECT root FROM roots ORDER BY ord")
	if err != nil {
		base.Close()
		return nil, err
	}
	for rows.Next() {
		var root string
		if err = rows.Scan(&root); err != nil {
			rows.Close()
			base.Close()
			return nil, err
		}
		base.roots = append(base.roots, root)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		base.Close()
		return nil, err
	}
	for key, text := range base.queries {
		stmt, e := db.Prepare(text)
		if e != nil {
			base.Close()
			return nil, e
		}
		base.stmts[key] = stmt
	}
	counts := map[string]int64{}
	rows, err = db.Query("SELECT name,n FROM fact_counts")
	if err != nil {
		base.Close()
		return nil, err
	}
	for rows.Next() {
		var name string
		var n int64
		if err = rows.Scan(&name, &n); err != nil {
			rows.Close()
			base.Close()
			return nil, err
		}
		counts[name] = n
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		base.Close()
		return nil, err
	}
	return &indexedQuery{store: &columnDuckStore{duckStore: base, counts: counts}}, nil
}

func (s *columnDuckStore) Calls(ctx context.Context, ids []string, dir navigation.NavigationQueryDirection) ([]rowCall, error) {
	selected := map[int]rowCall{}
	directions := []string{}
	if dir != navigation.NavigationQueryCallers && dir != navigation.NavigationQueryDependents {
		directions = append(directions, "out")
	}
	if dir != navigation.NavigationQueryCallees && dir != navigation.NavigationQueryDependencies {
		directions = append(directions, "in")
	}
	for _, key := range directions {
		rows, err := s.queryRows(ctx, key, ids)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var ord int
			var caller, target string
			var candidates any
			if err = rows.Scan(&ord, &caller, &target, &candidates); err != nil {
				rows.Close()
				return nil, err
			}
			row := rowCall{Ord: ord, Call: parser.NavigationCall{CallerID: caller, TargetID: target}}
			if candidates != nil {
				for _, v := range candidates.([]any) {
					row.Call.CandidateTargetIDs = append(row.Call.CandidateTargetIDs, v.(string))
				}
			}
			selected[ord] = row
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	ords := map[int]bool{}
	for ord := range selected {
		ords[ord] = true
	}
	result := []rowCall{}
	for _, ord := range sortedOrdinals(ords) {
		result = append(result, selected[ord])
	}
	return result, nil
}

func columnFacts[T any](ctx context.Context, s *columnDuckStore, key string, ids any) ([]T, error) {
	table := key
	if key == "defs" {
		table = "declarations"
	}
	if n, ok := s.counts[table]; ok && n == 0 {
		return []T{}, nil
	}
	if reflect.ValueOf(ids).Len() == 0 {
		return []T{}, nil
	}

	rows, err := s.queryRows(ctx, key, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []T{}
	for rows.Next() {
		var item T
		value := reflect.ValueOf(&item).Elem()
		destinations := make([]any, value.NumField())
		lists := make([]any, value.NumField())
		for i := range destinations {
			if value.Field(i).Kind() == reflect.Slice {
				destinations[i] = &lists[i]
			} else {
				destinations[i] = value.Field(i).Addr().Interface()
			}
		}
		if err = rows.Scan(destinations...); err != nil {
			return nil, err
		}
		for i, raw := range lists {
			if raw == nil {
				continue
			}
			values, ok := raw.([]any)
			if !ok {
				return nil, fmt.Errorf("invalid native list: %T", raw)
			}
			slice := reflect.MakeSlice(value.Field(i).Type(), len(values), len(values))
			for j, v := range values {
				str, ok := v.(string)
				if !ok {
					return nil, fmt.Errorf("invalid native string list item: %T", v)
				}
				slice.Index(j).SetString(str)
			}
			value.Field(i).Set(slice)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *columnDuckStore) Fragment(ctx context.Context, ids []string, ords []int) (parser.NavigationGraph, error) {
	graph := emptyGraph()
	graph.RepositoryRoots = s.roots
	var err error
	graph.Declarations, err = columnFacts[parser.NavigationDeclaration](ctx, s, "defs", ids)
	if err != nil {
		return graph, err
	}
	nums := make([]int64, len(ords))
	for i, ord := range ords {
		nums[i] = int64(ord)
	}
	graph.Calls, err = columnFacts[parser.NavigationCall](ctx, s, "calls", nums)
	if err != nil {
		return graph, err
	}
	owners, files := contextKeys(graph)
	graph.Fields, err = columnFacts[parser.NavigationField](ctx, s, "fields", keys(owners))
	if err != nil {
		return graph, err
	}
	for _, field := range graph.Fields {
		files[field.Path] = true
	}
	paths := keys(files)
	graph.TypeDeclarations, err = columnFacts[parser.NavigationTypeDeclaration](ctx, s, "types", paths)
	if err != nil {
		return graph, err
	}
	graph.Imports, err = columnFacts[parser.NavigationImport](ctx, s, "imports", paths)
	if err != nil {
		return graph, err
	}
	graph.Exports, err = columnFacts[parser.NavigationExport](ctx, s, "exports", paths)
	if err != nil {
		return graph, err
	}
	graph.TypeUsages, err = columnFacts[parser.NavigationTypeUsage](ctx, s, "usages", ids)
	if err != nil {
		return graph, err
	}
	graph.MemberAccesses, err = columnFacts[parser.NavigationMemberAccess](ctx, s, "members", ids)
	if err != nil {
		return graph, err
	}

	return graph, nil
}

func columnRecordSchema(t reflect.Type) (string, error) {
	fields := []string{}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		kind := ""
		switch field.Type.Kind() {
		case reflect.String:
			kind = "VARCHAR"
		case reflect.Int:
			kind = "BIGINT"
		case reflect.Bool:
			kind = "BOOLEAN"
		case reflect.Slice:
			if field.Type.Elem().Kind() == reflect.String {
				kind = "VARCHAR[]"
			}
		}
		if kind == "" {
			return "", fmt.Errorf("unsupported native field %s.%s: %s", t.Name(), field.Name, field.Type)
		}
		fields = append(fields, `"`+field.Name+`" `+kind)
	}
	return strings.Join(fields, ","), nil
}

func encodeColumnValues(item reflect.Value) []driver.Value {
	result := make([]driver.Value, item.NumField())
	for i := 0; i < item.NumField(); i++ {
		field := item.Field(i)
		switch field.Kind() {
		case reflect.String:
			result[i] = field.String()
		case reflect.Int:
			result[i] = field.Int()
		case reflect.Bool:
			result[i] = field.Bool()
		case reflect.Slice:
			if !field.IsNil() {
				list := make([]any, field.Len())
				for j := range list {
					list[j] = field.Index(j).String()
				}
				result[i] = list
			}
		}
	}
	return result
}

func buildColumnDuck(path string, graph parser.NavigationGraph) (time.Duration, error) {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return 0, fmt.Errorf("database path already exists: %s", path)
	}
	start := time.Now()
	db, err := sql.Open("duckdb", path+"?threads=1&memory_limit=512MB")
	if err != nil {
		return 0, err
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	collections := []struct {
		name string
		data any
		key  func(reflect.Value) string
	}{
		{"declarations", graph.Declarations, func(v reflect.Value) string { return v.FieldByName("ID").String() }},
		{"calls", graph.Calls, func(v reflect.Value) string { return v.FieldByName("CallerID").String() }},
		{"fields", graph.Fields, func(v reflect.Value) string { return terminal(v.FieldByName("OwnerType").String()) }},
		{"types", graph.TypeDeclarations, func(v reflect.Value) string { return v.FieldByName("Path").String() }},
		{"imports", graph.Imports, func(v reflect.Value) string { return v.FieldByName("Path").String() }},
		{"exports", graph.Exports, func(v reflect.Value) string { return v.FieldByName("Path").String() }},
		{"usages", graph.TypeUsages, func(v reflect.Value) string { return v.FieldByName("CallerID").String() }},
		{"members", graph.MemberAccesses, func(v reflect.Value) string { return v.FieldByName("CallerID").String() }},
	}
	if _, err = db.Exec("CREATE TABLE fact_counts(name VARCHAR,n BIGINT)"); err != nil {
		return 0, err
	}
	for _, collection := range collections {
		values := reflect.ValueOf(collection.data)
		if _, err = db.Exec("INSERT INTO fact_counts VALUES (?,?)", collection.name, values.Len()); err != nil {
			return 0, err
		}
		schema, e := columnRecordSchema(values.Type().Elem())
		if e != nil {
			return 0, e
		}
		if _, err = db.Exec("CREATE TABLE " + collection.name + "(_ord BIGINT,_key VARCHAR," + schema + ")"); err != nil {
			return 0, err
		}
		conn, e := db.Conn(context.Background())
		if e != nil {
			return 0, e
		}
		e = conn.Raw(func(raw any) error {
			appender, e := duckdb.NewAppenderFromConn(raw.(driver.Conn), "", collection.name)
			if e != nil {
				return e
			}
			for i := 0; i < values.Len(); i++ {
				item := values.Index(i)
				if e = appender.AppendRow(append([]driver.Value{int64(i), collection.key(item)}, encodeColumnValues(item)...)...); e != nil {
					appender.Close()
					return e
				}
			}
			return appender.Close()
		})
		conn.Close()
		if e != nil {
			return 0, e
		}
		if _, err = db.Exec("CREATE INDEX " + collection.name + "_key ON " + collection.name + "(_key)"); err != nil {
			return 0, err
		}
	}
	if _, err = db.Exec(`CREATE INDEX call_ord ON calls(_ord); CREATE TABLE targets AS SELECT _ord call_ord,unnest(CASE WHEN "TargetID"<>'' THEN ["TargetID"] ELSE "CandidateTargetIDs" END) AS target FROM calls; CREATE INDEX target_id ON targets(target); CREATE TABLE roots(ord BIGINT,root VARCHAR)`); err != nil {
		return 0, err
	}
	for i, root := range graph.RepositoryRoots {
		if _, err = db.Exec("INSERT INTO roots VALUES (?,?)", i, root); err != nil {
			return 0, err
		}
	}
	if _, err = db.Exec("CHECKPOINT"); err != nil {
		return 0, err
	}
	if err = db.Close(); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}
