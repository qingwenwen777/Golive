package auth

import (
	"errors"

	"github.com/qingwenwen777/golive/pkg/jwtauth"
)

// ErrInvalidToken is returned when a token is present but does not validate.
var ErrInvalidToken = errors.New("invalid token")

// Identity is what the WS handler attaches to each connection.
type Identity struct {
	UserID    string
	Anonymous bool
}

func (i Identity) CanChat() bool { return !i.Anonymous && i.UserID != "" }

// Verifier is intentionally tiny so tests can mock it.
type Verifier interface {
	Verify(token string) (Identity, error)
}

type HMACVerifier struct {
	keys *jwtauth.KeySet
}

func NewHMACVerifier(secret string) *HMACVerifier {
	keys, err := jwtauth.NewKeySet(secret, "", nil)
	if err != nil {
		panic("im-gateway jwt key set: " + err.Error())
	}
	return &HMACVerifier{keys: keys}
}

func NewHMACVerifierWithKeySet(keys *jwtauth.KeySet) *HMACVerifier {
	return &HMACVerifier{keys: keys}
}

// Verify implements the contract above. An empty token returns an anonymous
// identity (NOT an error). Any other failure returns ErrInvalidToken.
func (v *HMACVerifier) Verify(token string) (Identity, error) {
	if token == "" {
		return Identity{Anonymous: true}, nil
	}
	uid, err := v.keys.VerifyAccess(token)
	if err != nil {
		return Identity{}, ErrInvalidToken
	}
	return Identity{UserID: uid}, nil
}
