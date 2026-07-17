package server

import (
	"context"

	"github.com/yourname/nscan/internal/server/repositories"
	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

// SeedSensitiveRules 首次启动种子内置敏感信息规则（凭据泄漏正则）。
// 已存在数据时跳过；用户可在前端编辑/停用。
func SeedSensitiveRules(ctx context.Context, repo *repositories.SensitiveRuleRepo, log *zap.Logger) {
	count, _ := repo.Count(ctx)
	if count > 0 {
		log.Info("sensitive rules already seeded, skipping", zap.Int64("count", count))
		return
	}
	rules := []models.SensitiveRule{
		{Name: "AWS Access Key ID", Pattern: `AKIA[0-9A-Z]{16}`, Severity: "critical", Description: "AWS 访问密钥 ID"},
		{Name: "AWS Secret Access Key", Pattern: `(?i)aws(.{0,20})?(?-i)['"][0-9a-zA-Z/+]{40}['"]`, Severity: "critical", Description: "AWS 秘密访问密钥"},
		{Name: "GitHub Personal Token", Pattern: `ghp_[0-9a-zA-Z]{36}`, Severity: "high", Description: "GitHub 个人访问令牌"},
		{Name: "GitHub OAuth Token", Pattern: `gho_[0-9a-zA-Z]{36}`, Severity: "high", Description: "GitHub OAuth 令牌"},
		{Name: "GitHub App Token", Pattern: `(?:ghu|ghs)_[0-9a-zA-Z]{36}`, Severity: "high", Description: "GitHub App 令牌"},
		{Name: "Slack Token", Pattern: `xox[baprs]-[0-9a-zA-Z-]{10,}`, Severity: "high", Description: "Slack API 令牌"},
		{Name: "Slack Webhook", Pattern: `https://hooks\.slack\.com/services/T[0-9A-Z]{8,}/B[0-9A-Z]{8,}/[0-9a-zA-Z]{24}`, Severity: "medium", Description: "Slack Webhook URL"},
		{Name: "Google API Key", Pattern: `AIza[0-9A-Za-z_\-]{35}`, Severity: "high", Description: "Google Cloud API 密钥"},
		{Name: "Google OAuth Access Token", Pattern: `ya29\.[0-9A-Za-z_\-]+`, Severity: "high", Description: "Google OAuth Access Token"},
		{Name: "Stripe Secret Key", Pattern: `sk_live_[0-9a-zA-Z]{24,}`, Severity: "critical", Description: "Stripe 生产环境 Secret Key"},
		{Name: "Stripe Publishable Key", Pattern: `pk_live_[0-9a-zA-Z]{24,}`, Severity: "medium", Description: "Stripe 生产环境 Publishable Key"},
		{Name: "Twilio API Key", Pattern: `SK[0-9a-fA-F]{32}`, Severity: "high", Description: "Twilio API Key"},
		{Name: "SendGrid API Key", Pattern: `SG\.[0-9A-Za-z_\-]{22}\.[0-9A-Za-z_\-]{43}`, Severity: "high", Description: "SendGrid API Key"},
		{Name: "Mailgun API Key", Pattern: `key-[0-9a-zA-Z]{32}`, Severity: "high", Description: "Mailgun API Key"},
		{Name: "JWT Token", Pattern: `eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`, Severity: "medium", Description: "JSON Web Token（可能含敏感信息）"},
		{Name: "Private Key (PEM)", Pattern: `-----BEGIN (RSA|EC|DSA|OPENSSH|PGP| )?PRIVATE KEY-----`, Severity: "critical", Description: "PEM 格式私钥"},
		{Name: "Password in URL", Pattern: `[a-zA-Z]{3,10}://[^/\s:@]+:[^\s@]+@[^\s]+`, Severity: "high", Description: "URL 中携带的账号密码"},
		{Name: "Basic Auth", Pattern: `(?i)authorization:\s*basic\s+[A-Za-z0-9+/=]+`, Severity: "high", Description: "HTTP Basic 认证头"},
		{Name: "MongoDB Connection String", Pattern: `mongodb(\+srv)?://[^\s"']+`, Severity: "high", Description: "MongoDB 连接字符串"},
		{Name: "MySQL/PostgreSQL Connection String", Pattern: `(mysql|postgres(ql)?)://[^\s"']+`, Severity: "high", Description: "关系数据库连接字符串"},
		{Name: "Redis Connection String", Pattern: `redis://[^\s"']+`, Severity: "medium", Description: "Redis 连接字符串"},
		{Name: "Chinese ID Card", Pattern: `[1-9]\d{5}(19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dxX]`, Severity: "high", Description: "中国大陆身份证号"},
		{Name: "Chinese Mobile Phone", Pattern: `1[3-9]\d{9}`, Severity: "low", Description: "中国大陆手机号"},
		{Name: "Email Address", Pattern: `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`, Severity: "low", Description: "电子邮件地址（可能非敏感）"},
	}
	for i := range rules {
		rules[i].Builtin = true
		rules[i].Active = true
		rules[i].Color = severityColor(rules[i].Severity)
		if rules[i].Name == "Chinese Mobile Phone" {
			rules[i].Active = false
		}
	}
	n, err := repo.BatchInsert(ctx, rules)
	if err != nil {
		log.Error("seed sensitive rules failed", zap.Error(err))
		return
	}
	log.Info("sensitive rules seeded", zap.Int("count", n))
}

func severityColor(sev string) string {
	switch sev {
	case "critical":
		return "#f56c6c"
	case "high":
		return "#e6a23c"
	case "medium":
		return "#409eff"
	case "low":
		return "#909399"
	default:
		return "#909399"
	}
}
