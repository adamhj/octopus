package op

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/cache"
	"github.com/bestruirui/octopus/internal/utils/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var statsDailyCache model.StatsDaily
var statsDailyCacheLock sync.RWMutex

var statsTotalCache model.StatsTotal
var statsTotalCacheLock sync.RWMutex

var statsHourlyCache [24]model.StatsHourly
var statsHourlyCacheLock sync.RWMutex

var statsChannelCache = cache.New[int, model.StatsChannel](16)
var statsChannelCacheNeedUpdate = make(map[int]struct{})
var statsChannelCacheNeedUpdateLock sync.Mutex

var statsModelCache = cache.New[int, model.StatsModel](16)
var statsModelCacheNeedUpdate = make(map[int]struct{})
var statsModelCacheNeedUpdateLock sync.Mutex

var statsAPIKeyCache = cache.New[int, model.StatsAPIKey](16)
var statsAPIKeyCacheNeedUpdate = make(map[int]struct{})
var statsAPIKeyCacheNeedUpdateLock sync.Mutex

// statsDetailCache 明细统计内存缓存，key 格式为 "YYYYMMDD_HH_ChannelID_ActualModelName"。
// 只在有实际请求时才会创建对应组合行，不会产生全零行。
var statsDetailCache = make(map[string]*model.StatsDetail)
var statsDetailCacheLock sync.RWMutex

func StatsSaveDBTask() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	log.Debugf("stats save db task started")
	startTime := time.Now()
	defer func() {
		log.Debugf("stats save db task finished, save time: %s", time.Since(startTime))
	}()
	if err := StatsSaveDB(ctx); err != nil {
		log.Errorf("stats save db error: %v", err)
		return
	}
}

func StatsSaveDB(ctx context.Context) error {
	statsTotalCacheLock.RLock()
	totalSnap := statsTotalCache
	statsTotalCacheLock.RUnlock()
	if totalSnap.ID == 0 {
		totalSnap.ID = 1
	}

	statsDailyCacheLock.RLock()
	dailySnap := statsDailyCache
	statsDailyCacheLock.RUnlock()

	statsHourlyCacheLock.RLock()
	hourlyAll := statsHourlyCache
	statsHourlyCacheLock.RUnlock()

	statsChannelCacheNeedUpdateLock.Lock()
	channelIDs := make([]int, 0, len(statsChannelCacheNeedUpdate))
	for id := range statsChannelCacheNeedUpdate {
		channelIDs = append(channelIDs, id)
	}
	statsChannelCacheNeedUpdate = make(map[int]struct{})
	statsChannelCacheNeedUpdateLock.Unlock()

	statsModelCacheNeedUpdateLock.Lock()
	modelIDs := make([]int, 0, len(statsModelCacheNeedUpdate))
	for id := range statsModelCacheNeedUpdate {
		modelIDs = append(modelIDs, id)
	}
	statsModelCacheNeedUpdate = make(map[int]struct{})
	statsModelCacheNeedUpdateLock.Unlock()

	statsAPIKeyCacheNeedUpdateLock.Lock()
	apiKeyIDs := make([]int, 0, len(statsAPIKeyCacheNeedUpdate))
	for id := range statsAPIKeyCacheNeedUpdate {
		apiKeyIDs = append(apiKeyIDs, id)
	}
	statsAPIKeyCacheNeedUpdate = make(map[int]struct{})
	statsAPIKeyCacheNeedUpdateLock.Unlock()

	statsDetailCacheLock.Lock()
	statsDetailSnap := make([]model.StatsDetail, 0, len(statsDetailCache))
	for _, v := range statsDetailCache {
		statsDetailSnap = append(statsDetailSnap, *v)
	}
	statsDetailCache = make(map[string]*model.StatsDetail)
	statsDetailCacheLock.Unlock()

	return persistStatsSnapshots(ctx, totalSnap, dailySnap, hourlyAll, channelIDs, modelIDs, apiKeyIDs, statsDetailSnap)
}

