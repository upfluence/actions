package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/upfluence/errors"
)

const (
	// GitHub rejects app JWTs living more than 10 minutes, the issued at is
	// backdated to absorb clock drifts.
	jwtBackdate = time.Minute
	jwtLifetime = 9 * time.Minute

	jwtHeader = `{"alg":"RS256","typ":"JWT"}`
)

type kmsClient interface {
	Sign(context.Context, *kms.SignInput, ...func(*kms.Options)) (*kms.SignOutput, error)
}

type jwtClaims struct {
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	Issuer    string `json:"iss"`
}

// signAppJWT builds the JWT authenticating as the GitHub App, the private key
// never leaves KMS which only signs the digest of the token.
func signAppJWT(ctx context.Context, cl kmsClient, keyID, appID string, now time.Time) (string, error) {
	claims, err := json.Marshal(
		jwtClaims{
			IssuedAt:  now.Add(-jwtBackdate).Unix(),
			ExpiresAt: now.Add(jwtLifetime).Unix(),
			Issuer:    appID,
		},
	)

	if err != nil {
		return "", errors.Wrap(err, "encode jwt claims")
	}

	var (
		enc     = base64.RawURLEncoding
		payload = enc.EncodeToString([]byte(jwtHeader)) + "." + enc.EncodeToString(claims)
		digest  = sha256.Sum256([]byte(payload))
	)

	out, err := cl.Sign(
		ctx,
		&kms.SignInput{
			KeyId:            aws.String(keyID),
			Message:          digest[:],
			MessageType:      types.MessageTypeDigest,
			SigningAlgorithm: types.SigningAlgorithmSpecRsassaPkcs1V15Sha256,
		},
	)

	if err != nil {
		return "", errors.Wrap(err, "sign app jwt")
	}

	return payload + "." + enc.EncodeToString(out.Signature), nil
}
