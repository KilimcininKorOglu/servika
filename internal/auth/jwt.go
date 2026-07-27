package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID       int64  `json:"uid"`
	Username     string `json:"usr"`
	Role         string `json:"role"`
	TokenVersion int64  `json:"tv"`
	jwt.RegisteredClaims
}

func Issue(secret []byte, lifetimeSec int, uid int64, username, role string, tokenVersion int64) (string, error) {
	now := time.Now()
	c := Claims{
		UserID:       uid,
		Username:     username,
		Role:         role,
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(lifetimeSec) * time.Second)),
			Issuer:    "servika",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return tok.SignedString(secret)
}

func Parse(secret []byte, raw string) (*Claims, error) {
	if raw == "" {
		return nil, errors.New("empty token")
	}
	c := &Claims{}
	tok, err := jwt.ParseWithClaims(raw, c, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing algorithm")
		}
		return secret, nil
	})
	if err != nil || !tok.Valid {
		return nil, errors.New("invalid token")
	}
	if c.Issuer != "servika" || c.Role == "" {
		return nil, errors.New("not an administrator token")
	}
	return c, nil
}
