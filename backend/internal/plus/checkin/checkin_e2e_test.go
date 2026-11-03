//go:build integration

package checkin_test

// HTTP 端到端集成测试：直接构建 gin Engine + 真实 DB，覆盖完整请求链路。
// 运行：PLUS_CHECKIN_IT_DSN='postgres://...' go test -tags integration ./internal/plus/checkin -run ITE2E -v

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"database/sql"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/plus/checkin"
	"github.com/Wei-Shaw/sub2api/internal/plus/points"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

func openE2EDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("PLUS_CHECKIN_IT_DSN")
	if dsn == "" {
		t.Skip("PLUS_CHECKIN_IT_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, tbl := range []string{
		"plus_slot_feed", "plus_slot_pot_ledger", "plus_slot_spins", "plus_points_redeem_grants",
		"plus_checkin_ip_logs", "plus_checkin_logs", "plus_checkin_config",
		"plus_points_ledger", "plus_points_accounts", "plus_points_config",
		"users",
	} {
		if _, err := db.Exec("DELETE FROM " + tbl); err != nil {
			t.Fatalf("clean %s: %v", tbl, err)
		}
	}
	return db
}

func seedE2EUser(t *testing.T, db *sql.DB, email string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(
		`INSERT INTO users (email, password_hash, balance, role, created_at, updated_at)
		 VALUES ($1, 'x', 0, 'user', NOW(), NOW()) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func buildEngine(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	pointsSvc := points.NewService(points.NewRepository(db))
	svc := checkin.NewService(checkin.NewRepository(db), pointsSvc)
	h := checkin.NewHandler(svc)

	// 注入鉴权存根中间件
	authed := r.Group("/plus")
	authed.Use(func(c *gin.Context) {
		uid := int64(1) // 由测试通过 header 覆盖
		if v := c.GetHeader("X-Test-User-ID"); v != "" {
			var id int64
			if _, err := fmt.Sscan(v, &id); err == nil {
				uid = id
			}
		}
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: uid})
		c.Next()
	})
	h.RegisterUserRoutes(authed)

	admin := r.Group("/plus/admin")
	admin.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
		c.Set(string(middleware.ContextKeyUserRole), "admin")
		c.Next()
	})
	h.RegisterAdminRoutes(admin)
	return r
}

// TestITE2ECheckinStatus GET /plus/checkin/status 200
func TestITE2ECheckinStatus(t *testing.T) {
	db := openE2EDB(t)
	uid := seedE2EUser(t, db, "e2e_status@example.com")
	r := buildEngine(db)

	req := httptest.NewRequest(http.MethodGet, "/plus/checkin/status", nil)
	req.Header.Set("X-Test-User-ID", fmt.Sprint(uid))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Data struct {
			TodayChecked bool  `json:"today_checked"`
			Balance      int64 `json:"balance"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.TodayChecked {
		t.Fatal("fresh user should not have checked in")
	}
}

// TestITE2ECheckinFlow POST /plus/checkin → GET /plus/checkin/status
func TestITE2ECheckinFlow(t *testing.T) {
	db := openE2EDB(t)
	uid := seedE2EUser(t, db, "e2e_flow@example.com")
	r := buildEngine(db)

	// 签到
	req := httptest.NewRequest(http.MethodPost, "/plus/checkin", nil)
	req.Header.Set("X-Test-User-ID", fmt.Sprint(uid))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("checkin want 200, got %d: %s", w.Code, w.Body.String())
	}
	var checkinResp struct {
		Data struct {
			Streak        int   `json:"streak"`
			PointsGranted int64 `json:"points_granted"`
			Balance       int64 `json:"balance"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &checkinResp); err != nil {
		t.Fatal(err)
	}
	if checkinResp.Data.Streak != 1 {
		t.Fatalf("streak: want 1, got %d", checkinResp.Data.Streak)
	}
	if checkinResp.Data.PointsGranted != 10 {
		t.Fatalf("points: want 10, got %d", checkinResp.Data.PointsGranted)
	}

	// 重复签到应该返回 409
	req2 := httptest.NewRequest(http.MethodPost, "/plus/checkin", nil)
	req2.Header.Set("X-Test-User-ID", fmt.Sprint(uid))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("duplicate checkin want 409, got %d", w2.Code)
	}

	// 查询状态应反映已签到
	req3 := httptest.NewRequest(http.MethodGet, "/plus/checkin/status", nil)
	req3.Header.Set("X-Test-User-ID", fmt.Sprint(uid))
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	var statusResp struct {
		Data struct {
			TodayChecked bool  `json:"today_checked"`
			Balance      int64 `json:"balance"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w3.Body.Bytes(), &statusResp); err != nil {
		t.Fatal(err)
	}
	if !statusResp.Data.TodayChecked {
		t.Fatal("status.today_checked should be true after checkin")
	}
	if statusResp.Data.Balance != 10 {
		t.Fatalf("balance: want 10, got %d", statusResp.Data.Balance)
	}
}

// suppress unused import
var _ = response.Success
var _ = context.Background
