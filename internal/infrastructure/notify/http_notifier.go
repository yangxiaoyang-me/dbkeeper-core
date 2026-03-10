package notify

import (
	"context"
	"fmt"
	"strings"

	"dbkeeper-core/internal/config"
)

// HTTPNotifier 是基于 HTTP 协议的通知发送器。
// 支持配置多个通知渠道，每个渠道可以有独立的 URL、HTTP 方法和请求头。
// 使用场景：备份完成后向 webhook、监控系统或自定义 API 发送备份结果通知。
type HTTPNotifier struct {
	channels []config.NotifyChannel // 通知渠道列表
}

// NewHTTPNotifier 创建 HTTP 通知发送器。
// 主要逻辑：规范化渠道配置，如果没有有效渠道则返回 nil 禁用通知。
// 使用场景：工厂方法 NewNotifier() 内部调用。
func NewHTTPNotifier(cfg config.Notify) *HTTPNotifier {
	channels := normalizeChannels(cfg)
	if len(channels) == 0 {
		return nil
	}
	return &HTTPNotifier{channels: channels}
}

// Notify 向所有配置的渠道发送通知（实现 Notifier 接口）。
// 主要逻辑：遍历所有渠道依次发送，收集所有失败的错误信息。
// 使用场景：备份流程结束后调用，通知管理员备份结果。
func (n *HTTPNotifier) Notify(ctx context.Context, payload Payload) error {
	if n == nil || len(n.channels) == 0 {
		return nil
	}

	var errs []string
	for _, ch := range n.channels {
		if err := notifyOneChannel(ctx, ch, payload); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", ch.Name, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("notify failed: %s", strings.Join(errs, "; "))
	}
	return nil
}
