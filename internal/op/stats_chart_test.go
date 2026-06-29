package op

import (
	"reflect"
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/model"
)

// channelNames 模拟从 Channel 表查出的渠道 ID → 名称映射。
type channelNames map[int]string

func (c channelNames) GetName(id int) string {
	if name, ok := c[id]; ok {
		return name
	}
	return ""
}

func TestBuildChartBuckets_PeriodHour(t *testing.T) {
	start := parseTime(t, "2026-01-15 08:00")
	end := parseTime(t, "2026-01-15 12:00")

	// 11:00 没有数据，应保留空桶
	rows := []model.StatsDetail{
		{Date: "20260115", Hour: 8, ChannelID: 1, ActualModelName: "gpt-4o", StatsMetrics: model.StatsMetrics{InputToken: 100, OutputToken: 50, InputCost: 0.01, OutputCost: 0.02}, CacheReadTokens: 10, APICallCount: 3},
		{Date: "20260115", Hour: 8, ChannelID: 2, ActualModelName: "gpt-3.5", StatsMetrics: model.StatsMetrics{InputToken: 200, OutputToken: 80, InputCost: 0.01, OutputCost: 0.01}, CacheReadTokens: 5, APICallCount: 2},
		{Date: "20260115", Hour: 10, ChannelID: 1, ActualModelName: "gpt-4o", StatsMetrics: model.StatsMetrics{InputToken: 50, OutputToken: 30, InputCost: 0.005, OutputCost: 0.01}, CacheReadTokens: 0, APICallCount: 1},
		// 8:00~8:59 的另一条数据
		{Date: "20260115", Hour: 8, ChannelID: 1, ActualModelName: "claude-3", StatsMetrics: model.StatsMetrics{InputToken: 300, OutputToken: 100, InputCost: 0.03, OutputCost: 0.05}, CacheReadTokens: 20, APICallCount: 5},
	}

	result := buildChartBuckets(rows, start, end, "hour", channelNames{1: "OpenAI", 2: "Azure"}.GetName)

	// 应有 5 个桶: 08, 09, 10, 11, 12
	if len(result.Buckets) != 5 {
		t.Fatalf("expected 5 buckets, got %d", len(result.Buckets))
	}
	if result.Period != "hour" {
		t.Errorf("expected period 'hour', got '%s'", result.Period)
	}

	// 验证 08:00 桶
	b08 := result.Buckets[0]
	if b08.Time != "2026-01-15T08" {
		t.Errorf("bucket 0 time: got %s, want 2026-01-15T08", b08.Time)
	}
	if len(b08.Slots) != 3 {
		t.Fatalf("bucket 08 should have 3 slots, got %d", len(b08.Slots))
	}

	// 验证 09:00 桶（空）
	b09 := result.Buckets[1]
	if b09.Time != "2026-01-15T09" {
		t.Errorf("bucket 1 time: got %s, want 2026-01-15T09", b09.Time)
	}
	if len(b09.Slots) != 0 {
		t.Errorf("bucket 09 should be empty, got %d slots", len(b09.Slots))
	}

	// 验证 10:00 桶
	b10 := result.Buckets[2]
	if len(b10.Slots) != 1 {
		t.Fatalf("bucket 10 should have 1 slot, got %d", len(b10.Slots))
	}
}

