package main

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

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

// setProcessTitle 在 Linux 下修改进程在 ps aux 中的显示名称。
// 写入 /proc/self/comm（影响 ps -e 短名称）并覆盖原始 argv[0] 内存（影响 ps aux CMD 列）。
func setProcessTitle(title string) {
	if runtime.GOOS != "linux" {
		return
	}

	// 1. 写入 /proc/self/comm — 影响 ps -e / top 短名称（限 15 字符）
	comm := title
	if len(comm) > 15 {
		comm = comm[:15]
	}
	os.WriteFile("/proc/self/comm", []byte(comm), 0644)

	// 2. 覆盖原始 argv[0] 内存 — 影响 ps aux CMD 列
	// os.Args[0] 在 Linux 上指向内核保留的原始 argv[0] 所在栈区域，可安全写入
	oldArg := os.Args[0]
	oldPtr := unsafe.StringData(oldArg)

	const maxLen = 2048
	n := len(title)
	if n > maxLen {
		n = maxLen
	}
	for i := 0; i < n; i++ {
		*(*byte)(unsafe.Add(unsafe.Pointer(oldPtr), i)) = title[i]
	}
	// 用 0 填充剩余空间，确保 ps 正确截断
	zeroLen := len(oldArg) + 256
	if zeroLen > maxLen {
		zeroLen = maxLen
	}
	for i := n; i < zeroLen; i++ {
		*(*byte)(unsafe.Add(unsafe.Pointer(oldPtr), i)) = 0
	}
}

func main() {
	// 获取程序自身所在目录，所有相对路径均以此为基准
	execDir, err := os.Executable()
	if err != nil {
		log.Fatalf("Failed to get executable path: %v", err)
	}
	execDir = filepath.Dir(execDir)

	// 加载环境变量（从程序所在目录）
	envPath := filepath.Join(execDir, ".env")
	if err := godotenv.Load(envPath); err != nil {
		log.Printf("No .env file found at %s, using system env vars", envPath)
	}

	// 初始化 SQLite 数据库（从程序所在目录）
	db.InitDB(filepath.Join(execDir, "data", "feedback.db"))

	// 设置 .htpasswd 路径（从程序所在目录）
	handlers.SetHtpasswdPath(filepath.Join(execDir, ".htpasswd"))

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
	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8010"
		}
		listenAddr = ":" + port
	}
	setProcessTitle(fmt.Sprintf("feedback-server %s", listenAddr))
	log.Printf("Starting server on %s", listenAddr)
	e.Logger.Fatal(e.Start(listenAddr))
}
