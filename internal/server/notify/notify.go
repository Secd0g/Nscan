// Package notify 负责向用户配置的通知渠道（企业微信/钉钉/Slack/邮件）推送消息。
package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"time"

	"github.com/yourname/nscan/internal/server/repositories"
	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

type Notifier struct {
	repo   *repositories.NotifyRepo
	log    *zap.Logger
	client *http.Client
}

func New(repo *repositories.NotifyRepo, log *zap.Logger) *Notifier {
	return &Notifier{
		repo:   repo,
		log:    log,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Notify 向所有订阅了 event 的启用渠道异步推送一条消息。
func (n *Notifier) Notify(event, title, body string) {
	if n == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		channels, err := n.repo.EnabledForEvent(ctx, event)
		if err != nil {
			n.log.Warn("load notify channels failed", zap.Error(err))
			return
		}
		for i := range channels {
			ch := channels[i]
			if err := n.SendTo(&ch, title, body); err != nil {
				n.log.Warn("notify send failed", zap.String("channel", ch.Key), zap.Error(err))
			}
		}
	}()
}

// NotifyUser is kept for backward compatibility; userID is no longer used.
func (n *Notifier) NotifyUser(_ interface{}, event, title, body string) {
	n.Notify(event, title, body)
}

// SendTo 向指定渠道发送一条消息（供事件推送与测试复用）。
func (n *Notifier) SendTo(ch *models.NotifyChannel, title, body string) error {
	switch ch.Key {
	case "wecom":
		return n.sendWecom(ch.Config, title, body)
	case "dingtalk":
		return n.sendDingtalk(ch.Config, title, body)
	case "slack":
		return n.sendSlack(ch.Config, title, body)
	case "telegram":
		return n.sendTelegram(ch.Config, title, body)
	case "email":
		return n.sendEmail(ch.Config, title, body)
	default:
		return fmt.Errorf("未知渠道: %s", ch.Key)
	}
}

func (n *Notifier) postJSON(rawurl string, payload any) error {
	if rawurl == "" {
		return fmt.Errorf("webhook 未配置")
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, rawurl, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func (n *Notifier) sendWecom(cfg map[string]string, title, body string) error {
	content := "**" + title + "**\n" + body
	return n.postJSON(cfg["webhook"], map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]string{"content": content},
	})
}

func (n *Notifier) sendDingtalk(cfg map[string]string, title, body string) error {
	webhook := cfg["webhook"]
	// 若配置了加签密钥，按钉钉规则追加 timestamp 与 sign。
	if secret := cfg["secret"]; secret != "" && webhook != "" {
		ts := fmt.Sprintf("%d", time.Now().UnixMilli())
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(ts + "\n" + secret))
		sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))
		sep := "?"
		if strings.Contains(webhook, "?") {
			sep = "&"
		}
		webhook = webhook + sep + "timestamp=" + ts + "&sign=" + url.QueryEscape(sign)
	}
	return n.postJSON(webhook, map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]string{"title": title, "text": "### " + title + "\n" + body},
	})
}

func (n *Notifier) sendSlack(cfg map[string]string, title, body string) error {
	return n.postJSON(cfg["webhook"], map[string]string{"text": "*" + title + "*\n" + body})
}

func (n *Notifier) sendTelegram(cfg map[string]string, title, body string) error {
	token := cfg["bot_token"]
	chatID := cfg["chat_id"]
	if token == "" || chatID == "" {
		return fmt.Errorf("Telegram 配置不完整（需 bot_token / chat_id）")
	}
	text := "*" + title + "*\n" + body
	apiURL := "https://api.telegram.org/bot" + token + "/sendMessage"
	return n.postJSON(apiURL, map[string]string{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	})
}

func (n *Notifier) sendEmail(cfg map[string]string, title, body string) error {
	host := cfg["smtp_host"] // 形如 smtp.example.com:465
	from := cfg["from"]
	password := cfg["password"]
	to := splitList(cfg["to"])
	if host == "" || from == "" || len(to) == 0 {
		return fmt.Errorf("邮件配置不完整（需 smtp_host / from / to）")
	}
	hostname, _, err := net.SplitHostPort(host)
	if err != nil {
		return fmt.Errorf("smtp_host 需为 host:port 格式")
	}

	msg := buildEmail(from, to, title, body)
	auth := smtp.PlainAuth("", from, password, hostname)

	// 465 为隐式 TLS（SMTPS），net/smtp 的 SendMail 不支持，需手动建立 TLS 连接。
	if strings.HasSuffix(host, ":465") {
		return sendMailTLS(host, hostname, auth, from, to, msg)
	}
	// 其余端口（587/25）走 STARTTLS，由 SendMail 处理。
	return smtp.SendMail(host, auth, from, to, msg)
}

func sendMailTLS(addr, hostname string, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: hostname})
	if err != nil {
		return err
	}
	// 若 NewClient 失败，需自行关闭底层连接（smtp.Client 尚未接管）。
	c, err := smtp.NewClient(conn, hostname)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if err := c.Auth(auth); err != nil {
		return err
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err := c.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func buildEmail(from string, to []string, title, body string) []byte {
	var b strings.Builder
	b.WriteString("From: nscan <" + from + ">\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	b.WriteString("Subject: " + mimeEncode(title) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// mimeEncode 用 RFC 2047 编码主题，避免中文乱码。
func mimeEncode(s string) string {
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
