# yt-music-bot

一个用 Go 编写的 Telegram 机器人，搜索并下载 YouTube（含 YouTube Music）音频，
转换为 MP3 后发送到 Telegram。

## 功能

- `/music 关键词` —— 搜索 YouTube，分页（每页 5 条）展示，点击歌曲按钮即下载上传，支持 ◀️/▶️ 翻页
- 直接发送 YouTube / YouTube Music 链接 —— 解析并下载上传
- 自动抓取最高码率音频流，经 `ffmpeg` 转码为 MP3（含标题/艺人 ID3 标签）
- 下载过程实时进度条（百分比 + 已下载/总大小，每 1.4s 刷新）
- 自动抓取封面并缩放为 320×320 方形作为音频封面
- 上传音频携带 caption：🎵 标题 / 👤 艺人 / ⏱ 时长 / 🔗 来源链接
- 文件名美化：`标题.mp3`
- 支持代理（`PROXY` / `HTTP_PROXY` / `HTTPS_PROXY` / `ALL_PROXY`）
- 支持自建 [Telegram Bot API](https://github.com/tdlib/telegram-bot-api) 服务以突破 50MB 上传限制

## 依赖

- Go 1.26+（kkdai/youtube 要求）
- `ffmpeg`（系统 PATH 中可执行，需含 `libmp3lame`）

## 构建

```bash
go build -o yt-music-bot .
```

交叉编译（例如在 macOS 编出 Linux amd64 二进制）：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o yt-music-bot .
```

## 运行

```bash
export BOT_TOKEN=123456:ABC-DEF...
./yt-music-bot
```

或使用 `.env`：

```bash
cp .env.example .env
# 编辑 .env 后
set -a; . ./.env; set +a
./yt-music-bot
```

## Docker 部署（推荐，零主机依赖）

镜像为多阶段构建：`golang:1.26` 编译 + `debian:stable-slim` 运行（自带 ffmpeg）。

```bash
cp .env.example .env      # 填入 BOT_TOKEN 等
docker compose up -d --build
```

查看日志 / 停止：

```bash
docker compose logs -f
docker compose down
```

仅用 Docker（无 compose）：

```bash
docker build -t yt-music-bot .
docker run -d --restart unless-stopped --env-file .env --name yt-music-bot yt-music-bot
```

## 配置项

| 变量 | 必填 | 默认 | 说明 |
|------|------|------|------|
| `BOT_TOKEN` | 是 | — | Telegram bot token |
| `TG_API_ENDPOINT` | 否 | 官方 | 自建 Bot API 服务地址 |
| `PROXY` | 否 | — | 代理 URL |
| `MAX_DURATION_MIN` | 否 | 12 | 拒绝超过此时长的歌曲 |
| `SEARCH_LIMIT` | 否 | 8 | 搜索结果数量（≤20） |
| `DOWNLOAD_DIR` | 否 | 系统临时 | 临时下载目录 |
| `INNERTUBE_KEY` | 否 | 运行时抓取 | YouTube innertube 公共 web key（默认从 youtube.com 动态获取，无需设置） |
| `INNERTUBE_CLIENT_VERSION` | 否 | 内置默认 | innertube web client 版本，配合 `INNERTUBE_KEY` 使用 |

## 用法

1. 私聊机器人 `/music 周杰伦 晴天`
2. 在分页结果中点击对应歌曲按钮（可翻页）
3. 机器人下载并转码为 MP3，以音频形式发送（带封面、标签、来源链接）
4. 也可直接粘贴 `https://music.youtube.com/watch?v=xxxx` 链接

## 测试

```bash
# 离线：编译并运行单测（不需联网）
go test ./...

# 端到端：真实搜索 + 下载 + 转 MP3 + 封面（需联网与 ffmpeg）
go test -tags e2e -run TestE2ESearchDownload -v -timeout 180s ./internal/youtube/
```

## 架构

```
main.go                     入口
internal/config             环境变量 + 代理 HTTP 客户端
internal/youtube
  ├─ search.go              innertube 搜索 + 链接解析（递归解析 JSON）
  ├─ download.go            kkdai/youtube 取流 + 进度回调 + ffmpeg 转 MP3
  └─ thumbnail.go           抓取封面并缩放为 320×320
internal/bot
  ├─ bot.go                 长轮询 + goroutine 分发
  ├─ sessions.go            搜索结果会话存储（分页/回调用，30min TTL）
  └─ handlers.go            命令/链接/回调/进度条/封面/上传
```

## 许可

Apache-2.0
