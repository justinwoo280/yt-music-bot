package bot

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"yt-music-bot/internal/youtube"
)

const (
	pageSize         = 5
	maxTelegramBytes = 50 * 1024 * 1024
)

const helpText = "🎵 *YouTube Music Bot*\n\n" +
	"• `/music 关键词` — 搜索并列出结果，点击按钮下载\n" +
	"• 直接发送 YouTube / YouTube Music 链接 — 下载并上传\n\n" +
	"音频会转换为 MP3（带封面与标签）后发送。"

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)

	switch {
	case text == "/start" || text == "/start@"+b.api.Self.UserName,
		text == "/help" || text == "/help@"+b.api.Self.UserName:
		b.sendMD(chatID, helpText)

	case strings.HasPrefix(text, "/music"):
		query := strings.TrimSpace(strings.TrimPrefix(text, "/music"))
		query = strings.TrimPrefix(query, "@"+b.api.Self.UserName)
		query = strings.TrimSpace(query)
		b.handleSearch(ctx, chatID, query)

	default:
		if id := youtube.ExtractVideoID(text); id != "" {
			b.handleDownload(ctx, chatID, id)
			return
		}
		b.sendText(chatID, "发送 YouTube Music 链接即可下载，或使用 /music 关键词 搜索。")
	}
}

func (b *Bot) handleSearch(ctx context.Context, chatID int64, query string) {
	if query == "" {
		b.sendText(chatID, "用法：/music 关键词")
		return
	}

	status := b.sendText(chatID, "🔍 搜索中…")
	tracks, err := b.yt.Search(ctx, query, b.cfg.SearchLimit)
	b.deleteMessage(chatID, status)
	if err != nil {
		b.sendText(chatID, "❌ 搜索失败："+err.Error())
		return
	}
	if len(tracks) == 0 {
		b.sendText(chatID, "未找到结果。")
		return
	}

	sid := b.sessions.put(query, tracks)
	if _, err := b.renderPage(chatID, sid, 0, 0); err != nil {
		b.sendText(chatID, "❌ 发送结果失败："+err.Error())
	}
}

func (b *Bot) renderPage(chatID int64, sid string, page int, editMsgID int) (*tgbotapi.Message, error) {
	ss, ok := b.sessions.get(sid)
	if !ok {
		return nil, fmt.Errorf("结果已过期，请重新搜索")
	}
	tracks := ss.tracks
	pages := (len(tracks) + pageSize - 1) / pageSize
	if pages == 0 {
		pages = 1
	}
	if page < 0 {
		page = 0
	}
	if page > pages-1 {
		page = pages - 1
	}

	start := page * pageSize
	end := start + pageSize
	if end > len(tracks) {
		end = len(tracks)
	}

	header := fmt.Sprintf("🔎 *%s*（第 %d/%d 页）\n\n点击歌曲按钮下载：",
		escapeMD(ss.query), page+1, pages)

	var rows [][]tgbotapi.InlineKeyboardButton
	for i := start; i < end; i++ {
		t := tracks[i]
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(buttonLabel(i+1, t), fmt.Sprintf("d:%s:%d", sid, i)),
		))
	}

	var nav []tgbotapi.InlineKeyboardButton
	if page > 0 {
		nav = append(nav, tgbotapi.NewInlineKeyboardButtonData("◀️ 上一页", fmt.Sprintf("p:%s:%d", sid, page-1)))
	}
	if page < pages-1 {
		nav = append(nav, tgbotapi.NewInlineKeyboardButtonData("下一页 ▶️", fmt.Sprintf("p:%s:%d", sid, page+1)))
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	markup := tgbotapi.NewInlineKeyboardMarkup(rows...)

	if editMsgID != 0 {
		edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, editMsgID, header, markup)
		edit.ParseMode = tgbotapi.ModeMarkdown
		_, err := b.api.Send(edit)
		if err != nil {
			b.log.Printf("edit page: %v", err)
		}
		return nil, err
	}

	out := tgbotapi.NewMessage(chatID, header)
	out.ParseMode = tgbotapi.ModeMarkdown
	out.ReplyMarkup = markup
	m, err := b.api.Send(out)
	if err != nil {
		b.log.Printf("send page: %v", err)
		return nil, err
	}
	return &m, nil
}

func (b *Bot) handleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	parts := strings.SplitN(cb.Data, ":", 3)
	if len(parts) < 2 {
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "未知操作"))
		return
	}

	switch parts[0] {
	case "d":
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "⬇️ 开始下载…"))
		if len(parts) < 3 || cb.Message == nil {
			return
		}
		sid := parts[1]
		idx, err := strconv.Atoi(parts[2])
		if err != nil {
			return
		}
		ss, ok := b.sessions.get(sid)
		if !ok || idx < 0 || idx >= len(ss.tracks) {
			_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "⚠️ 结果已过期，请重新搜索"))
			return
		}
		b.handleDownload(ctx, cb.Message.Chat.ID, ss.tracks[idx].VideoID)

	case "p":
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
		if len(parts) < 3 || cb.Message == nil {
			return
		}
		sid := parts[1]
		page, err := strconv.Atoi(parts[2])
		if err != nil {
			return
		}
		_, _ = b.renderPage(cb.Message.Chat.ID, sid, page, cb.Message.MessageID)

	default:
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "未知操作"))
	}
}

