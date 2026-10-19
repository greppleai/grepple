package memory

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gofrs/flock"
)

func entryName(name string) (string, error) {
	if !ValidName(name) {
		return "", fmt.Errorf("invalid memory name %q", name)
	}
	return name + ".md", nil
}

// directory never creates on read; a symlinked memory directory is forbidden.
func directory(root string, create bool) (*os.Root, error) {
	base, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	for _, dir := range []string{".grepple", ".grepple/memories"} {
		info, e := base.Lstat(dir)
		if errors.Is(e, os.ErrNotExist) && create {
			if e = base.Mkdir(dir, 0700); e != nil {
				return nil, e
			}
			info, e = base.Lstat(dir)
		}
		if e != nil {
			return nil, e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("unsafe memory directory %s", dir)
		}
	}
	return base.OpenRoot(".grepple/memories")
}

func entryFile(dir *os.Root, name string) (*os.File, error) {
	fileName, err := entryName(name)
	if err != nil {
		return nil, err
	}
	info, err := dir.Lstat(fileName)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("memory is not a regular file")
	}
	return dir.Open(fileName)
}

func readEntry(dir *os.Root, name string) ([]byte, error) {
	file, err := entryFile(dir, name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, EntryLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > EntryLimit {
		return nil, errors.New("memory exceeds 64 KiB")
	}
	h, bodyAt, err := ParseHeader(data[:min(len(data), HeaderLimit)], name)
	if err != nil {
		return nil, err
	}
	if h.Name != name || len(bytes.TrimSpace(data[bodyAt:])) == 0 || !utf8.Valid(data[bodyAt:]) || bytes.IndexByte(data[bodyAt:], 0) >= 0 {
		return nil, errors.New("invalid memory body")
	}
	return data, nil
}

// Recall is the ONLY operation that reads a complete body.
func Recall(root, name string) ([]byte, Header, error) {
	if _, err := entryName(name); err != nil {
		return nil, Header{}, err
	}
	dir, err := directory(root, false)
	if err != nil {
		return nil, Header{}, err
	}
	defer dir.Close()
	data, err := readEntry(dir, name)
	if err != nil {
		return nil, Header{}, err
	}
	h, _, err := ParseHeader(data[:min(len(data), HeaderLimit)], name)
	return data, h, err
}

// HeaderOnly reads exactly through the closing delimiter, not the body.
func HeaderOnly(root, name string) (Header, error) {
	dir, err := directory(root, false)
	if err != nil {
		return Header{}, err
	}
	defer dir.Close()
	file, err := entryFile(dir, name)
	if err != nil {
		return Header{}, err
	}
	defer file.Close()
	buf := make([]byte, 0, 512)
	var b [1]byte
	for len(buf) < HeaderLimit {
		if _, err := io.ReadFull(file, b[:]); err != nil {
			return Header{}, errors.New("incomplete memory header")
		}
		buf = append(buf, b[0])
		if len(buf) >= 9 && bytes.HasSuffix(buf, []byte("\n---\n")) {
			h, _, err := ParseHeader(buf, name)
			return h, err
		}
	}
	return Header{}, errors.New("memory header exceeds 8 KiB")
}

// Save uses a shared cross-process lock and a same-directory temporary file.
// Store uses hard-link publication (atomic no-clobber); update renames only after
// rereading the old entry under the lock. Noncooperating external editors should
// not mutate entries concurrently with the CLI.
func Save(root string, h Header, body []byte, update bool) error {
	data, err := Encode(h, body)
	if err != nil {
		return err
	}
	dir, err := directory(root, true)
	if err != nil {
		return err
	}
	defer dir.Close()
	return withLock(root, dir, func() error {
		name, _ := entryName(h.Name)
		var previous [32]byte
		_, statErr := dir.Lstat(name)
		if update {
			if statErr != nil {
				return fmt.Errorf("update missing memory: %w", statErr)
			}
			existing, err := readEntry(dir, h.Name)
			if err != nil {
				return err
			}
			previous = sha256.Sum256(existing)
		} else if statErr == nil {
			return fmt.Errorf("memory %q already exists; use update", h.Name)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		var nonce [12]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return err
		}
		tempName := ".pending-" + hex.EncodeToString(nonce[:])
		temp, err := dir.OpenFile(tempName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		defer dir.Remove(tempName)
		if _, err = temp.Write(data); err != nil {
			temp.Close()
			return err
		}
		if err = temp.Sync(); err != nil {
			temp.Close()
			return err
		}
		if err = temp.Close(); err != nil {
			return err
		}
		if update {
			current, err := readEntry(dir, h.Name)
			if err != nil {
				return err
			}
			if sha256.Sum256(current) != previous {
				return errors.New("memory changed during update")
			}
			return dir.Rename(tempName, name)
		}
		return dir.Link(tempName, name)
	})
}

// Delete removes one explicitly named, validated repository-local memory.
func Delete(root, name string) error {
	if _, err := entryName(name); err != nil {
		return err
	}
	dir, err := directory(root, false)
	if err != nil {
		return err
	}
	defer dir.Close()
	return withLock(root, dir, func() error {
		if _, err := readEntry(dir, name); err != nil {
			return err
		}
		return dir.Remove(name + ".md")
	})
}

func withLock(root string, dir *os.Root, run func() error) error {
	info, err := dir.Lstat(".lock")
	if err == nil && !info.Mode().IsRegular() {
		return errors.New("unsafe memory lock file")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	lock := flock.New(filepath.Join(root, ".grepple", "memories", ".lock"))
	if err := lock.Lock(); err != nil {
		return err
	}
	defer lock.Unlock()
	info, err = dir.Lstat(".lock")
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("unsafe memory lock file")
	}
	return run()
}

// AttachmentStatuses never exposes body text. It deliberately rechecks current
// bytes and selector bounds without modifying stored memory.
func AttachmentStatuses(root string, h Header) []string {
	statuses := make([]string, 0, len(h.Files))
	for _, f := range h.Files {
		status := "current"
		_, checksum, err := Source(root, f.Path)
		if err != nil {
			status = "missing or inaccessible source"
		} else if checksum != f.Checksum {
			status = "stale checksum"
		} else if err := VerifyAttachment(root, f); err != nil {
			status = "unverified range or symbol"
		}
		statuses = append(statuses, fmt.Sprintf("%s: %s", strings.ReplaceAll(f.Path, "\n", ""), status))
	}
	return statuses
}
