// Package parser provides application source classification and structural
// parsing. Document owns an immutable source and syntax tree; Node handles remain
// tied to that document, DocumentView and ViewNode are valid only inside Read, and
// SyntaxNode is a persistent immutable snapshot. Tree-sitter types remain private.
// The package owns structural segments, outlines, and navigation facts without
// depending on search or transport models.
package parser
