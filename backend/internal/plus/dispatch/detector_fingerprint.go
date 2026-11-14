package dispatch

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	lmfpd "github.com/Ikaleio/lm-detector/sdk/go"
)

// FingerprintLibrary lm-detector 参考库。
// 同步分两层：SDK 负责磁盘缓存（超过 24 小时在后台比对 SHA256，有新版才下载分片并覆盖缓存文件）；
// 这里负责把缓存加载进内存，并定期检查缓存文件有没有变，变了就重新加载、原子替换。
// 重新加载失败时继续用旧库，检测不中断。
type FingerprintLibrary struct {
	mu          sync.Mutex
	det         *lmfpd.Detector
	models      map[string]string // 归一化 id → 原始 id
	builtAt     string            // 参考库建库时间（版本标识）
	loadedMtime time.Time         // 上次加载时缓存文件的修改时间
	failedAt    time.Time
	lastErr     error
	reloading   bool
}

const (
	libraryTTL        = 24 * time.Hour // SDK 多久去源站比对一次版本
	libraryCheckEvery = time.Hour      // 多久检查一次磁盘缓存有没有变
)

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

// fingerprintCachePath 缓存文件路径（总是显式给出，这样能看它的修改时间判断有没有更新）
func fingerprintCachePath() string {
	if p := os.Getenv("PLUS_LMFPD_CACHE"); p != "" {
		return p
	}
	if st, err := os.Stat("/app/data"); err == nil && st.IsDir() {
		return "/app/data/plus_lmfpd/data.bin"
	}
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "plus_lmfpd", "data.bin")
	}
	return filepath.Join(os.TempDir(), "plus_lmfpd", "data.bin")
}

func fileMtime(path string) time.Time {
	if st, err := os.Stat(path); err == nil {
		return st.ModTime()
	}
	return time.Time{}
}

// load 调 SDK 加载一次（有缓存时不联网；缓存超过 TTL 会顺带触发 SDK 的后台版本比对）
func loadLibrary(ctx context.Context) (*lmfpd.Detector, map[string]string, time.Time, error) {
	cache := fingerprintCachePath()
	_ = os.MkdirAll(filepath.Dir(cache), 0o755)
	mtime := fileMtime(cache) // 先记下来：SDK 后台更新会改掉它，下一轮检查时就会重新加载
	lctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	det, err := lmfpd.Load(lctx, lmfpd.LoadOptions{Cache: cache, TTL: libraryTTL})
	if err != nil {
		return nil, nil, mtime, err
	}
	if mtime.IsZero() { // 第一次下载：文件是 Load 刚写的
		mtime = fileMtime(cache)
	}
	models := cachedModels(cache)
	if models == nil {
		models = fetchLibraryModels(lctx)
	}
	return det, models, mtime, nil
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
	det, models, mtime, err := loadLibrary(ctx)
	if err != nil {
		l.failedAt, l.lastErr = time.Now(), err
		return nil, fmt.Errorf("指纹参考库加载失败：%w", err)
	}
	l.swap(det, models, mtime)
	return det, nil
}

// swap 换上新库（调用方持锁）
func (l *FingerprintLibrary) swap(det *lmfpd.Detector, models map[string]string, mtime time.Time) {
	if l.det != nil && det.BuiltAt() != l.builtAt {
		log.Printf("[plus.dispatch] 指纹参考库已更新：%s → %s，收录 %d 个模型", l.builtAt, det.BuiltAt(), len(models))
	}
	l.det, l.builtAt, l.loadedMtime = det, det.BuiltAt(), mtime
	if models != nil {
		l.models = models
	}
}

// Start 后台定期检查：启动时预加载一次，之后每小时看缓存文件，
// 文件变了（SDK 已下载新版）或超过 TTL（需要让 SDK 去比对版本）就重新加载。
func (l *FingerprintLibrary) Start(ctx context.Context) {
	go func() {
		if _, err := l.get(ctx); err != nil {
			log.Printf("[plus.dispatch] %v", err)
		}
		t := time.NewTicker(libraryCheckEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				l.maybeReload(ctx)
			}
		}
	}()
}

func (l *FingerprintLibrary) maybeReload(ctx context.Context) {
	l.mu.Lock()
	if l.det == nil {
		l.mu.Unlock()
		_, _ = l.get(ctx) // 还没加载成功过：按失败退避重试
		return
	}
	mtime := fileMtime(fingerprintCachePath())
	due := !mtime.Equal(l.loadedMtime) || time.Since(mtime) >= libraryTTL
	if !due || l.reloading {
		l.mu.Unlock()
		return
	}
	l.reloading = true
	l.mu.Unlock()

	det, models, loadedAt, err := loadLibrary(ctx) // 加载期间检测照常用旧库
	l.mu.Lock()
	defer l.mu.Unlock()
	l.reloading = false
	if err != nil {
		log.Printf("[plus.dispatch] 指纹参考库重新加载失败，继续用旧库：%v", err)
		return
	}
	l.swap(det, models, loadedAt)
}

// BuiltAt 当前参考库的建库时间（未加载时为空）
func (l *FingerprintLibrary) BuiltAt() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.builtAt
}

type libraryBundle struct {
	Models []struct {
		ID string `json:"id"`
	} `json:"models"`
}

func bundleModels(b libraryBundle) map[string]string {
	if len(b.Models) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, m := range b.Models {
		out[normalizeModel(m.ID)] = m.ID
	}
	return out
}

// cachedModels 从缓存文件里读收录的模型（文件格式：u32 小端长度 + bundle.json + 检测器），读不到返回 nil
func cachedModels(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var hdr [4]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return nil
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	if n == 0 || n > 16<<20 {
		return nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil
	}
	var b libraryBundle
	if json.Unmarshal(buf, &b) != nil {
		return nil
	}
	return bundleModels(b)
}

