package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBeijingDailyTaskDateUsesUTCPlus8(t *testing.T) {
	got := beijingDailyTaskDate(time.Date(2026, 5, 3, 16, 0, 0, 0, time.UTC))
	require.Equal(t, "2026-05-04", got)
}
