package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

const (
	// maxPageSize caps ?size= on every paginated endpoint. Each listed item
	// costs follow-up lookups, so an unbounded size is an easy way to make one
	// request do thousands of queries.
	maxPageSize = 100
	// maxPage is the deepest page a list serves, so a request can't force a
	// deep OFFSET scan: the repos answer deeper pages with no rows.
	maxPage = repo.MaxPage
)

// pageQuery reads ?page= and ?size=, falling back to the defaults for missing
// or invalid values and capping size at maxPageSize. A page past maxPage
// becomes maxPage+1: an empty page, not maxPage's items again, with the
// offset arithmetic kept small.
func pageQuery(c *gin.Context, defaultPage, defaultSize int) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", strconv.Itoa(defaultPage)))
	if page < 1 {
		page = defaultPage
	}
	if page > maxPage {
		page = maxPage + 1
	}
	return page, sizeQuery(c, defaultSize, maxPageSize)
}

// sizeQuery reads ?size= for endpoints without pages, falling back to
// defaultSize for missing or invalid values and clamping to maxSize.
func sizeQuery(c *gin.Context, defaultSize, maxSize int) int {
	size, _ := strconv.Atoi(c.DefaultQuery("size", strconv.Itoa(defaultSize)))
	if size < 1 {
		size = defaultSize
	}
	if size > maxSize {
		size = maxSize
	}
	return size
}
