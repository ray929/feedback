package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"feedback/internal/db"
	"feedback/internal/models"
	feedbackRedis "feedback/internal/redis"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

type SubmitRequest struct {
	FormID         string `json:"form_id"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	Phone          string `json:"phone"`
	Content        string `json:"content"`
	SourceURL      string `json:"source_url"`
	TurnstileToken string `json:"turnstile_token"`
	Lang           string `json:"lang"`
}

func SubmitForm(c echo.Context) error {
	var req SubmitRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid request payload"})
	}

	// 1. Rate limiting (single IP 10 submissions per hour)
	if !feedbackRedis.CheckRateLimit(c.RealIP(), 10, time.Hour) {
		return c.JSON(http.StatusTooManyRequests, map[string]string{"message": "Rate limit exceeded. Please try again later."})
	}

	// 2. Verify Turnstile (only if secret is configured)
	secret := os.Getenv("TURNSTILE_SECRET_KEY")
	if secret != "" {
		valid, err := verifyTurnstile(req.TurnstileToken, secret, c.RealIP())
		if err != nil || !valid {
			return c.JSON(http.StatusBadRequest, map[string]string{"message": "Captcha verification failed"})
		}
	}

	// 3. Validate DB - Check if form exists, resolve UUID to internal ID
	var formIntID int
	var formName string
	var notifyEmail sqlNullString
	err := db.DB.QueryRow("SELECT id, name, notify_email FROM forms WHERE uuid = ?", req.FormID).Scan(&formIntID, &formName, &notifyEmail.String)
	if err != nil {
		// Auto-create form if it doesn't exist
		formName = "Auto-created Form"
		res, errInsert := db.DB.Exec("INSERT INTO forms (uuid, name) VALUES (?, ?)", req.FormID, formName)
		if errInsert != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to create missing form"})
		}
		newID, _ := res.LastInsertId()
		formIntID = int(newID)
	}

	// 4. Save to DB
	res, err := db.DB.Exec(`
        INSERT INTO submissions (form_id, name, email, phone, content, source_url, client_ip) 
        VALUES (?, ?, ?, ?, ?, ?, ?)`,
		formIntID, req.Name, req.Email, req.Phone, req.Content, req.SourceURL, c.RealIP(),
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Database error"})
	}
	submissionId, _ := res.LastInsertId()

	// 4. Send Email via Resend (Async)
	emailToSendTo := notifyEmail.String
	if emailToSendTo == "" {
		emailToSendTo = os.Getenv("DEFAULT_RECIPIENT")
	}

	if emailToSendTo != "" {
		go sendEmailNotification(submissionId, formName, emailToSendTo, req, c.RealIP())
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "success", "message": "Feedback received"})
}

// helper for nullable strings
type sqlNullString struct {
	String string
	Valid  bool
}

func (s *sqlNullString) Scan(value interface{}) error {
	if value == nil {
		s.String, s.Valid = "", false
		return nil
	}
	s.Valid = true
	// convert []byte or string
	switch v := value.(type) {
	case []byte:
		s.String = string(v)
	case string:
		s.String = v
	}
	return nil
}

type QueryRequest struct {
	Password       string `json:"password"`
	TurnstileToken string `json:"turnstile_token"`
}

func QueryFormSubmissions(c echo.Context) error {
	formUUID := c.Param("id")
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit < 1 {
		limit = 20
	}

	// 0. Check if already authenticated via cookie
	cookie, err := c.Cookie("query_session_" + formUUID)
	if err == nil && cookie.Value != "" && feedbackRedis.ValidateQuerySession(formUUID, cookie.Value) {
		resp := fetchSubmissionsByUUID(formUUID, page, limit)
		return c.JSON(http.StatusOK, resp)
	}

	var req QueryRequest
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

	// 2. Fetch form and check query password
	var hash sqlNullString
	sqlErr := db.DB.QueryRow("SELECT query_password FROM forms WHERE uuid = ?", formUUID).Scan(&hash.String)
	if sqlErr != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"message": "Form not found"})
	}

	if hash.String == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "This form does not have a query password set"})
	}

	sqlErr = bcrypt.CompareHashAndPassword([]byte(hash.String), []byte(req.Password))
	if sqlErr != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Invalid password"})
	}

	// 3. Set cookie scoped to this specific form's query path
	sessionToken := uuid.New().String()
	feedbackRedis.SaveQuerySession(formUUID, sessionToken, 24*time.Hour)
	c.SetCookie(&http.Cookie{
		Name:     "query_session_" + formUUID,
		Value:    sessionToken,
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		Path:     "/api/query/" + formUUID,
		SameSite: http.SameSiteLaxMode,
	})

	// 4. Return submissions
	resp := fetchSubmissionsByUUID(formUUID, page, limit)
	return c.JSON(http.StatusOK, resp)
}

func fetchSubmissionsByUUID(formUUID string, page, limit int) models.PaginatedResponse {
	offset := (page - 1) * limit

	var formIntID int
	var total int
	db.DB.QueryRow("SELECT id FROM forms WHERE uuid = ?", formUUID).Scan(&formIntID)
	db.DB.QueryRow("SELECT COUNT(*) FROM submissions WHERE form_id = ?", formIntID).Scan(&total)

	rows, err := db.DB.Query("SELECT id, name, email, phone, content, source_url, client_ip, created_at FROM submissions WHERE form_id = ? ORDER BY id DESC LIMIT ? OFFSET ?", formIntID, limit, offset)
	if err != nil {
		return models.PaginatedResponse{Items: []models.Submission{}, Total: 0, Page: page, Limit: limit}
	}
	defer rows.Close()

	var submissions []models.Submission
	for rows.Next() {
		var s models.Submission
		var email, phone, sourceUrl, clientIp sqlNullString
		if err := rows.Scan(&s.ID, &s.Name, &email.String, &phone.String, &s.Content, &sourceUrl.String, &clientIp.String, &s.CreatedAt); err != nil {
			continue
		}
		s.Email = email.String
		s.Phone = phone.String
		s.SourceURL = sourceUrl.String
		s.ClientIP = clientIp.String
		submissions = append(submissions, s)
	}

	if submissions == nil {
		submissions = []models.Submission{}
	}

	return models.PaginatedResponse{
		Items: submissions,
		Total: total,
		Page:  page,
		Limit: limit,
	}
}

func QueryLogout(c echo.Context) error {
	id := c.Param("id")

	// Clear the query session cookie
	c.SetCookie(&http.Cookie{
		Name:     "query_session_" + id,
		Value:    "",
		Expires:  time.Now().Add(-24 * time.Hour),
		HttpOnly: true,
		Path:     "/api/query/" + id,
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

func verifyTurnstile(token, secret, ip string) (bool, error) {
	formData := url.Values{}
	formData.Set("secret", secret)
	formData.Set("response", token)
	formData.Set("remoteip", ip)

	client := &http.Client{}

	// Check if proxy is set
	proxyUrlStr := os.Getenv("HTTP_PROXY")
	if proxyUrlStr != "" {
		proxyUrl, err := url.Parse(proxyUrlStr)
		if err == nil {
			client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
		}
	}

	req, _ := http.NewRequest("POST", "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	return result.Success, nil
}
