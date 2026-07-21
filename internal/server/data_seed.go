package server

import (
	"context"
	"embed"
	"encoding/json"
	"strings"

	"github.com/yourname/nscan/internal/server/repositories"
	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

//go:embed data
var dataFS embed.FS

func SeedFingerprints(ctx context.Context, repo *repositories.FingerprintRepo, log *zap.Logger) {
	count, _ := repo.Count(ctx)
	if count > 0 {
		log.Info("fingerprints already seeded, skipping", zap.Int64("count", count))
		return
	}

	data, err := dataFS.ReadFile("data/fingerprint_nscan.json")
	if err != nil {
		log.Error("read embedded fingerprint data failed", zap.Error(err))
		return
	}

	var fps []models.Fingerprint
	if err := json.Unmarshal(data, &fps); err != nil {
		log.Error("parse fingerprint data failed", zap.Error(err))
		return
	}

	for i := range fps {
		fps[i].Builtin = true
		fps[i].Enabled = true
		fps[i].FpType = "passive"
	}

	inserted, err := repo.BatchInsert(ctx, fps)
	if err != nil {
		log.Error("seed fingerprints failed", zap.Error(err))
		return
	}
	log.Info("fingerprints seeded", zap.Int("count", inserted))
}

type dictSeedEntry struct {
	Category    string
	Name        string
	Description string
	File        string
}

func SeedDicts(ctx context.Context, repo *repositories.DictRepo, log *zap.Logger) {
	entries := []dictSeedEntry{
		{"subdomain", "子域名爆破字典", "", "data/subdomain.txt"},
		{"directory", "目录扫描字典", "", "data/dir.txt"},
	}

	for _, e := range entries {
		existing, _ := repo.List(ctx, e.Category)
		hasBuiltin := false
		for _, d := range existing {
			if d.Builtin {
				hasBuiltin = true
				break
			}
		}
		if hasBuiltin {
			log.Info("dict already seeded, skipping", zap.String("category", e.Category))
			continue
		}

		data, err := dataFS.ReadFile(e.File)
		if err != nil {
			log.Error("read embedded dict failed", zap.String("file", e.File), zap.Error(err))
			continue
		}

		lines := splitTextLines(string(data))
		if len(lines) == 0 {
			continue
		}

		d := &models.Dict{
			Category:    e.Category,
			Name:        e.Name,
			Description: e.Description,
			Builtin:     true,
			Active:      true,
		}
		if err := repo.Create(ctx, d, lines); err != nil {
			log.Error("seed dict failed", zap.String("name", e.Name), zap.Error(err))
			continue
		}
		log.Info("dict seeded", zap.String("name", e.Name), zap.Int("lines", len(lines)))
	}

	// 按协议分开的弱口令字典（内置爆破工具用）
	seedBruteDicts(ctx, repo, log)
}

// ── 按协议分开的爆破字典（user:pass 合并格式）────────────────────────────────

// 内置默认弱口令，多数协议共享
var defaultBrutePasswords = []string{
	"", "123456", "admin", "password", "root", "admin123", "123456789",
	"12345678", "1234", "test", "admin@123", "P@ssw0rd", "123123",
	"abc123", "111111", "000000", "qwerty", "letmein", "master",
}

var bruteProtocolSeeds = []struct {
	Service string
	Label   string
	Users   []string
}{
	{"ssh", "SSH", []string{"root", "admin", "ubuntu", "test", "user", "deploy"}},
	{"ftp", "FTP", []string{"root", "admin", "ftp", "anonymous", "test", "www"}},
	{"mysql", "MySQL", []string{"root", "admin", "mysql", "test", "dba"}},
	{"redis", "Redis", []string{""}},
	{"mongodb", "MongoDB", []string{"admin", "root", "test"}},
	{"postgresql", "PostgreSQL", []string{"postgres", "admin", "test"}},
	{"mssql", "MSSQL", []string{"sa", "admin", "test"}},
}

func seedBruteDicts(ctx context.Context, repo *repositories.DictRepo, log *zap.Logger) {
	// 清理旧的分离式字典（users/passwords kind）
	dropObsoleteBruteDicts(ctx, repo, log)

	for _, sp := range bruteProtocolSeeds {
		if bruteBuiltinExists(ctx, repo, sp.Service) {
			continue
		}
		creds := buildCredentials(sp.Users, defaultBrutePasswords)
		d := &models.Dict{
			Category:    "password",
			Service:     sp.Service,
			Name:        sp.Label + " 默认弱口令",
			Description: sp.Label + " 常见用户名 × 常见密码组合（一行一组 user:pass）",
			Builtin:     true,
			Active:      true,
		}
		if err := repo.Create(ctx, d, creds); err != nil {
			log.Error("seed brute dict failed", zap.String("service", sp.Service), zap.Error(err))
			continue
		}
		log.Info("brute dict seeded", zap.String("service", sp.Service), zap.Int("lines", len(creds)))
	}
}

// buildCredentials 用 users × passwords 展开为 "user:pass" 合并列表
func buildCredentials(users, passwords []string) []string {
	out := make([]string, 0, len(users)*len(passwords))
	for _, u := range users {
		for _, p := range passwords {
			out = append(out, u+":"+p)
		}
	}
	return out
}

func bruteBuiltinExists(ctx context.Context, repo *repositories.DictRepo, service string) bool {
	list, err := repo.Query(ctx, repositories.ListFilter{Category: "password", Service: service})
	if err != nil {
		return false
	}
	for _, d := range list {
		if d.Builtin && d.Kind == "" {
			return true
		}
	}
	return false
}

// dropObsoleteBruteDicts 清理旧版本的内置字典：
//   1. Kind=users / Kind=passwords 的分离式字典（当前已合并为 credentials）
//   2. Service 为空的通用「默认弱口令字典」（hydra 时代遗留）
func dropObsoleteBruteDicts(ctx context.Context, repo *repositories.DictRepo, log *zap.Logger) {
	for _, kind := range []string{"users", "passwords"} {
		list, err := repo.Query(ctx, repositories.ListFilter{Category: "password", Kind: kind})
		if err != nil {
			continue
		}
		for _, d := range list {
			if !d.Builtin {
				continue
			}
			if err := repo.Delete(ctx, d.ID); err == nil {
				log.Info("obsolete brute dict removed", zap.String("name", d.Name))
			}
		}
	}
	// 通用 service 空的旧内置字典（Service="*" 关闭 service 过滤）
	list, err := repo.Query(ctx, repositories.ListFilter{Category: "password", Service: "*"})
	if err != nil {
		return
	}
	for _, d := range list {
		if !d.Builtin || d.Service != "" {
			continue
		}
		if err := repo.Delete(ctx, d.ID); err == nil {
			log.Info("obsolete generic brute dict removed", zap.String("name", d.Name))
		}
	}
}

func LoadEmbeddedFingerprints() ([]models.Fingerprint, error) {
	data, err := dataFS.ReadFile("data/fingerprint_nscan.json")
	if err != nil {
		return nil, err
	}
	var fps []models.Fingerprint
	if err := json.Unmarshal(data, &fps); err != nil {
		return nil, err
	}
	for i := range fps {
		fps[i].Builtin = true
		fps[i].Enabled = true
		fps[i].FpType = "passive"
	}
	return fps, nil
}

func LoadEmbeddedDict(category string) ([]string, error) {
	fileMap := map[string]string{
		"subdomain": "data/subdomain.txt",
		"directory": "data/dir.txt",
		"password":  "data/weakpass.txt",
	}
	file, ok := fileMap[category]
	if !ok {
		return nil, nil
	}
	data, err := dataFS.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return splitTextLines(string(data)), nil
}

func splitTextLines(s string) []string {
	raw := strings.Split(s, "\n")
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
