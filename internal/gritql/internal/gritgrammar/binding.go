// Package gritgrammar binds the pinned, generated tree-sitter GritQL grammar.
package gritgrammar

// #cgo CFLAGS: -std=c11 -fPIC
// #include "tree_sitter/parser.h"
// const TSLanguage *tree_sitter_gritql(void);
import "C"

import "unsafe"

// Language returns the generated tree-sitter language descriptor.
func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_gritql())
}
