package navigation

// PagedReadStats counts actual successful numeric/name/context page reads.
// Directory bytes are separate; native detail bytes and kernel faults are not
// included, and physical storage reads may differ because of filesystem cache.
type PagedReadStats struct{ DirectoryBytes, CoreBytesRead, CorePagesRead uint64 }
