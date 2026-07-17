package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yourname/nscan/internal/server/repositories"
	"github.com/yourname/nscan/pkg/models"
)

type Config struct {
	Type     string `json:"type"`
	BaseURL  string `json:"base_url"`
	Token    string `json:"token"`
	Model    string `json:"model"`
	ProxyURL string `json:"proxy_url,omitempty"`
}

func Analyze(ctx context.Context, cfg Config, task *models.Task, assets *repositories.AssetRepo, logf ...func(string)) (string, error) {
	log := func(s string) {
		for _, f := range logf {
			f(s)
		}
	}
	log("开始读取扫描结果")
	f := repositories.AssetFilter{TaskID: task.ID.Hex(), Limit: 2000}
	sub, _, _ := assets.ListSubdomains(ctx, f)
	ports, _, _ := assets.ListPorts(ctx, f)
	httpAssets, _, _ := assets.ListHTTP(ctx, f)
	vulns, _, _ := assets.ListVulns(ctx, f)
	sensitive, _, _ := assets.ListSensitive(ctx, f)
	log(fmt.Sprintf("扫描结果读取完成：子域名 %d、端口 %d、HTTP %d、漏洞 %d、敏感信息 %d", len(sub), len(ports), len(httpAssets), len(vulns), len(sensitive)))
	data, _ := json.Marshal(map[string]any{"subdomains": sub, "ports": ports, "http_assets": httpAssets, "vulnerabilities": vulns, "sensitive": sensitive})
	prompt := fmt.Sprintf("你是网络安全分析师。请分析以下扫描任务结果，输出中文 Markdown，包含：总体风险概览、重点漏洞（按严重程度）、暴露面与弱点、去重后的处置建议。不要臆造不存在的漏洞。任务：%s，目标：%v\n结果：%s", task.Name, task.Targets, data)
	body, _ := json.Marshal(map[string]any{"model": cfg.Model, "messages": []map[string]string{{"role": "system", "content": "你负责严谨分析安全扫描结果。"}, {"role": "user", "content": prompt}}, "temperature": 0.2})
	endpoint := strings.TrimRight(cfg.BaseURL, "/")
	gemini := cfg.Type == "gemini" || strings.Contains(endpoint, "generativelanguage.googleapis.com")
	anthropic := cfg.Type == "anthropic" || strings.Contains(endpoint, "api.anthropic.com")
	if gemini {
		if !strings.Contains(endpoint, ":generateContent") {
			endpoint += "/v1beta/models/" + cfg.Model + ":generateContent"
		}
		body, _ = json.Marshal(map[string]any{"contents": []any{map[string]any{"parts": []any{map[string]string{"text": prompt}}}}})
	} else if anthropic {
		if !strings.HasSuffix(endpoint, "/messages") {
			endpoint += "/v1/messages"
		}
		body, _ = json.Marshal(map[string]any{"model": cfg.Model, "max_tokens": 8192, "system": "你负责严谨分析安全扫描结果。", "messages": []map[string]string{{"role": "user", "content": prompt}}})
	} else if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/v1/chat/completions"
	}
	log("正在调用 AI 接口")
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if gemini {
		req.Header.Set("X-goog-api-key", cfg.Token)
	} else if anthropic {
		req.Header.Set("x-api-key", cfg.Token)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	req.Header.Set("Content-Type", "application/json")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.ProxyURL != "" {
		proxy, err := url.Parse(cfg.ProxyURL)
		if err != nil {
			return "", fmt.Errorf("代理地址无效: %v", err)
		}
		transport.Proxy = http.ProxyURL(proxy)
		log("已启用代理: " + cfg.ProxyURL)
	}
	client := &http.Client{Timeout: 10 * time.Minute, Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("AI 接口返回 HTTP %d", resp.StatusCode)
	}
	if gemini {
		var out struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return "", err
		}
		if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
			return "", fmt.Errorf("Gemini 接口未返回分析内容")
		}
		log("AI 接口返回分析结果")
		return out.Candidates[0].Content.Parts[0].Text, nil
	}
	if anthropic {
		var out struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return "", err
		}
		if len(out.Content) == 0 || out.Content[0].Text == "" {
			return "", fmt.Errorf("Anthropic 接口未返回分析内容")
		}
		log("AI 接口返回分析结果")
		return out.Content[0].Text, nil
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("AI 接口未返回分析内容")
	}
	log("AI 接口返回分析结果")
	return out.Choices[0].Message.Content, nil
}
