// Package feedback — [plus] 模块装配（Composition Root）
package feedback

import (
	"context"
	"database/sql"
	"time"

	"github.com/gin-gonic/gin"
)

// Register 装配反馈中心并注册路由。withPoints=false 时不能发积分（积分模块不可用）
func Register(db *sql.DB, withPoints bool, userGroup, adminGroup *gin.RouterGroup) *Service {
	deps := Deps{
		Repo:     NewRepository(db),
		Accounts: sqlAccounts{db: db},
		Errors:   sqlErrorLogs{db: db},
		Blobs:    newLocalBlobs(),
	}
	if withPoints {
		deps.Rewarder = pointsRewarder{}
	}
	svc := NewService(deps)
	h := NewHandler(svc)
	h.RegisterUserRoutes(userGroup)
	h.RegisterAdminRoutes(adminGroup)
	svc.StartWorker(context.Background(), time.Hour)
	return svc
}
