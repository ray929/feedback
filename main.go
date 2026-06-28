package main

import (
	"embed"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"

	"feedback/internal/db"
	"feedback/internal/handlers"
	feedbackRedis "feedback/internal/redis"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

//go:embed public/*
var publicFS embed.FS

// TemplateRegistry is a custom html/template renderer for Echo framework
type TemplateRegistry struct {
	templates *template.Template
}

// Render renders a template document
func (t *TemplateRegistry) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func main() {
	// 加载环境变量
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found or error loading it, using system env vars")
	}

	// 初始化 SQLite 数据库
	db.InitDB("./data/feedback.db")

	// 初始化 Redis 连接
	feedbackRedis.InitRedis()

	// 初始化 Echo 实例
	e := echo.New()

	// 基础中间件
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// 安全响应头（允许被任何网站 iframe 嵌入）
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set("Content-Security-Policy", "frame-ancestors *")
			return next(c)
		}
	})

	// 解析嵌入的模板
	templates, err := template.ParseFS(publicFS, "public/templates/*.html")
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}
	e.Renderer = &TemplateRegistry{
		templates: templates,
	}

	// 静态文件服务 (利用 go:embed)
	staticFS, err := fs.Sub(publicFS, "public")
	if err != nil {
		log.Fatalf("Failed to create sub filesystem: %v", err)
	}

	// === 路由组 ===

	// 首页空白页面 (必须放在 /* 之前)
	e.GET("/", func(c echo.Context) error {
		return c.HTML(http.StatusOK, "<!DOCTYPE html><html><head><meta charset='UTF-8'><title></title></head><body></body></html>")
	})

	// 根路径静态文件，比如 /embed.js 和 /css/*
	assetHandler := http.FileServer(http.FS(staticFS))
	e.GET("/*", echo.WrapHandler(http.StripPrefix("/", assetHandler)))

	// 1. 后台管理页面
	e.GET("/p", handlers.RenderAdmin)

	// 2. 表单渲染页面 (供 iframe 嵌套)
	e.GET("/f/:id", handlers.RenderForm)

	// 2.5. 数据查询页面
	e.GET("/q/:id", handlers.RenderQuery)

	// 3. API 路由
	api := e.Group("/api")

	// Auth
	api.POST("/login", handlers.Login)
	api.POST("/logout", handlers.Logout)

	// 提交表单接口
	api.POST("/submit", handlers.SubmitForm)

	// 数据查询接口 (独立验证，不需要 admin cookie)
	api.POST("/query/:id", handlers.QueryFormSubmissions)
	api.POST("/query/:id/logout", handlers.QueryLogout)

	// 后台管理 API (受 AuthMiddleware 保护)
	adminAPI := api.Group("/admin", handlers.AuthMiddleware)
	adminAPI.GET("/forms", handlers.GetForms)
	adminAPI.POST("/forms", handlers.CreateForm)
	adminAPI.PUT("/forms/:id", handlers.UpdateForm)
	adminAPI.DELETE("/forms/:id", handlers.DeleteForm)
	adminAPI.GET("/forms/:id/submissions", handlers.GetSubmissions)

	// 启动服务器
	port := os.Getenv("PORT")
	if port == "" {
		port = "8010" // 默认 8010
	}
	log.Printf("Starting server on port %s", port)
	e.Logger.Fatal(e.Start(":" + port))
}
