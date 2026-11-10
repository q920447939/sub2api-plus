package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	lmfpd "github.com/Ikaleio/lm-detector/sdk/go"
)

// FingerprintLibrary lm-detector 参考库（懒加载，失败后 10 分钟内不重试）
type FingerprintLibrary struct {
	mu       sync.Mutex
	det      *lmfpd.Detector
	models   map[string]string // 归一化 id → 原始 id
	failedAt time.Time
	lastErr  error
}

var dateSuffix = regexp.MustCompile(`-(\d{8}|latest)$`)

// normalizeModel claude-opus-4.5 / claude-opus-4-5-20251101 → claude-opus-4-5
func normalizeModel(m string) string {
	m = strings.ToLower(strings.TrimSpace(m))
	if i := strings.LastIndex(m, "/"); i >= 0 {
		m = m[i+1:]
	}
	m = strings.NewReplacer(".", "-", "_", "-", " ", "-").Replace(m)
	return dateSuffix.ReplaceAllString(m, "")
}

func fingerprintCachePath() string {
	if p := os.Getenv("PLUS_LMFPD_CACHE"); p != "" {
		return p
	}
	if st, err := os.Stat("/app/data"); err == nil && st.IsDir() {
		return "/app/data/plus_lmfpd/data.bin"
	}
	return "" // SDK 默认的系统缓存目录
}

func (l *FingerprintLibrary) get(ctx context.Context) (*lmfpd.Detector, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.det != nil {
		return l.det, nil
	}
	if !l.failedAt.IsZero() && time.Since(l.failedAt) < 10*time.Minute {
		return nil, fmt.Errorf("指纹参考库加载失败（%v），10 分钟后重试", l.lastErr)
	}
	cache := fingerprintCachePath()
	if cache != "" {
		_ = os.MkdirAll(filepath.Dir(cache), 0o755)
	}
	lctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	det, err := lmfpd.Load(lctx, lmfpd.LoadOptions{Cache: cache})
	if err != nil {
		l.failedAt, l.lastErr = time.Now(), err
		return nil, fmt.Errorf("指纹参考库加载失败：%w", err)
	}
	l.det = det
	if l.models == nil {
		l.models = fetchLibraryModels(lctx)
	}
	return det, nil
}

// fetchLibraryModels 参考库收录的模型（读 bundle.json；失败时返回 nil，表示未知）
func fetchLibraryModels(ctx context.Context) map[string]string {
	for _, base := range lmfpd.DefaultDataURLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/bundle.json", nil)
		if err != nil {
			continue
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		var b struct {
			Models []struct {
				ID string `json:"id"`
			} `json:"models"`
		}
		if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &b) != nil || len(b.Models) == 0 {
			continue
		}
		out := map[string]string{}
		for _, m := range b.Models {
			out[normalizeModel(m.ID)] = m.ID
		}
		return out
	}
	return nil
}

// Models 已知收录的模型（未知时 nil）
func (l *FingerprintLibrary) Models() map[string]string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.models
}

func (l *FingerprintLibrary) learn(results []lmfpd.AnalysisResult) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.models != nil || len(results) < 5 {
		return
	}
	l.models = map[string]string{}
	for _, r := range results {
		l.models[normalizeModel(r.Model)] = r.Model
	}
}

// FingerprintRule 判定规则
type FingerprintRule struct {
	BadConsecutive int     `json:"bad_consecutive"` // 连续几次识别成别的模型记异常（1 次起记可疑）
	MinConfidence  float64 `json:"min_confidence"`  // 置信度低于这个百分比的结果不计入
}

func (r *FingerprintRule) normalize() {
	if r.BadConsecutive <= 0 {
		r.BadConsecutive = 3
	}
	if r.MinConfidence <= 0 || r.MinConfidence > 100 {
		r.MinConfidence = 60
	}
}

// FingerprintDetector 模型指纹识别：让模型凭第一反应写约 300 个整数，按数字偏好和参考库比对
type FingerprintDetector struct {
	LLM     Completer
	Library *FingerprintLibrary
}

var _ ActiveDetector = (*FingerprintDetector)(nil)

func (d *FingerprintDetector) Spec() TypeSpec {
	return TypeSpec{
		Key: "fingerprint", Name: "模型指纹识别", Active: true, Judges: true,
		Description: "让模型凭第一反应写约 300 个 1–355 的整数（每次 3 道），按它偏好的数字和参考库比对，识别它实际是哪个模型。基于 lm-detector（MIT）。",
		Supports:    "参考库收录的模型（按版本收录，库里没有的版本测不了）",
		JudgeDesc:   "连续 N 次识别成别的模型；置信度低的不计入",
		PlanFields:  []string{"models", "pick", "every", "timeout"},
		Outcomes: []OutcomeSpec{
			{Key: OutcomePass, Label: "和声称的一致", Tone: "ok"},
			{Key: OutcomeFail, Label: "识别为其他模型", Tone: "bad"},
			{Key: OutcomeLow, Label: "置信度不够", Tone: "few"},
			{Key: OutcomeSkipped, Label: "参考库没有这个模型", Tone: "few"},
			{Key: OutcomeError, Label: "请求出错", Tone: "err"},
		},
	}
}

