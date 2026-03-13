package notify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"dbkeeper-core/internal/config"
)

func buildDingTalkURL(endpoint, accessToken, secret string, now time.Time) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", fmt.Errorf("parse notify url failed: %w", err)
	}

	q := parsed.Query()
	if q.Get("access_token") == "" && strings.TrimSpace(accessToken) != "" {
		q.Set("access_token", strings.TrimSpace(accessToken))
	}

	secret = strings.TrimSpace(secret)
	if secret != "" {
		ts := strconv.FormatInt(now.UnixMilli(), 10)
		sig := signDingTalk(ts, secret)
		q.Set("timestamp", ts)
		q.Set("sign", sig)
	}

	parsed.RawQuery = q.Encode()
	return parsed.String(), nil
}

func signDingTalk(timestamp, secret string) string {
	src := []byte(timestamp + "\n" + secret)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(src)
	sum := mac.Sum(nil)
	return base64.StdEncoding.EncodeToString(sum)
}

func buildDingTalkBody(payload Payload, keyword string, at config.NotifyAt) ([]byte, error) {
	content := strings.TrimSpace(payload.Message)
	if content == "" {
		content = fmt.Sprintf("备份完成：成功%d，失败%d，总耗时 %.3f 秒。", payload.SuccessCount, payload.FailedCount, payload.TotalDurationS)
	}

	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		content += "\n关键字：" + keyword
	}

	cleanMobiles := make([]string, 0, len(at.AtMobiles))
	for _, mobile := range at.AtMobiles {
		mobile = strings.TrimSpace(mobile)
		if mobile == "" {
			continue
		}
		cleanMobiles = append(cleanMobiles, mobile)
	}
	if !at.IsAtAll && len(cleanMobiles) > 0 {
		mentions := make([]string, 0, len(cleanMobiles))
		for _, mobile := range cleanMobiles {
			mentions = append(mentions, "@"+mobile)
		}
		content += "\n" + strings.Join(mentions, " ")
	}

	msg := map[string]any{
		"msgtype": "text",
		"text": map[string]string{
			"content": content,
		},
		"at": map[string]any{
			"isAtAll": at.IsAtAll,
		},
	}
	if !at.IsAtAll && len(cleanMobiles) > 0 {
		msg["at"].(map[string]any)["atMobiles"] = cleanMobiles
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal dingtalk payload failed: %w", err)
	}
	return data, nil
}
