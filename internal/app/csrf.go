package app

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	csrfCookieName  = "csrf_token"
	csrfHeaderName  = "X-CSRF-Token"
	csrfCookieHours = 12
	csrfTokenBytes  = 32
)

func (a *App) handleGetCSRFToken(c *gin.Context) {
	token, err := generateCSRFToken()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "csrf_generate_failed", "CSRF Token 生成失败")
		return
	}
	http.SetCookie(c.Writer, buildCSRFCookie(token, csrfCookieHours))
	writeJSON(c, http.StatusOK, gin.H{"csrf_token": token})
}

func generateCSRFToken() (string, error) {
	raw := make([]byte, csrfTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func buildCSRFCookie(value string, hours int) *http.Cookie {
	return &http.Cookie{
		Name:     csrfCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   hours * int(time.Hour/time.Second),
	}
}

func validateCSRFToken(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "csrf_required", false
	}
	header := strings.TrimSpace(r.Header.Get(csrfHeaderName))
	if header == "" {
		return "csrf_required", false
	}
	if subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) != 1 {
		return "csrf_invalid", false
	}
	return "", true
}