func persistStatsSnapshots(
	ctx context.Context,
	totalSnap model.StatsTotal,
	dailySnap model.StatsDaily,
	hourlyAll [24]model.StatsHourly,
	channelIDs []int,
	modelIDs []int,
	apiKeyIDs []int,
	statsDetailSnap []model.StatsDetail,
) error {
	dbConn := db.GetDB().WithContext(ctx)

	if result := dbConn.Save(&totalSnap); result.Error != nil {
		return result.Error
	}
	if result := dbConn.Save(&dailySnap); result.Error != nil {
		return result.Error
	}

	todayDate := time.Now().Format("20060102")
	hourlyStats := make([]model.StatsHourly, 0, 24)
	for hour := 0; hour < 24; hour++ {
		if hourlyAll[hour].Date == todayDate {
			hourlyStats = append(hourlyStats, hourlyAll[hour])
		}
	}
	if len(hourlyStats) > 0 {
		if result := dbConn.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "hour"}},
			UpdateAll: true,
		}).Create(&hourlyStats); result.Error != nil {
			return result.Error
		}
	}

	for _, id := range channelIDs {
		ch, ok := statsChannelCache.Get(id)
		if !ok {
			continue
		}
		if result := dbConn.Save(&ch); result.Error != nil {
			return result.Error
		}
	}

	for _, id := range modelIDs {
		m, ok := statsModelCache.Get(id)
		if !ok {
			continue
		}
		if result := dbConn.Save(&m); result.Error != nil {
			return result.Error
		}
	}

	for _, id := range apiKeyIDs {
		ak, ok := statsAPIKeyCache.Get(id)
		if !ok {
			continue
		}
		if result := dbConn.Save(&ak); result.Error != nil {
			return result.Error
		}
	}

	// 明细统计：按复合主键 UPSERT，累加已有行
	if len(statsDetailSnap) > 0 {
		if result := dbConn.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "date"},
				{Name: "hour"},
				{Name: "channel_id"},
				{Name: "actual_model_name"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"input_token", "output_token", "input_cost", "output_cost",
				"wait_time", "request_success", "request_failed",
				"cache_read_tokens", "api_call_count",
			}),
		}).Create(&statsDetailSnap); result.Error != nil {
			return result.Error
		}
	}

	return nil
}

func statsSaveDBWithDailyOverride(ctx context.Context, dailyOverride model.StatsDaily) error {
	statsTotalCacheLock.RLock()
	totalSnap := statsTotalCache
	statsTotalCacheLock.RUnlock()
	if totalSnap.ID == 0 {
		totalSnap.ID = 1
	}

	statsHourlyCacheLock.RLock()
	hourlyAll := statsHourlyCache
	statsHourlyCacheLock.RUnlock()

	statsChannelCacheNeedUpdateLock.Lock()
	channelIDs := make([]int, 0, len(statsChannelCacheNeedUpdate))
	for id := range statsChannelCacheNeedUpdate {
		channelIDs = append(channelIDs, id)
	}
	statsChannelCacheNeedUpdate = make(map[int]struct{})
	statsChannelCacheNeedUpdateLock.Unlock()

	statsModelCacheNeedUpdateLock.Lock()
	modelIDs := make([]int, 0, len(statsModelCacheNeedUpdate))
	for id := range statsModelCacheNeedUpdate {
		modelIDs = append(modelIDs, id)
	}
	statsModelCacheNeedUpdate = make(map[int]struct{})
	statsModelCacheNeedUpdateLock.Unlock()

	statsAPIKeyCacheNeedUpdateLock.Lock()
	apiKeyIDs := make([]int, 0, len(statsAPIKeyCacheNeedUpdate))
	for id := range statsAPIKeyCacheNeedUpdate {
		apiKeyIDs = append(apiKeyIDs, id)
	}
	statsAPIKeyCacheNeedUpdate = make(map[int]struct{})
	statsAPIKeyCacheNeedUpdateLock.Unlock()

	statsDetailCacheLock.Lock()
	statsDetailSnap := make([]model.StatsDetail, 0, len(statsDetailCache))
	for _, v := range statsDetailCache {
		statsDetailSnap = append(statsDetailSnap, *v)
	}
	statsDetailCache = make(map[string]*model.StatsDetail)
	statsDetailCacheLock.Unlock()

	return persistStatsSnapshots(ctx, totalSnap, dailyOverride, hourlyAll, channelIDs, modelIDs, apiKeyIDs, statsDetailSnap)
}

