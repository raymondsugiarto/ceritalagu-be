// Package pagination holds the shared list input/output shapes.
// It has no HTTP dependency, so services can return these types directly.
package pagination

// Defaults applied by Normalize, and the largest page size a client may request.
const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Input is embedded in list inputs: ?page=1&page_size=20
type Input struct {
	Page     int `query:"page"      validate:"omitempty,min=1"`
	PageSize int `query:"page_size" validate:"omitempty,min=1,max=100"`
}

// Normalize fills defaults for omitted values.
func (in *Input) Normalize() {
	if in.Page == 0 {
		in.Page = DefaultPage
	}
	if in.PageSize == 0 {
		in.PageSize = DefaultPageSize
	}
}

// Offset is the number of rows to skip for the current page.
func (in Input) Offset() int { return (in.Page - 1) * in.PageSize }

// Output is the "pagination" object of every list response.
type Output struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

// ListOutput is the "data" of every list endpoint. Lists are never returned as a bare array,
// so "items" is always present (even when empty) and pagination can be added without breaking clients.
type ListOutput[T any] struct {
	Items      []T    `json:"items"`
	Pagination Output `json:"pagination"`
}

// NewList builds a list response. A nil items slice becomes [] so "items" is never null.
func NewList[T any](items []T, in Input, total int64) ListOutput[T] {
	if items == nil {
		items = []T{}
	}
	pages := 0
	if in.PageSize > 0 {
		pages = int((total + int64(in.PageSize) - 1) / int64(in.PageSize))
	}
	return ListOutput[T]{
		Items:      items,
		Pagination: Output{Page: in.Page, PageSize: in.PageSize, TotalItems: total, TotalPages: pages},
	}
}
