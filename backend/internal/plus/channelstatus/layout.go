// Package channelstatus — [plus] 展示编排：把管理员保存的编排与当前分组目录合并
package channelstatus

// platformNames 平台 Tab 默认名；未列出的平台直接显示平台标识
var platformNames = map[string]string{
	"anthropic":   "Claude",
	"openai":      "OpenAI",
	"gemini":      "Gemini",
	"antigravity": "Antigravity",
	"grok":        "Grok",
	"kiro":        "Kiro",
	"kimi":        "Kimi",
	"zhipu":       "GLM",
	"deepseek":    "DeepSeek",
	"composite":   "混合",
}

// PlatformDisplayName 平台默认展示名
func PlatformDisplayName(platform string) string {
	if n, ok := platformNames[platform]; ok {
		return n
	}
	return platform
}

// MergedLayout 合并结果
type MergedLayout struct {
	Layout      []PlatformLayout `json:"layout"`
	NewGroupIDs []int64          `json:"new_group_ids"` // 编排里还没有的分组（管理端标"新"）
}

// MergeLayout 规则：
//   - 保留编排里的平台/分组顺序；已删除或停用的分组剔除
//   - 编排里没有的分组追加到其所属平台末尾（平台不存在则在末尾新建），显隐取 autoShow
//   - 分组可以被管理员放在任意平台下，合并时不会把它移回原平台
func MergeLayout(saved []PlatformLayout, catalog []GroupInfo, autoShow bool) MergedLayout {
	byID := make(map[int64]GroupInfo, len(catalog))
	for _, g := range catalog {
		byID[g.ID] = g
	}
	placed := map[int64]bool{}
	platIdx := map[string]int{}
	out := make([]PlatformLayout, 0, len(saved))
	for _, p := range saved {
		np := PlatformLayout{Platform: p.Platform, Visible: p.Visible, TabName: p.TabName, Groups: []GroupLayout{}}
		for _, g := range p.Groups {
			if _, ok := byID[g.GroupID]; !ok || placed[g.GroupID] {
				continue
			}
			placed[g.GroupID] = true
			np.Groups = append(np.Groups, g)
		}
		platIdx[p.Platform] = len(out)
		out = append(out, np)
	}

	var newIDs []int64
	for _, g := range catalog {
		if placed[g.ID] {
			continue
		}
		idx, ok := platIdx[g.Platform]
		if !ok {
			idx = len(out)
			platIdx[g.Platform] = idx
			out = append(out, PlatformLayout{Platform: g.Platform, Visible: true, Groups: []GroupLayout{}})
		}
		out[idx].Groups = append(out[idx].Groups, GroupLayout{GroupID: g.ID, Visible: autoShow})
		newIDs = append(newIDs, g.ID)
	}
	if newIDs == nil {
		newIDs = []int64{}
	}
	return MergedLayout{Layout: out, NewGroupIDs: newIDs}
}