func StatsDailyUpdate(ctx context.Context, metrics model.StatsMetrics) error {
	today := time.Now().Format("20060102")

	statsDailyCacheLock.Lock()
	if statsDailyCache.Date == today {
		statsDailyCache.StatsMetrics.Add(metrics)
		statsDailyCacheLock.Unlock()
		return nil
	}

	prevDaily := statsDailyCache
	statsDailyCache = model.StatsDaily{Date: today}
	statsDailyCache.StatsMetrics.Add(metrics)
	statsDailyCacheLock.Unlock()

	return statsSaveDBWithDailyOverride(ctx, prevDaily)
}

func StatsTotalUpdate(metrics model.StatsMetrics) error {
	statsTotalCacheLock.Lock()
	defer statsTotalCacheLock.Unlock()
	if statsTotalCache.ID == 0 {
		statsTotalCache.ID = 1
	}
	statsTotalCache.StatsMetrics.Add(metrics)
	return nil
}

func StatsChannelUpdate(channelID int, metrics model.StatsMetrics) error {
	channelCache, ok := statsChannelCache.Get(channelID)
	if !ok {
		channelCache = model.StatsChannel{
			ChannelID: channelID,
		}
	}
	channelCache.StatsMetrics.Add(metrics)
	statsChannelCache.Set(channelID, channelCache)
	statsChannelCacheNeedUpdateLock.Lock()
	statsChannelCacheNeedUpdate[channelID] = struct{}{}
	statsChannelCacheNeedUpdateLock.Unlock()
	return nil
}

func StatsHourlyUpdate(metrics model.StatsMetrics) error {
	now := time.Now()
	nowHour := now.Hour()
	todayDate := time.Now().Format("20060102")

	statsHourlyCacheLock.Lock()
	defer statsHourlyCacheLock.Unlock()

	if statsHourlyCache[nowHour].Date != todayDate {
		statsHourlyCache[nowHour] = model.StatsHourly{
			Hour: nowHour,
			Date: todayDate,
		}
	}

	statsHourlyCache[nowHour].StatsMetrics.Add(metrics)
	return nil
}

func StatsModelUpdate(stats model.StatsModel) error {
	modelCache, ok := statsModelCache.Get(stats.ID)
	if !ok {
		modelCache = model.StatsModel{
			ID: stats.ID,
		}
	}
	modelCache.StatsMetrics.Add(stats.StatsMetrics)
	statsModelCache.Set(stats.ID, modelCache)
	statsModelCacheNeedUpdateLock.Lock()
	statsModelCacheNeedUpdate[stats.ID] = struct{}{}
	statsModelCacheNeedUpdateLock.Unlock()
	return nil
}

func StatsAPIKeyUpdate(apiKeyID int, metrics model.StatsMetrics) error {
	apiKeyCache, ok := statsAPIKeyCache.Get(apiKeyID)
	if !ok {
		apiKeyCache = model.StatsAPIKey{
			APIKeyID: apiKeyID,
		}
	}
	apiKeyCache.StatsMetrics.Add(metrics)
	statsAPIKeyCache.Set(apiKeyID, apiKeyCache)
	statsAPIKeyCacheNeedUpdateLock.Lock()
	statsAPIKeyCacheNeedUpdate[apiKeyID] = struct{}{}
	statsAPIKeyCacheNeedUpdateLock.Unlock()
	return nil
}

