package app

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	authCaptchaTTL         = 5 * time.Minute
	authCaptchaMaxAttempts = 5
	authCaptchaCodeLength  = 5
)

const authCaptchaAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // gitleaks:allow -- public captcha alphabet, not a credential

var glyphs5x7 = map[rune][7]string{
	'A': {"01110", "10001", "10001", "11111", "10001", "10001", "10001"},
	'B': {"11110", "10001", "10001", "11110", "10001", "10001", "11110"},
	'C': {"01111", "10000", "10000", "10000", "10000", "10000", "01111"},
	'D': {"11110", "10001", "10001", "10001", "10001", "10001", "11110"},
	'E': {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'F': {"11111", "10000", "10000", "11110", "10000", "10000", "10000"},
	'G': {"01111", "10000", "10000", "10111", "10001", "10001", "01111"},
	'H': {"10001", "10001", "10001", "11111", "10001", "10001", "10001"},
	'J': {"00111", "00010", "00010", "00010", "10010", "10010", "01100"},
	'K': {"10001", "10010", "10100", "11000", "10100", "10010", "10001"},
	'L': {"10000", "10000", "10000", "10000", "10000", "10000", "11111"},
	'M': {"10001", "11011", "10101", "10101", "10001", "10001", "10001"},
	'N': {"10001", "11001", "10101", "10011", "10001", "10001", "10001"},
	'P': {"11110", "10001", "10001", "11110", "10000", "10000", "10000"},
	'Q': {"01110", "10001", "10001", "10001", "10101", "10010", "01101"},
	'R': {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
	'S': {"01111", "10000", "10000", "01110", "00001", "00001", "11110"},
	'T': {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
	'U': {"10001", "10001", "10001", "10001", "10001", "10001", "01110"},
	'V': {"10001", "10001", "10001", "10001", "10001", "01010", "00100"},
	'W': {"10001", "10001", "10001", "10101", "10101", "10101", "01010"},
	'X': {"10001", "10001", "01010", "00100", "01010", "10001", "10001"},
	'Y': {"10001", "10001", "01010", "00100", "00100", "00100", "00100"},
	'Z': {"11111", "00001", "00010", "00100", "01000", "10000", "11111"},
	'2': {"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	'3': {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4': {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5': {"11111", "10000", "10000", "11110", "00001", "00001", "11110"},
	'6': {"01110", "10000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01111", "00001", "00001", "01110"},
}

func (a *App) handleGetAuthCaptcha(c *gin.Context) {
	purpose := strings.TrimSpace(c.Query("purpose"))
	if purpose != "user_login" && purpose != "admin_login" {
		writeError(c, http.StatusBadRequest, "invalid_purpose", "验证码用途不正确")
		return
	}

	code, err := randomCaptchaCode()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "captcha_generate_failed", "验证码生成失败")
		return
	}
	imageBase64, err := renderCaptchaPNGBase64(code)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "captcha_generate_failed", "验证码生成失败")
		return
	}

	captchaID := uuid.NewString()
	challenge := AuthCaptchaChallenge{
		CaptchaID: captchaID,
		Purpose:   purpose,
		CodeHash:  hashAuthCaptchaCode(captchaID, purpose, code),
		ExpiresAt: time.Now().Add(authCaptchaTTL),
		IPAddress: sourceIPAddress(c.Request),
	}
	if err := a.db.Create(&challenge).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "captcha_store_failed", "验证码保存失败")
		return
	}
	writeJSON(c, http.StatusOK, gin.H{
		"captcha_id":   captchaID,
		"image_base64": imageBase64,
		"expires_in":   int(authCaptchaTTL / time.Second),
	})
}

