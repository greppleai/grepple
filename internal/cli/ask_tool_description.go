package cli

const greppleResearchToolDescription = `Run read-only Grepple CLI research commands as an argv array, without the grepple executable and without a shell.

Choose the smallest useful output:
- Unknown scope: count matches, then list matching files before retrieving bodies.
- File orientation: use --outline and optionally --depth.
- Exact declaration: use --at PATH:LINE or PATH:START-END.
- Control flow or impact: add --related; use --follow-related 1 first and at most 3.
- Structural syntax patterns: use grit; this is syntax matching, not type/data-flow proof.
- Architecture: use architecture directory, architecture resolve, or architecture why.
- Complete graph analysis: use graph with exactly one of --json or --compact.
- Completeness-sensitive work: inspect sources explain and report skipped/failed/truncated work.
- Remote work: select one exact repository with --repo OWNER/REPO[@REF]. Remote --at paths are repository-relative.

Useful argv examples:
- {"args":["-F","Symbol","--count","src"]}
- {"args":["-F","Symbol","--files-with-matches","src"]}
- {"args":["--outline","internal/cli/ask.go"]}
- {"args":["--at","internal/cli/ask.go:61","--related"]}
- {"args":["--follow-related","1","-F","BuildNavigationGraph(","search"]}
- {"args":["grit","$x == nil","--language","go","internal"]}
- {"args":["architecture","directory","--compact","."]}
- {"args":["architecture","resolve","parser.Document","."]}
- {"args":["graph","callees","--compact","--at","search/related.go:40","."]}
- {"args":["sources","explain","."]}
- {"args":["--server","http://127.0.0.1:8080","--repo","sourcegraph/zoekt","--related","-F","NewDirectorySearcher"]}
- {"args":["get","sourcegraph/zoekt","query/query.go","--server","http://127.0.0.1:8080","--lines","20:80"]}
- {"args":["tree","sourcegraph/zoekt","--server","http://127.0.0.1:8080"]}
- {"args":["help","graph"]}

Search defaults to JavaScript regex; add -F for literals. Default result count and output are bounded, so narrow paths and use --limit deliberately. Results may include continuation commands and incompleteness metadata. Navigation is syntax-based: candidate edges are hypotheses, while exact/import-resolved/context-resolved edges are stronger evidence. Cite exact repository/path:line ranges from returned evidence.

Pass every flag and value as its own args element. Never pass shell syntax, pipes, redirections, command substitutions, or a single shell command string. The harness adds --no-spill and caps tool output at 64 KiB. Mutating/authentication commands and recursive ask are rejected.`