// StatsDetailUpdate 更新明细统计缓存。按日期+小时+渠道+上游实际模型名定位唯一组合行，
// 累加基础指标、缓存读取 Token 数与 API 调用次数。只有在实际请求发生时才会创建对应组合行。
func StatsDetailUpdate(date string, hour int, channelID int, actualModelName string, metrics model.StatsMetrics, cacheRead int64) error {
	key := date + "_" + strconv.Itoa(hour) + "_" + strconv.Itoa(channelID) + "_" + actualModelName

	statsDetailCacheLock.Lock()
	defer statsDetailCacheLock.Unlock()

	detail, ok := statsDetailCache[key]
	if !ok {
		detail = &model.StatsDetail{
			Date:            date,
			Hour:            hour,
			ChannelID:       channelID,
			ActualModelName: actualModelName,
		}
		statsDetailCache[key] = detail
	}
	detail.StatsMetrics.Add(metrics)
	detail.CacheReadTokens += cacheRead
	detail.APICallCount++
	return nil
}

// StatsDetailQuery 按任意维度组合查询明细统计。传入空值表示该维度不参与过滤。
func StatsDetailQuery(ctx context.Context, date string, hour *int, channelID *int, actualModelName string) ([]model.StatsDetail, error) {
	query := db.GetDB().WithContext(ctx).Model(&model.StatsDetail{})
	if date != "" {
		query = query.Where("date = ?", date)
	}
	if hour != nil {
		query = query.Where("hour = ?", *hour)
	}
	if channelID != nil {
		query = query.Where("channel_id = ?", *channelID)
	}
	if actualModelName != "" {
		query = query.Where("actual_model_name = ?", actualModelName)
	}
	query = query.Order("date ASC, hour ASC, channel_id ASC, actual_model_name ASC")

	var result []model.StatsDetail
	if err := query.Find(&result).Error; err != nil {
		return nil, err
	}
	return result, nil
}

func StatsChannelDel(id int) error {
	if _, ok := statsChannelCache.Get(id); !ok {
		return nil
	}
	statsChannelCache.Del(id)
	statsChannelCacheNeedUpdateLock.Lock()
	delete(statsChannelCacheNeedUpdate, id)
	statsChannelCacheNeedUpdateLock.Unlock()
	return db.GetDB().Delete(&model.StatsChannel{}, id).Error
}

func StatsAPIKeyDel(id int) error {
	if _, ok := statsAPIKeyCache.Get(id); !ok {
		return nil
	}
	statsAPIKeyCache.Del(id)
	statsAPIKeyCacheNeedUpdateLock.Lock()
	delete(statsAPIKeyCacheNeedUpdate, id)
	statsAPIKeyCacheNeedUpdateLock.Unlock()
	return db.GetDB().Delete(&model.StatsAPIKey{}, id).Error
}

func StatsTotalGet() model.StatsTotal {
	statsTotalCacheLock.RLock()
	defer statsTotalCacheLock.RUnlock()
	return statsTotalCache
}

func StatsTodayGet() model.StatsDaily {
	statsDailyCacheLock.RLock()
	defer statsDailyCacheLock.RUnlock()
	return statsDailyCache
}

func StatsChannelGet(id int) model.StatsChannel {
	stats, ok := statsChannelCache.Get(id)
	if !ok {
		tmp := model.StatsChannel{
			ChannelID: id,
		}
		statsChannelCache.Set(id, tmp)
		statsChannelCacheNeedUpdateLock.Lock()
		statsChannelCacheNeedUpdate[id] = struct{}{}
		statsChannelCacheNeedUpdateLock.Unlock()
		return tmp
	}
	return stats
}

func StatsAPIKeyGet(id int) model.StatsAPIKey {
	stats, ok := statsAPIKeyCache.Get(id)
	if !ok {
		tmp := model.StatsAPIKey{
			APIKeyID: id,
		}
		statsAPIKeyCache.Set(id, tmp)
		statsAPIKeyCacheNeedUpdateLock.Lock()
		statsAPIKeyCacheNeedUpdate[id] = struct{}{}
		statsAPIKeyCacheNeedUpdateLock.Unlock()
		return tmp
	}
	return stats
}

