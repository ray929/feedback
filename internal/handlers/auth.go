package handlers

import (
	"bufio"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	feedbackRedis "feedback/internal/redis"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

type LoginRequest struct {
	Password       string `json:"password"`
	TurnstileToken string `json:"turnstile_token"`
}

var htpasswdPath = ".htpasswd"

// SetHtpasswdPath 设置 .htpasswd 文件路径，应在初始化时调用
func SetHtpasswdPath(path string) {
	htpasswdPath = path
}

// 检查 .htpasswd 文件中的 bcrypt 密码
func checkPassword(password string) bool {
	file, err := os.Open(htpasswdPath)
	if err != nil {
		log.Printf("DEBUG checkPassword: failed to open %s: %v", htpasswdPath, err)
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("DEBUG checkPassword: line=%s", line)
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && parts[0] == "admin" {
			found = true
			hash := parts[1]
			log.Printf("DEBUG checkPassword: found admin, hash prefix=%s, hash len=%d", hash[:4], len(hash))
			if strings.HasPrefix(hash, "$2") {
				err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
				if err != nil {
					log.Printf("DEBUG checkPassword: bcrypt mismatch: %v", err)
				}
				return err == nil
			} else {
				log.Printf("DEBUG checkPassword: hash does not start with $2")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("DEBUG checkPassword: scanner error: %v", err)
		return false
	}
	if !found {
		log.Printf("DEBUG checkPassword: no admin entry found in %s", htpasswdPath)
	}
	return false
}

func Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid request payload"})
	}

	// 1. Verify Turnstile
	secret := os.Getenv("TURNSTILE_SECRET_KEY")
	if secret != "" {
		valid, err := verifyTurnstile(req.TurnstileToken, secret, c.RealIP())
		if err != nil || !valid {
			return c.JSON(http.StatusBadRequest, map[string]string{"message": "Captcha verification failed"})
		}
	}

	// 2. Verify Password
	if !checkPassword(req.Password) {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Invalid password"})
	}

	// 3. Set HTTP-Only Cookie
	sessionToken := uuid.New().String()
	feedbackRedis.SaveSession(sessionToken, 24*time.Hour)

	cookie := new(http.Cookie)
	cookie.Name = "admin_session"
	cookie.Value = sessionToken
	cookie.Expires = time.Now().Add(24 * time.Hour)
	cookie.HttpOnly = true
	cookie.Path = "/"
	cookie.SameSite = http.SameSiteLaxMode

	// If you are running HTTPS, set Secure = true
	// cookie.Secure = true

	c.SetCookie(cookie)

	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

func Logout(c echo.Context) error {
	// Clear the session cookie
	cookie, err := c.Cookie("admin_session")
	if err == nil && cookie.Value != "" {
		feedbackRedis.DeleteSession(cookie.Value)
	}

	c.SetCookie(&http.Cookie{
		Name:     "admin_session",
		Value:    "",
		Expires:  time.Now().Add(-24 * time.Hour),
		HttpOnly: true,
		Path:     "/",
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// AuthMiddleware protects routes
func AuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		cookie, err := c.Cookie("admin_session")
		if err != nil || cookie.Value == "" {
			return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
		}
		if !feedbackRedis.ValidateSession(cookie.Value) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Session expired"})
		}
		return next(c)
	}
}
