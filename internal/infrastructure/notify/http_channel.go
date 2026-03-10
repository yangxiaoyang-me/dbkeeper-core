package notify

import (
	"context"
	"encoding/json"
	"fmt"
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
	method := strings.ToUpper(strings.TrimSpace(ch.Method))
	if method == "" {
		method = http.MethodGet
	}

	for _, endpoint := range ch.URLs {
		reqURL := endpoint
		var body *strings.Reader

		if method == http.MethodGet {
			if strings.EqualFold(strings.TrimSpace(ch.ChannelType), "chuckfang") {
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
			body = strings.NewReader("")
		} else {
			data, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("marshal notify payload failed: %w", err)
			}
			body = strings.NewReader(string(data))
		}

		req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
		if err != nil {
			return fmt.Errorf("create notify request failed: %w", err)
		}
		if method != http.MethodGet {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range ch.Headers {
			req.Header.Set(k, v)
		}

		log.Printf("[notify] channel=%s method=%s url=%s", ch.Name, method, reqURL)
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[notify] channel=%s method=%s url=%s err=%v", ch.Name, method, reqURL, err)
			return fmt.Errorf("send notify failed: %w", err)
		}
		log.Printf("[notify] channel=%s method=%s url=%s status=%s", ch.Name, method, reqURL, resp.Status)
		_ = resp.Body.Close()
	}
	return nil
}