func StatsAPIKeyList() []model.StatsAPIKey {
	apiKeys := make([]model.StatsAPIKey, 0, statsAPIKeyCache.Len())
	for _, v := range statsAPIKeyCache.GetAll() {
		apiKeys = append(apiKeys, v)
	}
	return apiKeys
}

func StatsHourlyGet() []model.StatsHourly {
	now := time.Now()
	currentHour := now.Hour()
	todayDate := time.Now().Format("20060102")

	statsHourlyCacheLock.RLock()
	defer statsHourlyCacheLock.RUnlock()

	result := make([]model.StatsHourly, 0, currentHour+1)

	for hour := 0; hour <= currentHour; hour++ {
		if statsHourlyCache[hour].Date == todayDate {
			result = append(result, statsHourlyCache[hour])
		} else {
			result = append(result, model.StatsHourly{
				Hour: hour,
				Date: todayDate,
			})
		}
	}

	return result
}

func StatsGetDaily(ctx context.Context) ([]model.StatsDaily, error) {
	var statsDaily []model.StatsDaily
	result := db.GetDB().WithContext(ctx).Find(&statsDaily)
	if result.Error != nil {
		return nil, result.Error
	}
	return statsDaily, nil
}

func statsRefreshCache(ctx context.Context) error {
	dbConn := db.GetDB().WithContext(ctx)
	today := time.Now().Format("20060102")

	var loadedDaily model.StatsDaily
	result := dbConn.Last(&loadedDaily)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to get daily stats: %v", result.Error)
	}
	if result.RowsAffected == 0 || loadedDaily.Date != today {
		loadedDaily = model.StatsDaily{Date: today}
	}

	var loadedTotal model.StatsTotal
	result = dbConn.First(&loadedTotal)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to get total stats: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		loadedTotal = model.StatsTotal{ID: 1}
	} else if loadedTotal.ID == 0 {
		loadedTotal.ID = 1
	}

	var loadedChannels []model.StatsChannel
	result = dbConn.Find(&loadedChannels)
	if result.Error != nil {
		return fmt.Errorf("failed to get channels: %v", result.Error)
	}

	var loadedHourly []model.StatsHourly
	result = dbConn.Find(&loadedHourly)
	if result.Error != nil {
		return fmt.Errorf("failed to get hourly stats: %v", result.Error)
	}

	statsDailyCacheLock.Lock()
	statsDailyCache = loadedDaily
	statsDailyCacheLock.Unlock()

	statsTotalCacheLock.Lock()
	statsTotalCache = loadedTotal
	statsTotalCacheLock.Unlock()

	statsChannelCache.Clear()
	statsChannelCacheNeedUpdateLock.Lock()
	statsChannelCacheNeedUpdate = make(map[int]struct{})
	statsChannelCacheNeedUpdateLock.Unlock()
	for _, v := range loadedChannels {
		statsChannelCache.Set(v.ChannelID, v)
	}

	var loadedAPIKeys []model.StatsAPIKey
	result = dbConn.Find(&loadedAPIKeys)
	if result.Error != nil {
		return fmt.Errorf("failed to get api key stats: %v", result.Error)
	}

	statsAPIKeyCache.Clear()
	statsAPIKeyCacheNeedUpdateLock.Lock()
	statsAPIKeyCacheNeedUpdate = make(map[int]struct{})
	statsAPIKeyCacheNeedUpdateLock.Unlock()
	for _, v := range loadedAPIKeys {
		statsAPIKeyCache.Set(v.APIKeyID, v)
	}

	statsHourlyCacheLock.Lock()
	statsHourlyCache = [24]model.StatsHourly{}
	for _, v := range loadedHourly {
		if v.Hour >= 0 && v.Hour < 24 {
			statsHourlyCache[v.Hour] = v
		}
	}
	statsHourlyCacheLock.Unlock()

	// 加载今日明细统计到内存缓存，避免重启后当日数据丢失累加
	var loadedDetail []model.StatsDetail
	result = dbConn.Where("date = ?", today).Find(&loadedDetail)
	if result.Error != nil {
		return fmt.Errorf("failed to get stats_details: %v", result.Error)
	}
	statsDetailCacheLock.Lock()
	statsDetailCache = make(map[string]*model.StatsDetail, len(loadedDetail))
	for i := range loadedDetail {
		d := loadedDetail[i]
		key := d.Date + "_" + strconv.Itoa(d.Hour) + "_" + strconv.Itoa(d.ChannelID) + "_" + d.ActualModelName
		statsDetailCache[key] = &d
	}
	statsDetailCacheLock.Unlock()

	return nil
}

