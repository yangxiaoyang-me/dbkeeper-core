package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dbkeeper-core/internal/config"
)

// notifyOneChannel 向单个通知渠道发送通知。
// 主要逻辑：
//   - GET 请求：将通知载荷编码到 URL 参数中（chuckfang 类型拼接到路径）
//   - POST 请求：将通知载荷序列化为 JSON 放入请求体
//   - 支持自定义 HTTP 头（如认证 Token）
//   - 遍历渠道中的所有 URL 依次发送
//
// 使用场景：HTTPNotifier.Notify() 内部调用，处理单个渠道的发送逻辑。
func notifyOneChannel(ctx context.Context, ch config.NotifyChannel, payload Payload) error {
	client := &http.Client{Timeout: time.Duration(ch.TimeoutMS) * time.Millisecond}
	channelType := strings.ToLower(strings.TrimSpace(ch.ChannelType))
	method := strings.ToUpper(strings.TrimSpace(ch.Method))
	if method == "" {
		method = defaultHTTPMethod(channelType)
	}
	if channelType == "dingtalk" {
		method = http.MethodPost
	}
	if channelType == "chuckfang" {
		method = http.MethodGet
	}

	endpoint := strings.TrimSpace(ch.URL)
	if endpoint == "" {
		return fmt.Errorf("notify url is empty")
	}
	reqURL := endpoint
	var body []byte

	if channelType == "dingtalk" {
		var err error
		reqURL, err = buildDingTalkURL(endpoint, ch.AccessToken, ch.Sign, time.Now())
		if err != nil {
			return err
		}
		body, err = buildDingTalkBody(payload, ch.Keyword)
		if err != nil {
			return err
		}
	} else if method == http.MethodGet {
		if channelType == "chuckfang" {
			u, err := buildChuckfangURL(endpoint, payload)
			if err != nil {
				return err
			}
			reqURL = u
		} else {
			parsed, err := url.Parse(endpoint)
			if err != nil {
				return fmt.Errorf("parse notify url failed: %w", err)
			}
			q := parsed.Query()
			appendPayloadQuery(q, payload)
			parsed.RawQuery = q.Encode()
			reqURL = parsed.String()
		}
	} else {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal notify payload failed: %w", err)
		}
		body = data
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create notify request failed: %w", err)
	}
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range ch.Headers {
		req.Header.Set(k, v)
	}

	log.Printf("[notify] channel=%s method=%s url=%s", ch.Name, method, maskNotifyURL(reqURL))
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[notify] channel=%s method=%s url=%s err=%v", ch.Name, method, maskNotifyURL(reqURL), err)
		return fmt.Errorf("send notify failed: %w", err)
	}
	respBody, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		log.Printf("[notify] channel=%s method=%s url=%s status=%s read_body_err=%v", ch.Name, method, maskNotifyURL(reqURL), resp.Status, readErr)
	} else {
		log.Printf("[notify] channel=%s method=%s url=%s status=%s resp_body=%s", ch.Name, method, maskNotifyURL(reqURL), resp.Status, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func maskNotifyURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	for _, key := range []string{"access_token", "sign"} {
		if q.Get(key) != "" {
			q.Set(key, "***")
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}
