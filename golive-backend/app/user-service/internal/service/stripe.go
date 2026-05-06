package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	stripe "github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/checkout/session"
)

var ErrStripeNotConfigured = errors.New("stripe is not configured")

type StripeOptions struct {
	PublishableKey       string
	SecretKey            string
	Currency             string
	CoinsPerCurrencyUnit int64
}

type StripeService struct {
	publishableKey       string
	secretKey            string
	currency             string
	coinsPerCurrencyUnit int64
	api                  StripeAPI
}

type StripeAPI interface {
	CreateCheckoutSession(ctx context.Context, req StripeCheckoutCreateRequest) (*StripeCheckoutSession, error)
	RetrieveCheckoutSession(ctx context.Context, id string) (*StripeCheckoutSession, error)
}

type StripeCheckoutCreateRequest struct {
	UserID      string
	UserEmail   string
	AmountCoins int64
	AmountMinor int64
	Currency    string
	SuccessURL  string
	CancelURL   string
	ProductName string
	Metadata    map[string]string
}

type StripeCheckoutSession struct {
	ID                string
	URL               string
	PaymentStatus     string
	ClientReferenceID string
	Metadata          map[string]string
}

func NewStripeService(opts StripeOptions) *StripeService {
	return NewStripeServiceWithAPI(opts, liveStripeAPI{secretKey: strings.TrimSpace(opts.SecretKey)})
}

func NewStripeServiceWithAPI(opts StripeOptions, api StripeAPI) *StripeService {
	currency := strings.ToLower(strings.TrimSpace(opts.Currency))
	if currency == "" {
		currency = "usd"
	}
	coinsPerUnit := opts.CoinsPerCurrencyUnit
	if coinsPerUnit <= 0 {
		coinsPerUnit = 10
	}
	return &StripeService{
		publishableKey:       strings.TrimSpace(opts.PublishableKey),
		secretKey:            strings.TrimSpace(opts.SecretKey),
		currency:             currency,
		coinsPerCurrencyUnit: coinsPerUnit,
		api:                  api,
	}
}

func (s *StripeService) Configured() bool {
	return s != nil && strings.TrimSpace(s.secretKey) != "" && s.api != nil
}

func (s *StripeService) PublishableKey() string {
	if s == nil {
		return ""
	}
	return s.publishableKey
}

func (s *StripeService) Currency() string {
	if s == nil || s.currency == "" {
		return "usd"
	}
	return s.currency
}

func (s *StripeService) CoinsPerCurrencyUnit() int64 {
	if s == nil || s.coinsPerCurrencyUnit <= 0 {
		return 10
	}
	return s.coinsPerCurrencyUnit
}

func (s *StripeService) MoneyMinorForCoins(coins int64) (int64, error) {
	if coins <= 0 {
		return 0, errors.New("coin amount must be positive")
	}
	minor := coins * 100 / s.CoinsPerCurrencyUnit()
	if minor <= 0 {
		return 0, errors.New("coin amount is too small for stripe")
	}
	return minor, nil
}

func (s *StripeService) CreateTopupCheckout(ctx context.Context, userID, email string, amountCoins int64, successURL, cancelURL string) (*StripeCheckoutSession, error) {
	if !s.Configured() {
		return nil, ErrStripeNotConfigured
	}
	amountMinor, err := s.MoneyMinorForCoins(amountCoins)
	if err != nil {
		return nil, err
	}
	metadata := map[string]string{
		"kind":         "coin_topup",
		"user_id":      strings.TrimSpace(userID),
		"coins":        fmt.Sprintf("%d", amountCoins),
		"currency":     s.Currency(),
		"amount_minor": fmt.Sprintf("%d", amountMinor),
	}
	return s.api.CreateCheckoutSession(ctx, StripeCheckoutCreateRequest{
		UserID:      strings.TrimSpace(userID),
		UserEmail:   strings.TrimSpace(email),
		AmountCoins: amountCoins,
		AmountMinor: amountMinor,
		Currency:    s.Currency(),
		SuccessURL:  successURL,
		CancelURL:   cancelURL,
		ProductName: fmt.Sprintf("GoLive %d coins", amountCoins),
		Metadata:    metadata,
	})
}

func (s *StripeService) RetrieveCheckoutSession(ctx context.Context, id string) (*StripeCheckoutSession, error) {
	if !s.Configured() {
		return nil, ErrStripeNotConfigured
	}
	return s.api.RetrieveCheckoutSession(ctx, strings.TrimSpace(id))
}

type liveStripeAPI struct {
	secretKey string
}

func (a liveStripeAPI) ensureKey() error {
	if strings.TrimSpace(a.secretKey) == "" {
		return ErrStripeNotConfigured
	}
	stripe.Key = strings.TrimSpace(a.secretKey)
	return nil
}

func (a liveStripeAPI) CreateCheckoutSession(ctx context.Context, req StripeCheckoutCreateRequest) (*StripeCheckoutSession, error) {
	if err := a.ensureKey(); err != nil {
		return nil, err
	}
	params := &stripe.CheckoutSessionParams{
		CancelURL:         stripe.String(req.CancelURL),
		ClientReferenceID: stripe.String(req.UserID),
		CustomerEmail:     optionalString(req.UserEmail),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency: stripe.String(req.Currency),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String(req.ProductName),
					},
					UnitAmount: stripe.Int64(req.AmountMinor),
				},
				Quantity: stripe.Int64(1),
			},
		},
		Metadata:           req.Metadata,
		Mode:               stripe.String(string(stripe.CheckoutSessionModePayment)),
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		PaymentIntentData: &stripe.CheckoutSessionPaymentIntentDataParams{
			Metadata: req.Metadata,
		},
		SuccessURL: stripe.String(req.SuccessURL),
	}
	params.Context = ctx
	sess, err := session.New(params)
	if err != nil {
		return nil, err
	}
	return checkoutSessionFromStripe(sess), nil
}

func (a liveStripeAPI) RetrieveCheckoutSession(ctx context.Context, id string) (*StripeCheckoutSession, error) {
	if err := a.ensureKey(); err != nil {
		return nil, err
	}
	params := &stripe.CheckoutSessionParams{}
	params.Context = ctx
	sess, err := session.Get(id, params)
	if err != nil {
		return nil, err
	}
	return checkoutSessionFromStripe(sess), nil
}

func checkoutSessionFromStripe(sess *stripe.CheckoutSession) *StripeCheckoutSession {
	if sess == nil {
		return nil
	}
	return &StripeCheckoutSession{
		ID:                sess.ID,
		URL:               sess.URL,
		PaymentStatus:     string(sess.PaymentStatus),
		ClientReferenceID: sess.ClientReferenceID,
		Metadata:          sess.Metadata,
	}
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return stripe.String(strings.TrimSpace(value))
}
