package pagination

import (
	"net/http"
	"strconv"
)

// Params represents pagination parameters
type Params struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
	Limit   int `json:"-"` // Calculated from PerPage
	Offset  int `json:"-"` // Calculated from Page and PerPage
}

// Response represents a paginated response
type Response[T any] struct {
	Page         int `json:"page"`
	PerPage      int `json:"per_page"`
	TotalPages   int `json:"total_pages"`
	TotalResults int `json:"total_results"`
	Results      []T `json:"results"`
}

// DefaultPerPage is the default number of items per page
const DefaultPerPage = 20

// MaxPerPage is the maximum allowed items per page
const MaxPerPage = 100

// ParseParams extracts pagination parameters from HTTP request
func ParseParams(r *http.Request) Params {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 {
		perPage = DefaultPerPage
	}
	if perPage > MaxPerPage {
		perPage = MaxPerPage
	}

	return Params{
		Page:    page,
		PerPage: perPage,
		Limit:   perPage,
		Offset:  (page - 1) * perPage,
	}
}

// NewResponse creates a new paginated response
func NewResponse[T any](results []T, page, perPage, totalResults int) Response[T] {
	return Response[T]{
		Page:         page,
		PerPage:      perPage,
		TotalPages:   CalculateTotalPages(totalResults, perPage),
		TotalResults: totalResults,
		Results:      results,
	}
}

// CalculateTotalPages calculates the total number of pages
func CalculateTotalPages(totalResults, perPage int) int {
	if perPage <= 0 {
		return 0
	}
	pages := (totalResults + perPage - 1) / perPage
	if pages < 1 {
		return 1
	}
	return pages
}
