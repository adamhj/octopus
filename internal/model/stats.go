package model

type StatsMetrics struct {
	InputToken     int64   `json:"input_token" gorm:"bigint"`
	OutputToken    int64   `json:"output_token" gorm:"bigint"`
	InputCost      float64 `json:"input_cost" gorm:"type:real"`
	OutputCost     float64 `json:"output_cost" gorm:"type:real"`
	WaitTime       int64   `json:"wait_time" gorm:"bigint"`
	RequestSuccess int64   `json:"request_success" gorm:"bigint"`
	RequestFailed  int64   `json:"request_failed" gorm:"bigint"`
}

type StatsTotal struct {
	ID int `gorm:"primaryKey"`
	StatsMetrics
}

type StatsHourly struct {
	Hour int    `json:"hour" gorm:"primaryKey"`
	Date string `json:"date" gorm:"not null"` // 记录最后更新日期，格式：20060102
	StatsMetrics
}

type StatsDaily struct {
	Date string `json:"date" gorm:"primaryKey"`
	StatsMetrics
}

type StatsModel struct {
	ID        int    `json:"id" gorm:"primaryKey"`
	Name      string `json:"name" gorm:"not null"`
	ChannelID int    `json:"channel_id" gorm:"not null"`
	StatsMetrics
}

type StatsChannel struct {
	ChannelID int `json:"channel_id" gorm:"primaryKey"`
	StatsMetrics
}

type StatsAPIKey struct {
	APIKeyID int `json:"api_key_id" gorm:"primaryKey"`
	StatsMetrics
}

// StatsDetail 明细统计，按日期+小时+渠道+上游实际模型名四个维度交叉聚合，
// 同时记录缓存读取 Token 分项数与 API 调用次数。只有在实际请求发生时才会写入对应组合行。
type StatsDetail struct {
	Date            string `json:"date" gorm:"primaryKey;size:8"`       // 日期，格式 20060102
	Hour            int    `json:"hour" gorm:"primaryKey"`              // 小时 0-23
	ChannelID       int    `json:"channel_id" gorm:"primaryKey"`        // 渠道 ID
	ActualModelName string `json:"actual_model_name" gorm:"primaryKey"` // 上游实际模型名
	StatsMetrics           // 嵌入 6 项基础指标
	CacheReadTokens int64  `json:"cache_read_tokens" gorm:"bigint"` // 缓存读取 Token 数
	APICallCount    int64  `json:"api_call_count" gorm:"bigint"`    // API 调用次数（每次 relay 计 1）
}

// StatsChartResult 图表统计查询的完整响应结构。
type StatsChartResult struct {
	Period  string             `json:"period"`
	Buckets []StatsChartBucket `json:"buckets"`
}

// StatsChartBucket 图表统计中的一个时间桶，包含该时间段内按渠道+模型聚合的槽位列表。
// 即使某时间段无数据，桶也会保留（slots 为空数组），确保横轴连续。
type StatsChartBucket struct {
	Time  string           `json:"time"`
	Slots []StatsChartSlot `json:"slots"`
}

// StatsChartSlot 时间桶内按 channel_id + actual_model_name 分组聚合后的统计槽位。
// 只有 input_token + output_token > 0 的组合才会出现在结果中。
type StatsChartSlot struct {
	ChannelID       int     `json:"channel_id"`
	ChannelName     string  `json:"channel_name"`
	ActualModelName string  `json:"actual_model_name"`
	InputToken      int64   `json:"input_token"`
	OutputToken     int64   `json:"output_token"`
	CacheReadTokens int64   `json:"cache_read_tokens"`
	APICallCount    int64   `json:"api_call_count"`
	InputCost       float64 `json:"input_cost"`
	OutputCost      float64 `json:"output_cost"`
}

// Add aggregates another StatsMetrics into the current one.
func (s *StatsMetrics) Add(delta StatsMetrics) {
	s.InputToken += delta.InputToken
	s.OutputToken += delta.OutputToken
	s.InputCost += delta.InputCost
	s.OutputCost += delta.OutputCost
	s.WaitTime += delta.WaitTime
	s.RequestSuccess += delta.RequestSuccess
	s.RequestFailed += delta.RequestFailed
}
