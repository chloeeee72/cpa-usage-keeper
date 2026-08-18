package migration

import (
	"fmt"

	"gorm.io/gorm"
)

// dropRankingTablesMigration 彻底删除已废弃的 Ranking 相关存储。
func dropRankingTablesMigration(db *gorm.DB) error {
	if db.Migrator().HasTable("local_ranking_period_stats") {
		if err := db.Migrator().DropTable("local_ranking_period_stats"); err != nil {
			return fmt.Errorf("drop local_ranking_period_stats: %w", err)
		}
	}
	if db.Migrator().HasColumn("cpa_api_keys", "local_ranking_avatar_id") {
		if err := db.Exec("ALTER TABLE cpa_api_keys DROP COLUMN local_ranking_avatar_id").Error; err != nil {
			return fmt.Errorf("drop cpa_api_keys.local_ranking_avatar_id: %w", err)
		}
	}
	return nil
}
