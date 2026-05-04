package userlevel

import "testing"

func TestRequiredCoinsForLevelBounds(t *testing.T) {
	if MaxTopupCoin != 10_000_000 {
		t.Fatalf("max topup coin = %d, want 10000000", MaxTopupCoin)
	}
	if got := RequiredCoinsForLevel(1); got != 0 {
		t.Fatalf("level 1 threshold = %d, want 0", got)
	}
	if got := RequiredCoinsForLevel(MaxLevel); got != MaxTopupCoin {
		t.Fatalf("max level threshold = %d, want %d", got, MaxTopupCoin)
	}
	prev := int64(-1)
	for level := 1; level <= MaxLevel; level++ {
		got := RequiredCoinsForLevel(level)
		if got <= prev {
			t.Fatalf("threshold not increasing at level %d: %d <= %d", level, got, prev)
		}
		prev = got
	}
}

func TestSnapshotForTotalTopup(t *testing.T) {
	if got := SnapshotForTotalTopup(0); got.Level != 1 || got.CoinsToNextLevel <= 0 {
		t.Fatalf("zero topup snapshot = %+v", got)
	}
	l2 := RequiredCoinsForLevel(2)
	if got := SnapshotForTotalTopup(l2); got.Level != 2 {
		t.Fatalf("level 2 threshold snapshot = %+v", got)
	}
	if got := SnapshotForTotalTopup(MaxTopupCoin); got.Level != MaxLevel || got.CoinsToNextLevel != 0 {
		t.Fatalf("max topup snapshot = %+v", got)
	}
}
