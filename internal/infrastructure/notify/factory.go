package notify

import (
	"fmt"
	"net/http"
	"strings"

	"dbkeeper-core/internal/config"
)

// NewNotifier 根据通知配置创建 Notifier 实例（工厂方法）。
// 主要逻辑：当配置类型为 http 或未指定时，创建 HTTPNotifier；
// 其他类型返回 nil 禁用通知。
// 使用场景：应用启动时根据配置初始化通知组件。
func NewNotifier(cfg config.Notify) Notifier {
	if strings.TrimSpace(cfg.Type) != "" && !strings.EqualFold(strings.TrimSpace(cfg.Type), "http") {
		return nil
	}
	return NewHTTPNotifier(cfg)
}

// normalizeChannels 将通知配置规范化为统一的渠道列表。
// 支持两种配置方式：
//  1. 多渠道模式（推荐）：通过 channels 数组配置多个通知目标
//  2. 单渠道模式（向后兼容）：通过顶层 urls/method/channel_type 配置
//
// 主要逻辑：设置默认 HTTP 方法（GET）、超时时间（5000ms）和渠道名称。
// 使用场景：内部使用，将用户配置统一为标准格式。
func normalizeChannels(cfg config.Notify) []config.NotifyChannel {
	// 优先使用显式的 channels 配置
	if len(cfg.Channels) > 0 {
		out := make([]config.NotifyChannel, 0, len(cfg.Channels))
		for i, ch := range cfg.Channels {
			chType := strings.TrimSpace(ch.Type)
			if chType == "" {
				chType = "http"
			}
			if !strings.EqualFold(chType, "http") {
				continue
			}
			ch.URL = strings.TrimSpace(ch.URL)
			if ch.URL == "" {
				continue
			}
			ch.Method = resolveHTTPMethod(ch.ChannelType, ch.Method)
			if ch.TimeoutMS <= 0 {
				ch.TimeoutMS = 5000
			}
			if strings.TrimSpace(ch.Name) == "" {
				ch.Name = fmt.Sprintf("channel-%d", i+1)
			}
			out = append(out, ch)
		}
		return out
	}

	cfg.URL = strings.TrimSpace(cfg.URL)
	if cfg.URL == "" {
		return nil
	}

	method := resolveHTTPMethod(cfg.ChannelType, cfg.Method)
	timeout := cfg.TimeoutMS
	if timeout <= 0 {
		timeout = 5000
	}

	return []config.NotifyChannel{
		{
			Name:        "default",
			URL:         cfg.URL,
			Method:      method,
			ChannelType: cfg.ChannelType,
			Keyword:     cfg.Keyword,
			At:          cfg.At,
			AccessToken: cfg.AccessToken,
			Sign:        cfg.Sign,
			Headers:     cfg.Headers,
			TimeoutMS:   timeout,
		},
	}
}

func defaultHTTPMethod(channelType string) string {
	if strings.EqualFold(strings.TrimSpace(channelType), "dingtalk") {
		return http.MethodPost
	}
	return http.MethodGet
}

func resolveHTTPMethod(channelType, configuredMethod string) string {
	t := strings.ToLower(strings.TrimSpace(channelType))
	switch t {
	case "dingtalk":
		return http.MethodPost
	case "chuckfang":
		return http.MethodGet
	default:
		m := strings.ToUpper(strings.TrimSpace(configuredMethod))
		if m == "" {
			return defaultHTTPMethod(channelType)
		}
		return m
	}
}
