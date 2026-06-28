package handlers

import (
	"net/http"
	"strconv"

	"feedback/internal/db"
	"feedback/internal/models"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

type FormPayload struct {
	Name          string `json:"name"`
	QueryPassword string `json:"query_password"`
	NotifyEmail   string `json:"notify_email"`
}

func GetForms(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	// Get total count
	var total int
	db.DB.QueryRow("SELECT COUNT(*) FROM forms").Scan(&total)

	rows, err := db.DB.Query(`
		SELECT f.id, f.name, f.notify_email, f.created_at, 
		       COALESCE((SELECT COUNT(*) FROM submissions s WHERE s.form_id = f.id), 0) as submission_count
		FROM forms f ORDER BY f.created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Database error"})
	}
	defer rows.Close()

	var forms []models.Form
	for rows.Next() {
		var f models.Form
		if err := rows.Scan(&f.ID, &f.Name, &f.NotifyEmail, &f.CreatedAt, &f.SubmissionCount); err != nil {
			continue
		}
		forms = append(forms, f)
	}

	if forms == nil {
		forms = []models.Form{}
	}

	return c.JSON(http.StatusOK, models.PaginatedResponse{
		Items: forms,
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

func CreateForm(c echo.Context) error {
	var payload FormPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid payload"})
	}

	if payload.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Name is required"})
	}

	id := uuid.New().String()
	var hash string
	if payload.QueryPassword != "" {
		hashedBytes, err := bcrypt.GenerateFromPassword([]byte(payload.QueryPassword), bcrypt.DefaultCost)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to hash password"})
		}
		hash = string(hashedBytes)
	}

	_, err := db.DB.Exec("INSERT INTO forms (id, name, query_password, notify_email) VALUES (?, ?, ?, ?)",
		id, payload.Name, hash, payload.NotifyEmail)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to create form"})
	}

	return c.JSON(http.StatusOK, map[string]string{"id": id, "message": "Form created successfully"})
}

func UpdateForm(c echo.Context) error {
	id := c.Param("id")
	var payload FormPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid payload"})
	}

	if payload.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Name is required"})
	}

	if payload.QueryPassword != "" {
		// Update with new password
		hashedBytes, _ := bcrypt.GenerateFromPassword([]byte(payload.QueryPassword), bcrypt.DefaultCost)
		_, err := db.DB.Exec("UPDATE forms SET name = ?, query_password = ?, notify_email = ? WHERE id = ?",
			payload.Name, string(hashedBytes), payload.NotifyEmail, id)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to update form"})
		}
	} else {
		// Update without touching password
		_, err := db.DB.Exec("UPDATE forms SET name = ?, notify_email = ? WHERE id = ?",
			payload.Name, payload.NotifyEmail, id)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to update form"})
		}
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Form updated successfully"})
}

func DeleteForm(c echo.Context) error {
	id := c.Param("id")

	// Delete associated submissions first to maintain referential integrity
	_, err := db.DB.Exec("DELETE FROM submissions WHERE form_id = ?", id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to delete submissions"})
	}

	_, err = db.DB.Exec("DELETE FROM forms WHERE id = ?", id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to delete form"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Form deleted successfully"})
}

func GetSubmissions(c echo.Context) error {
	id := c.Param("id")
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	var total int
	db.DB.QueryRow("SELECT COUNT(*) FROM submissions WHERE form_id = ?", id).Scan(&total)

	rows, err := db.DB.Query("SELECT id, name, email, phone, content, source_url, client_ip, created_at FROM submissions WHERE form_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?", id, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Database error"})
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

	return c.JSON(http.StatusOK, models.PaginatedResponse{
		Items: submissions,
		Total: total,
		Page:  page,
		Limit: limit,
	})
}