func TestBuildChartBuckets_PeriodDay(t *testing.T) {
	start := parseTime(t, "2026-01-15 12:00")
	end := parseTime(t, "2026-01-17 08:00")

	rows := []model.StatsDetail{
		// 15 日两条
		{Date: "20260115", Hour: 13, ChannelID: 1, ActualModelName: "gpt-4o", StatsMetrics: model.StatsMetrics{InputToken: 100, OutputToken: 50, InputCost: 0.01, OutputCost: 0.02}, CacheReadTokens: 10, APICallCount: 3},
		{Date: "20260115", Hour: 14, ChannelID: 1, ActualModelName: "gpt-4o", StatsMetrics: model.StatsMetrics{InputToken: 200, OutputToken: 100, InputCost: 0.02, OutputCost: 0.04}, CacheReadTokens: 15, APICallCount: 5},
		// 16 日
		{Date: "20260116", Hour: 0, ChannelID: 2, ActualModelName: "gpt-3.5", StatsMetrics: model.StatsMetrics{InputToken: 50, OutputToken: 20, InputCost: 0.005, OutputCost: 0.005}, CacheReadTokens: 0, APICallCount: 1},
	}

	result := buildChartBuckets(rows, start, end, "day", channelNames{1: "OpenAI", 2: "Azure"}.GetName)

	if len(result.Buckets) != 3 {
		t.Fatalf("expected 3 buckets (Jan 15, 16, 17), got %d", len(result.Buckets))
	}
	if result.Period != "day" {
		t.Errorf("expected period 'day'")
	}

	// 15 日桶：聚合了 13:00 和 14:00 两小时
	b15 := result.Buckets[0]
	if b15.Time != "2026-01-15" {
		t.Errorf("bucket 0 time: got %s", b15.Time)
	}
	if len(b15.Slots) != 1 {
		t.Fatalf("Jan 15 bucket should have 1 slot (gpt-4o), got %d", len(b15.Slots))
	}
	slot := b15.Slots[0]
	if slot.InputToken != 300 {
		t.Errorf("Jan 15 gpt-4o input_token: got %d, want 300", slot.InputToken)
	}
	if slot.OutputToken != 150 {
		t.Errorf("Jan 15 gpt-4o output_token: got %d, want 150", slot.OutputToken)
	}
}

func TestBuildChartBuckets_PeriodWeek(t *testing.T) {
	// 2026-01-15 是周四，所在周为 01-12(周一) ~ 01-18(周日)
	start := parseTime(t, "2026-01-15 12:00")
	end := parseTime(t, "2026-01-20 08:00")

	rows := []model.StatsDetail{
		{Date: "20260115", Hour: 13, ChannelID: 1, ActualModelName: "gpt-4o", StatsMetrics: model.StatsMetrics{InputToken: 100, OutputToken: 50, InputCost: 0.01, OutputCost: 0.02}, CacheReadTokens: 10, APICallCount: 1},
		{Date: "20260119", Hour: 0, ChannelID: 1, ActualModelName: "gpt-4o", StatsMetrics: model.StatsMetrics{InputToken: 200, OutputToken: 80, InputCost: 0.02, OutputCost: 0.03}, CacheReadTokens: 5, APICallCount: 2},
	}

	result := buildChartBuckets(rows, start, end, "week", channelNames{1: "OpenAI"}.GetName)

	// 2026-01-12(Mon) ~ 2026-01-18(Sun) 和 2026-01-19(Mon) ~ 2026-01-25(Sun)
	// end 在 01-20，所以只有 2 个桶
	if len(result.Buckets) != 2 {
		t.Fatalf("expected 2 week buckets, got %d", len(result.Buckets))
	}
	if result.Buckets[0].Time != "2026-01-12" {
		t.Errorf("week bucket 0: got %s, want 2026-01-12", result.Buckets[0].Time)
	}
	if result.Buckets[1].Time != "2026-01-19" {
		t.Errorf("week bucket 1: got %s, want 2026-01-19", result.Buckets[1].Time)
	}
}

func TestBuildChartBuckets_PeriodMonth(t *testing.T) {
	start := parseTime(t, "2026-01-15 12:00")
	end := parseTime(t, "2026-03-10 08:00")

	rows := []model.StatsDetail{
		{Date: "20260120", Hour: 0, ChannelID: 1, ActualModelName: "gpt-4o", StatsMetrics: model.StatsMetrics{InputToken: 500, OutputToken: 200, InputCost: 0.05, OutputCost: 0.08}, CacheReadTokens: 50, APICallCount: 10},
		{Date: "20260205", Hour: 0, ChannelID: 2, ActualModelName: "gpt-3.5", StatsMetrics: model.StatsMetrics{InputToken: 100, OutputToken: 40, InputCost: 0.01, OutputCost: 0.01}, CacheReadTokens: 0, APICallCount: 2},
	}

	result := buildChartBuckets(rows, start, end, "month", channelNames{1: "OpenAI", 2: "Azure"}.GetName)

	// Jan, Feb, Mar = 3 个桶
	if len(result.Buckets) != 3 {
		t.Fatalf("expected 3 month buckets, got %d", len(result.Buckets))
	}
	if result.Buckets[0].Time != "2026-01" {
		t.Errorf("month bucket 0: got %s", result.Buckets[0].Time)
	}
	if result.Buckets[1].Time != "2026-02" {
		t.Errorf("month bucket 1: got %s", result.Buckets[1].Time)
	}
	if result.Buckets[2].Time != "2026-03" {
		t.Errorf("month bucket 2: got %s", result.Buckets[2].Time)
	}
}

