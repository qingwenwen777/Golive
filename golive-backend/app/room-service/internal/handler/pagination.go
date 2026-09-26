package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	// maxPageSize caps ?size= on every paginated endpoint. Each listed item
	// costs follow-up lookups, so an unbounded size is an easy way to make one
	// request do thousands of queries.
	maxPageSize = 100
	// maxPage bounds ?page= so a request can't force a deep OFFSET scan.
	maxPage = 1000
)

// pageQuery reads ?page= and ?size=, falling back to the defaults for missing
// or invalid values and clamping both to maxPage / maxPageSize.
func pageQuery(c *gin.Context, defaultPage, defaultSize int) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", strconv.Itoa(defaultPage)))
	if page < 1 {
		page = defaultPage
	}
	if page > maxPage {
		page = maxPage
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
