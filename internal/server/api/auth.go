package api

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type LoginRequest struct {
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password" binding:"required"`
	CaptchaID string `json:"captcha_id" binding:"required"`
	Captcha   string `json:"captcha" binding:"required"`
}

type ChangePasswordRequest struct {
	OldPassword     string `json:"old_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=8"`
	ConfirmPassword string `json:"confirm_password" binding:"required"`
}

type captchaValue struct {
	code      string
	expiresAt time.Time
}

var captchaStore = struct {
	sync.Mutex
	values map[string]captchaValue
}{values: make(map[string]captchaValue)}

func (h *Handler) Captcha(c *gin.Context) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		errResp(c, http.StatusInternalServerError, "failed to generate captcha")
		return
	}
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code := make([]byte, 4)
	for i := range code {
		code[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		errResp(c, http.StatusInternalServerError, "failed to generate captcha")
		return
	}
	id := fmt.Sprintf("%x", idBytes)
	captchaStore.Lock()
	now := time.Now()
	for key, item := range captchaStore.values {
		if now.After(item.expiresAt) {
			delete(captchaStore.values, key)
		}
	}
	captchaStore.values[id] = captchaValue{code: string(code), expiresAt: now.Add(5 * time.Minute)}
	captchaStore.Unlock()

	// SVG is returned as an image data URI, keeping the endpoint dependency-free.
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="40" viewBox="0 0 120 40"><rect width="120" height="40" fill="#eef3ff"/><path d="M0 10L120 30M0 34L120 5" stroke="#b7c9f5"/><text x="60" y="28" text-anchor="middle" font-family="Arial" font-size="22" font-weight="700" letter-spacing="4" fill="#3159a6">%s</text></svg>`, code)
	c.JSON(http.StatusOK, gin.H{"captcha_id": id, "image": "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))})
}

func consumeCaptcha(id, code string) bool {
	captchaStore.Lock()
	defer captchaStore.Unlock()
	item, ok := captchaStore.values[id]
	delete(captchaStore.values, id)
	return ok && time.Now().Before(item.expiresAt) && strings.EqualFold(item.code, strings.TrimSpace(code))
}

func (h *Handler) Me(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"id":       c.GetString("user_id"),
		"username": c.GetString("username"),
		"role":     c.GetString("role"),
	})
}

func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	if !consumeCaptcha(req.CaptchaID, req.Captcha) {
		errResp(c, http.StatusUnauthorized, "验证码错误或已过期")
		return
	}

	user, err := h.users.FindByUsername(c.Request.Context(), req.Username)
	if err != nil || user == nil {
		errResp(c, http.StatusUnauthorized, "invalid username or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		errResp(c, http.StatusUnauthorized, "invalid username or password")
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  user.ID.Hex(),
		"username": user.Username,
		"role":     user.Role,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	})

	tokenString, err := token.SignedString([]byte(h.jwtSecret))
	if err != nil {
		errResp(c, http.StatusInternalServerError, "failed to generate token")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": tokenString,
		"user": gin.H{
			"id":       user.ID.Hex(),
			"username": user.Username,
			"role":     user.Role,
		},
	})
}

func (h *Handler) ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, "新密码至少需要 8 位")
		return
	}
	if req.OldPassword == req.NewPassword {
		errResp(c, http.StatusBadRequest, "新密码不能与旧密码相同")
		return
	}
	if req.NewPassword != req.ConfirmPassword {
		errResp(c, http.StatusBadRequest, "两次输入的新密码不一致")
		return
	}
	user, err := h.users.FindByUsername(c.Request.Context(), c.GetString("username"))
	if err != nil || user == nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)) != nil {
		errResp(c, http.StatusBadRequest, "旧密码错误")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		errResp(c, http.StatusInternalServerError, "修改密码失败")
		return
	}
	if err := h.users.UpdatePassword(c.Request.Context(), user.ID, string(hash)); err != nil {
		errResp(c, http.StatusInternalServerError, "修改密码失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