// handleDownload fetches audio, shows a progress bar, then uploads it.
func (b *Bot) handleDownload(ctx context.Context, chatID int64, videoID string) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	status := b.sendText(chatID, "⬇️ 准备下载…")
	ui := &progressUI{bot: b, chatID: chatID, msg: status}

	res, err := b.yt.Download(ctx, videoID, ui.report)
	if err != nil {
		b.editText(chatID, status, "❌ 下载失败："+err.Error())
		return
	}
	defer os.Remove(res.Path)

	if b.cfg.TGAPIEndpoint == "" && res.FileSize > maxTelegramBytes {
		b.editText(chatID, status,
			fmt.Sprintf("❌ 文件过大（%d MB），超过 Telegram 50MB 限制。\n可配置自建 Bot API（TG_API_ENDPOINT）以支持更大文件。",
				res.FileSize/(1024*1024)))
		return
	}

	b.editText(chatID, status, "🎨 获取封面…")
	thumb, _ := b.yt.FetchThumbnail(ctx, videoID)

	b.editText(chatID, status, "⬆️ 上传中…")
	if err := b.sendAudio(chatID, res, videoID, thumb); err != nil {
		b.editText(chatID, status, "❌ 上传失败："+err.Error())
		return
	}

	b.deleteMessage(chatID, status)
}

func (b *Bot) sendAudio(chatID int64, res *youtube.DownloadResult, videoID string, thumb []byte) error {
	f, err := os.Open(res.Path)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	audio := tgbotapi.NewAudio(chatID, tgbotapi.FileReader{Name: audioFilename(res), Reader: f})
	audio.Title = res.Title
	audio.Performer = res.Artist
	audio.Duration = res.Seconds
	audio.Caption = audioCaption(res, videoID)
	if len(thumb) > 0 {
		audio.Thumb = tgbotapi.FileBytes{Name: "cover.jpg", Bytes: thumb}
	}
	if _, err := b.api.Send(audio); err != nil {
		return err
	}
	return nil
}

// ---------- progress UI ----------

type progressUI struct {
	bot    *Bot
	chatID int64
	msg    *tgbotapi.Message
	last   time.Time
}

func (p *progressUI) report(downloaded, total int64) {
	if p.msg == nil {
		return
	}
	finished := total > 0 && downloaded >= total
	now := time.Now()
	if !finished && now.Sub(p.last) < 1400*time.Millisecond {
		return
	}
	p.last = now
	p.bot.editText(p.chatID, p.msg, progressBar(downloaded, total))
}

func progressBar(downloaded, total int64) string {
	if total > 0 {
		pct := int(float64(downloaded) * 100 / float64(total))
		if pct > 100 {
			pct = 100
		}
		filled := pct / 5 // 20 cells
		bar := strings.Repeat("█", filled) + strings.Repeat("░", 20-filled)
		return fmt.Sprintf("⬇️ 下载中…\n%s %d%%\n%s / %s",
			bar, pct, humanBytes(downloaded), humanBytes(total))
	}
	return fmt.Sprintf("⬇️ 下载中…\n%s", humanBytes(downloaded))
}

func humanBytes(n int64) string {
	const mb = 1024 * 1024
	if n >= mb {
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	}
	return fmt.Sprintf("%.0f KB", float64(n)/1024)
}

// ---------- audio presentation helpers ----------

func audioFilename(res *youtube.DownloadResult) string {
	name := clean(res.Title)
	if name == "" {
		name = "audio"
	}
	if r := []rune(name); len(r) > 120 {
		name = string(r[:120])
	}
	return name + ".mp3"
}

func audioCaption(res *youtube.DownloadResult, videoID string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "🎵 %s", clean(res.Title))
	if a := clean(res.Artist); a != "" {
		fmt.Fprintf(&sb, "\n👤 %s", a)
	}
	if res.Seconds > 0 {
		fmt.Fprintf(&sb, "\n⏱ %s", fmtDuration(res.Seconds))
	}
	fmt.Fprintf(&sb, "\n🔗 https://music.youtube.com/watch?v=%s", videoID)
	return sb.String()
}

func buttonLabel(index int, t youtube.Track) string {
	label := fmt.Sprintf("%d. %s", index, t.Title)
	if t.Artist != "" {
		label += " — " + t.Artist
	}
	if t.Duration != "" {
		label += " (" + t.Duration + ")"
	}
	return truncate(label, 64)
}

// ---------- messaging helpers ----------

func (b *Bot) sendText(chatID int64, text string) *tgbotapi.Message {
	return b.send(chatID, text, false)
}

func (b *Bot) sendMD(chatID int64, text string) *tgbotapi.Message {
	return b.send(chatID, text, true)
}

func (b *Bot) send(chatID int64, text string, markdown bool) *tgbotapi.Message {
	msg := tgbotapi.NewMessage(chatID, text)
	if markdown {
		msg.ParseMode = tgbotapi.ModeMarkdown
	}
	msg.DisableWebPagePreview = true
	m, err := b.api.Send(msg)
	if err != nil {
		b.log.Printf("send message: %v", err)
		return nil
	}
	return &m
}

func (b *Bot) editText(chatID int64, target *tgbotapi.Message, text string) {
	if target == nil {
		return
	}
	edit := tgbotapi.NewEditMessageText(chatID, target.MessageID, text)
	if _, err := b.api.Send(edit); err != nil {
		b.log.Printf("edit message: %v", err)
	}
}

func (b *Bot) deleteMessage(chatID int64, target *tgbotapi.Message) {
	if target == nil {
		return
	}
	del := tgbotapi.NewDeleteMessage(chatID, target.MessageID)
	if _, err := b.api.Request(del); err != nil {
		b.log.Printf("delete message: %v", err)
	}
}

// ---------- small utils ----------

func clean(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\x00", "")
	s = strings.ReplaceAll(s, "/", "-")
	return s
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func fmtDuration(sec int) string {
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

// escapeMD escapes MarkdownV1 special characters in user-supplied content.
func escapeMD(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "_", "\\_")
	s = strings.ReplaceAll(s, "*", "\\*")
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "[", "\\[")
	return s
}