func TestBuildChartBuckets_SkipEmptySlots(t *testing.T) {
	start := parseTime(t, "2026-01-15 00:00")
	end := parseTime(t, "2026-01-16 00:00")

	// 一条全部为 0 的行（input_token + output_token = 0）不应出现在 slots 中
	rows := []model.StatsDetail{
		{Date: "20260115", Hour: 0, ChannelID: 1, ActualModelName: "gpt-4o", StatsMetrics: model.StatsMetrics{InputToken: 100, OutputToken: 0, InputCost: 0.01, OutputCost: 0}, CacheReadTokens: 0, APICallCount: 1},
		{Date: "20260115", Hour: 0, ChannelID: 2, ActualModelName: "gpt-3.5", StatsMetrics: model.StatsMetrics{InputToken: 0, OutputToken: 0, InputCost: 0, OutputCost: 0}, CacheReadTokens: 10, APICallCount: 2},
	}

	result := buildChartBuckets(rows, start, end, "day", channelNames{1: "OpenAI", 2: "Azure"}.GetName)

	if len(result.Buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(result.Buckets))
	}

	b15 := result.Buckets[0]
	// gpt-4o 有 100 input tokens → 保留；gpt-3.5 input+output=0 → 过滤掉
	if len(b15.Slots) != 1 {
		t.Fatalf("bucket 0 should have 1 slot (gpt-3.5 filtered), got %d", len(b15.Slots))
	}
	if b15.Slots[0].ChannelID != 1 {
		t.Errorf("expected channel 1, got %d", b15.Slots[0].ChannelID)
	}
}

func TestBuildChartBuckets_SlotSorting(t *testing.T) {
	start := parseTime(t, "2026-01-15 00:00")
	end := parseTime(t, "2026-01-16 00:00")

	rows := []model.StatsDetail{
		{Date: "20260115", Hour: 0, ChannelID: 3, ActualModelName: "aaa", StatsMetrics: model.StatsMetrics{InputToken: 1, OutputToken: 0, InputCost: 0, OutputCost: 0}, CacheReadTokens: 0, APICallCount: 1},
		{Date: "20260115", Hour: 0, ChannelID: 1, ActualModelName: "bbb", StatsMetrics: model.StatsMetrics{InputToken: 1, OutputToken: 0, InputCost: 0, OutputCost: 0}, CacheReadTokens: 0, APICallCount: 1},
		{Date: "20260115", Hour: 0, ChannelID: 1, ActualModelName: "aaa", StatsMetrics: model.StatsMetrics{InputToken: 1, OutputToken: 0, InputCost: 0, OutputCost: 0}, CacheReadTokens: 0, APICallCount: 1},
	}

	result := buildChartBuckets(rows, start, end, "day", channelNames{1: "C1", 3: "C3"}.GetName)

	b := result.Buckets[0]
	if len(b.Slots) != 3 {
		t.Fatalf("expected 3 slots, got %d", len(b.Slots))
	}
	// 期望顺序: channel_id=1, aaa; channel_id=1, bbb; channel_id=3, aaa
	expected := []struct {
		chID int
		name string
	}{
		{1, "aaa"},
		{1, "bbb"},
		{3, "aaa"},
	}
	for i, exp := range expected {
		if b.Slots[i].ChannelID != exp.chID || b.Slots[i].ActualModelName != exp.name {
			t.Errorf("slot %d: got channel=%d model=%s, want channel=%d model=%s",
				i, b.Slots[i].ChannelID, b.Slots[i].ActualModelName, exp.chID, exp.name)
		}
	}
}

