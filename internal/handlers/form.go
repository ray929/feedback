package handlers

import (
	"feedback/internal/db"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
)

func RenderForm(c echo.Context) error {
	id := c.Param("id")
	theme := c.QueryParam("theme")
	if theme == "" {
		theme = "light" // default to light as per spec
	}
	lang := c.QueryParam("lang")
	if lang == "" {
		lang = "en" // default to English
	}
	source := c.QueryParam("source")

	siteKey := os.Getenv("TURNSTILE_SITE_KEY")

	data := map[string]interface{}{
		"FormID":    id,
		"Theme":     theme,
		"Lang":      lang,
		"SourceURL": source,
		"SiteKey":   siteKey,
	}

	return c.Render(http.StatusOK, "form.html", data)
}

func RenderAdmin(c echo.Context) error {
	siteKey := os.Getenv("TURNSTILE_SITE_KEY")
	lang := c.QueryParam("lang")
	if lang == "" {
		lang = "en"
	}
	data := map[string]interface{}{
		"SiteKey": siteKey,
		"Lang":    lang,
	}
	return c.Render(http.StatusOK, "admin.html", data)
}

func RenderQuery(c echo.Context) error {
	id := c.Param("id")
	siteKey := os.Getenv("TURNSTILE_SITE_KEY")
	lang := c.QueryParam("lang")
	if lang == "" {
		lang = "en"
	}

	// Query form name
	var formName string
	if err := db.DB.QueryRow("SELECT name FROM forms WHERE id = ?", id).Scan(&formName); err != nil {
		formName = id // fallback
	}

	data := map[string]interface{}{
		"FormID":   id,
		"FormName": formName,
		"SiteKey":  siteKey,
		"Lang":     lang,
	}
	return c.Render(http.StatusOK, "query.html", data)
}
