package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"math/big"
	"net"
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

	emailCodeMaxVerifyFailures = 5
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
	_ = s.rdb.Del(ctx, emailCodeFailKey(target)).Err()
	if err := s.mailer.SendVerificationCode(ctx, cleanEmail, code, s.ttl); err != nil {
		_ = s.rdb.Del(ctx, emailCodeKey(target), emailCodeRateKey(target), emailCodeFailKey(target)).Err()
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
		if err := s.recordEmailCodeVerifyFailure(ctx, target); err != nil {
			return err
		}
		return ErrInvalidEmailCode
	}
	_ = s.rdb.Del(ctx, emailCodeKey(target), emailCodeRateKey(target), emailCodeFailKey(target)).Err()
	return nil
}

func (s *EmailCodeService) recordEmailCodeVerifyFailure(ctx context.Context, target string) error {
	failKey := emailCodeFailKey(target)
	count, err := s.rdb.Incr(ctx, failKey).Result()
	if err != nil {
		return err
	}
	if count == 1 {
		ttl := s.ttl
		if codeTTL, err := s.rdb.TTL(ctx, emailCodeKey(target)).Result(); err == nil && codeTTL > 0 {
			ttl = codeTTL
		}
		if err := s.rdb.Expire(ctx, failKey, ttl).Err(); err != nil {
			return err
		}
	}
	if count >= emailCodeMaxVerifyFailures {
		return s.rdb.Del(ctx, emailCodeKey(target), failKey).Err()
	}
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

func emailCodeFailKey(target string) string {
	return "email_code_fail:" + target
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
	timeout     time.Duration
}

type SMTPMailerConfig struct {
	Host        string
	Port        int
	Username    string
	Password    string
	SenderEmail string
	SenderName  string
	Timeout     time.Duration
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
	if c.Timeout <= 0 {
		c.Timeout = 10 * time.Second
	}
	return &SMTPMailer{
		host:        c.Host,
		port:        c.Port,
		username:    c.Username,
		password:    c.Password,
		senderEmail: c.SenderEmail,
		senderName:  c.SenderName,
		timeout:     c.Timeout,
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
	return sendSMTPMessage(ctx, addr, m.host, m.port == 465, auth, m.senderEmail, []string{toEmail}, []byte(msg), m.timeout)
}

func sendSMTPMessage(ctx context.Context, addr, host string, implicitTLS bool, auth smtp.Auth, from string, to []string, msg []byte, timeout time.Duration) error {
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}

	var client *smtp.Client
	if implicitTLS {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.Handshake(); err != nil {
			return err
		}
		client, err = smtp.NewClient(tlsConn, host)
	} else {
		client, err = smtp.NewClient(conn, host)
	}
	if err != nil {
		return err
	}
	defer client.Close()

	if !implicitTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		}
	}
	if auth != nil {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
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