// -------------------- Chart API --------------------

// periodBucketFunc 为每种周期定义：如何将 StatsDetail 行映射为桶键、如何格式化桶键为展示字符串、
// 以及如何生成从 start 到 end 的所有桶键。
type periodBucketFunc struct {
	// rowKey 从一行明细数据生成桶键
	rowKey func(d model.StatsDetail) string
	// bucketLabel 将桶键转为前端可展示的时间字符串
	bucketLabel func(bucketKey string) string
	// generateKeys 生成从 start 到 end（含）的所有桶键，按时间顺序排列
	generateKeys func(start, end time.Time) []string
	// bucketTime 将桶键解析回 time.Time，用于范围过滤判断
	bucketTime func(bucketKey string) time.Time
}

// getPeriodFunc 根据周期名返回对应的桶处理函数集。
// 返回 nil 表示不支持的周期。
func getPeriodFunc(period string) *periodBucketFunc {
	switch period {
	case "hour":
		return &periodBucketFunc{
			rowKey: func(d model.StatsDetail) string {
				return d.Date + "_" + fmt.Sprintf("%02d", d.Hour)
			},
			bucketLabel: func(key string) string {
				// key: "20260115_08" → "2026-01-15T08"
				return key[:4] + "-" + key[4:6] + "-" + key[6:8] + "T" + key[9:]
			},
			generateKeys: generateHourKeys,
			bucketTime: func(key string) time.Time {
				t, _ := time.ParseInLocation("20060102_15", key, time.UTC)
				return t
			},
		}
	case "day":
		return &periodBucketFunc{
			rowKey: func(d model.StatsDetail) string {
				return d.Date
			},
			bucketLabel: func(key string) string {
				// key: "20260115" → "2026-01-15"
				return key[:4] + "-" + key[4:6] + "-" + key[6:8]
			},
			generateKeys: generateDayKeys,
			bucketTime: func(key string) time.Time {
				t, _ := time.ParseInLocation("20060102", key, time.UTC)
				return t
			},
		}
	case "week":
		return &periodBucketFunc{
			rowKey: func(d model.StatsDetail) string {
				t, _ := time.ParseInLocation("20060102", d.Date, time.UTC)
				return weekMonday(t).Format("20060102")
			},
			bucketLabel: func(key string) string {
				return key[:4] + "-" + key[4:6] + "-" + key[6:8]
			},
			generateKeys: generateWeekKeys,
			bucketTime: func(key string) time.Time {
				t, _ := time.ParseInLocation("20060102", key, time.UTC)
				return t
			},
		}
	case "month":
		return &periodBucketFunc{
			rowKey: func(d model.StatsDetail) string {
				return d.Date[:6]
			},
			bucketLabel: func(key string) string {
				// key: "202601" → "2026-01"
				return key[:4] + "-" + key[4:6]
			},
			generateKeys: generateMonthKeys,
			bucketTime: func(key string) time.Time {
				t, _ := time.ParseInLocation("200601", key, time.UTC)
				return t
			},
		}
	}
	return nil
}

