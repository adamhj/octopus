package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/gin-gonic/gin"
)

func init() {
	router.NewGroupRouter("/api/v1/stats").
		Use(middleware.Auth()).
		AddRoute(
			router.NewRoute("/today", http.MethodGet).
				Handle(getStatsToday),
		).
		AddRoute(
			router.NewRoute("/daily", http.MethodGet).
				Handle(getStatsDaily),
		).
		AddRoute(
			router.NewRoute("/hourly", http.MethodGet).
				Handle(getStatsHourly),
		).
		AddRoute(
			router.NewRoute("/total", http.MethodGet).
				Handle(getStatsTotal),
		).
		AddRoute(
			router.NewRoute("/apikey", http.MethodGet).
				Handle(getStatsAPIKey),
		).
		AddRoute(
			router.NewRoute("/detail", http.MethodGet).
				Handle(getStatsDetail),
		).
		AddRoute(
			router.NewRoute("/detail/chart", http.MethodGet).
				Handle(getStatsDetailChart),
		)
}

func getStatsToday(c *gin.Context) {
	resp.Success(c, op.StatsTodayGet())
}

func getStatsDaily(c *gin.Context) {
	statsDaily, err := op.StatsGetDaily(c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, statsDaily)
}

func getStatsHourly(c *gin.Context) {
	resp.Success(c, op.StatsHourlyGet())
}

func getStatsTotal(c *gin.Context) {
	resp.Success(c, op.StatsTotalGet())
}

func getStatsAPIKey(c *gin.Context) {
	resp.Success(c, op.StatsAPIKeyList())
}

// getStatsDetail 查询明细统计，支持按日期、小时、渠道ID、上游实际模型名任意组合过滤。
// 所有参数均可省略，省略的维度不参与过滤。
func getStatsDetail(c *gin.Context) {
	date := c.Query("date")

	var hour *int
	if hourStr := c.Query("hour"); hourStr != "" {
		h, err := strconv.Atoi(hourStr)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, "invalid hour")
			return
		}
		hour = &h
	}

	var channelID *int
	if channelIDStr := c.Query("channel_id"); channelIDStr != "" {
		cid, err := strconv.Atoi(channelIDStr)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, "invalid channel_id")
			return
		}
		channelID = &cid
	}

	actualModelName := c.Query("model")

	details, err := op.StatsDetailQuery(c.Request.Context(), date, hour, channelID, actualModelName)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, details)
}

// getStatsDetailChart 查询图表统计，按指定的聚合周期返回分桶统计数据。
// 请求参数：
//   - start_time (必填): 格式 "2006-01-02 15:04"，分钟只支持 :00
//   - end_time   (必填): 同上
//   - period     (必填): hour / day / week / month
//   - channel_id (可选): 渠道 ID 过滤
//   - model      (可选): 上游实际模型名过滤
func getStatsDetailChart(c *gin.Context) {
	startTimeStr := c.Query("start_time")
	endTimeStr := c.Query("end_time")
	period := c.Query("period")

	if startTimeStr == "" || endTimeStr == "" || period == "" {
		resp.Error(c, http.StatusBadRequest, "start_time, end_time, period are required")
		return
	}

	startTime, err := time.ParseInLocation("2006-01-02 15:04", startTimeStr, time.UTC)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, "invalid start_time, expected YYYY-MM-DD HH:00")
		return
	}
	endTime, err := time.ParseInLocation("2006-01-02 15:04", endTimeStr, time.UTC)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, "invalid end_time, expected YYYY-MM-DD HH:00")
		return
	}
	if startTime.Minute() != 0 || startTime.Second() != 0 ||
		endTime.Minute() != 0 || endTime.Second() != 0 {
		resp.Error(c, http.StatusBadRequest, "start_time and end_time must be aligned to HH:00")
		return
	}
	if !endTime.After(startTime) {
		resp.Error(c, http.StatusBadRequest, "end_time must be after start_time")
		return
	}

	var channelID *int
	if channelIDStr := c.Query("channel_id"); channelIDStr != "" {
		cid, err := strconv.Atoi(channelIDStr)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, "invalid channel_id")
			return
		}
		channelID = &cid
	}

	actualModelName := c.Query("model")

	result, err := op.StatsDetailChartQuery(c.Request.Context(), startTime, endTime, channelID, actualModelName, period)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, result)
}
