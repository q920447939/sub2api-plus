// Package iq — [plus] 用户端 DTO。
//
// 这里的每个字段都会直接发给普通用户。不要加账号、Key、错误信息、耗时、Token 等字段：
// dto_user_test.go 用白名单锁住了 JSON 字段，加字段会让测试失败（需要先确认它可以给用户看）。
package iq

import "time"

// UserMeta 侧边栏开关
type UserMeta struct {
	Enabled  bool   `json:"enabled"`
	MenuName string `json:"menu_name"`
}

// UserGallery 用户端画廊
type UserGallery struct {
	Groups []UserGroup `json:"groups"`
}

// UserGroup 一个分组一行
type UserGroup struct {
	ID       int64      `json:"id"`
	Name     string     `json:"name"`
	Platform string     `json:"platform"`
	Tasks    []UserTask `json:"tasks"`
}

// UserTask 分组下的一个模型（多个模型时上下堆叠）
type UserTask struct {
	Model   string       `json:"model"`
	Samples []UserSample `json:"samples"`
}

// UserSample 一张卡片：只有时间和预览
type UserSample struct {
	ID         int64     `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	PreviewURL string    `json:"preview_url"`
}
