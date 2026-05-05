package service

import (
	"context"
	"fmt"
	"strconv"

	"google.golang.org/api/idtoken"
)

type defaultGoogleVerifier struct{}

func (defaultGoogleVerifier) VerifyGoogleCredential(ctx context.Context, credential, audience string) (*GoogleProfile, error) {
	payload, err := idtoken.Validate(ctx, credential, audience)
	if err != nil {
		return nil, err
	}

	email, _ := payload.Claims["email"].(string)
	name, _ := payload.Claims["name"].(string)
	picture, _ := payload.Claims["picture"].(string)
	emailVerified := googleBoolClaim(payload.Claims["email_verified"])
	if email == "" {
		return nil, fmt.Errorf("google id token missing email")
	}

	return &GoogleProfile{
		Subject:       payload.Subject,
		Email:         email,
		EmailVerified: emailVerified,
		Name:          name,
		Picture:       picture,
	}, nil
}

func googleBoolClaim(v any) bool {
	switch value := v.(type) {
	case bool:
		return value
	case string:
		parsed, _ := strconv.ParseBool(value)
		return parsed
	default:
		return false
	}
}
