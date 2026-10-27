package oidc

import (
	"context"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/upfluence/errors"
)

type VerifierConfig struct {
	IssuerURL string
	ClientID  string

	ClaimsAssertion ClaimsAssertion
}

// DefaultVerifierConfig targets GitHub Actions. ClientID is left empty on
// purpose: each service must set its own audience, otherwise tokens minted
// for one service would be accepted by every other one.
var DefaultVerifierConfig = VerifierConfig{
	IssuerURL: "https://token.actions.githubusercontent.com",
}

type IDToken struct {
	*oidc.IDToken

	Claims Claims
}

type Verifier struct {
	verifier        *oidc.IDTokenVerifier
	claimsAssertion ClaimsAssertion
}

func NewVerifier(ctx context.Context, cfg VerifierConfig) (*Verifier, error) {
	p, err := oidc.NewProvider(ctx, cfg.IssuerURL)

	if err != nil {
		return nil, errors.Wrap(err, "discover oidc provider")
	}

	v := p.VerifierContext(ctx, &oidc.Config{ClientID: cfg.ClientID})

	return &Verifier{verifier: v, claimsAssertion: cfg.ClaimsAssertion}, nil
}

func (v *Verifier) Verify(ctx context.Context, raw string) (*IDToken, error) {
	tok, err := v.verifier.Verify(ctx, raw)

	if err != nil {
		return nil, errors.Wrap(err, "verify id token")
	}

	var c Claims

	if err := tok.Claims(&c); err != nil {
		return nil, errors.Wrap(err, "decode claims")
	}

	if v.claimsAssertion != nil {
		if err := v.claimsAssertion(c); err != nil {
			return nil, errors.Wrap(err, "assert claims")
		}
	}

	return &IDToken{IDToken: tok, Claims: c}, nil
}
