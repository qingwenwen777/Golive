// Package seed populates the gift catalog. Mirrors src/mocks/fixtures/gifts.ts
// when present; otherwise produces a realistic set spanning all three
// categories so the frontend grid renders.
package seed

import (
	"context"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
)

var defaults = []model.Gift{
	// basic
	{ID: "flower", Name: "Flower", NameJa: "Flower", Icon: "\U0001F33C", PriceCoin: 10, Category: "basic", Tier: 0, UnlockLevel: 1},
	{ID: "donut", Name: "Donut", NameJa: "Donut", Icon: "\U0001F369", PriceCoin: 50, Category: "basic", Tier: 0, UnlockLevel: 1},
	{ID: "cake", Name: "Cake", NameJa: "Cake", Icon: "\U0001F370", PriceCoin: 100, Category: "basic", Tier: 0, UnlockLevel: 1},
	{ID: "ramen", Name: "Ramen", NameJa: "Ramen", Icon: "\U0001F35C", PriceCoin: 200, Category: "basic", Tier: 0, UnlockLevel: 1},

	// premium
	{ID: "rocket", Name: "Rocket", NameJa: "Rocket", Icon: "\U0001F680", PriceCoin: 500, Category: "premium", Animation: "fly", Tier: 1, UnlockLevel: 1},
	{ID: "starlight", Name: "Starlight", NameJa: "Starlight", Icon: "\U0001F31F", PriceCoin: 800, Category: "premium", Animation: "rain", Tier: 1, UnlockLevel: 6},
	{ID: "fan_light", Name: "Fan Light", NameJa: "Fan Light", Icon: "\U0001F4A1", PriceCoin: 1000, Category: "premium", Animation: "rain", Tier: 2, UnlockLevel: 1},
	{ID: "crown", Name: "Crown", NameJa: "Crown", Icon: "\U0001F451", PriceCoin: 1000, Category: "premium", Animation: "explode", Tier: 2, UnlockLevel: 1},
	{ID: "gem", Name: "Gem", NameJa: "Gem", Icon: "\U0001F48E", PriceCoin: 2000, Category: "premium", Animation: "rain", Tier: 2, UnlockLevel: 1},
	{ID: "aurora", Name: "Aurora", NameJa: "Aurora", Icon: "\U0001F308", PriceCoin: 3000, Category: "premium", Animation: "rain", Tier: 2, UnlockLevel: 12},

	// luxury
	{ID: "yacht", Name: "Yacht", NameJa: "Yacht", Icon: "\U0001F6E5\uFE0F", PriceCoin: 5000, Category: "luxury", Animation: "rain", Tier: 3, UnlockLevel: 1},
	{ID: "castle", Name: "Castle", NameJa: "Castle", Icon: "\U0001F3F0", PriceCoin: 10000, Category: "luxury", Animation: "rain", Tier: 3, UnlockLevel: 1},
	{ID: "meteor", Name: "Meteor", NameJa: "Meteor", Icon: "\u2604\uFE0F", PriceCoin: 12000, Category: "luxury", Animation: "explode", Tier: 3, UnlockLevel: 24},
	{ID: "galaxy_ship", Name: "Galaxy Ship", NameJa: "Galaxy Ship", Icon: "\U0001F6F8", PriceCoin: 50000, Category: "luxury", Animation: "fly", Tier: 3, UnlockLevel: 42},
	{ID: "royal_crown", Name: "Royal Crown", NameJa: "Royal Crown", Icon: "\U0001F48D", PriceCoin: 120000, Category: "luxury", Animation: "explode", Tier: 3, UnlockLevel: 60},
	{ID: "nebula_ring", Name: "Nebula Ring", NameJa: "Nebula Ring", Icon: "\U0001FA90", PriceCoin: 520000, Category: "luxury", Animation: "rain", Tier: 3, UnlockLevel: 78},
	{ID: "eternal_scepter", Name: "Eternal Scepter", NameJa: "Eternal Scepter", Icon: "\u2728", PriceCoin: 1314000, Category: "luxury", Animation: "explode", Tier: 3, UnlockLevel: 92},
}

func SeedGifts(ctx context.Context, r *repo.GiftRepo) error {
	for i := range defaults {
		if defaults[i].UnlockLevel <= 0 {
			defaults[i].UnlockLevel = 1
		}
		if err := r.Upsert(ctx, &defaults[i]); err != nil {
			return err
		}
	}
	return nil
}
