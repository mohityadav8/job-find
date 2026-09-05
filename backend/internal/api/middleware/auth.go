package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Auth provides JWT issuance and verification for the Phase-2 company dashboard.
// Tokens carry the user id, role and (optional) company id so handlers can
// authorize writes to a company's offices/jobs without a DB round-trip.

type contextKey string

const (
	// CtxUserID / CtxRole / CtxCompanyID are the keys under which the auth
	// middleware stashes claims on the request context.
	CtxUserID    contextKey = "auth.user_id"
	CtxRole      contextKey = "auth.role"
	CtxCompanyID contextKey = "auth.company_id"
)

// Authenticator signs and validates tokens with a shared secret.
type Authenticator struct {
	secret []byte
	ttl    time.Duration
}

// NewAuthenticator builds an Authenticator. A empty secret is rejected by the
// caller (config), so we don't guard it here.
func NewAuthenticator(secret string, ttl time.Duration) *Authenticator {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Authenticator{secret: []byte(secret), ttl: ttl}
}

// Claims is the JWT payload.
type Claims struct {
	UserID    int64  `json:"uid"`
	Role      string `json:"role"`
	CompanyID *int64 `json:"cid,omitempty"`
	jwt.RegisteredClaims
}

// Issue mints a signed token for a user.
func (a *Authenticator) Issue(userID int64, role string, companyID *int64) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:    userID,
		Role:      role,
		CompanyID: companyID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(a.ttl)),
			Issuer:    "job-find",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(a.secret)
}

// parse validates a raw token string and returns its claims.
func (a *Authenticator) parse(raw string) (*Claims, error) {
	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return a.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !tok.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// Require is middleware that rejects requests without a valid Bearer token and,
// on success, injects the claims into the request context.
func (a *Authenticator) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bearerToken(r)
		if raw == "" {
			http.Error(w, `{"error":"missing authorization token"}`, http.StatusUnauthorized)
			return
		}
		claims, err := a.parse(raw)
		if err != nil {
			http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), CtxUserID, claims.UserID)
		ctx = context.WithValue(ctx, CtxRole, claims.Role)
		if claims.CompanyID != nil {
			ctx = context.WithValue(ctx, CtxCompanyID, *claims.CompanyID)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// bearerToken pulls the token out of an "Authorization: Bearer <t>" header.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if strings.HasPrefix(h, prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// UserIDFromContext returns the authenticated user id, if any.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	v, ok := ctx.Value(CtxUserID).(int64)
	return v, ok
}

// CompanyIDFromContext returns the authenticated user's company id, if linked.
func CompanyIDFromContext(ctx context.Context) (int64, bool) {
	v, ok := ctx.Value(CtxCompanyID).(int64)
	return v, ok
}
