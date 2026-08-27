package minimax

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const reviewSystemPrompt = `你是应用商店评价截图审核员。只输出一个 JSON 对象，不要 markdown，不要其它文字。
字段：
pass: 布尔，是否同时满足全部规则
app_name: 截图中被评价的应用名称
rating: 数字 1-5，五星或五分记为 5
review_text: 评价正文（不含应用名）
negative: 布尔，评价是否负面（吐槽、差评、骂人、骗奖励等）
reason: 中文，不通过时的简短原因

必须同时满足才 pass=true：
1. 被评价应用是「轻羽云笔记」（允许轻微截断）
2. rating 为 5
3. review_text 字符数大于 15（标点、空格计入）
4. 评价可以无意义，但不能负面`

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// VisionVerdict MiniMax 视觉审核结果。
type VisionVerdict struct {
	Pass       bool   `json:"pass"`
	AppName    string `json:"app_name"`
	Rating     int    `json:"rating"`
	ReviewText string `json:"review_text"`
	Negative   bool   `json:"negative"`
	Reason     string `json:"reason"`
}

// ReviewAppStoreScreenshot 调用 MiniMax 视觉接口审核应用商店评价截图。
func ReviewAppStoreScreenshot(ctx context.Context, baseURL, apiKey, model, imageURLOrData, userHint string) (VisionVerdict, error) {
	var zero VisionVerdict
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	key := strings.TrimSpace(apiKey)
	model = strings.TrimSpace(model)
	if base == "" || key == "" || model == "" {
		return zero, fmt.Errorf("minimax_not_configured")
	}
	hint := strings.TrimSpace(userHint)
	if hint == "" {
		hint = "请审核这张应用商店评价截图。"
	}
	img := strings.TrimSpace(imageURLOrData)
	if img == "" {
		return zero, fmt.Errorf("empty_image")
	}
	body := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: reviewSystemPrompt},
			{Role: "user", Content: []contentPart{
				{Type: "text", Text: hint},
				{Type: "image_url", ImageURL: &imageURL{URL: img}},
			}},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return zero, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return zero, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	cli := &http.Client{Timeout: 90 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return zero, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, fmt.Errorf("minimax_http_%d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return zero, err
	}
	if len(parsed.Choices) == 0 {
		return zero, fmt.Errorf("minimax_empty_choices")
	}
	text, err := contentToText(parsed.Choices[0].Message.Content)
	if err != nil {
		return zero, err
	}
	v, err := ParseVisionVerdict(text)
	if err != nil {
		return zero, err
	}
	return v, nil
}

func contentToText(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", fmt.Errorf("empty_content")
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}
		return s, nil
	}
	var parts []contentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			b.WriteString(p.Text)
		}
		return b.String(), nil
	}
	return string(raw), nil
}

var jsonObjectRe = regexp.MustCompile(`\{[\s\S]*\}`)

// ParseVisionVerdict 从模型输出中提取 JSON 审核结果。
func ParseVisionVerdict(text string) (VisionVerdict, error) {
	var zero VisionVerdict
	s := strings.TrimSpace(text)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		m := jsonObjectRe.FindString(s)
		if m == "" {
			return zero, fmt.Errorf("no_json")
		}
		s = m
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return zero, err
	}
	v := VisionVerdict{
		Pass:       boolVal(raw["pass"]),
		AppName:    strings.TrimSpace(fmt.Sprint(raw["app_name"])),
		ReviewText: strings.TrimSpace(fmt.Sprint(raw["review_text"])),
		Negative:   boolVal(raw["negative"]),
		Reason:     strings.TrimSpace(fmt.Sprint(raw["reason"])),
		Rating:     intVal(raw["rating"]),
	}
	if v.AppName == "<nil>" {
		v.AppName = ""
	}
	if v.ReviewText == "<nil>" {
		v.ReviewText = ""
	}
	if v.Reason == "<nil>" {
		v.Reason = ""
	}
	return v, nil
}

func boolVal(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "true" || s == "1" || s == "yes"
	case float64:
		return t != 0
	default:
		return false
	}
}

func intVal(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

// ApplyHardRules 服务端硬规则：应用名、五星、字数、非负面。
func ApplyHardRules(v VisionVerdict) (ok bool, reason string) {
	name := strings.ReplaceAll(v.AppName, " ", "")
	name = strings.ReplaceAll(name, "\n", "")
	if !strings.Contains(name, "轻羽云笔记") {
		return false, "截图中评价的应用不是轻羽云笔记"
	}
	if v.Rating != 5 {
		return false, "评分必须是五星或五分"
	}
	if utf8.RuneCountInString(v.ReviewText) <= 15 {
		return false, "评价内容须超过 15 个字符"
	}
	if v.Negative {
		return false, "评价内容不能为负面"
	}
	if !v.Pass {
		if strings.TrimSpace(v.Reason) != "" {
			return false, v.Reason
		}
		return false, "截图未通过审核"
	}
	return true, ""
}
