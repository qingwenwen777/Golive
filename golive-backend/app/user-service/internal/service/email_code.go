package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	mailaddr "net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v9"

	"github.com/qingwenwen777/golive/pkg/errcode"
)

const (
	EmailPurposeRegister      = "register"
	EmailPurposePasswordReset = "password_reset"
	EmailPurposeEmailChange   = "email_change"
)

var (
	ErrInvalidEmailCode   = errcode.New(http.StatusBadRequest, "Invalid email verification code").WithReason("invalid_email_code")
	ErrEmailCodeTooSoon   = errcode.New(http.StatusTooManyRequests, "Please wait before requesting another email code").WithReason("email_code_too_soon")
	ErrEmailNotConfigured = errcode.New(http.StatusServiceUnavailable, "Email verification is not configured").WithReason("email_not_configured")
	ErrEmailSendFailed    = errcode.New(http.StatusBadGateway, "Could not send email verification code").WithReason("email_send_failed")
)

type EmailCodeSendResult struct {
	OK        bool `json:"ok"`
	ExpiresIn int  `json:"expiresIn"`
}

type EmailCodeMailer interface {
	SendVerificationCode(ctx context.Context, toEmail, code string, ttl time.Duration) error
}

type EmailCodeService struct {
	rdb            *redis.Client
	mailer         EmailCodeMailer
	ttl            time.Duration
	resendInterval time.Duration
}

func NewEmailCodeService(rdb *redis.Client, mailer EmailCodeMailer, ttl, resendInterval time.Duration) *EmailCodeService {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if resendInterval <= 0 {
		resendInterval = time.Minute
	}
	return &EmailCodeService{
		rdb:            rdb,
		mailer:         mailer,
		ttl:            ttl,
		resendInterval: resendInterval,
	}
}

func (s *EmailCodeService) Send(ctx context.Context, purpose, email string) (*EmailCodeSendResult, error) {
	purpose, ok := normalizeEmailPurpose(purpose)
	if !ok {
		return nil, ErrInvalidRegister.WithReason("invalid_email_code_purpose")
	}
	cleanEmail, ok := normalizeEmail(email)
	if !ok {
		return nil, ErrInvalidRegister.WithReason("invalid_email")
	}
	if s == nil || s.rdb == nil || s.mailer == nil {
		return nil, ErrEmailNotConfigured
	}

	target := emailCodeTarget(purpose, cleanEmail)
	allowed, err := s.rdb.SetNX(ctx, emailCodeRateKey(target), "1", s.resendInterval).Result()
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrEmailCodeTooSoon
	}

	code, err := randomEmailCode(6)
	if err != nil {
		return nil, err
	}
	if err := s.rdb.Set(ctx, emailCodeKey(target), code, s.ttl).Err(); err != nil {
		return nil, err
	}
	if err := s.mailer.SendVerificationCode(ctx, cleanEmail, code, s.ttl); err != nil {
		_ = s.rdb.Del(ctx, emailCodeKey(target), emailCodeRateKey(target)).Err()
		return nil, ErrEmailSendFailed
	}
	return &EmailCodeSendResult{OK: true, ExpiresIn: int(s.ttl.Seconds())}, nil
}

func (s *EmailCodeService) Verify(ctx context.Context, purpose, email, code string) error {
	purpose, ok := normalizeEmailPurpose(purpose)
	if !ok {
		return ErrInvalidEmailCode
	}
	cleanEmail, ok := normalizeEmail(email)
	if !ok {
		return ErrInvalidEmailCode
	}
	cleanCode := strings.TrimSpace(code)
	if cleanCode == "" || s == nil || s.rdb == nil {
		return ErrInvalidEmailCode
	}

	target := emailCodeTarget(purpose, cleanEmail)
	expected, err := s.rdb.Get(ctx, emailCodeKey(target)).Result()
	if err != nil {
		return ErrInvalidEmailCode
	}
	if subtle.ConstantTimeCompare([]byte(cleanCode), []byte(expected)) != 1 {
		return ErrInvalidEmailCode
	}
	_ = s.rdb.Del(ctx, emailCodeKey(target), emailCodeRateKey(target)).Err()
	return nil
}

func normalizeEmailPurpose(raw string) (string, bool) {
	switch strings.TrimSpace(raw) {
	case EmailPurposeRegister:
		return EmailPurposeRegister, true
	case EmailPurposePasswordReset:
		return EmailPurposePasswordReset, true
	case EmailPurposeEmailChange:
		return EmailPurposeEmailChange, true
	default:
		return "", false
	}
}

func emailCodeTarget(purpose, email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return purpose + ":" + hex.EncodeToString(sum[:])
}

func emailCodeKey(target string) string {
	return "email_code:" + target
}

func emailCodeRateKey(target string) string {
	return "email_code_rate:" + target
}

func randomEmailCode(length int) (string, error) {
	var b strings.Builder
	b.Grow(length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		b.WriteByte(byte('0' + n.Int64()))
	}
	return b.String(), nil
}

type SMTPMailer struct {
	host        string
	port        int
	username    string
	password    string
	senderEmail string
	senderName  string
}

type SMTPMailerConfig struct {
	Host        string
	Port        int
	Username    string
	Password    string
	SenderEmail string
	SenderName  string
}

func NewSMTPMailer(c SMTPMailerConfig) (*SMTPMailer, error) {
	c.Host = strings.TrimSpace(c.Host)
	c.Username = strings.TrimSpace(c.Username)
	c.SenderEmail = strings.TrimSpace(c.SenderEmail)
	c.SenderName = strings.TrimSpace(c.SenderName)
	if c.Port == 0 {
		c.Port = 587
	}
	if c.Host == "" || c.Username == "" || c.Password == "" || c.SenderEmail == "" {
		return nil, ErrEmailNotConfigured
	}
	if _, err := mailaddr.ParseAddress(c.SenderEmail); err != nil {
		return nil, fmt.Errorf("parse sender email: %w", err)
	}
	if c.SenderName == "" {
		c.SenderName = "GoLive"
	}
	return &SMTPMailer{
		host:        c.Host,
		port:        c.Port,
		username:    c.Username,
		password:    c.Password,
		senderEmail: c.SenderEmail,
		senderName:  c.SenderName,
	}, nil
}

func (m *SMTPMailer) SendVerificationCode(ctx context.Context, toEmail, code string, ttl time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	from := mailaddr.Address{Name: m.senderName, Address: m.senderEmail}
	to := mailaddr.Address{Address: toEmail}
	msg := buildEmailCodeMessage(from.String(), to.String(), code, ttl)
	addr := m.host + ":" + strconv.Itoa(m.port)
	auth := smtp.PlainAuth("", m.username, m.password, m.host)
	return smtp.SendMail(addr, auth, m.senderEmail, []string{toEmail}, []byte(msg))
}

func buildEmailCodeMessage(from, to, code string, ttl time.Duration) string {
	minutes := int(ttl.Round(time.Minute).Minutes())
	if minutes <= 0 {
		minutes = 10
	}
	body := fmt.Sprintf("Your GoLive verification code is: %s\n\nThis code expires in %d minutes. If you did not request it, you can ignore this email.\n", code, minutes)
	headers := []string{
		"From: " + from,
		"To: " + to,
		"Subject: GoLive email verification code",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
	}
	return strings.Join(headers, "\r\n") + "\r\n\r\n" + body
}
