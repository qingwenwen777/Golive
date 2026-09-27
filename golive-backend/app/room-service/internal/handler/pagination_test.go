package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func queryContext(rawQuery string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?"+rawQuery, nil)
	return c
}

func TestPageQueryClampsPageAndSize(t *testing.T) {
	cases := []struct {
		query    string
		page     int
		size     int
		scenario string
	}{
		{"", 1, 24, "defaults"},
		{"page=3&size=50", 3, 50, "in range"},
		{"page=0&size=0", 1, 24, "zero falls back to defaults"},
		{"page=-4&size=-1", 1, 24, "negative falls back to defaults"},
		{"page=abc&size=xyz", 1, 24, "garbage falls back to defaults"},
		{"size=100000", 1, maxPageSize, "size capped"},
		{"page=1000", maxPage, 24, "last page served"},
		{"page=99999999", maxPage + 1, 24, "past the last page"},
	}
	for _, tc := range cases {
		page, size := pageQuery(queryContext(tc.query), 1, 24)
		require.Equal(t, tc.page, page, tc.scenario)
		require.Equal(t, tc.size, size, tc.scenario)
	}
}

func TestPageSizeHelpersShareTheCap(t *testing.T) {
	c := queryContext("page=2&size=5000")
	page, size := pageSize(c, 1, 20)
	require.Equal(t, 2, page)
	require.Equal(t, maxPageSize, size)
	page, size = postPageQuery(c, 1, 10)
	require.Equal(t, 2, page)
	require.Equal(t, maxPageSize, size)
}

func TestSizeQuery(t *testing.T) {
	require.Equal(t, 12, sizeQuery(queryContext(""), 12, 40))
	require.Equal(t, 7, sizeQuery(queryContext("size=7"), 12, 40))
	require.Equal(t, 12, sizeQuery(queryContext("size=0"), 12, 40))
	require.Equal(t, 40, sizeQuery(queryContext("size=41"), 12, 40))
}
