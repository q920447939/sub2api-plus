// Package iq — [plus] 账号归因：网关每个请求都有 client request id（响应头 X-Client-Request-ID），
//   - 成功：usage_logs.request_id = "client:<id>"，带最终服务的 account_id
//   - 失败：ops_error_logs.client_request_id = <id>，带 account_id，upstream_errors 里有每次尝试的账号
//
// 只读上游表，不改上游代码。
package iq

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// UsageLogAttributor 用 usage_logs + ops_error_logs 反查
type UsageLogAttributor struct{ DB *sql.DB }

var _ Attributor = UsageLogAttributor{}

type upstreamEvent struct {
	AccountID          int64  `json:"account_id"`
	UpstreamStatusCode int    `json:"upstream_status_code"`
	Message            string `json:"message"`
	Kind               string `json:"kind"`
}

func (a UsageLogAttributor) Attribute(ctx context.Context, id string, since time.Time) ([]AccountHop, bool, error) {
	if id == "" {
		return nil, true, nil
	}
	since = since.Add(-time.Minute) // 时钟误差余量；同时帮助分区裁剪

	var hops []AccountHop
	found := false

	// 失败的尝试（故障切换前的账号，或最终失败）
	var (
		errAccount sql.NullInt64
		status     sql.NullInt64
		msg        sql.NullString
		events     []byte
	)
	err := a.DB.QueryRowContext(ctx,
		`SELECT account_id, status_code, error_message, upstream_errors FROM ops_error_logs
		 WHERE client_request_id = $1 AND created_at >= $2 ORDER BY id DESC LIMIT 1`, id, since).
		Scan(&errAccount, &status, &msg, &events)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return nil, false, err
	default:
		found = true
		var evs []upstreamEvent
		if len(events) > 0 && json.Unmarshal(events, &evs) == nil {
			for _, ev := range evs {
				if ev.AccountID > 0 {
					hops = appendHop(hops, AccountHop{AccountID: ev.AccountID, StatusCode: ev.UpstreamStatusCode, Message: truncate(ev.Message, 200)})
				}
			}
		}
		if errAccount.Valid && errAccount.Int64 > 0 {
			hops = appendHop(hops, AccountHop{AccountID: errAccount.Int64, StatusCode: int(status.Int64), Message: truncate(msg.String, 200)})
		}
	}

	// 成功：最终服务的账号
	var okAccount int64
	err = a.DB.QueryRowContext(ctx,
		`SELECT account_id FROM usage_logs WHERE request_id = $1 AND created_at >= $2 ORDER BY id DESC LIMIT 1`,
		"client:"+id, since).Scan(&okAccount)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return nil, false, err
	default:
		found = true
		if n := len(hops); n > 0 && hops[n-1].AccountID == okAccount {
			hops = hops[:n-1]
		}
		hops = append(hops, AccountHop{AccountID: okAccount, OK: true})
	}
	return hops, found, nil
}

// appendHop 合并连续重复的同一账号（同账号重试）
func appendHop(hops []AccountHop, h AccountHop) []AccountHop {
	if n := len(hops); n > 0 && hops[n-1].AccountID == h.AccountID {
		if hops[n-1].Message == "" {
			hops[n-1] = h
		}
		return hops
	}
	return append(hops, h)
}