func (d *FingerprintDetector) NormalizeItem(params, rule json.RawMessage) (json.RawMessage, json.RawMessage, error) {
	var r FingerprintRule
	if err := decodeInto(rule, &r); err != nil {
		return nil, nil, err
	}
	r.normalize()
	return json.RawMessage(`{}`), mustJSON(r), nil
}

func (d *FingerprintDetector) Supports(_ context.Context, _ string, model string) (bool, string) {
	ms := d.Library.Models()
	if ms == nil {
		return true, ""
	}
	if _, ok := ms[normalizeModel(model)]; ok {
		return true, ""
	}
	return false, fmt.Sprintf("指纹参考库没有收录 %s", model)
}

func (d *FingerprintDetector) Execute(ctx context.Context, in ExecInput) ExecResult {
	det, err := d.Library.get(ctx)
	if err != nil {
		return errResult("", err)
	}
	chs, err := lmfpd.GenerateChallenges(3)
	if err != nil {
		return errResult("", err)
	}
	type reply struct {
		comp Completion
		err  error
	}
	replies := make([]reply, len(chs))
	var wg sync.WaitGroup
	for i, ch := range chs {
		wg.Add(1)
		go func(i int, prompt string) {
			defer wg.Done()
			c, err := d.LLM.Complete(ctx, in.Target, prompt, 4000)
			replies[i] = reply{c, err}
		}(i, ch.Prompt)
	}
	wg.Wait()

	prompts := make([]string, len(chs))
	texts := make([]string, len(chs))
	outputs := make([]lmfpd.Output, 0, len(chs))
	var in1, out1 int
	var firstErr error
	for i, ch := range chs {
		prompts[i] = fmt.Sprintf("【第 %d 道】\n%s", i+1, ch.Prompt)
		r := replies[i]
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			texts[i] = fmt.Sprintf("【第 %d 道】出错：%v", i+1, r.err)
			continue
		}
		texts[i] = fmt.Sprintf("【第 %d 道】\n%s", i+1, truncateRunes(r.comp.Text, 3000))
		outputs = append(outputs, lmfpd.Output{Text: r.comp.Text, ExpectedCount: ch.ExpectedCount})
		if r.comp.InputTokens != nil {
			in1 += *r.comp.InputTokens
		}
		if r.comp.OutputTokens != nil {
			out1 += *r.comp.OutputTokens
		}
	}
	res := ExecResult{Prompt: strings.Join(prompts, "\n\n"), Reply: strings.Join(texts, "\n\n"), InputTokens: &in1, OutputTokens: &out1}
	if len(outputs) == 0 {
		res.Outcome, res.Summary, res.Error = OutcomeError, "请求出错", firstErr.Error()
		return res
	}
	an, err := det.Analyze(outputs, false)
	if err != nil {
		res.Outcome, res.Summary, res.Error = OutcomeError, "回答无法评分", err.Error()
		return res
	}
	d.Library.learn(an.Results)
	if an.Decision != "scored" && an.Decision != "partial" {
		res.Outcome, res.Summary = OutcomeError, "回答无法评分"
		res.Error = strings.TrimSpace(an.Evidence.Label + " " + an.Evidence.Reason)
		if res.Error == "" {
			res.Error = "有效回答不足（" + an.Decision + "）"
		}
		return res
	}
	declared := in.Target.Model
	if ok, why := d.Supports(ctx, in.Platform, declared); !ok {
		res.Outcome, res.Summary = OutcomeSkipped, why
	}
	// 有效回答不到 3 条时 lm-detector 只给部分排名、没有校准置信度：记为未知，不计入判定
	conf, confKnown := 0.0, an.Probability != nil
	if confKnown {
		conf = *an.Probability * 100
	}
	var confVal any
	if confKnown {
		confVal = round1(conf)
	}
	top := make([]map[string]any, 0, 5)
	for i, r := range an.Results {
		if i >= 5 {
			break
		}
		p := 0.0
		if r.Probability != nil {
			p = *r.Probability * 100
		}
		top = append(top, map[string]any{"model": r.Model, "probability": round1(p)})
	}
	match := normalizeModel(an.Prediction) == normalizeModel(declared)
	res.Detail = map[string]any{
		"declared": declared, "prediction": an.Prediction, "confidence": confVal, "match": match,
		"top": top, "decision": an.Decision, "used_outputs": an.UsedOutputs, "library": det.BuiltAt(),
	}
	if res.Outcome == OutcomeSkipped {
		return res
	}
	res.Outcome = d.Classify(in.Item, Run{Detail: res.Detail})
	switch res.Outcome {
	case OutcomePass:
		res.Summary = fmt.Sprintf("识别为 %s（置信 %.0f%%），和声称的一致", an.Prediction, conf)
	case OutcomeFail:
		res.Summary = fmt.Sprintf("声称 %s，识别为 %s（置信 %.0f%%）", declared, an.Prediction, conf)
	default:
		if confKnown {
			res.Summary = fmt.Sprintf("最像 %s，但置信只有 %.0f%%", an.Prediction, conf)
		} else {
			res.Summary = fmt.Sprintf("最像 %s，但只有 %d 条有效回答，没有置信度，不计入", an.Prediction, an.UsedOutputs)
		}
	}
	return res
}

