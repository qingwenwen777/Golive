package service_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-redis/redis/v9"
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

// parkedKey marks a Verify call for inFlightHook; its value is a *sync.Once.
type parkedKey struct{}

// inFlightHook parks each marked Verify call right after its first
// successful Redis command until release is closed, so the calls are in
// flight together at a fixed point. A verify that reads the code and counts
// the failure afterwards is parked between the two.
type inFlightHook struct {
	parked  sync.WaitGroup
	release chan struct{}
}

func (h *inFlightHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *inFlightHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func (h *inFlightHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		err := next(ctx, cmd)
		if once, ok := ctx.Value(parkedKey{}).(*sync.Once); ok && err == nil {
			once.Do(func() {
				h.parked.Done()
				<-h.release
			})
		}
		return err
	}
}

// verifyInFlight starts one Verify per guess and returns once all of them are
// parked in flight. finish releases them and returns their errors in order.
func verifyInFlight(t *testing.T, codes *service.EmailCodeService, hook *inFlightHook, email string, guesses []string) (finish func() []error) {
	t.Helper()
	hook.release = make(chan struct{})
	hook.parked.Add(len(guesses))
	errs := make([]error, len(guesses))
	var done sync.WaitGroup
	for i, guess := range guesses {
		done.Add(1)
		go func() {
			defer done.Done()
			ctx := context.WithValue(context.Background(), parkedKey{}, &sync.Once{})
			errs[i] = codes.Verify(ctx, service.EmailPurposePasswordReset, email, guess)
		}()
	}
	allParked := make(chan struct{})
	go func() {
		hook.parked.Wait()
		close(allParked)
	}()
	select {
	case <-allParked:
	case <-time.After(5 * time.Second):
		t.Fatal("verify calls did not reach Redis")
	}
	return func() []error {
		close(hook.release)
		done.Wait()
		return errs
	}
}

// otherCodes returns n six-digit codes that all differ from code.
func otherCodes(code string, n int) []string {
	base, _ := strconv.Atoi(code)
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%06d", (base+i+1)%1000000)
	}
	return out
}

// Guesses sent in parallel must not all be checked against the code before
// their failures are counted: with five wrong guesses in flight, a sixth
// guess finds the code already invalidated, even when it is right.
func TestEmailCodeParallelGuessesCannotOutrunFailureLimit(t *testing.T) {
	_, rdb := newMiniredis(t)
	hook := &inFlightHook{}
	rdb.AddHook(hook)
	mailer := &captureMailer{}
	codes := service.NewEmailCodeService(rdb, mailer, 10*time.Minute, time.Minute)
	_, err := codes.Send(context.Background(), service.EmailPurposePasswordReset, "user@example.com")
	require.NoError(t, err)

	finish := verifyInFlight(t, codes, hook, "user@example.com", otherCodes(mailer.code, 5))
	err = codes.Verify(context.Background(), service.EmailPurposePasswordReset, "user@example.com", mailer.code)
	for _, guessErr := range finish() {
		require.ErrorIs(t, guessErr, service.ErrInvalidEmailCode)
	}
	require.ErrorIs(t, err, service.ErrInvalidEmailCode)
}

// A code is single-use even when the right code arrives many times at once.
func TestEmailCodeParallelCorrectGuessesSucceedOnce(t *testing.T) {
	_, rdb := newMiniredis(t)
	hook := &inFlightHook{}
	rdb.AddHook(hook)
	mailer := &captureMailer{}
	codes := service.NewEmailCodeService(rdb, mailer, 10*time.Minute, time.Minute)
	_, err := codes.Send(context.Background(), service.EmailPurposePasswordReset, "user@example.com")
	require.NoError(t, err)

	guesses := make([]string, 10)
	for i := range guesses {
		guesses[i] = mailer.code
	}
	succeeded := 0
	for _, err := range verifyInFlight(t, codes, hook, "user@example.com", guesses)() {
		if err == nil {
			succeeded++
		} else {
			require.ErrorIs(t, err, service.ErrInvalidEmailCode)
		}
	}
	require.Equal(t, 1, succeeded)
}

// Resending must not hand out unlimited fresh sets of guesses: each address
// gets a few codes per purpose per hour.
func TestEmailCodeSendsAreCappedPerHour(t *testing.T) {
	mr, rdb := newMiniredis(t)
	codes := service.NewEmailCodeService(rdb, &captureMailer{}, 10*time.Minute, time.Minute)
	ctx := context.Background()
	send := func(purpose, email string) error {
		_, err := codes.Send(ctx, purpose, email)
		return err
	}

	for i := range service.DefaultEmailCodesPerHour {
		require.NoError(t, send(service.EmailPurposePasswordReset, "user@example.com"), "code %d", i+1)
		mr.FastForward(time.Minute)
	}
	require.ErrorIs(t, send(service.EmailPurposePasswordReset, "User@Example.com"), service.ErrEmailCodeLimit)
	// Other addresses and purposes have their own allowance.
	require.NoError(t, send(service.EmailPurposePasswordReset, "other@example.com"))
	require.NoError(t, send(service.EmailPurposeRegister, "user@example.com"))

	// The allowance returns an hour after the first code.
	mr.FastForward(time.Hour - service.DefaultEmailCodesPerHour*time.Minute - time.Second)
	require.ErrorIs(t, send(service.EmailPurposePasswordReset, "user@example.com"), service.ErrEmailCodeLimit)
	mr.FastForward(time.Second)
	require.NoError(t, send(service.EmailPurposePasswordReset, "user@example.com"))

	codes.SetMaxCodesPerHour(1)
	require.NoError(t, send(service.EmailPurposeEmailChange, "user@example.com"))
	mr.FastForward(time.Minute)
	require.ErrorIs(t, send(service.EmailPurposeEmailChange, "user@example.com"), service.ErrEmailCodeLimit)
}

type failingMailer struct{}

func (failingMailer) SendVerificationCode(context.Context, string, string, time.Duration) error {
	return errors.New("smtp down")
}

// A code that could not be emailed is withdrawn and does not use up the
// address's allowance.
func TestEmailCodeFailedSendDoesNotCount(t *testing.T) {
	_, rdb := newMiniredis(t)
	failing := service.NewEmailCodeService(rdb, failingMailer{}, 10*time.Minute, time.Minute)
	for range service.DefaultEmailCodesPerHour + 1 {
		_, err := failing.Send(context.Background(), service.EmailPurposeRegister, "user@example.com")
		require.ErrorIs(t, err, service.ErrEmailSendFailed)
	}

	mailer := &captureMailer{}
	codes := service.NewEmailCodeService(rdb, mailer, 10*time.Minute, time.Minute)
	_, err := codes.Send(context.Background(), service.EmailPurposeRegister, "user@example.com")
	require.NoError(t, err)
	require.NoError(t, codes.Verify(context.Background(), service.EmailPurposeRegister, "user@example.com", mailer.code))
}
