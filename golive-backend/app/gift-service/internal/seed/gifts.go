// Package seed populates the gift catalog. Mirrors src/mocks/fixtures/gifts.ts
// when present; otherwise produces a minimal but realistic set spanning all
// three categories so the frontend grid renders.
package seed

import (
	"context"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
)

var defaults = []model.Gift{
	// basic
	{ID: "flower", Name: "Flower", NameJa: "花", Icon: "🌸", PriceCoin: 10, Category: "basic", Tier: 0},
	{ID: "donut", Name: "Donut", NameJa: "ドーナツ", Icon: "🍩", PriceCoin: 50, Category: "basic", Tier: 0},
	{ID: "cake", Name: "Cake", NameJa: "ケーキ", Icon: "🍰", PriceCoin: 100, Category: "basic", Tier: 0},
	{ID: "ramen", Name: "Ramen", NameJa: "ラーメン", Icon: "🍜", PriceCoin: 200, Category: "basic", Tier: 0},
	// premium
	{ID: "rocket", Name: "Rocket", NameJa: "ロケット", Icon: "🚀", PriceCoin: 500, Category: "premium", Animation: "fly", Tier: 1},
	{ID: "crown", Name: "Crown", NameJa: "王冠", Icon: "👑", PriceCoin: 1000, Category: "premium", Animation: "explode", Tier: 2},
	{ID: "gem", Name: "Gem", NameJa: "宝石", Icon: "💎", PriceCoin: 2000, Category: "premium", Animation: "rain", Tier: 2},
	// luxury
	{ID: "yacht", Name: "Yacht", NameJa: "ヨット", Icon: "🛥️", PriceCoin: 5000, Category: "luxury", Animation: "rain", Tier: 3},
	{ID: "castle", Name: "Castle", NameJa: "城", Icon: "🏰", PriceCoin: 10000, Category: "luxury", Animation: "rain", Tier: 3},
}

func SeedGifts(ctx context.Context, r *repo.GiftRepo) error {
	for i := range defaults {
		if err := r.Upsert(ctx, &defaults[i]); err != nil {
			return err
		}
	}
	return nil
}
