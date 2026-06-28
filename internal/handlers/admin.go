package handlers

import (
	"net/http"

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
	rows, err := db.DB.Query("SELECT id, name, notify_email, created_at FROM forms ORDER BY created_at DESC")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Database error"})
	}
	defer rows.Close()

	var forms []models.Form
	for rows.Next() {
		var f models.Form
		if err := rows.Scan(&f.ID, &f.Name, &f.NotifyEmail, &f.CreatedAt); err != nil {
			continue
		}
		forms = append(forms, f)
	}

	// Ensure we return an empty array instead of null if no forms exist
	if forms == nil {
		forms = []models.Form{}
	}

	return c.JSON(http.StatusOK, forms)
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
	rows, err := db.DB.Query("SELECT id, name, email, phone, content, source_url, client_ip, created_at FROM submissions WHERE form_id = ? ORDER BY created_at DESC", id)
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

	return c.JSON(http.StatusOK, submissions)
}
