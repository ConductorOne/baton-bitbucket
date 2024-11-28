package bitbucket

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/conductorone/baton-sdk/pkg/uhttp"
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

func (c *Client) getUrl(path string, args ...string) *url.URL {
	escapedArgs := make([]string, 0)
	for _, arg := range args {
		escapedArgs = append(escapedArgs, url.PathEscape(arg))
	}
	return c.baseUrl.JoinPath(fmt.Sprintf(path, escapedArgs))
}

func (c *Client) delete(ctx context.Context, url *url.URL) error {
	req, err := c.createRequest(ctx, url, http.MethodDelete, nil, nil)
	if err != nil {
		return err
	}

	var errRes errorResponse
	r, err := c.wrapper.Do(req, uhttp.WithErrorResponse(&errRes))
	if err != nil {
		return err
	}

	defer r.Body.Close()

	return nil
}

func (c *Client) get(
	ctx context.Context,
	url *url.URL,
	resourceResponse interface{},
	paramOptions []QueryParam,
) error {
	req, err := c.createRequest(ctx, url, http.MethodGet, nil, paramOptions)
	if err != nil {
		return err
	}

	var errRes errorResponse
	r, err := c.wrapper.Do(
		req,
		uhttp.WithErrorResponse(&errRes),
		uhttp.WithJSONResponse(resourceResponse),
	)
	if err != nil {
		return err
	}

	defer r.Body.Close()

	return nil
}

func (c *Client) put(
	ctx context.Context,
	url *url.URL,
	data, resourceResponse interface{},
	paramOptions []QueryParam,
) error {
	request, err := c.createRequest(ctx, url, http.MethodPut, data, paramOptions)
	if err != nil {
		return err
	}

	var errRes errorResponse
	r, err := c.wrapper.Do(
		request,
		uhttp.WithErrorResponse(&errRes),
		uhttp.WithJSONResponse(resourceResponse),
	)
	if err != nil {
		return err
	}

	defer r.Body.Close()

	return nil
}

func (c *Client) createRequest(
	ctx context.Context,
	url0 *url.URL,
	method string,
	data interface{},
	paramOptions []QueryParam,
) (*http.Request, error) {
	opts := []uhttp.RequestOption{
		uhttp.WithAcceptJSONHeader(),
	}
	if data != nil {
		opts = append(opts, uhttp.WithJSONBody(data))
	}

	request, err := c.wrapper.NewRequest(
		ctx,
		method,
		url0,
		opts...,
	)
	if err != nil {
		return nil, err
	}

	if paramOptions != nil {
		queryParams := url.Values{}
		for _, q := range paramOptions {
			q.setup(&queryParams)
		}

		request.URL.RawQuery = queryParams.Encode()
	}

	return request, nil
}
