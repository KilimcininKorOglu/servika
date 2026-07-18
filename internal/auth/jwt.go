package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   int64  `json:"uid"`
	Username string `json:"usr"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func Issue(secret []byte, lifetimeSec int, uid int64, username, role string) (string, error) {
	now := time.Now()
	c := Claims{
		UserID:   uid,
		Username: username,
		Role:     role,
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

// ===== Customer token (domain owner) =====

type CustomerClaims struct {
	FTPAccountID int64  `json:"fhid"`
	DomainID     int64  `json:"did"`
	Username     string `json:"usr"`
	DomainName   string `json:"domain"`
	Type         string `json:"type"` // "customer"
	jwt.RegisteredClaims
}

func GenerateCustomer(secret []byte, c CustomerClaims, lifetimeSec int64) (string, int64, error) {
	now := time.Now()
	c.Type = "customer"
	c.RegisteredClaims = jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(lifetimeSec) * time.Second)),
		Issuer:    "servika-customer",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	s, err := tok.SignedString(secret)
	return s, c.ExpiresAt.Unix(), err
}

func ParseCustomer(secret []byte, raw string) (*CustomerClaims, error) {
	c := &CustomerClaims{}
	tok, err := jwt.ParseWithClaims(raw, c, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, jwt.ErrSignatureInvalid
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !tok.Valid || c.Type != "customer" {
		return nil, jwt.ErrTokenMalformed
	}
	return c, nil
}
