//go:build duckdb

package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

type duckStore struct {
	queries map[string]string
	mu      sync.Mutex
	db      *sql.DB
	stmts   map[string]*sql.Stmt
	roots   []string
}

var storageSQL = map[string]string{
	"existing":      "SELECT DISTINCT id FROM declarations WHERE id IN (SELECT unnest(?::VARCHAR[]))",
	"incoming-ords": "SELECT DISTINCT call_ord FROM targets WHERE target IN (SELECT unnest(?::VARCHAR[])) ORDER BY call_ord",
	"call-rows":     "SELECT ord,payload FROM calls WHERE ord IN (SELECT unnest(?::BIGINT[])) ORDER BY ord",
	"defs":          "SELECT payload FROM declarations WHERE id IN (SELECT unnest(?::VARCHAR[])) ORDER BY ord",
	"calls":         "SELECT payload FROM calls WHERE ord IN (SELECT unnest(?::BIGINT[])) ORDER BY ord",
	"out":           "SELECT ord,payload FROM calls WHERE caller IN (SELECT unnest(?::VARCHAR[])) ORDER BY ord",
	"in":            "SELECT DISTINCT c.ord,c.payload FROM calls c JOIN targets t ON t.call_ord=c.ord WHERE t.target IN (SELECT unnest(?::VARCHAR[])) ORDER BY c.ord",
	"both":          "SELECT DISTINCT c.ord,c.payload FROM calls c JOIN targets t ON t.call_ord=c.ord WHERE c.caller IN (SELECT unnest(?::VARCHAR[])) OR t.target IN (SELECT unnest(?::VARCHAR[])) ORDER BY c.ord",
	"fields":        "SELECT payload FROM fields WHERE owner IN (SELECT unnest(?::VARCHAR[])) ORDER BY ord",
	"types":         "SELECT payload FROM types WHERE path IN (SELECT unnest(?::VARCHAR[])) ORDER BY ord",
	"imports":       "SELECT payload FROM imports WHERE path IN (SELECT unnest(?::VARCHAR[])) ORDER BY ord",
	"exports":       "SELECT payload FROM exports WHERE path IN (SELECT unnest(?::VARCHAR[])) ORDER BY ord",
	"usages":        "SELECT payload FROM usages WHERE caller IN (SELECT unnest(?::VARCHAR[])) ORDER BY ord",
	"members":       "SELECT payload FROM members WHERE caller IN (SELECT unnest(?::VARCHAR[])) ORDER BY ord",
}

