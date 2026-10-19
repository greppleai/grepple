// Package memory implements explicit repository-local memory commands.
package memory

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/memory"
	"github.com/greppleai/grepple/internal/sources"
)

type WriteArgs struct {
	Name            string   `arg:"positional,required" placeholder:"NAME"`
	Description     string   `arg:"--description" placeholder:"TEXT" help:"single-line hint (1-160 bytes); visible in search references"`
	BodyFile        string   `arg:"--body-file" placeholder:"PATH" help:"complete Markdown body from PATH or - (stdin); required for store/update"`
	DomainAreas     []string `arg:"--domain-area" placeholder:"LABEL" help:"repeatable exact metadata domain label"`
	FunctionalAreas []string `arg:"--functional-area" placeholder:"LABEL" help:"repeatable exact metadata functional label"`
	Keywords        []string `arg:"--keyword" placeholder:"LABEL" help:"repeatable exact metadata keyword label"`
	Scopes          []string `arg:"--scope,separate" placeholder:"PATH" help:"repeatable selected-path hint: directory/, directory/*.ext, or directory/**/*.ext; not checksum-verified"`
	Files           []string `arg:"--file" placeholder:"PATH" help:"selected repository-root-relative whole-file attachment (repeatable)"`
	Ranges          []string `arg:"--line-range" placeholder:"PATH:START-END" help:"inclusive matched-line attachment (repeatable)"`
	Symbols         []string `arg:"--symbol" placeholder:"PATH@KIND:START-END" help:"exact parser-verified declaration attachment (repeatable)"`
}
type NameArgs struct {
	Name string `arg:"positional,required" placeholder:"NAME"`
}
type Args struct {
	Store  *WriteArgs `arg:"subcommand:store" help:"create one repository-local memory"`
	Recall *NameArgs  `arg:"subcommand:recall" help:"display one full memory and attachment status"`
	Update *WriteArgs `arg:"subcommand:update" help:"replace one existing memory explicitly"`
	Delete *NameArgs  `arg:"subcommand:delete" help:"delete one named memory"`
}

func (Args) Description() string {
	return "Manage repository-local memories. Store and update require a complete body and all labels, scopes and attachments (update replaces, never merges). Only recall displays full Markdown; search shows brief references to matched source, file-owned labels or path scopes. See docs/repository-memories.md."
}

func Execute(app cliruntime.Context, args *Args) error {
	if args == nil || args.Store == nil && args.Update == nil && args.Recall == nil && args.Delete == nil {
		return errors.New("memory requires store, recall, update or delete")
	}
	root, err := memory.Root(app.Repository().WorkingDirectory())
	if err != nil {
		return err
	}
	switch {
	case args.Store != nil:
		return save(app, root, args.Store, false)
	case args.Update != nil:
		return save(app, root, args.Update, true)
	case args.Recall != nil:
		data, h, err := memory.Recall(root, args.Recall.Name)
		if err != nil {
			return err
		}
		if _, err = app.Stdout().Write(data); err != nil {
			return err
		}
		for _, status := range memory.AttachmentStatuses(root, h) {
			if _, err = fmt.Fprintln(app.Stderr(), status); err != nil {
				return err
			}
		}
		return nil
	case args.Delete != nil:
		return memory.Delete(root, args.Delete.Name)
	default:
		return errors.New("memory requires store, recall, update or delete")
	}
}

func save(app cliruntime.Context, root string, args *WriteArgs, update bool) error {
	if args.BodyFile == "" {
		return errors.New("--body-file is required")
	}
	var reader io.Reader
	if args.BodyFile == "-" {
		reader = app.Stdin()
	} else {
		file, err := os.Open(args.BodyFile)
		if err != nil {
			return err
		}
		defer file.Close()
		reader = file
	}
	body, err := io.ReadAll(io.LimitReader(reader, memory.EntryLimit+1))
	if err != nil {
		return err
	}
	if len(body) > memory.EntryLimit {
		return errors.New("memory body exceeds 64 KiB")
	}
	h := memory.Header{Schema: "grepple-memory-v1", Name: args.Name, Description: args.Description, DomainAreas: args.DomainAreas, FunctionalAreas: args.FunctionalAreas, Keywords: args.Keywords}
	for _, value := range args.Scopes {
		scope, err := memory.ScopeForStore(root, value)
		if err != nil {
			return err
		}
		h.Scopes = append(h.Scopes, scope)
	}
	attach := func(relative string, lines []int, symbol string) error {
		if !memory.ValidPath(relative) {
			return fmt.Errorf("invalid attachment path %q", relative)
		}
		policy, err := app.Repository().ScopeOptions()
		if err != nil {
			return err
		}
		files, err := sources.ListWithPolicy(app.Invocation(), []string{relative}, root, policy)
		if err != nil {
			return err
		}
		expected := filepath.Clean(filepath.Join(root, filepath.FromSlash(relative)))
		selected := false
		for _, file := range files {
			absolute, _ := filepath.Abs(file)
			if absolute == expected {
				selected = true
				break
			}
		}
		if !selected {
			return fmt.Errorf("source %q is not selected by repository policy", relative)
		}
		_, checksum, err := memory.Source(root, relative)
		if err != nil {
			return err
		}
		attachment := memory.Attachment{Path: relative, Checksum: checksum, LineRange: lines}
		if symbol != "" {
			attachment.Symbols = []string{symbol}
		}
		if err := memory.VerifyAttachment(root, attachment); err != nil {
			return err
		}
		h.Files = append(h.Files, attachment)
		return nil
	}
	for _, value := range args.Files {
		if err := attach(value, nil, ""); err != nil {
			return err
		}
	}
	for _, value := range args.Ranges {
		pos := strings.LastIndexByte(value, ':')
		if pos < 1 {
			return fmt.Errorf("--line-range requires PATH:START-END")
		}
		bounds := strings.Split(value[pos+1:], "-")
		if len(bounds) != 2 {
			return errors.New("--line-range requires START-END")
		}
		start, e1 := strconv.Atoi(bounds[0])
		end, e2 := strconv.Atoi(bounds[1])
		if e1 != nil || e2 != nil || start < 1 || end < start {
			return errors.New("invalid --line-range bounds")
		}
		if err := attach(value[:pos], []int{start, end}, ""); err != nil {
			return err
		}
	}
	for _, value := range args.Symbols {
		pos := strings.LastIndexByte(value, '@')
		if pos < 1 {
			return errors.New("--symbol requires PATH@KIND:START-END")
		}
		if err := attach(value[:pos], nil, value[pos:]); err != nil {
			return err
		}
	}
	return memory.Save(root, h, body, update)
}
