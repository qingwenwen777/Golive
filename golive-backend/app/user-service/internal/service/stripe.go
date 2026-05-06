package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	stripe "github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/account"
	"github.com/stripe/stripe-go/v84/accountlink"
	"github.com/stripe/stripe-go/v84/checkout/session"
	"github.com/stripe/stripe-go/v84/transfer"
)

var ErrStripeNotConfigured = errors.New("stripe is not configured")

type StripeOptions struct {
	PublishableKey       string
	SecretKey            string
	Currency             string
	CoinsPerCurrencyUnit int64
	ConnectCountry       string
}

type StripeService struct {
	publishableKey       string
	secretKey            string
	currency             string
	coinsPerCurrencyUnit int64
	connectCountry       string
	api                  StripeAPI
}

type StripeAPI interface {
	CreateCheckoutSession(ctx context.Context, req StripeCheckoutCreateRequest) (*StripeCheckoutSession, error)
	RetrieveCheckoutSession(ctx context.Context, id string) (*StripeCheckoutSession, error)
	CreateExpressAccount(ctx context.Context, req StripeAccountCreateRequest) (*StripeAccount, error)
	RetrieveAccount(ctx context.Context, id string) (*StripeAccount, error)
	CreateAccountLink(ctx context.Context, req StripeAccountLinkCreateRequest) (string, error)
	CreateTransfer(ctx context.Context, req StripeTransferCreateRequest) (*StripeTransfer, error)
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

type StripeAccountCreateRequest struct {
	UserID   string
	Email    string
	Country  string
	Currency string
}

type StripeAccount struct {
	ID               string   `json:"accountId,omitempty"`
	ChargesEnabled   bool     `json:"chargesEnabled"`
	PayoutsEnabled   bool     `json:"payoutsEnabled"`
	DetailsSubmitted bool     `json:"detailsSubmitted"`
	TransfersStatus  string   `json:"transfersStatus,omitempty"`
	CurrentlyDue     []string `json:"currentlyDue,omitempty"`
	DisabledReason   string   `json:"disabledReason,omitempty"`
}

type StripeAccountLinkCreateRequest struct {
	AccountID  string
	ReturnURL  string
	RefreshURL string
}

type StripeTransferCreateRequest struct {
	AmountMinor    int64
	Currency       string
	Destination    string
	Description    string
	IdempotencyKey string
	Metadata       map[string]string
}

type StripeTransfer struct {
	ID string
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
	country := strings.ToUpper(strings.TrimSpace(opts.ConnectCountry))
	if country == "" {
		country = "US"
	}
	return &StripeService{
		publishableKey:       strings.TrimSpace(opts.PublishableKey),
		secretKey:            strings.TrimSpace(opts.SecretKey),
		currency:             currency,
		coinsPerCurrencyUnit: coinsPerUnit,
		connectCountry:       country,
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

func (s *StripeService) TestMode() bool {
	if s == nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(s.secretKey), "sk_test_")
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

func (s *StripeService) CreateExpressAccount(ctx context.Context, userID, email string) (*StripeAccount, error) {
	if !s.Configured() {
		return nil, ErrStripeNotConfigured
	}
	return s.api.CreateExpressAccount(ctx, StripeAccountCreateRequest{
		UserID:   strings.TrimSpace(userID),
		Email:    strings.TrimSpace(email),
		Country:  s.connectCountry,
		Currency: s.Currency(),
	})
}

func (s *StripeService) RetrieveAccount(ctx context.Context, id string) (*StripeAccount, error) {
	if !s.Configured() {
		return nil, ErrStripeNotConfigured
	}
	return s.api.RetrieveAccount(ctx, strings.TrimSpace(id))
}

func (s *StripeService) CreateAccountLink(ctx context.Context, accountID, returnURL, refreshURL string) (string, error) {
	if !s.Configured() {
		return "", ErrStripeNotConfigured
	}
	return s.api.CreateAccountLink(ctx, StripeAccountLinkCreateRequest{
		AccountID:  strings.TrimSpace(accountID),
		ReturnURL:  returnURL,
		RefreshURL: refreshURL,
	})
}

func (s *StripeService) CreateTransfer(ctx context.Context, req StripeTransferCreateRequest) (*StripeTransfer, error) {
	if !s.Configured() {
		return nil, ErrStripeNotConfigured
	}
	if req.Currency == "" {
		req.Currency = s.Currency()
	}
	return s.api.CreateTransfer(ctx, req)
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

func (a liveStripeAPI) CreateExpressAccount(ctx context.Context, req StripeAccountCreateRequest) (*StripeAccount, error) {
	if err := a.ensureKey(); err != nil {
		return nil, err
	}
	params := &stripe.AccountParams{
		BusinessType:    stripe.String(string(stripe.AccountBusinessTypeIndividual)),
		Capabilities:    &stripe.AccountCapabilitiesParams{Transfers: &stripe.AccountCapabilitiesTransfersParams{Requested: stripe.Bool(true)}},
		Country:         stripe.String(req.Country),
		DefaultCurrency: stripe.String(req.Currency),
		Email:           optionalString(req.Email),
		Metadata: map[string]string{
			"golive_user_id": req.UserID,
		},
		Type: stripe.String(string(stripe.AccountTypeExpress)),
	}
	params.Context = ctx
	acct, err := account.New(params)
	if err != nil {
		return nil, err
	}
	return accountFromStripe(acct), nil
}

func (a liveStripeAPI) RetrieveAccount(ctx context.Context, id string) (*StripeAccount, error) {
	if err := a.ensureKey(); err != nil {
		return nil, err
	}
	params := &stripe.AccountParams{}
	params.Context = ctx
	acct, err := account.GetByID(id, params)
	if err != nil {
		return nil, err
	}
	return accountFromStripe(acct), nil
}

func (a liveStripeAPI) CreateAccountLink(ctx context.Context, req StripeAccountLinkCreateRequest) (string, error) {
	if err := a.ensureKey(); err != nil {
		return "", err
	}
	params := &stripe.AccountLinkParams{
		Account:    stripe.String(req.AccountID),
		RefreshURL: stripe.String(req.RefreshURL),
		ReturnURL:  stripe.String(req.ReturnURL),
		Type:       stripe.String("account_onboarding"),
	}
	params.Context = ctx
	link, err := accountlink.New(params)
	if err != nil {
		return "", err
	}
	return link.URL, nil
}

func (a liveStripeAPI) CreateTransfer(ctx context.Context, req StripeTransferCreateRequest) (*StripeTransfer, error) {
	if err := a.ensureKey(); err != nil {
		return nil, err
	}
	params := &stripe.TransferParams{
		Amount:      stripe.Int64(req.AmountMinor),
		Currency:    stripe.String(req.Currency),
		Description: stripe.String(req.Description),
		Destination: stripe.String(req.Destination),
		Metadata:    req.Metadata,
	}
	params.Context = ctx
	if req.IdempotencyKey != "" {
		params.SetIdempotencyKey(req.IdempotencyKey)
	}
	tr, err := transfer.New(params)
	if err != nil {
		return nil, err
	}
	return &StripeTransfer{ID: tr.ID}, nil
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

func accountFromStripe(acct *stripe.Account) *StripeAccount {
	if acct == nil {
		return nil
	}
	out := &StripeAccount{
		ID:               acct.ID,
		ChargesEnabled:   acct.ChargesEnabled,
		PayoutsEnabled:   acct.PayoutsEnabled,
		DetailsSubmitted: acct.DetailsSubmitted,
	}
	if acct.Capabilities != nil {
		out.TransfersStatus = string(acct.Capabilities.Transfers)
	}
	if acct.Requirements != nil {
		out.CurrentlyDue = acct.Requirements.CurrentlyDue
		out.DisabledReason = string(acct.Requirements.DisabledReason)
	}
	return out
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return stripe.String(strings.TrimSpace(value))
}