func openNativeDuck(path string) (queryIndex, error) {
	db, err := sql.Open("duckdb", path+"?access_mode=read_only&threads=1&memory_limit=512MB")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &duckStore{db: db, stmts: map[string]*sql.Stmt{}, queries: storageSQL}
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	var encoded string
	if err = db.QueryRow("SELECT payload FROM metadata WHERE key='roots'").Scan(&encoded); err != nil {
		s.Close()
		return nil, err
	}
	if err = json.Unmarshal([]byte(encoded), &s.roots); err != nil {
		s.Close()
		return nil, err
	}
	for key, text := range storageSQL {
		stmt, e := db.Prepare(text)
		if e != nil {
			s.Close()
			return nil, e
		}
		s.stmts[key] = stmt
	}
	return &indexedQuery{store: s}, nil
}
func (s *duckStore) Close() error {
	for _, stmt := range s.stmts {
		stmt.Close()
	}
	return s.db.Close()
}
func (s *duckStore) Calls(ctx context.Context, frontier []string, dir navigation.NavigationQueryDirection) ([]rowCall, error) {
	selected := map[int]rowCall{}
	if dir != navigation.NavigationQueryCallers && dir != navigation.NavigationQueryDependents {
		rows, err := s.queryRows(ctx, "out", frontier)
		if err != nil {
			return nil, err
		}
		calls, err := readCallRows(rows)
		if err != nil {
			return nil, err
		}
		for _, call := range calls {
			selected[call.Ord] = call
		}
	}
	if dir != navigation.NavigationQueryCallees && dir != navigation.NavigationQueryDependencies {
		rows, err := s.queryRows(ctx, "incoming-ords", frontier)
		if err != nil {
			return nil, err
		}
		ords := []int64{}
		for rows.Next() {
			var n int64
			if err = rows.Scan(&n); err != nil {
				rows.Close()
				return nil, err
			}
			ords = append(ords, n)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if len(ords) > 0 {
			rows, err = s.queryRows(ctx, "call-rows", ords)
			if err != nil {
				return nil, err
			}
			calls, err := readCallRows(rows)
			if err != nil {
				return nil, err
			}
			for _, call := range calls {
				selected[call.Ord] = call
			}
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
func readCallRows(rows *sql.Rows) ([]rowCall, error) {
	defer rows.Close()
	result := []rowCall{}
	for rows.Next() {
		var row rowCall
		var encoded string
		if err := rows.Scan(&row.Ord, &encoded); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(encoded), &row.Call); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
func (s *duckStore) queryRows(ctx context.Context, key string, ids any) (*sql.Rows, error) {
	args := []any{}
	switch values := ids.(type) {
	case []string:
		for _, v := range values {
			args = append(args, v)
		}
	case []int64:
		for _, v := range values {
			args = append(args, v)
		}
	}
	// Scalar IN predicates enable point/ART scans for small frontiers. Large
	// frontiers retain vector-list binding to avoid giant SQL/prepared caches.
	if len(args) == 0 || len(args) > 128 {
		s.mu.Lock()
		stmt := s.stmts[key]
		s.mu.Unlock()
		return stmt.QueryContext(ctx, ids)
	}
	cacheKey := fmt.Sprintf("%s-%d", key, len(args))
	s.mu.Lock()
	stmt := s.stmts[cacheKey]
	if stmt == nil {
		text := s.queries[key]
		placeholder := "(" + strings.TrimSuffix(strings.Repeat("?,", len(args)), ",") + ")"
		text = strings.ReplaceAll(text, "(SELECT unnest(?::VARCHAR[]))", placeholder)
		text = strings.ReplaceAll(text, "(SELECT unnest(?::BIGINT[]))", placeholder)
		var err error
		stmt, err = s.db.Prepare(text)
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		s.stmts[cacheKey] = stmt
	}
	s.mu.Unlock()
	return stmt.QueryContext(ctx, args...)
}
func selectFacts[T any](ctx context.Context, s *duckStore, key string, ids any) ([]T, error) {
	rows, err := s.queryRows(ctx, key, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []T{}
	for rows.Next() {
		var encoded string
		if err = rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var item T
		if err = json.Unmarshal([]byte(encoded), &item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
func (s *duckStore) Fragment(ctx context.Context, ids []string, ords []int) (parser.NavigationGraph, error) {
	g := emptyGraph()
	g.RepositoryRoots = s.roots
	var err error
	g.Declarations, err = selectFacts[parser.NavigationDeclaration](ctx, s, "defs", ids)
	if err != nil {
		return g, err
	}
	nums := make([]int64, len(ords))
	for i, n := range ords {
		nums[i] = int64(n)
	}
	g.Calls, err = selectFacts[parser.NavigationCall](ctx, s, "calls", nums)
	if err != nil {
		return g, err
	}
	owners, files := contextKeys(g)
	g.Fields, err = selectFacts[parser.NavigationField](ctx, s, "fields", keys(owners))
	if err != nil {
		return g, err
	}
	for _, f := range g.Fields {
		files[f.Path] = true
	}
	paths := keys(files)
	g.TypeDeclarations, err = selectFacts[parser.NavigationTypeDeclaration](ctx, s, "types", paths)
	if err != nil {
		return g, err
	}
	g.Imports, err = selectFacts[parser.NavigationImport](ctx, s, "imports", paths)
	if err != nil {
		return g, err
	}
	g.Exports, err = selectFacts[parser.NavigationExport](ctx, s, "exports", paths)
	if err != nil {
		return g, err
	}
	g.TypeUsages, err = selectFacts[parser.NavigationTypeUsage](ctx, s, "usages", ids)
	if err != nil {
		return g, err
	}
	g.MemberAccesses, err = selectFacts[parser.NavigationMemberAccess](ctx, s, "members", ids)
	return g, err
}
func buildNativeDuck(path string, g parser.NavigationGraph) (time.Duration, error) {
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
	ctx := context.Background()
	ddl := `CREATE TABLE declarations(ord BIGINT,id VARCHAR,payload VARCHAR); CREATE TABLE calls(ord BIGINT,id VARCHAR,caller VARCHAR,payload VARCHAR); CREATE TABLE targets(call_ord BIGINT,target VARCHAR);
 CREATE TABLE fields(ord BIGINT,owner VARCHAR,payload VARCHAR); CREATE TABLE types(ord BIGINT,path VARCHAR,payload VARCHAR); CREATE TABLE imports(ord BIGINT,path VARCHAR,payload VARCHAR); CREATE TABLE exports(ord BIGINT,path VARCHAR,payload VARCHAR); CREATE TABLE usages(ord BIGINT,caller VARCHAR,payload VARCHAR); CREATE TABLE members(ord BIGINT,caller VARCHAR,payload VARCHAR); CREATE TABLE metadata(key VARCHAR,payload VARCHAR);`
	if _, err = db.Exec(ddl); err != nil {
		return 0, err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	err = conn.Raw(func(raw any) error {
		makeAppender := func(table string) (*duckdb.Appender, error) {
			return duckdb.NewAppenderFromConn(raw.(driver.Conn), "", table)
		}
		def, e := makeAppender("declarations")
		if e != nil {
			return e
		}
		for i, d := range g.Declarations {
			p, _ := json.Marshal(d)
			if e = def.AppendRow(int64(i), d.ID, string(p)); e != nil {
				def.Close()
				return e
			}
		}
		if e = def.Close(); e != nil {
			return e
		}
		calls, e := makeAppender("calls")
		if e != nil {
			return e
		}
		targets, e := makeAppender("targets")
		if e != nil {
			calls.Close()
			return e
		}
		for i, c := range g.Calls {
			p, _ := json.Marshal(c)
			if e = calls.AppendRow(int64(i), c.ID, c.CallerID, string(p)); e != nil {
				return e
			}
			for _, target := range callTargets(c) {
				if e = targets.AppendRow(int64(i), target); e != nil {
					return e
				}
			}
		}
		if e = calls.Close(); e != nil {
			return e
		}
		if e = targets.Close(); e != nil {
			return e
		}
		if e = appendFacts(raw, g.Fields, "fields", func(f parser.NavigationField) string { return terminal(f.OwnerType) }); e != nil {
			return e
		}
		if e = appendFacts(raw, g.TypeDeclarations, "types", func(f parser.NavigationTypeDeclaration) string { return f.Path }); e != nil {
			return e
		}
		if e = appendFacts(raw, g.Imports, "imports", func(f parser.NavigationImport) string { return f.Path }); e != nil {
			return e
		}
		if e = appendFacts(raw, g.Exports, "exports", func(f parser.NavigationExport) string { return f.Path }); e != nil {
			return e
		}
		if e = appendFacts(raw, g.TypeUsages, "usages", func(f parser.NavigationTypeUsage) string { return f.CallerID }); e != nil {
			return e
		}
		return appendFacts(raw, g.MemberAccesses, "members", func(f parser.NavigationMemberAccess) string { return f.CallerID })
	})
	if err != nil {
		return 0, err
	}
	conn.Close()
	roots, _ := json.Marshal(g.RepositoryRoots)
	if _, err = db.Exec("INSERT INTO metadata VALUES ('roots', ?)", string(roots)); err != nil {
		return 0, err
	}
	for _, q := range []string{"CREATE INDEX decl_id ON declarations(id)", "CREATE INDEX call_caller ON calls(caller)", "CREATE INDEX call_ord ON calls(ord)", "CREATE INDEX target_id ON targets(target)", "CREATE INDEX target_call ON targets(call_ord)", "CREATE INDEX field_owner ON fields(owner)", "CREATE INDEX type_path ON types(path)", "CREATE INDEX import_path ON imports(path)", "CREATE INDEX export_path ON exports(path)", "CREATE INDEX usage_caller ON usages(caller)", "CREATE INDEX member_caller ON members(caller)", "CHECKPOINT"} {
		if _, err = db.Exec(q); err != nil {
			return 0, err
		}
	}
	if err = db.Close(); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}
func appendFacts[T any](raw any, items []T, table string, key func(T) string) error {
	a, err := duckdb.NewAppenderFromConn(raw.(driver.Conn), "", table)
	if err != nil {
		return err
	}
	for i, item := range items {
		payload, e := json.Marshal(item)
		if e != nil {
			a.Close()
			return e
		}
		if e = a.AppendRow(int64(i), key(item), string(payload)); e != nil {
			a.Close()
			return e
		}
	}
	return a.Close()
}

func (s *duckStore) Existing(ctx context.Context, ids []string) ([]string, error) {
	rows, err := s.queryRows(ctx, "existing", ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func init() {
	selectedDuckOpener = openNativeDuck
	selectedDuckBuilder = buildNativeDuck
	selectedColumnOpener = openColumnDuck
	selectedColumnBuilder = buildColumnDuck
	duckDBEnabled = true
}
