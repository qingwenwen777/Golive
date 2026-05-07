package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/user-service/internal/service"
)

type captureMailer struct {
	to   string
	code string
	ttl  time.Duration
}

func (m *captureMailer) SendVerificationCode(_ context.Context, toEmail, code string, ttl time.Duration) error {
	m.to = toEmail
	m.code = code
	m.ttl = ttl
	return nil
}

func TestEmailCodeSendAndVerify(t *testing.T) {
	_, rdb := newMiniredis(t)
	mailer := &captureMailer{}
	codes := service.NewEmailCodeService(rdb, mailer, 10*time.Minute, time.Minute)

	resp, err := codes.Send(context.Background(), service.EmailPurposeRegister, "User@Example.COM")
	require.NoError(t, err)
	require.True(t, resp.OK)
	require.Equal(t, 600, resp.ExpiresIn)
	require.Equal(t, "user@example.com", mailer.to)
	require.Len(t, mailer.code, 6)
	require.Equal(t, 10*time.Minute, mailer.ttl)

	require.ErrorIs(t, codes.Verify(context.Background(), service.EmailPurposeRegister, "user@example.com", "000000"), service.ErrInvalidEmailCode)
	require.NoError(t, codes.Verify(context.Background(), service.EmailPurposeRegister, "user@example.com", mailer.code))
	require.ErrorIs(t, codes.Verify(context.Background(), service.EmailPurposeRegister, "user@example.com", mailer.code), service.ErrInvalidEmailCode)
}

func TestEmailCodeSendRateLimit(t *testing.T) {
	_, rdb := newMiniredis(t)
	codes := service.NewEmailCodeService(rdb, &captureMailer{}, 10*time.Minute, time.Minute)

	_, err := codes.Send(context.Background(), service.EmailPurposePasswordReset, "user@example.com")
	require.NoError(t, err)
	_, err = codes.Send(context.Background(), service.EmailPurposePasswordReset, "user@example.com")
	require.ErrorIs(t, err, service.ErrEmailCodeTooSoon)
}

func TestEmailCodeVerifyInvalidatesAfterTooManyFailures(t *testing.T) {
	_, rdb := newMiniredis(t)
	mailer := &captureMailer{}
	codes := service.NewEmailCodeService(rdb, mailer, 10*time.Minute, time.Minute)

	_, err := codes.Send(context.Background(), service.EmailPurposePasswordReset, "user@example.com")
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		require.ErrorIs(t, codes.Verify(context.Background(), service.EmailPurposePasswordReset, "user@example.com", "000000"), service.ErrInvalidEmailCode)
	}
	require.ErrorIs(t, codes.Verify(context.Background(), service.EmailPurposePasswordReset, "user@example.com", mailer.code), service.ErrInvalidEmailCode)
}
