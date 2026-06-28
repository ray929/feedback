# Feedback — 多租户反馈表单系统

轻量级、可嵌入的反馈表单系统，供多个网站通过 iframe 嵌入使用。设计风格简洁高级，支持中英双语和暗黑/明亮模式。

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go + Echo |
| 数据库 | SQLite（CGO-free，`modernc.org/sqlite`） |
| 缓存/限流 | Redis |
| 前端 | Alpine.js + 手写 CSS |
| 验证码 | Cloudflare Turnstile |
| 邮件 | Resend |
| 部署 | `go:embed` 嵌入静态资源，单二进制文件 |

## 功能

- **多租户**：创建多个表单，每个表单有独立 UUID、独立提交和查询入口
- **可嵌入**：通过 `<script src="embed.js" data-id="UUID">` 零代码嵌入任意网页，iframe 自动适应高度
- **双语 + 双主题**：支持 `?lang=zh/en` 和 `?theme=light/dark`
- **反垃圾**：Cloudflare Turnstile 人机验证 + IP 限流（10 次/小时）
- **邮件通知**：表单提交后自动发送邮件通知（通过 Resend）
- **后台管理**：极简单管理员模式，仅需密码登录
- **数据查询**：独立查询页面，通过密码查看提交记录

## 路由

| 路径 | 说明 |
|---|---|
| `/p` | 后台管理页面 |
| `/f/:id` | 表单页面（供 iframe 嵌入） |
| `/q/:id` | 数据查询页面 |
| `/embed.js` | 嵌入脚本（自动 iframe + 高度自适应） |
| `/api/login` | 管理员登录 |
| `/api/logout` | 管理员登出 |
| `/api/submit` | 提交表单 |
| `/api/query/:id` | 查询提交记录（密码验证） |
| `/api/query/:id/logout` | 查询页面登出 |
| `/api/admin/forms` | 表单 CRUD |
| `/api/admin/forms/:id/submissions` | 查看提交记录 |

## 环境变量

```env
# 应用配置
APP_ENV="development"              # development 或 production
SITE_NAME="Forms"
LISTEN_ADDR="0.0.0.0:8010"          # 监听地址，留空则使用 PORT

# Cloudflare Turnstile
TURNSTILE_SITE_KEY="1x00000000000000000000AA"
TURNSTILE_SECRET_KEY="1x0000000000000000000000000000000AA"
HTTP_PROXY="http://127.0.0.1:10808"   # 本地调试 Turnstile 时的代理

# 邮件通知（Resend）
RESEND_API_KEY="re_xxxx"
DEFAULT_RECIPIENT="your-email@example.com"
FROM_EMAIL="noreply@example.com"
FROM_NAME="Form Messenger"

# Redis
REDIS_HOST="127.0.0.1"
REDIS_PORT="6379"
REDIS_PASSWORD=""
REDIS_DB="0"
REDIS_PREFIX="feedback"
```

## 密码文件

管理员密码存储在项目根目录的 `.htpasswd` 文件中，格式为标准 htpasswd：

```
admin:$2a$10$...
```

密码为 bcrypt 加密。可以使用 `htpasswd` 工具或在线 bcrypt 生成器生成。

## 本地开发

### 前置条件

- Go 1.25+
- Redis（用于限流，开发环境可选择性启动）

### 安装并运行

```bash
# 安装依赖
go mod download

# 安装 air（热重载工具）
go install github.com/air-verse/air@latest

# 启动开发服务器（热重载）
air

# 或直接运行
go run main.go
```

服务启动后访问 `http://localhost:8010`。

### 调试 Turnstile

Turnstile 在本地环境始终通过验证（使用测试密钥）。如需调试远程验证，可配置 `HTTP_PROXY` 代理。

## 构建

```bash
# Linux amd64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o feedback-server .

# 运行
./feedback-server
```

构建产物为单文件，包含所有静态资源。部署时只需：

1. 上传 `feedback-server` 二进制文件
2. 配置 `.env` 和 `.htpasswd`
3. 启动服务

`data/` 目录会自动创建 SQLite 数据库文件。

### 部署（systemd）

所有文件放在 `/data/www/feedback/`，以 `www` 用户运行：

```bash
# 创建目录和用户
mkdir -p /data/www/feedback
useradd -r -s /sbin/nologin www

# 上传文件
cp feedback-server /data/www/feedback/
cp .env /data/www/feedback/
cp .htpasswd /data/www/feedback/
chown -R www:www /data/www/feedback
chmod +x /data/www/feedback/feedback-server
```

创建 `/etc/systemd/system/feedback.service`：

```ini
[Unit]
Description=Feedback Server
After=network.target redis.service

[Service]
Type=simple
User=www
Group=www
WorkingDirectory=/data/www/feedback
EnvironmentFile=/data/www/feedback/.env
ExecStart=/data/www/feedback/feedback-server
Restart=always
RestartSec=5

# 安全加固
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/data/www/feedback
PrivateTmp=yes

[Install]
WantedBy=multi-user.target
```

启动服务：

```bash
systemctl daemon-reload
systemctl enable --now feedback
systemctl status feedback
```

### GitHub Release

通过 GitHub Actions 手动构建发布：

1. 进入 Actions → Release → Run workflow
2. 输入版本号（如 `1.0.0`）
3. 自动创建 tag、编译 Linux 二进制文件并发布到 Release 页面

## 嵌入方式

在任意网页中引入脚本即可：

```html
<script src="https://your-domain/embed.js"
        data-id="表单UUID"
        data-theme="light"
        data-lang="zh">
</script>
```

| 属性 | 必填 | 说明 |
|---|---|---|
| `data-id` | 是 | 表单的 UUID |
| `data-theme` | 否 | `light`（默认）或 `dark` |
| `data-lang` | 否 | `zh` 或 `en`（默认） |

## 数据模型

### Form（表单）

| 字段 | 说明 |
|---|---|
| `uuid` | 唯一标识符，用于嵌入和查询 |
| `name` | 表单名称 |
| `query_password` | 查询密码（bcrypt） |
| `notify_email` | 通知邮箱 |

### Submission（提交记录）

| 字段 | 说明 |
|---|---|
| `form_id` | 所属表单 |
| `name` | 提交者姓名 |
| `email` | 邮箱 |
| `phone` | 电话 |
| `content` | 反馈内容 |
| `source_url` | 来源页面 URL |
| `client_ip` | 客户端 IP |

## 目录结构

```
.
├── main.go                  # 入口，路由注册
├── internal/
│   ├── db/                  # SQLite 初始化与迁移
│   ├── handlers/            # 路由处理
│   │   ├── auth.go          # 登录/登出
│   │   ├── admin.go         # 后台 CRUD
│   │   ├── api.go           # 表单提交 + 查询 + Turnstile
│   │   ├── email.go         # Resend 邮件发送
│   │   └── form.go          # 页面渲染
│   ├── models/              # 数据模型
│   └── redis/               # Redis 连接 + 限流
├── public/
│   ├── css/style.css        # 全局样式
│   ├── embed.js             # 嵌入脚本（iframe + 高度自适应）
│   └── templates/           # HTML 模板
│       ├── form.html        # 表单页面
│       ├── admin.html       # 后台管理
│       └── query.html       # 数据查询
├── .air.toml                # 热重载配置
├── go.mod / go.sum
└── .github/workflows/       # CI/CD
```
