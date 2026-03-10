// Package id 提供分布式唯一 ID 生成功能。
//
// 基于 Twitter Snowflake 算法实现，生成 64 位有序唯一 ID。
// ID 结构（从高位到低位）：
//   - 41 位：毫秒级时间戳（相对于自定义纪元 2024-01-01）
//   - 5 位：数据中心 ID（0~31）
//   - 5 位：机器 ID（0~31）
//   - 12 位：序列号（同一毫秒内最多 4096 个 ID）
//
// 使用场景：为备份记录和存储记录生成全局唯一的主键 ID。
package id

import (
	"fmt"
	"sync"
	"time"
)

// Snowflake 是雪花算法 ID 生成器（线程安全）。
// 使用场景：应用启动时创建一个实例，备份流程中每次需要主键时调用 NextIDString()。
type Snowflake struct {
	mu           sync.Mutex // 互斥锁，保证并发安全
	lastMillis   int64      // 上次生成 ID 的毫秒时间戳
	sequence     int64      // 当前毫秒内的序列号（0~4095）
	workerID     int64      // 机器 ID（0~31）
	datacenterID int64      // 数据中心 ID（0~31）
}

// 雪花算法的位分配常量。
const (
	timestampBits   = int64(41)                                  // 时间戳占用位数
	datacenterBits  = int64(5)                                   // 数据中心 ID 占用位数
	workerBits      = int64(5)                                   // 机器 ID 占用位数
	sequenceBits    = int64(12)                                  // 序列号占用位数
	maxDatacenterID = int64(-1) ^ (int64(-1) << datacenterBits)  // 最大数据中心 ID = 31
	maxWorkerID     = int64(-1) ^ (int64(-1) << workerBits)      // 最大机器 ID = 31
	sequenceMask    = int64(-1) ^ (int64(-1) << sequenceBits)    // 序列号掩码 = 4095
	workerShift     = sequenceBits                               // 机器 ID 左移位数 = 12
	datacenterShift = sequenceBits + workerBits                  // 数据中心 ID 左移位数 = 17
	timestampShift  = sequenceBits + workerBits + datacenterBits // 时间戳左移位数 = 22
)

// New 创建雪花 ID 生成器实例。
// 参数 datacenterID 范围 0~31，workerID 范围 0~31。
// 使用场景：应用启动时创建，通常使用默认值 (1, 1)。
func New(datacenterID, workerID int64) (*Snowflake, error) {
	if datacenterID < 0 || datacenterID > maxDatacenterID {
		return nil, fmt.Errorf("数据中心ID范围错误: %d", datacenterID)
	}
	if workerID < 0 || workerID > maxWorkerID {
		return nil, fmt.Errorf("机器ID范围错误: %d", workerID)
	}
	return &Snowflake{datacenterID: datacenterID, workerID: workerID}, nil
}

// NextIDString 生成下一个唯一 ID 并返回十进制字符串形式。
// 使用场景：插入元数据记录时生成主键，字符串形式便于日志展示和 JSON 序列化。
func (s *Snowflake) NextIDString() string {
	return fmt.Sprintf("%d", s.nextID())
}

// nextID 生成下一个 64 位雪花 ID（内部方法）。
// 主要逻辑：
//  1. 获取当前毫秒时间戳
//  2. 同一毫秒内递增序列号，序列号溢出时等待下一毫秒
//  3. 时钟回拨时使用上一毫秒时间戳（容错处理）
//  4. 组合时间戳、数据中心ID、机器ID、序列号生成最终ID
func (s *Snowflake) nextID() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	millis := time.Now().UnixMilli()
	if millis == s.lastMillis {
		s.sequence = (s.sequence + 1) & sequenceMask
		if s.sequence == 0 {
			for millis <= s.lastMillis {
				millis = time.Now().UnixMilli()
			}
		}
	} else {
		s.sequence = 0
	}
	if millis < s.lastMillis {
		// 时钟回拨时，强制使用上一毫秒并继续递增序列
		millis = s.lastMillis
	}
	s.lastMillis = millis

	return ((millis - customEpochMillis()) << timestampShift) |
		(s.datacenterID << datacenterShift) |
		(s.workerID << workerShift) |
		s.sequence
}

// customEpochMillis 返回自定义纪元时间（2024-01-01 00:00:00 UTC）的毫秒时间戳。
// 使用自定义纪元而非 Unix 纪元（1970-01-01），可以缩短时间戳位数，
// 延长 41 位时间戳的可用年限（约 69 年，即到 2093 年）。
func customEpochMillis() int64 {
	return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
}
