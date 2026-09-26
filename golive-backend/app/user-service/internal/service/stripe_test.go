package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/user-service/internal/service"
)

func paidTopupSession(coins, amountMinor string, amountTotal int64) *service.StripeCheckoutSession {
	return &service.StripeCheckoutSession{
		ID:            "cs_test",
		PaymentStatus: "paid",
		AmountTotal:   amountTotal,
		Currency:      "usd",
		Metadata: map[string]string{
			"coins":        coins,
			"amount_minor": amountMinor,
			"currency":     "usd",
		},
	}
}

func TestTopupCoinsFromSession(t *testing.T) {
	stripe := service.NewStripeServiceWithAPI(service.StripeOptions{SecretKey: "sk_test", CoinsPerCurrencyUnit: 10}, nil)

	coins, err := stripe.TopupCoinsFromSession(paidTopupSession("100", "1000", 1000))
	require.NoError(t, err)
	require.Equal(t, int64(100), coins)

	rejected := map[string]*service.StripeCheckoutSession{
		"nil session":          nil,
		"charged less":         paidTopupSession("100", "1000", 100),
		"coins above cap":      paidTopupSession("4611686018427387914", "100", 100),
		"missing amount_minor": paidTopupSession("100", "", 1000),
		"non-numeric coins":    paidTopupSession("lots", "1000", 1000),
		"zero charge":          paidTopupSession("100", "0", 0),
		"currency mismatch": func() *service.StripeCheckoutSession {
			s := paidTopupSession("100", "1000", 1000)
			s.Currency = "jpy"
			return s
		}(),
	}
	for name, sess := range rejected {
		_, err := stripe.TopupCoinsFromSession(sess)
		require.ErrorIs(t, err, service.ErrTopupAmountInvalid, name)
	}
}

func TestMoneyMinorForCoinsRejectsAmountAboveCap(t *testing.T) {
	stripe := service.NewStripeServiceWithAPI(service.StripeOptions{SecretKey: "sk_test", CoinsPerCurrencyUnit: 10}, nil)

	minor, err := stripe.MoneyMinorForCoins(service.MaxTopupCoins)
	require.NoError(t, err)
	require.Equal(t, service.MaxTopupCoins*10, minor)

	_, err = stripe.MoneyMinorForCoins(service.MaxTopupCoins + 1)
	require.Error(t, err)
}
