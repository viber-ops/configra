package management

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

func parseInventoryQuery(response http.ResponseWriter, request *http.Request, inactiveFlag string, extra ...string) (mysqlstore.InventoryQuery, url.Values, bool) {
	query := mysqlstore.InventoryQuery{Limit: 50}
	values, err := url.ParseQuery(request.URL.RawQuery)
	allowed := []string{"limit", "offset", "q"}
	if inactiveFlag != "" {
		allowed = append(allowed, "key", "status", inactiveFlag)
	}
	allowed = append(allowed, extra...)
	if err == nil {
		for name, entries := range values {
			if !slices.Contains(allowed, name) || len(entries) != 1 {
				err = mysqlstore.ErrValidation
				break
			}
		}
	}
	if err == nil && values.Has("limit") {
		query.Limit, err = strconv.Atoi(values.Get("limit"))
	}
	if err == nil && values.Has("offset") {
		query.Offset, err = strconv.Atoi(values.Get("offset"))
	}
	query.Search = strings.TrimSpace(values.Get("q"))
	query.Key = values.Get("key")
	if query.Limit < 1 || query.Limit > mysqlstore.MaxInventoryPageSize || query.Offset < 0 || len(query.Search) > 256 || len(query.Key) > 128 {
		err = mysqlstore.ErrValidation
	}
	if values.Has(inactiveFlag) {
		switch values.Get(inactiveFlag) {
		case "true":
			query.IncludeInactive = true
		case "false", "":
		default:
			err = mysqlstore.ErrValidation
		}
	}
	if values.Has("status") {
		if values.Has(inactiveFlag) {
			err = mysqlstore.ErrValidation
		}
		switch values.Get("status") {
		case "active":
		case "all":
			query.IncludeInactive = true
		case strings.TrimPrefix(inactiveFlag, "include_"):
			query.IncludeInactive, query.OnlyInactive = true, true
		default:
			err = mysqlstore.ErrValidation
		}
	}
	if err != nil {
		writeError(response, http.StatusBadRequest, "invalid_request")
		return mysqlstore.InventoryQuery{}, nil, false
	}
	return query, values, true
}
