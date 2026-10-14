package search

// NavigationSelection is an internal selected-row witness, not a cursor or auth
// token. The host must recheck repository authorization and exact revision.
type NavigationSelection struct {
	Path      string `json:"path"`
	Discovery bool   `json:"discovery,omitempty"`
	More      bool   `json:"more,omitempty"`
	Language  string `json:"language,omitempty"`
	Digest    string `json:"digest,omitempty"`
	Revision  string `json:"revision"`
	Lines     []int  `json:"lines,omitempty"`
}
