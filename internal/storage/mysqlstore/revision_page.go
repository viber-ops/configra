package mysqlstore

const MaxRevisionPageSize = 100

// RevisionQuery reads immutable revisions newest first. Before is exclusive;
// zero starts at the newest revision. Limit must be in [1, MaxRevisionPageSize].
type RevisionQuery struct {
	Before uint64
	Limit  int
}

type RevisionPage[T any] struct {
	Items      []T    `json:"items"`
	NextBefore uint64 `json:"next_before,omitempty"`
}
