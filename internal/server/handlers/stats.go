package handlers

import (
	"net/http"
	"strconv"

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
