package migrate

import (
	"fmt"

	"github.com/bestruirui/octopus/internal/utils/log"
	"gorm.io/gorm"
)

func init() {
	RegisterAfterAutoMigration(Migration{
		Version: 3,
		Up:      migrateStatsDetail,
	})
}

// 003: 引入明细统计表（stats_details）与 RelayLog 缓存读取 Token 记录。
// GORM AutoMigrate 已自动完成 stats_details 建表和 relay_logs 加列，
// 此迁移主要处理 SQLite 对复合主键的兼容性校验，并记录旧数据状态。
func migrateStatsDetail(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	dialect := db.Dialector.Name()

	// 校验 stats_details 表是否已由 AutoMigrate 创建
	if !db.Migrator().HasTable("stats_details") {
		return fmt.Errorf("stats_details table not found after AutoMigrate")
	}

	// SQLite 对复合主键的支持有限，校验主键列是否正确建立
	if dialect == "sqlite" {
		if err := verifyStatsDetailSQLitePrimaryKey(db); err != nil {
			return fmt.Errorf("verify stats_details primary key: %w", err)
		}
	}

	// 记录旧 RelayLog 的缓存字段状态：迁移前已有的日志行缓存字段默认为 0
	var oldLogCount int64
	if err := db.Table("relay_logs").
		Where("cache_read_tokens = 0").
		Count(&oldLogCount).Error; err != nil {
		// 列可能不存在（极端情况），仅记录警告不中断
		log.Warnf("003 migration: failed to count old relay_logs with zero cache tokens: %v", err)
	} else if oldLogCount > 0 {
		log.Infof("003 migration: %d existing relay_logs have zero cache_read tokens (expected for pre-migration data)", oldLogCount)
	}

	return nil
}

// verifyStatsDetailSQLitePrimaryKey 校验 SQLite 下 stats_details 表的复合主键是否包含全部 4 个字段。
// SQLite 的主键信息存储在 sqlite_master 的建表 DDL 中，通过 pragma_table_info 的 pk 列判断。
func verifyStatsDetailSQLitePrimaryKey(db *gorm.DB) error {
	type columnInfo struct {
		Name string `gorm:"column:name"`
		PK   int    `gorm:"column:pk"`
	}
	var columns []columnInfo
	if err := db.Raw("SELECT name, pk FROM pragma_table_info('stats_details') ORDER BY pk").Scan(&columns).Error; err != nil {
		return fmt.Errorf("failed to query stats_details columns: %w", err)
	}

	expectedPKColumns := map[string]bool{
		"date":              false,
		"hour":              false,
		"channel_id":        false,
		"actual_model_name": false,
	}

	pkCount := 0
	for _, col := range columns {
		if col.PK > 0 {
			pkCount++
			if _, ok := expectedPKColumns[col.Name]; ok {
				expectedPKColumns[col.Name] = true
			}
		}
	}

	for name, found := range expectedPKColumns {
		if !found {
			return fmt.Errorf("stats_details primary key missing column: %s", name)
		}
	}

	if pkCount != 4 {
		return fmt.Errorf("stats_details expected 4 primary key columns, got %d", pkCount)
	}

	return nil
}