// weekMonday 返回 t 所在周的周一 00:00 UTC。
func weekMonday(t time.Time) time.Time {
	weekday := t.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	offset := int(weekday) - int(time.Monday)
	monday := t.AddDate(0, 0, -offset)
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
}

// generateHourKeys 生成从 start 到 end（含）之间所有整点小时的桶键。
func generateHourKeys(start, end time.Time) []string {
	startHour := time.Date(start.Year(), start.Month(), start.Day(), start.Hour(), 0, 0, 0, time.UTC)
	endHour := time.Date(end.Year(), end.Month(), end.Day(), end.Hour(), 0, 0, 0, time.UTC)
	var keys []string
	for t := startHour; !t.After(endHour); t = t.Add(time.Hour) {
		keys = append(keys, t.Format("20060102_15"))
	}
	return keys
}

// generateDayKeys 生成从 start 到 end（含）之间所有自然日的桶键。
func generateDayKeys(start, end time.Time) []string {
	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	endDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	var keys []string
	for t := startDay; !t.After(endDay); t = t.AddDate(0, 0, 1) {
		keys = append(keys, t.Format("20060102"))
	}
	return keys
}

// generateWeekKeys 生成从 start 所在周的周一到 end 所在周的周一（含）的所有周桶键。
func generateWeekKeys(start, end time.Time) []string {
	startMonday := weekMonday(start)
	endMonday := weekMonday(end)
	var keys []string
	for t := startMonday; !t.After(endMonday); t = t.AddDate(0, 0, 7) {
		keys = append(keys, t.Format("20060102"))
	}
	return keys
}

// generateMonthKeys 生成从 start 所在月到 end 所在月（含）的所有月桶键。
func generateMonthKeys(start, end time.Time) []string {
	startMonth := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	endMonth := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
	var keys []string
	for t := startMonth; !t.After(endMonth); t = t.AddDate(0, 1, 0) {
		keys = append(keys, t.Format("200601"))
	}
	return keys
}

// generateBucketTimes 生成从 start 到 end（含）之间的桶时间标签列表（给测试用）。
// start 和 end 的小时已对齐到 HH:00（由 handler 层保证），分钟/秒为零。
func generateBucketTimes(start, end time.Time, period string) []string {
	pf := getPeriodFunc(period)
	if pf == nil {
		return nil
	}
	keys := pf.generateKeys(start, end)
	labels := make([]string, len(keys))
	for i, k := range keys {
		labels[i] = pf.bucketLabel(k)
	}
	return labels
}

// slotKey 用于在聚合时区分同一桶内不同 (channel_id, actual_model_name) 组合。
type slotKey struct {
	ChannelID       int
	ActualModelName string
}

