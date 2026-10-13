package jira

import (
	"context"
	"iter"
	"net/url"
	"strconv"
)

// Jira has three pagination styles. Each helper returns an iterator that
// fetches pages lazily and stops early when the caller breaks out of the loop:
//
//	for p, err := range c.Projects(ctx) { … }
//
// An error is yielded once, as the last element.

// offsetPage is {"startAt","maxResults","total","isLast","values":[…]}.
type offsetPage[T any] struct {
	StartAt    int  `json:"startAt"`
	MaxResults int  `json:"maxResults"`
	Total      int  `json:"total"`
	IsLast     bool `json:"isLast"`
	Values     []T  `json:"values"`
}

func pagesByOffset[T any](ctx context.Context, c *Client, path string, q url.Values, pageSize int) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		start := 0
		for {
			qq := cloneQuery(q)
			qq.Set("startAt", strconv.Itoa(start))
			qq.Set("maxResults", strconv.Itoa(pageSize))
			var page offsetPage[T]
			if err := c.get(ctx, path, qq, &page); err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, v := range page.Values {
				if !yield(v, nil) {
					return
				}
			}
			start += len(page.Values)
			// Stop on isLast, on an empty page (guards against a server that
			// ignores startAt), or when the total is reached.
			if page.IsLast || len(page.Values) == 0 || (page.Total > 0 && start >= page.Total) {
				return
			}
		}
	}
}

// pagesOfArray is for endpoints that return a bare JSON array and accept
// startAt/maxResults (e.g. /user/assignable/search). A short page ends it.
func pagesOfArray[T any](ctx context.Context, c *Client, path string, q url.Values, pageSize int) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		start := 0
		for {
			qq := cloneQuery(q)
			qq.Set("startAt", strconv.Itoa(start))
			qq.Set("maxResults", strconv.Itoa(pageSize))
			var page []T
			if err := c.get(ctx, path, qq, &page); err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, v := range page {
				if !yield(v, nil) {
					return
				}
			}
			if len(page) < pageSize {
				return
			}
			start += len(page)
		}
	}
}

// Collect drains an iterator into a slice.
func Collect[T any](seq iter.Seq2[T, error]) ([]T, error) {
	var out []T
	for v, err := range seq {
		if err != nil {
			return out, err
		}
		out = append(out, v)
	}
	return out, nil
}

func cloneQuery(q url.Values) url.Values {
	out := url.Values{}
	for k, v := range q {
		out[k] = append([]string(nil), v...)
	}
	return out
}
