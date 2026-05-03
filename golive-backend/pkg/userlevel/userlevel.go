package userlevel

const (
	MaxLevel     = 99
	MaxTopupCoin = int64(1_000_000_000)
)

type Snapshot struct {
	Level                int   `json:"level"`
	MaxLevel             int   `json:"maxLevel"`
	TotalTopupCoins      int64 `json:"totalTopupCoins"`
	CurrentLevelMinCoins int64 `json:"currentLevelMinCoins"`
	NextLevelTargetCoins int64 `json:"nextLevelTargetCoins"`
	CoinsToNextLevel     int64 `json:"coinsToNextLevel"`
}

func RequiredCoinsForLevel(level int) int64 {
	if level <= 1 {
		return 0
	}
	if level >= MaxLevel {
		return MaxTopupCoin
	}
	step := int64(level - 1)
	maxStep := int64(MaxLevel - 1)
	num := MaxTopupCoin * step * step * step
	den := maxStep * maxStep * maxStep
	return (num + den - 1) / den
}

func LevelForTotalTopup(total int64) int {
	if total <= 0 {
		return 1
	}
	if total >= MaxTopupCoin {
		return MaxLevel
	}
	level := 1
	for next := 2; next <= MaxLevel; next++ {
		if total < RequiredCoinsForLevel(next) {
			break
		}
		level = next
	}
	return level
}

func SnapshotForTotalTopup(total int64) Snapshot {
	if total < 0 {
		total = 0
	}
	level := LevelForTotalTopup(total)
	currentMin := RequiredCoinsForLevel(level)
	nextTarget := MaxTopupCoin
	toNext := int64(0)
	if level < MaxLevel {
		nextTarget = RequiredCoinsForLevel(level + 1)
		toNext = nextTarget - total
		if toNext < 0 {
			toNext = 0
		}
	}
	return Snapshot{
		Level:                level,
		MaxLevel:             MaxLevel,
		TotalTopupCoins:      total,
		CurrentLevelMinCoins: currentMin,
		NextLevelTargetCoins: nextTarget,
		CoinsToNextLevel:     toNext,
	}
}
