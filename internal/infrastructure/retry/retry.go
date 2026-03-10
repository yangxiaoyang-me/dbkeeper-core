// Package retry 提供指数退避重试功能。
//
// 当操作失败时，按指数递增的延迟间隔自动重试，直到成功或达到最大尝试次数。
// 延迟计算公式：delay = min(initialDelay * multiplier^(attempt-1), maxDelay)。
// 支持 context 取消，在等待延迟期间可响应 context 取消信号。
// 使用场景：远端存储上传和通知发送失败时自动重试，提高成功率。
package retry

import (
	"context"
	"fmt"
	"time"
)

// Config 是重试配置，控制重试行为的各项参数。
// 使用场景：在配置文件中通过 retry 节点设置，或使用 DefaultConfig() 获取默认值。
type Config struct {
	MaxAttempts  int           // 最大尝试次数（含首次执行，如设为 3 则最多重试 2 次）
	InitialDelay time.Duration // 首次重试的等待延迟
	MaxDelay     time.Duration // 延迟上限（防止等待时间过长）
	Multiplier   float64       // 延迟倍增系数（每次重试延迟乘以此系数）
}

// DefaultConfig 返回默认重试配置。
// 默认值：最多 3 次尝试，初始延迟 1 秒，最大延迟 10 秒，倍增系数 2.0。
// 使用场景：配置文件未指定重试参数时使用。
func DefaultConfig() Config {
	return Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
	}
}

// Do 执行带指数退避重试的操作。
// 主要逻辑：循环调用 fn，失败后按指数递增延迟等待后重试，
// 直到成功返回 nil 或达到最大尝试次数。
// 等待期间支持 context 取消（如收到 SIGTERM 信号）。
// 使用场景：包装远端存储上传和通知发送操作，提高网络不稳定时的成功率。
func Do(ctx context.Context, cfg Config, fn func() error) error {
	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err := fn(); err != nil {
			lastErr = err
			if attempt == cfg.MaxAttempts {
				return fmt.Errorf("failed after %d attempts: %w", cfg.MaxAttempts, lastErr)
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				delay = time.Duration(float64(delay) * cfg.Multiplier)
				if delay > cfg.MaxDelay {
					delay = cfg.MaxDelay
				}
			}
			continue
		}
		return nil
	}
	return lastErr
}
