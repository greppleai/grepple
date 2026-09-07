package parser

// Segment describes a structural slice of a file. Lines segments contain source
// lines; summary segments contain a compact declaration or heading.
type Segment struct {
	Kind       string
	Start, End int
	Text       string
}

// Symbol is a single definition found in a file, with a 1-based line range.
type Symbol struct {
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Signature string   `json:"signature,omitempty"`
	Start     int      `json:"start"`
	End       int      `json:"end"`
	Children  []Symbol `json:"children,omitempty"`
}

// FileOutline is the structural map of one file in source order.
type FileOutline struct {
	Path     string   `json:"path"`
	Language string   `json:"language"`
	Symbols  []Symbol `json:"symbols"`
}
