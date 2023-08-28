package bitbucket

import (
	"net/url"
	"strconv"
	"strings"
)

type QueryParam interface {
	setup(params *url.Values)
}

type PaginationVars struct {
	Limit int
	Page  string
}

func (pV *PaginationVars) setup(params *url.Values) {
	// add limit
	if pV.Limit != 0 {
		params.Set("pagelen", strconv.Itoa(pV.Limit))
	}

	// add page
	if pV.Page != "" {
		params.Set("page", pV.Page)
	}
}

type FilterVars struct {
	SearchId string
	Fields   []string
}

func (fV *FilterVars) setup(params *url.Values) {
	if fV.SearchId != "" {
		params.Set("q", fV.SearchId)
	}

	// add filters to minimize response size
	if len(fV.Fields) != 0 {
		params.Set("fields", strings.Join(fV.Fields, ","))
	}
}

var defaultFilters = []string{
	"-links",
	"-*.links",
	"-*.*.links",
}

func composeFilters(filters []string, newFilters ...string) []string {
	return append(filters, newFilters...)
}

func prepareFilters(searchId string, filters ...string) *FilterVars {
	var id string
	fs := defaultFilters

	if searchId != "" {
		id = searchId
	}

	if len(filters) != 0 {
		fs = composeFilters(defaultFilters, filters...)
	}

	return &FilterVars{
		SearchId: id,
		Fields:   fs,
	}
}
