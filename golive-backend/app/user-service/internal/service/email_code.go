package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	// EmailPurposeLogin codes stand in for the login captcha for people who
	// cannot read its image. They go only to an account's verified email.
	EmailPurposeLogin = "login"

	emailCodeMaxVerifyFailures = 5
	// DefaultEmailCodesPerHour caps the codes sent to one address for one
	// purpose per hour. Every code allows emailCodeMaxVerifyFailures guesses,
	// so the cap also bounds the guesses per address.
	DefaultEmailCodesPerHour = 5
	emailCodeSendWindow      = time.Hour
)

var (
	ErrInvalidEmailCode   = errcode.New(http.StatusBadRequest, "Invalid email verification code").WithReason("invalid_email_code")
	ErrEmailCodeTooSoon   = errcode.New(http.StatusTooManyRequests, "Please wait before requesting another email code").WithReason("email_code_too_soon")
	ErrEmailCodeLimit     = errcode.New(http.StatusTooManyRequests, "Too many email codes were requested for this address. Please try again later").WithReason("email_code_limit")
	ErrEmailNotConfigured = errcode.New(http.StatusServiceUnavailable, "Email verification is not configured").WithReason("email_not_configured")
	ErrEmailSendFailed    = errcode.New(http.StatusBadGateway, "Could not send email verification code").WithReason("email_send_failed")
)

// emailCodeIssueScript stores a new code, replacing any earlier one and its
// failed guesses. It returns "too_soon" while the resend interval runs,
// "limit" once the address had all its codes for the window, else "ok".
var emailCodeIssueScript = redis.NewScript(`
local codeKey, resendKey, failKey, sendsKey = KEYS[1], KEYS[2], KEYS[3], KEYS[4]
local code, codeTTL, resendTTL = ARGV[1], ARGV[2], ARGV[3]
local maxSends, window = tonumber(ARGV[4]), ARGV[5]

if redis.call("EXISTS", resendKey) == 1 then
	return "too_soon"
end
if tonumber(redis.call("GET", sendsKey) or "0") >= maxSends then
	return "limit"
end
redis.call("SET", resendKey, "1", "PX", resendTTL)
redis.call("INCR", sendsKey)
if redis.call("PTTL", sendsKey) < 0 then
	redis.call("PEXPIRE", sendsKey, window)
end
redis.call("SET", codeKey, code, "PX", codeTTL)
redis.call("DEL", failKey)
return "ok"
`)

// emailCodeWithdrawScript drops a code whose email could not be sent, so the
// address can ask again right away, and takes it off the window's count.
var emailCodeWithdrawScript = redis.NewScript(`
local codeKey, resendKey, failKey, sendsKey = KEYS[1], KEYS[2], KEYS[3], KEYS[4]

redis.call("DEL", codeKey, resendKey, failKey)
if tonumber(redis.call("GET", sendsKey) or "0") > 0 then
	redis.call("DECR", sendsKey)
end
return 0
`)

// emailCodeVerifyScript checks a guess and records the result in one step, so
// guesses sent in parallel cannot all be compared with the code before their
// failures are counted. A match returns 1 and deletes the code (it is
// single-use) with its resend and failure keys. A miss returns 0 and counts a
// failure, deleting the code at the limit; a missing code also returns 0.
// SHA-1 digests are compared so the comparison's timing says nothing about
// the code.
var emailCodeVerifyScript = redis.NewScript(`
local codeKey, resendKey, failKey = KEYS[1], KEYS[2], KEYS[3]
local guess, maxFailures, fallbackTTL = ARGV[1], tonumber(ARGV[2]), ARGV[3]

local code = redis.call("GET", codeKey)
if not code then
	return 0
end
if redis.sha1hex(code) == redis.sha1hex(guess) then
	redis.call("DEL", codeKey, resendKey, failKey)
	return 1
end
local failures = redis.call("INCR", failKey)
if failures == 1 then
	local ttl = redis.call("PTTL", codeKey)
	if ttl <= 0 then
		ttl = fallbackTTL
	end
	redis.call("PEXPIRE", failKey, ttl)
end
if failures >= maxFailures then
	redis.call("DEL", codeKey, failKey)
end
return 0
`)

type EmailCodeSendResult struct {
	OK        bool `json:"ok"`
	ExpiresIn int  `json:"expiresIn"`
}

type EmailCodeMailer interface {
	SendVerificationCode(ctx context.Context, toEmail, code string, ttl time.Duration) error
}

type EmailCodeService struct {
	rdb             *redis.Client
	mailer          EmailCodeMailer
	ttl             time.Duration
	resendInterval  time.Duration
	maxCodesPerHour int
}

func NewEmailCodeService(rdb *redis.Client, mailer EmailCodeMailer, ttl, resendInterval time.Duration) *EmailCodeService {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if resendInterval <= 0 {
		resendInterval = time.Minute
	}
	return &EmailCodeService{
		rdb:             rdb,
		mailer:          mailer,
		ttl:             ttl,
		resendInterval:  resendInterval,
		maxCodesPerHour: DefaultEmailCodesPerHour,
	}
}

// SetMaxCodesPerHour caps the codes sent to one address for one purpose per
// hour; n <= 0 keeps DefaultEmailCodesPerHour.
func (s *EmailCodeService) SetMaxCodesPerHour(n int) {
	if n > 0 {
		s.maxCodesPerHour = n
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

	code, err := randomEmailCode(6)
	if err != nil {
		return nil, err
	}
	target := emailCodeTarget(purpose, cleanEmail)
	keys := []string{emailCodeKey(target), emailCodeRateKey(target), emailCodeFailKey(target), emailCodeSendsKey(target)}
	issued, err := emailCodeIssueScript.Run(ctx, s.rdb, keys, code, s.ttl.Milliseconds(),
		s.resendInterval.Milliseconds(), s.maxCodesPerHour, emailCodeSendWindow.Milliseconds()).Text()
	if err != nil {
		return nil, err
	}
	switch issued {
	case "too_soon":
		return nil, ErrEmailCodeTooSoon
	case "limit":
		return nil, ErrEmailCodeLimit
	}
	if err := s.mailer.SendVerificationCode(ctx, cleanEmail, code, s.ttl); err != nil {
		_ = emailCodeWithdrawScript.Run(ctx, s.rdb, keys).Err()
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
	keys := []string{emailCodeKey(target), emailCodeRateKey(target), emailCodeFailKey(target)}
	matched, err := emailCodeVerifyScript.Run(ctx, s.rdb, keys, cleanCode, emailCodeMaxVerifyFailures, s.ttl.Milliseconds()).Int()
	if err != nil {
		return err
	}
	if matched != 1 {
		return ErrInvalidEmailCode
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
	case EmailPurposeLogin:
		return EmailPurposeLogin, true
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

// emailCodeSendsKey counts the codes sent to target in the current window.
func emailCodeSendsKey(target string) string {
	return "email_code_sends:" + target
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
