package app

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type SessionClaims struct {
	Role        string `json:"role"`
	UserID      uint   `json:"user_id,omitempty"`
	AdminUserID uint   `json:"admin_user_id,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	InviteID    uint   `json:"invite_id,omitempty"`
	jwt.RegisteredClaims
}

type IssuedUserSession struct {
	Token     string
	ExpiresAt time.Time
}

func (a *App) issueUserSession(w http.ResponseWriter, r *http.Request, user User, rememberLogin bool) (*IssuedUserSession, error) {
	now := time.Now()
	sessionHours := a.userSessionHours(rememberLogin)
	session := UserSession{
		UserID:     user.ID,
		TokenID:    uuid.NewString(),
		IPAddress:  sourceIPAddress(r),
		UserAgent:  strings.TrimSpace(r.UserAgent()),
		ExpiresAt:  now.Add(time.Duration(sessionHours) * time.Hour),
		LastSeenAt: &now,
	}
	if err := a.db.Create(&session).Error; err != nil {
		return nil, err
	}

	claims := SessionClaims{
		Role:      "user",
		UserID:    user.ID,
		SessionID: session.TokenID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(session.ExpiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(a.cfg.JWTSecret))
	if err != nil {
		return nil, err
	}
	http.SetCookie(w, buildCookie(userSessionCookie, token, sessionHours))
	return &IssuedUserSession{Token: token, ExpiresAt: session.ExpiresAt}, nil
}

func (a *App) issueAdminCookie(w http.ResponseWriter, admin AdminUser, rememberLogin bool) error {
	now := time.Now()
	sessionHours := a.adminSessionHours(rememberLogin)
	session := AdminSession{
		AdminUserID: admin.ID,
		TokenID:     uuid.NewString(),
		ExpiresAt:   now.Add(time.Duration(sessionHours) * time.Hour),
	}
	if err := a.db.Create(&session).Error; err != nil {
		return err
	}

	claims := SessionClaims{
		Role:        "admin",
		AdminUserID: admin.ID,
		SessionID:   session.TokenID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(session.ExpiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(a.cfg.JWTSecret))
	if err != nil {
		return err
	}
	http.SetCookie(w, buildCookie(adminSessionCookie, token, sessionHours))
	return nil
}

func (a *App) userSessionHours(rememberLogin bool) int {
	if rememberLogin {
		return a.cfg.UserRememberSessionHours
	}
	return a.cfg.UserSessionHours
}

func (a *App) adminSessionHours(rememberLogin bool) int {
	if rememberLogin {
		return a.cfg.AdminRememberSessionHours
	}
	return a.cfg.AdminSessionHours
}

func sourceIPAddress(r *http.Request) string {
	forwardedFor := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		if first := strings.TrimSpace(parts[0]); first != "" {
			return first
		}
	}
	realIP := strings.TrimSpace(r.Header.Get("X-Real-IP"))
	if realIP != "" {
		return realIP
	}
	host := r.RemoteAddr
	if index := strings.LastIndex(host, ":"); index > 0 {
		return host[:index]
	}
	return host
}

func buildCookie(name, value string, hours int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   hours * int(time.Hour/time.Second),
	}
}

func clearCookie(name string) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}

func (a *App) parseSessionCookie(r *http.Request, cookieName string) (*SessionClaims, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return nil, err
	}
	return a.parseSessionToken(cookie.Value)
}

func (a *App) parseBearerSession(r *http.Request) (*SessionClaims, error) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		return nil, errors.New("authorization header missing")
	}
	parts := strings.Fields(authHeader)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return nil, errors.New("invalid bearer token")
	}
	return a.parseSessionToken(parts[1])
}

func (a *App) parseSessionToken(tokenValue string) (*SessionClaims, error) {
	token, err := jwt.ParseWithClaims(strings.TrimSpace(tokenValue), &SessionClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(a.cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*SessionClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