// fetchLibraryModels 参考库收录的模型（缓存里读不到时从源站读 bundle.json；失败时返回 nil，表示未知）
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
		var b libraryBundle
		if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &b) != nil {
			continue
		}
		if out := bundleModels(b); out != nil {
			return out
		}
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

// FingerprintRule 判定规则：最近 window 次有效结果里有 bad_min 次识别成别的模型 → 未通过；置信度低于 min_confidence 的那次不算
type FingerprintRule struct {
	WindowRule
	BadMin        int     `json:"bad_min"`
	MinConfidence float64 `json:"min_confidence"`
}

func (r *FingerprintRule) normalize() {
	r.WindowRule.normalize(3)
	if r.BadMin <= 0 {
		r.BadMin = 3
	}
	if r.BadMin > r.Window {
		r.BadMin = r.Window
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

var _ Detector = (*FingerprintDetector)(nil)

func (d *FingerprintDetector) Spec() TypeSpec {
	return TypeSpec{
		Key: "fingerprint", Name: "模型指纹识别", Judges: true, DefaultEvery: 60,
		Description: "让模型凭第一反应写约 300 个 1–355 的整数（每次 3 道），按它偏好的数字和参考库比对，识别它实际是哪个模型。基于 lm-detector（MIT）。",
		RuleText:    "最近 {window} 次有效结果里有 {bad_min} 次识别成别的模型算未通过；置信度低于 {min_confidence}% 的那次不算；满 {min_valid} 次才下结论",
		RuleFields:  []RuleField{{Key: "window", Min: 1, Max: 50}, {Key: "bad_min", Min: 1, Max: 50}, {Key: "min_confidence", Min: 1, Max: 100, Unit: "%"}, {Key: "min_valid", Min: 1, Max: 50}},
		Outcomes: []OutcomeSpec{
			{Key: OutcomePass, Label: "和声称的一致", Tone: "ok"},
			{Key: OutcomeFail, Label: "识别为其他模型", Tone: "bad"},
			{Key: OutcomeIgnored, Label: "已标误报", Tone: "few"},
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
	res.Summary = d.Describe(res.Outcome, res.Detail)
	return res
}

// Describe 按归类结果写一句话（规则改了以后历史记录的说法跟着变）
func (d *FingerprintDetector) Describe(outcome string, detail map[string]any) string {
	pred, _ := detail["prediction"].(string)
	declared, _ := detail["declared"].(string)
	conf, confKnown := detail["confidence"].(float64)
	used := 0
	if v, ok := detail["used_outputs"].(float64); ok {
		used = int(v)
	} else if v, ok := detail["used_outputs"].(int); ok {
		used = v
	}
	switch outcome {
	case OutcomePass:
		return fmt.Sprintf("识别为 %s（置信 %.0f%%），和声称的一致", pred, conf)
	case OutcomeFail, OutcomeIgnored:
		return fmt.Sprintf("声称 %s，识别为 %s（置信 %.0f%%）", declared, pred, conf)
	}
	if confKnown {
		return fmt.Sprintf("最像 %s，但置信只有 %.0f%%", pred, conf)
	}
	return fmt.Sprintf("最像 %s，但只有 %d 条有效回答，没有置信度，不计入", pred, used)
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

func (d *FingerprintDetector) Judge(item Item, rs []Run) *Verdict {
	var r FingerprintRule
	_ = json.Unmarshal(item.Rule, &r)
	r.normalize()
	valid, ignored := validRuns(rs, r.Window)
	var bad []string
	for _, x := range valid {
		if x.Outcome == OutcomeFail {
			p, _ := x.Detail["prediction"].(string)
			bad = append(bad, p)
		}
	}
	n := len(valid)
	v := &Verdict{Valid: n, Bad: len(bad), Ignored: ignored > 0}
	switch {
	case len(bad) >= r.BadMin:
		// 多数时候识别成同一个模型才说「像谁」；每次都不一样就只说不像
		like, k := mostCommon(bad)
		v.Level = LevelBad
		if like != "" && k*2 > len(bad) {
			v.Short = "像 " + like
			v.Long = fmt.Sprintf("最近 %d 次有效结果里 %d 次识别成 %s", n, len(bad), like)
		} else {
			v.Short = "不像声称的模型"
			v.Long = fmt.Sprintf("最近 %d 次有效结果里 %d 次识别成别的模型（%s）", n, len(bad), strings.Join(uniq(bad, 3), "、"))
		}
	case n < r.MinValid:
		if n == 0 && len(rs) > 0 && rs[0].Outcome == OutcomeSkipped {
			return &Verdict{Level: LevelFew, Short: "测不了", Long: rs[0].Summary}
		}
		f := fewVerdict(n, r.MinValid)
		f.Bad, f.Ignored = len(bad), ignored > 0
		return f
	case len(bad) > 0:
		v.Level, v.Short = LevelOK, fmt.Sprintf("%d 次里 %d 次不一致", n, len(bad))
		v.Long = fmt.Sprintf("最近 %d 次有效结果里 %d 次识别成别的模型，没到 %d 次", n, len(bad), r.BadMin)
	default:
		v.Level, v.Short, v.Long = LevelOK, "和声称的一致", fmt.Sprintf("最近 %d 次有效结果都和声称的一致", n)
	}
	return v
}

// uniq 去重后最多取 n 个，多出来的写「等」
func uniq(vs []string, n int) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range vs {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	if len(out) > n {
		out = append(out[:n], "等")
	}
	return out
}