func (a *App) validateAuthCaptcha(c *gin.Context, purpose, captchaID, captchaCode string) bool {
	captchaID = strings.TrimSpace(captchaID)
	captchaCode = strings.ToUpper(strings.TrimSpace(captchaCode))
	if captchaID == "" || captchaCode == "" {
		writeError(c, http.StatusBadRequest, "captcha_required", "请完成图形验证码")
		return false
	}

	var challenge AuthCaptchaChallenge
	if err := a.db.Where("captcha_id = ? AND purpose = ?", captchaID, purpose).First(&challenge).Error; err != nil {
		status := http.StatusInternalServerError
		code := "captcha_check_failed"
		message := "验证码校验失败"
		if err == gorm.ErrRecordNotFound {
			status = http.StatusUnauthorized
			code = "captcha_invalid"
			message = "验证码错误或已过期"
		}
		writeError(c, status, code, message)
		return false
	}
	now := time.Now()
	if challenge.ConsumedAt != nil || !challenge.ExpiresAt.After(now) {
		writeError(c, http.StatusUnauthorized, "captcha_invalid", "验证码错误或已过期")
		return false
	}
	if challenge.AttemptCount >= authCaptchaMaxAttempts {
		writeError(c, http.StatusTooManyRequests, "captcha_attempts_exceeded", "验证码错误次数过多，请刷新后重试")
		return false
	}
	if challenge.CodeHash != hashAuthCaptchaCode(captchaID, purpose, captchaCode) {
		nextAttempts := challenge.AttemptCount + 1
		_ = a.db.Model(&challenge).UpdateColumn("attempt_count", nextAttempts).Error
		if nextAttempts >= authCaptchaMaxAttempts {
			writeError(c, http.StatusTooManyRequests, "captcha_attempts_exceeded", "验证码错误次数过多，请刷新后重试")
			return false
		}
		writeError(c, http.StatusUnauthorized, "captcha_invalid", "验证码错误或已过期")
		return false
	}

	updates := map[string]any{
		"consumed_at":   &now,
		"attempt_count": gorm.Expr("attempt_count + ?", 1),
	}
	if err := a.db.Model(&challenge).Updates(updates).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "captcha_check_failed", "验证码校验失败")
		return false
	}
	return true
}

func hashAuthCaptchaCode(captchaID, purpose, code string) string {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	sum := sha256.Sum256([]byte(captchaID + "|" + purpose + "|" + normalized))
	return hex.EncodeToString(sum[:])
}

func randomCaptchaCode() (string, error) {
	var builder strings.Builder
	builder.Grow(authCaptchaCodeLength)
	max := big.NewInt(int64(len(authCaptchaAlphabet)))
	for i := 0; i < authCaptchaCodeLength; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		builder.WriteByte(authCaptchaAlphabet[n.Int64()])
	}
	return builder.String(), nil
}

func renderCaptchaPNGBase64(code string) (string, error) {
	const width = 150
	const height = 54
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	bg := color.RGBA{R: 248, G: 251, B: 255, A: 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, bg)
		}
	}

	for i := 0; i < 180; i++ {
		x := secureRandInt(width)
		y := secureRandInt(height)
		img.Set(x, y, color.RGBA{R: uint8(90 + secureRandInt(120)), G: uint8(120 + secureRandInt(90)), B: uint8(150 + secureRandInt(80)), A: 130})
	}
	for i := 0; i < 5; i++ {
		drawLine(img, secureRandInt(width), secureRandInt(height), secureRandInt(width), secureRandInt(height), color.RGBA{R: uint8(60 + secureRandInt(90)), G: uint8(80 + secureRandInt(90)), B: uint8(130 + secureRandInt(90)), A: 130})
	}

	for index, char := range strings.ToUpper(code) {
		x := 12 + index*27 + secureRandInt(5) - 2
		y := 10 + secureRandInt(9) - 4
		scale := 4
		ink := color.RGBA{R: uint8(25 + secureRandInt(50)), G: uint8(45 + secureRandInt(50)), B: uint8(95 + secureRandInt(80)), A: 255}
		drawGlyph(img, char, x, y, scale, ink)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func drawGlyph(img *image.RGBA, char rune, x, y, scale int, ink color.RGBA) {
	rows, ok := glyphs5x7[char]
	if !ok {
		return
	}
	for rowIndex, row := range rows {
		for colIndex, pixel := range row {
			if pixel != '1' {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.Set(x+colIndex*scale+dx, y+rowIndex*scale+dy, ink)
				}
			}
		}
	}
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, ink color.RGBA) {
	dx := absInt(x1 - x0)
	dy := -absInt(y1 - y0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	errValue := dx + dy
	for {
		img.Set(x0, y0, ink)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * errValue
		if e2 >= dy {
			errValue += dy
			x0 += sx
		}
		if e2 <= dx {
			errValue += dx
			y0 += sy
		}
	}
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func secureRandInt(max int) int {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}
