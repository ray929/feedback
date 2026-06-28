package handlers

import (
	"bufio"
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

// 检查 .htpasswd 文件中的 bcrypt 密码
func checkPassword(password string) bool {
	file, err := os.Open(".htpasswd")
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && parts[0] == "admin" {
			hash := parts[1]
			// Check if it's bcrypt
			if strings.HasPrefix(hash, "$2") {
				err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
				return err == nil
			}
		}
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