func TestBuildChartBuckets_PartialFirstLastBucket(t *testing.T) {
	// 测试首尾不完整桶只含用户范围内的数据
	start := parseTime(t, "2026-01-15 10:00")
	end := parseTime(t, "2026-01-15 14:00")

	// 只有 10:00~14:00 之间的数据，范围外不应出现
	rows := []model.StatsDetail{
		{Date: "20260115", Hour: 9, ChannelID: 1, ActualModelName: "gpt-4o", StatsMetrics: model.StatsMetrics{InputToken: 999, OutputToken: 999, InputCost: 0, OutputCost: 0}, CacheReadTokens: 0, APICallCount: 1},
		{Date: "20260115", Hour: 10, ChannelID: 1, ActualModelName: "gpt-4o", StatsMetrics: model.StatsMetrics{InputToken: 100, OutputToken: 50, InputCost: 0.01, OutputCost: 0.02}, CacheReadTokens: 0, APICallCount: 1},
		{Date: "20260115", Hour: 14, ChannelID: 1, ActualModelName: "gpt-4o", StatsMetrics: model.StatsMetrics{InputToken: 200, OutputToken: 80, InputCost: 0.02, OutputCost: 0.03}, CacheReadTokens: 0, APICallCount: 1},
		{Date: "20260115", Hour: 15, ChannelID: 1, ActualModelName: "gpt-4o", StatsMetrics: model.StatsMetrics{InputToken: 999, OutputToken: 999, InputCost: 0, OutputCost: 0}, CacheReadTokens: 0, APICallCount: 1},
	}

	result := buildChartBuckets(rows, start, end, "hour", channelNames{1: "OpenAI"}.GetName)

	// 应该有 5 个桶: 10, 11, 12, 13, 14
	if len(result.Buckets) != 5 {
		t.Fatalf("expected 5 buckets (10-14), got %d", len(result.Buckets))
	}

	// 范围外数据（hour=9, hour=15）不应出现在任何桶中
	for _, b := range result.Buckets {
		for _, s := range b.Slots {
			if s.InputToken == 999 {
				t.Errorf("out-of-range data appeared in bucket %s", b.Time)
			}
		}
	}
}

func TestGenerateBucketTimes_Hour(t *testing.T) {
	start := parseTime(t, "2026-01-15 08:00")
	end := parseTime(t, "2026-01-15 12:00")
	buckets := generateBucketTimes(start, end, "hour")
	expected := []string{"2026-01-15T08", "2026-01-15T09", "2026-01-15T10", "2026-01-15T11", "2026-01-15T12"}
	if !reflect.DeepEqual(buckets, expected) {
		t.Errorf("got %v, want %v", buckets, expected)
	}
}

func TestGenerateBucketTimes_Day(t *testing.T) {
	start := parseTime(t, "2026-01-15 12:00")
	end := parseTime(t, "2026-01-17 08:00")
	buckets := generateBucketTimes(start, end, "day")
	expected := []string{"2026-01-15", "2026-01-16", "2026-01-17"}
	if !reflect.DeepEqual(buckets, expected) {
		t.Errorf("got %v, want %v", buckets, expected)
	}
}

func TestGenerateBucketTimes_Week(t *testing.T) {
	// 2026-01-15 is Thursday, week starts Monday 01-12
	start := parseTime(t, "2026-01-15 12:00")
	end := parseTime(t, "2026-01-22 08:00")
	buckets := generateBucketTimes(start, end, "week")
	expected := []string{"2026-01-12", "2026-01-19"}
	if !reflect.DeepEqual(buckets, expected) {
		t.Errorf("got %v, want %v", buckets, expected)
	}
}

func TestGenerateBucketTimes_Month(t *testing.T) {
	start := parseTime(t, "2026-01-15 12:00")
	end := parseTime(t, "2026-03-10 08:00")
	buckets := generateBucketTimes(start, end, "month")
	expected := []string{"2026-01", "2026-02", "2026-03"}
	if !reflect.DeepEqual(buckets, expected) {
		t.Errorf("got %v, want %v", buckets, expected)
	}
}

func TestGenerateBucketTimes_SameBucket(t *testing.T) {
	start := parseTime(t, "2026-01-15 08:00")
	end := parseTime(t, "2026-01-15 08:30")
	buckets := generateBucketTimes(start, end, "hour")
	expected := []string{"2026-01-15T08"}
	if !reflect.DeepEqual(buckets, expected) {
		t.Errorf("got %v, want %v", buckets, expected)
	}
}

func parseTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.ParseInLocation("2006-01-02 15:04", s, time.UTC)
	if err != nil {
		t.Fatalf("failed to parse time %q: %v", s, err)
	}
	return tm
}