// buildChartBuckets 纯函数：根据给定条件从明细行构建图表响应。
// rows 应为按 start~end 过滤后的数据，不包含范围外数据。
// getChannelName 用于从渠道 ID 查找渠道名称。
func buildChartBuckets(
	rows []model.StatsDetail,
	start, end time.Time,
	period string,
	getChannelName func(channelID int) string,
) model.StatsChartResult {
	pf := getPeriodFunc(period)
	if pf == nil {
		return model.StatsChartResult{Period: period, Buckets: []model.StatsChartBucket{}}
	}

	// 1. 将行映射到桶键
	type bucketData struct {
		slots    map[slotKey]*model.StatsChartSlot
		slotList []*model.StatsChartSlot // 用于保持插入顺序
	}
	bucketMap := make(map[string]*bucketData)

	for _, row := range rows {
		bk := pf.rowKey(row)
		bd, ok := bucketMap[bk]
		if !ok {
			bd = &bucketData{
				slots:    make(map[slotKey]*model.StatsChartSlot),
				slotList: make([]*model.StatsChartSlot, 0),
			}
			bucketMap[bk] = bd
		}

		sk := slotKey{ChannelID: row.ChannelID, ActualModelName: row.ActualModelName}
		slot, ok := bd.slots[sk]
		if !ok {
			slot = &model.StatsChartSlot{
				ChannelID:       row.ChannelID,
				ActualModelName: row.ActualModelName,
				ChannelName:     getChannelName(row.ChannelID),
			}
			bd.slots[sk] = slot
			bd.slotList = append(bd.slotList, slot)
		}

		slot.InputToken += row.InputToken
		slot.OutputToken += row.OutputToken
		slot.CacheReadTokens += row.CacheReadTokens
		slot.APICallCount += row.APICallCount
		slot.InputCost += row.InputCost
		slot.OutputCost += row.OutputCost
	}

	// 2. 构建最终 buckets（保留空桶）
	bucketKeys := pf.generateKeys(start, end)
	buckets := make([]model.StatsChartBucket, 0, len(bucketKeys))

	for _, bk := range bucketKeys {
		slots := make([]model.StatsChartSlot, 0)
		bd := bucketMap[bk]
		if bd != nil {
			// 过滤空槽位（input + output = 0）
			for _, s := range bd.slotList {
				if s.InputToken > 0 || s.OutputToken > 0 {
					slots = append(slots, *s)
				}
			}
			// 按 channel_id + actual_model_name 排序
			sortSlots(slots)
		}

		buckets = append(buckets, model.StatsChartBucket{
			Time:  pf.bucketLabel(bk),
			Slots: slots,
		})
	}

	return model.StatsChartResult{
		Period:  period,
		Buckets: buckets,
	}
}

// sortSlots 对槽位列表按 channel_id ASC, actual_model_name ASC 排序。
func sortSlots(slots []model.StatsChartSlot) {
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].ChannelID != slots[j].ChannelID {
			return slots[i].ChannelID < slots[j].ChannelID
		}
		return slots[i].ActualModelName < slots[j].ActualModelName
	})
}

// StatsDetailChartQuery 查询图表统计数据。从 StatsDetail 表按时间范围+可选过滤条件读取原始行，
// 然后按指定的聚合周期生成分桶图表响应。
func StatsDetailChartQuery(
	ctx context.Context,
	startTime, endTime time.Time,
	channelID *int,
	modelName string,
	period string,
) (model.StatsChartResult, error) {
	// 校验周期
	if getPeriodFunc(period) == nil {
		return model.StatsChartResult{}, fmt.Errorf("unsupported period: %s", period)
	}

	// 构造查询
	startDate := startTime.Format("20060102")
	endDate := endTime.Format("20060102")
	startHour := startTime.Hour()
	endHour := endTime.Hour()

	query := db.GetDB().WithContext(ctx).Model(&model.StatsDetail{})

	// 时间范围过滤：
	// 跨天时：date >= startDate AND date <= endDate，首尾天不额外限制小时
	// 同一天时：date = startDate AND hour BETWEEN startHour AND endHour
	if startDate == endDate {
		query = query.Where("date = ? AND hour >= ? AND hour <= ?", startDate, startHour, endHour)
	} else {
		// 跨天：使用 OR 条件精确限定首尾天的小时范围，中间天全包含
		query = query.Where(
			"(date = ? AND hour >= ?) OR (date > ? AND date < ?) OR (date = ? AND hour <= ?)",
			startDate, startHour,
			startDate, endDate,
			endDate, endHour,
		)
	}

	if channelID != nil {
		query = query.Where("channel_id = ?", *channelID)
	}
	if modelName != "" {
		query = query.Where("actual_model_name = ?", modelName)
	}

	var rows []model.StatsDetail
	if err := query.Find(&rows).Error; err != nil {
		return model.StatsChartResult{}, err
	}

	// 使用 channelCache 提供渠道名称映射
	getName := func(id int) string {
		ch, ok := channelCache.Get(id)
		if !ok {
			return ""
		}
		return ch.Name
	}

	return buildChartBuckets(rows, startTime, endTime, period, getName), nil
}
