package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
	"github.com/qingwenwen777/golive/app/gift-service/internal/service"
)

// Super chat text used to be checked only by the web client: text longer
// than the ledger's description column (varchar(255)) failed in MySQL with
// "Data too long", a 500. It must be refused up front, and the longest text
// a tier allows must store in every column that keeps it.
func TestMySQLSuperChat_TextLimitFitsColumns(t *testing.T) {
	db, _ := newMySQLTestDB(t)
	insertMySQLUser(t, db, "u-owner", 0)
	insertMySQLUser(t, db, "u-fan", 100000)
	svc := service.NewSuperChatService(repo.NewOrderRepo(db))
	ctx := context.Background()

	_, _, err := svc.Send(ctx, service.SendSuperChatReq{
		UserID: "u-fan", RoomID: "r1", Amount: 10000, Text: strings.Repeat("😀", 300), RequestID: "sc-too-long",
	})
	require.ErrorIs(t, err, service.ErrSuperChatTextTooLong)
	require.Equal(t, int64(100000), balanceOf(t, db, "u-fan"))

	// Four-byte runes at the top tier's limit.
	longest := strings.Repeat("😀", service.SuperChatMaxText(service.AmountToTier(10000)))
	order, _, err := svc.Send(ctx, service.SendSuperChatReq{
		UserID: "u-fan", RoomID: "r1", Amount: 10000, Text: longest, RequestID: "sc-longest",
	})
	require.NoError(t, err)
	var stored model.SuperChatOrder
	require.NoError(t, db.Where("order_id = ?", order.OrderID).Take(&stored).Error)
	require.Equal(t, longest, stored.Text)
	var descriptions []string
	require.NoError(t, db.Model(&model.CoinTransaction{}).
		Where("source_id = ?", order.OrderID).
		Pluck("description", &descriptions).Error)
	require.Equal(t, []string{longest, longest}, descriptions, "spend and income ledger rows")
	var payload string
	require.NoError(t, db.Model(&model.LocalMessage{}).
		Where("biz_id = ?", order.OrderID).
		Pluck("payload", &payload).Error)
	require.Contains(t, payload, longest)
	require.Equal(t, int64(90000), balanceOf(t, db, "u-fan"))
}
