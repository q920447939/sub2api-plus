// Package iq — [plus] 模型智商检测（鹈鹕画图）。
//
// 定时把同一道创作题（鹈鹕骑自行车的单文件 HTML/SVG 动画）通过本站网关发给各分组的模型，
// 结果只存档预览、不打分。用户端只看成功样本；管理端额外看失败记录、耗时和实际命中的账号。
//
// 分层（依赖方向自上而下）：
//
//	handler_user / handler_admin   HTTP 层，用户与管理员各自独立的路由、DTO（菜单即身份）
//	UserService / AdminService     用例
//	Executor / Scheduler           执行一次检测 / 定时与排队
//	ports.go                       Channel、Challenge、LLMClient、Attributor、各类 Store 接口
//	实现：GatewayChannel、PelicanChallenge、StreamClient、UsageLogAttributor、Repository
package iq

import (
	"fmt"
	"net/url"
	"strings"
)

// PelicanPrompt 鹈鹕题原文，来自 pelican-iq（作者已授权使用）
const PelicanPrompt = "请生成可直接运行的单文件HTML，使用内联SVG绘制鹈鹕骑自行车的二维循环动画。画面以鹈鹕和自行车为主体，展示清晰的身体结构、踩踏动作和车轮转动，配合协调的背景、配色与层次。动画应流畅自然、衔接连续，并适配不同屏幕尺寸。禁止依赖外部资源，只输出完整HTML，不要代码围栏或解释文字。"

// Config 全局配置（单行 JSON + 乐观锁）
type Config struct {
	Enabled            bool   `json:"enabled"`               // 调度总开关，关闭后不再定时检测（手动仍可）
	UserEnabled        bool   `json:"user_enabled"`          // 对用户开放「模型画廊」
	MenuName           string `json:"menu_name"`             // 用户端菜单名称
	UserSamplesPerTask int    `json:"user_samples_per_task"` // 用户端每个任务显示的样本数
	DefaultInterval    int    `json:"default_interval"`      // 新建任务预填的间隔（分钟）
	DefaultRetention   int    `json:"default_retention"`     // 新建任务预填的保留条数
	Concurrency        int    `json:"concurrency"`           // 全局同时运行的检测数
	TimeoutSeconds     int    `json:"timeout_seconds"`       // 单次检测超时
	MaxOutputTokens    int    `json:"max_output_tokens"`     // 正文输出上限（思考预算另算）
	Endpoint           string `json:"endpoint"`              // 本站网关地址
	KeyPrefix          string `json:"key_prefix"`            // 自动生成 Key 的名称前缀
	Prompt             string `json:"prompt"`                // 题目
	PromptVersion      int    `json:"prompt_version"`        // 题目版本，改题目时 +1
}

func DefaultConfig() Config {
	return Config{
		Enabled:            true,
		UserEnabled:        false,
		MenuName:           "模型画廊",
		UserSamplesPerTask: 24,
		DefaultInterval:    30,
		DefaultRetention:   48,
		Concurrency:        2,
		TimeoutSeconds:     600,
		MaxOutputTokens:    16000,
		Endpoint:           "http://127.0.0.1:8080",
		KeyPrefix:          "IQ-",
		Prompt:             PelicanPrompt,
		PromptVersion:      1,
	}
}

// fillDefaults 旧数据缺字段时补默认值（JSON 反序列化到默认值之上已覆盖大部分情况）
func (c *Config) fillDefaults() {
	d := DefaultConfig()
	if strings.TrimSpace(c.Prompt) == "" {
		c.Prompt, c.PromptVersion = d.Prompt, d.PromptVersion
	}
	if c.PromptVersion <= 0 {
		c.PromptVersion = 1
	}
	if strings.TrimSpace(c.MenuName) == "" {
		c.MenuName = d.MenuName
	}
}

// Validate 校验管理员提交的配置
func (c *Config) Validate() error {
	c.MenuName = strings.TrimSpace(c.MenuName)
	c.Endpoint = strings.TrimRight(strings.TrimSpace(c.Endpoint), "/")
	c.KeyPrefix = strings.TrimSpace(c.KeyPrefix)
	c.Prompt = strings.TrimSpace(c.Prompt)
	switch {
	case c.MenuName == "" || len([]rune(c.MenuName)) > 20:
		return invalid("菜单名称需为 1-20 个字")
	case c.UserSamplesPerTask < 1 || c.UserSamplesPerTask > 100:
		return invalid("用户端显示条数需在 1-100 之间")
	case c.DefaultInterval < MinInterval || c.DefaultInterval > MaxInterval:
		return invalid(fmt.Sprintf("默认间隔需在 %d-%d 分钟之间", MinInterval, MaxInterval))
	case c.DefaultRetention < 1 || c.DefaultRetention > MaxRetention:
		return invalid(fmt.Sprintf("默认保留条数需在 1-%d 之间", MaxRetention))
	case c.Concurrency < 1 || c.Concurrency > 10:
		return invalid("并发上限需在 1-10 之间")
	case c.TimeoutSeconds < 30 || c.TimeoutSeconds > 1800:
		return invalid("单次超时需在 30-1800 秒之间")
	case c.MaxOutputTokens < 1000 || c.MaxOutputTokens > 128000:
		return invalid("最大输出 Token 需在 1000-128000 之间")
	case len([]rune(c.KeyPrefix)) > 20:
		return invalid("Key 前缀最多 20 个字")
	case c.Prompt == "" || len([]rune(c.Prompt)) > 4000:
		return invalid("题目不能为空，且不超过 4000 字")
	}
	if u, err := url.Parse(c.Endpoint); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return invalid("网关地址格式不正确，需以 http:// 或 https:// 开头")
	}
	return nil
}
