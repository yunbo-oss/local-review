package bootstrap

import (
	"fmt"

	"local-review-go/internal/model"

	"gorm.io/gorm"
)

// Migrate creates every table required by the API, seed, Agent memory and eval
// persistence. Keeping this list in one package prevents the server and Docker
// migrate job from drifting apart.
func Migrate(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("mysql db is nil")
	}
	return db.AutoMigrate(
		&model.User{},
		&model.UserInfo{},
		&model.Shop{},
		&model.ShopType{},
		&model.Blog{},
		&model.BlogComments{},
		&model.Voucher{},
		&model.SecKillVoucher{},
		&model.VoucherOrder{},
		&model.Follow{},
		&model.UserAgentProfile{},
		&model.UserAgentProfileEvent{},
		&model.AgentRun{},
		&model.AgentToolCall{},
	)
}