func (d *FingerprintDetector) classify(r FingerprintRule, match, confKnown bool, conf float64) string {
	r.normalize()
	switch {
	case !confKnown:
		return OutcomeLow
	case match:
		return OutcomePass
	case conf >= r.MinConfidence:
		return OutcomeFail
	}
	return OutcomeLow
}

// Classify 按当前规则重新归类
func (d *FingerprintDetector) Classify(item Item, run Run) string {
	if run.Outcome == OutcomeError || run.Outcome == OutcomeSkipped {
		return run.Outcome
	}
	var r FingerprintRule
	_ = json.Unmarshal(item.Rule, &r)
	match, _ := run.Detail["match"].(bool)
	conf, confKnown := run.Detail["confidence"].(float64)
	return d.classify(r, match, confKnown, conf)
}

func (d *FingerprintDetector) Judge(env *JudgeEnv, item Item, s Subject) Verdict {
	var r FingerprintRule
	_ = json.Unmarshal(item.Rule, &r)
	r.normalize()
	rs := env.RunsOf(item.ID, s.CredKey, s.Model)
	if len(rs) == 0 {
		return Verdict{Level: LevelNone, Short: "还没执行", Long: "计划已覆盖，还没执行过"}
	}
	var valid []Run
	for _, run := range rs {
		if c := d.Classify(item, run); c == OutcomePass || c == OutcomeFail {
			valid = append(valid, run)
		}
	}
	if len(valid) == 0 {
		if rs[0].Outcome == OutcomeSkipped {
			return Verdict{Level: LevelNone, Short: "测不了", Long: rs[0].Summary}
		}
		return Verdict{Level: LevelFew, Short: "无有效结果", Long: "还没有有效结果（都出错或置信度太低）" + staleNote(rs)}
	}
	streak := 0
	for _, run := range valid {
		if d.Classify(item, run) != OutcomeFail {
			break
		}
		streak++
	}
	last := valid[0]
	pred, _ := last.Detail["prediction"].(string)
	conf, _ := last.Detail["confidence"].(float64)
	dist := map[string]int{}
	for i, run := range valid {
		if i >= 30 {
			break
		}
		p, _ := run.Detail["prediction"].(string)
		dist[p]++
	}
	type kv struct {
		Model string `json:"model"`
		N     int    `json:"n"`
	}
	var ds []kv
	for k, v := range dist {
		ds = append(ds, kv{k, v})
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i].N > ds[j].N })
	ev := map[string]any{"streak": streak, "bad_consecutive": r.BadConsecutive, "distribution": ds, "errors": leadingErrors(rs)}
	if streak == 0 {
		return Verdict{Level: LevelOK, Short: "和声称的一致", Evidence: ev,
			Long: fmt.Sprintf("最近一次识别为 %s（置信 %.0f%%），和声称的一致%s", pred, conf, staleNote(rs))}
	}
	lv := LevelWarn
	if streak >= r.BadConsecutive {
		lv = LevelBad
	}
	long := fmt.Sprintf("声称 %s，最近 %d 次都识别为 %s（置信 %.0f%%）", s.Model, streak, pred, conf)
	if streak == 1 {
		long = fmt.Sprintf("声称 %s，最近一次识别为 %s（置信 %.0f%%）", s.Model, pred, conf)
	}
	if lv == LevelWarn {
		long += fmt.Sprintf("；连续 %d 次才判异常", r.BadConsecutive)
	}
	return Verdict{Level: lv, Short: "像 " + pred, Long: long + staleNote(rs), Evidence: ev}
}
