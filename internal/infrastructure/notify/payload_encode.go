package notify

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// appendPayloadQuery 将通知载荷的关键字段追加到 URL 查询参数中。
// 编码字段包括：total_db、async_snapshots、success_count、failed_count、
// total_duration_s、success_items、failed_items、message、status。
// 使用场景：GET 方式发送通知时，将统计数据编码到 URL 参数中。
func appendPayloadQuery(q url.Values, payload Payload) {
	if payload.TotalDB > 0 {
		q.Set("total_db", strconv.Itoa(payload.TotalDB))
	}
	if payload.AsyncSnapshots > 0 {
		q.Set("async_snapshots", strconv.Itoa(payload.AsyncSnapshots))
	}
	q.Set("success_count", strconv.Itoa(payload.SuccessCount))
	q.Set("failed_count", strconv.Itoa(payload.FailedCount))
	if payload.TotalDurationS > 0 {
		q.Set("total_duration_s", strconv.FormatFloat(payload.TotalDurationS, 'f', 3, 64))
	}
	if len(payload.SuccessItems) > 0 {
		q.Set("success_items", strings.Join(payload.SuccessItems, ","))
	}
	if len(payload.FailedItems) > 0 {
		q.Set("failed_items", strings.Join(payload.FailedItems, ","))
	}
	if payload.Message != "" {
		q.Set("message", payload.Message)
	}
	if payload.Status != "" {
		q.Set("status", payload.Status)
	}
}

// buildChuckfangURL 构建 chuckfang 渠道类型的通知 URL。
// 主要逻辑：将通知消息直接拼接到 URL 路径末尾（不使用查询参数）。
// 如果消息为空，自动生成 "备份成功X，失败Y" 的默认消息。
// 使用场景：chuckfang 类型渠道的 GET 请求 URL 构建。
func buildChuckfangURL(endpoint string, payload Payload) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse notify url failed: %w", err)
	}

	content := payload.Message
	if strings.TrimSpace(content) == "" {
		content = fmt.Sprintf("备份成功%d，失败%d", payload.SuccessCount, payload.FailedCount)
	}
	parsed.Path = parsed.Path + content
	parsed.RawQuery = ""
	return parsed.String(), nil
}
