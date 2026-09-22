package mysqlstore

import "strings"

const (
	MaxInventoryPageSize    = 100
	SummaryEnvironmentLimit = 3
)

type InventoryQuery struct {
	Limit           int
	Offset          int
	Search          string
	Key             string
	IncludeInactive bool
	OnlyInactive    bool
}

type InventoryPage[T any] struct {
	Items []T    `json:"items"`
	Total uint64 `json:"total"`
}

func (query InventoryQuery) valid() bool {
	return query.Limit >= 1 && query.Limit <= MaxInventoryPageSize && query.Offset >= 0 && len(query.Search) <= 256 && len(query.Key) <= 128
}

// Column expressions are package-owned SQL, never request values. Search uses
// literal substring matching: '%' and '_' in an input are not SQL wildcards.
func (query InventoryQuery) filter(keyColumn, inactiveColumn string, searchColumns ...string) (string, []any) {
	where := "TRUE"
	var arguments []any
	if query.OnlyInactive {
		where += " AND " + inactiveColumn + " IS NOT NULL"
	} else if !query.IncludeInactive {
		where += " AND " + inactiveColumn + " IS NULL"
	}
	if query.Key != "" {
		where += " AND " + keyColumn + " = ?"
		arguments = append(arguments, query.Key)
	}
	if query.Search != "" {
		where += " AND LOCATE(LOWER(?), LOWER(CONCAT_WS(' ', " + strings.Join(searchColumns, ", ") + "))) > 0"
		arguments = append(arguments, query.Search)
	}
	return where, arguments
}
