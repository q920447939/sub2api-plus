# PRD：巡检中心（plus/patrol）：主动巡检、AI 诊断调度与报警自愈

| 项 | 内容 |
|---|---|
| 版本 | v1.0 评审稿（2026-11-17） |
| 范围 | sub2api-plus fork；后端 `backend/internal/plus/patrol/`，前端 `frontend/src/plus/patrol/`，迁移 `backend/migrations/264_plus_patrol.sql` |
| 来源 | 现状盘点 5 份；最小可用、风险可控、运营者体验三份方案；41 条代码事实已逐条核对（附录 A） |
| 状态 | 待评审；第 11 节列出需要拍板的问题 |

---

## 0. 一页摘要

现在是管理员盯后台，目标是系统盯后台、管理员看结论。新建旁路模块"巡检中心"。它不重造指标和判定，只做五件事：
1. 定时读取现有的判定结果，包括 ops 告警事件、ops 仪表盘、调度台看板、渠道状态、账号运行时状态和任务心跳。
2. 把信号归并成事件（Incident），一件事只通知一次。
3. AI 为事件写诊断卡和日报。
4. 可逆的问题按剧本自动处理，处理后自己验证，失败自己回滚。
5. 只在真正需要人的时候打断人。

**六条定案**

1. **不另立第三套口径。** 阈值一律引用已有配置：ops 告警规则、`ops_metric_thresholds`、channelstatus Rules、dispatch 检测规则。巡检只补上游没有的部分，包括账号与线路、状态迁移、基线对比、数据新鲜度和到期。
2. **只有"事件"会通知人。** 信号先去重、上卷、抑制，再按影响定级。
3. **AI 只能刹车，不能加油门。** AI 可以写诊断、对候选排序、否决自动动作；不能执行，不能提名候选集以外的目标，也不能提高自治级别。自动动作只由确定性规则触发。
4. **降流量和恢复采用不对称策略。** 自动降流量只用 dispatch 的"账号×分组×模型"软暂停（`hard=false`，带 TTL，可撤销），不碰 accounts 表上的永久状态。恢复则必须先拿到主动探测的正面证据。
5. **每个动作都要走完整链路：** 预演 → 闸门 → 执行 → 分钟级验证 → 自动回滚（带 CAS 校验）→ 三处留痕。
6. **分三期逐级放开自动化。** 每一期都有数据准入门槛，所有自动能力默认关闭。

---

## 1. 背景与问题

### 1.1 管理员现在怎么看后台

| 关注点 | 现在去哪里看 | 方式 |
|---|---|---|
| 整体是否健康 | Ops 仪表盘 `OpsDashboard.vue`（健康分、错误趋势、告警事件卡） | 手动打开，健康分现算、不落库 |
| 哪些上游账号要处理 | 调度台 `/admin/dispatch/board` 的"需要处理"、调用失败多、检测未通过 | 手动打开，结论现算，没有推送 |
| 用户可见的分组状态 | 渠道状态页 `/plus/channel-status` | 只算 visible 分组；兜底探测只在有人打开页面时才跑 |
| 账号被限流、冷却或禁用 | 账号列表按 `rate_limited / temp_unschedulable / error` 筛选 | 手动筛，原因只能看 `temp_unschedulable_reason` 最近一条 |
| 告警 | ops 告警规则，邮件通知 | 只有邮件；没有 ack、升级和恢复通知（`include_resolved_alerts` 存了但没人读） |
| 系统昨晚做了什么 | 基本无从得知 | 内置的十余种冷却和禁用只写 accounts 列和 slog，几乎不写 audit_log |

### 1.2 痛点

1. **信息分散，口径不一。** 至少 4 个页面。channelstatus 和 dispatch 对用户侧错误的排除条件不同（前者用 AND，后者用 OR），同一时段可能给出互相矛盾的结论。
2. **只能被动查看。** 调度台和渠道状态没有任何推送。ops 告警只有邮件，没有 IM。
3. **告警逐条到达，没有合并。** 一次上游故障会触发 N 条，没有去重、上卷、静默列表或升级。
4. **状态不落库，事后无法回看。** 健康分、分组状态、可用性都是现算的，"昨晚 2 点发生了什么"答不上来。
5. **处置靠手工。** 调度台的暂停和恢复要人去点。永久 error 的账号，除非事先建了 scheduled test plan，否则不会自动恢复。检测结论不会联动任何动作。
6. **夜间没有覆盖**，巡检本身挂了也没人知道。

### 1.3 机会

积木基本齐全：16 种 ops 指标和告警评估器、dispatch 的账号×模型健康与可逆权重规则（hook 已接入选号热路径）、channelstatus 统一打分、上游 `RecoverAccountState` 与 scheduled test 的 auto_recover、`iq.StreamClient` 多协议 LLM 客户端、audit_logs 的 system 写法。缺的是把它们串起来的编排层、通知通道和安全闸门。

---

## 2. 目标、非目标与成功指标

### 2.1 目标
- 管理员不必定时打开后台：每天 09:00 看晨报，白天只被少量高价值事件打断，夜间只有 P0 才会叫醒人。
- 每条推送都写明四件事：发生了什么、证据、系统做了什么、需要你做什么，并附带处理入口。
- 可逆问题由系统按剧本处理。所有动作都留记录，可验证、可回滚、可追责。

### 2.2 非目标
- 不做工单系统：没有指派、SLA 或值班轮换（与反馈中心"流程轻"的取向一致）。
- 不替代上游 Ops 仪表盘，不改上游告警评估器的逻辑（只加一个回调挂点），不新增上游 metric_type。
- 系统不提供以下执行器：删除账号、修改凭证、永久禁用（SetError 或 `schedulable=false`）、改余额/订阅/价格、改全局 settings（冷却阈值、熔断参数）、关闭 dispatch `apply` 总开关、硬暂停或整组暂停、清审计。
- AI 不进入请求热路径，不持有写工具，不使用 MCP 写权限。
- 不做跨站点或多集群。

### 2.3 成功指标

| 维度 | 指标 | 目标 |
|---|---|---|
| 少打扰 | 日均 IM 推送事件数（平台故障日除外） | ≤ 5 |
| 推送值得看 | 推送中被认领或处理、未被标为噪声的比例 | ≥ 60%（P2 结束时 ≥ 70%） |
| 晨报 | 读完用时；待办条数 | ≤ 5 分钟；≤ 5 条，每条可一键跳转 |
| 夜间 | 非 P0 的 @ 提醒次数 | 0 |
| 发现快 | P0 从发生到推送；ops 规则类；账号×模型故障 | ≤ 2 分钟；≤ 2 分钟；≤ 15 分钟 |
| 自愈可信 | 自动动作的验证通过率 / 回滚率 / 人工推翻率 | ≥ 80% / ≤ 10% / ≤ 5% |
| 自愈安全 | 自动动作导致"分组无号可用"或 `Next.Dead` 的次数 | 0 |
| AI | 诊断卡"有用"反馈率；Schema 与证据校验通过率；日成本 | ≥ 70%；≥ 95%；不超过预算 |
| 自身可靠 | 晨报按时送达率 | 100%（晨报没到即视为异常） |

---

## 3. 现状能力复用表

| 能力 | 已有实现（代码位置） | 复用方式 | 缺口 → 新建 |
|---|---|---|---|
| 全局与分组指标告警 | `OpsAlertEvaluatorService` → `ops_alert_events`（service/ops_alert_evaluator_service.go、ops_alerts.go、repository/ops_repo_alerts.go） | 订阅：P0 按 id 游标轮询，P1 改为 `[plus]` 回调 | 没有 ack、升级、恢复通知、IM → 由事件中心和通知补齐 |
| 阈值口径 | `ops_metric_thresholds`、`ops_alert_rules`（service/ops_settings.go） | 直接引用；"推荐规则包"一键创建到上游 | — |
| 系统指标与心跳 | `ops_system_metrics`、`ops_job_heartbeats`（OpsRepository.GetLatestSystemMetrics / ListJobHeartbeats） | 只读；patrol 自己也 upsert 心跳 `job_name=plus_patrol`，自动计入上游健康分的任务项 | — |
| 健康分 | `computeDashboardHealthScore`（service/ops_health_score.go） | 复用算法，经 `GetDashboardOverview` 读取 | 没有历史 → `plus_patrol_series` 每 5 分钟落库 |
| 分组可用性与容量 | `OpsService.GetAccountAvailabilityStats`、`GetConcurrencyStats`（ops_account_availability.go、ops_concurrency.go） | 只读；口径与 `IsSchedulable` 一致 | 没有历史 → 写入 series；GetConcurrencyStats 需要 ops 监控开启，否则闸门 fail-closed |
| 分组状态 | channelstatus Rules / DefaultScorer / ThresholdClassifier；私有的 `currentVerdicts`（plus/channelstatus/service.go） | 新增公开方法 `Verdicts(ctx, window)`，覆盖全部活跃分组（Snapshot 只算 visible 分组，且在分组被动样本不足、探测已启用时会顺带触发兜底探测） | 状态迁移不落库 → 由巡检记录边沿 |
| 分组主动探测 | `Prober.RunNow`（plus/channelstatus/prober.go） | 新增 `ProbeGroup(ctx, groupID)`，用已保存的配置调用 | RunNow 会绕过 DailyLimit → 巡检自带次数上限 |
| 账号×模型健康 | dispatch `Board`、`Health`、`FailModels`、`Verdict`（plus/dispatch/service_board.go、verdict.go） | 进程内调用（`dispatch.Register` 改为返回 `*Service`） | 没有事件流 → 巡检做边沿检测 |
| 主动复测 | dispatch `Retest/EnqueueOne`（只支持 Direct：apikey/upstream，且已配 api_key 和可用 base_url）；上游 `AccountTestService.RunTestBackground` | Direct 走 dispatch；非 Direct 走新 bridge（注意副作用，见 H2） | 复测结果没有回调 → 把 `Executor.OnFinish` 改造为 `func(Run)` |
| 流量调度执行 | dispatch `SheetOpen/SheetPreview/SheetSave/Restore/Undo` + `hook.Engine`（service_weight.go、hook/engine.go） | 作为软暂停执行器 | 没有 TTL、SheetSave 丢弃 record id、Undo 不检查当前状态、写入没有并发保护、Engine 没有计数 → 见 §7.3 |
| 调整效果 | `effect.go`（纯函数，小时级） | 用于复盘和 D6 巡检项 | 粒度太粗 → 新建分钟级 `verify.go` |
| 账号运行时恢复 | `RateLimitService.RecoverAccountState / ClearTempUnschedulable`；scheduled_test 的 auto_recover（经 `RecoverAccountAfterSuccessfulTest`，空 options；管理端 recover-state 传的是 `InvalidateToken: true`） | 经新 bridge 调用 | 没有"只清 temp_unsched"的细粒度接口 → 新建 `PlusClearTempUnschedOnly` |
| 账号阻断原因 | `TempUnschedState`（matched_keyword）；`notifyAccountSchedulingBlocked`（只在内存里，ratelimit_service.go:147/154） | P0 对 accounts 表做差分；P1 在 `[plus]` 回调里拿结构化 reason 码 | 没有状态历史 → 记入 `plus_patrol_signal` |
| 审计 | audit_logs（参照 `openai_quota_auto_reset.go:785` 的 system 写法）；`plus_dispatch_audit`（`operator()` 在 Email 非空时直接取 Email） | 双写：dispatch 操作人为 `system:patrol`；账号级动作经 bridge 写 audit_logs | plus 路由没挂上游审计中间件 → 人工确认时显式写一条 |
| LLM 调用 | `iq.StreamClient`（anthropic/openai_chat/openai_responses/gemini）、KeyStore | AI 诊断 | 预算、校验和兜底均需新建 |
| 后台任务模式 | OpsMetricsCollector 的选主、心跳和 interval 模式；dispatch 每 6 小时清理一次的写法 | 照搬 | plus 拿不到 Redis → 改用 DB 租约 |
| 邮件 | `NotificationEmailService` 优先；只有模板错误或配置错误时才回退到 `EmailService.SendEmail`，SMTP 投递失败不回退（notification_email_service.go `shouldFallbackNotificationEmail`） | 经 bridge 复用，回退逻辑与 ops 告警一致 | 投递失败由 patrol 自己重试，并写 notify_log |
| IM 通道 | 没有（钉钉代码只是 OAuth 登录） | — | 新建 Notifier：钉钉、飞书、企微、通用 webhook |
| 静默 | `ops_alert_silences`：只能创建；platform、group_id、region 必须与规则 filters 完全相等才命中；命中后只是不建 firing 事件。另有一套运行时静默 `OpsAlertRuntimeSettings.Silencing`，只抑制邮件 | 不复用 | 新建 `plus_patrol_silence`，支持列表、删除、提前结束 |
| 入口拒绝 | `ops_ingress_reject_aggregates` | 只读 | — |
| MCP 管理面 | `managementmcp/catalog.go`（声明式） | 后续以只读方式暴露巡检 | — |
| 报告 | `OpsScheduledReportService`：只发邮件，固定 4 种报表（daily/weekly/error_digest/account_health），正文 HTML 由代码生成，外层用可编辑的通知邮件模板 | 不改；patrol 自有报告参照它的 cron 和 last_run 思路 | 结构化输出和 AI 摘要需新建 |

---

## 4. 整体架构

### 4.1 五个组件

| 组件 | 职责 | 关键约束 |
|---|---|---|
| 巡检引擎 `check/` | Check 注册表（"加一行注册一个"，同 dispatch Detector Registry）；每项声明 Name/Interval/Collect，产出 `[]Signal` | 只读；重查询优先走现有缓存（dispatch Snapshotter TTL 1 分钟，channelstatus CachedSource TTL 30 秒） |
| 事件中心 `incident/` | 指纹去重、层级上卷、因果和预期抑制、静默、按影响定级、状态机、时间线 | 只有事件会通知人 |
| AI 诊断与调度 `ai/` | 构建上下文包和脱敏、调用 LLM、Schema 与证据校验、预算、兜底模板 | 异步执行；超时或失败不影响通知和规则动作 |
| 自愈执行器 `heal/` | Playbook 注册表（Match/Propose/Guard/Verify/Rollback）、闸门链、台账状态机、分钟级 Verifier、TTL Reaper | 动作只能来自白名单执行器；fail-closed |
| 通知 `notify/` | Notifier 接口、合并、限速、夜间策略、重复提醒、升级、恢复通知、报告投递 | 投递记录和限速状态落库，重启不丢 |

### 4.2 数据流

```
┌──────────────────────── 数据源（全部是现有能力，只读）────────────────────────┐
│ ops_alert_events · OpsService(overview/可用性/并发/心跳/system metrics)        │
│ dispatch.Service(Board/Verdict/Changes) · channelstatus.Verdicts · probe_state │
│ accounts 差分 / [plus] 阻断回调 · watermarks · ingress_reject · sink health    │
└──────────────────────────────────┬────────────────────────────────────────────┘
                                   ▼   （只有持有 DB 租约的 leader 执行）
┌───────────── 巡检引擎 check/：Collect() → Signal（plus_patrol_signal）───────────┐
└──────────────────────────────────┬────────────────────────────────────────────┘
                                   ▼
┌───────────── 事件中心 incident/ ──────────────────────────────────────────────┐
│ 指纹去重 → 层级上卷(平台>分组>账号>账号×模型) → 因果/预期抑制 → 静默           │
│ → 按影响定级 → 状态机 + 时间线（plus_patrol_incident / _timeline）             │
└──────┬───────────────────────────┬──────────────────────────────┬─────────────┘
       │ 异步                       │                              │
       ▼                           ▼                              ▼
┌─ AI 诊断 ai/ ────────┐   ┌─ 自愈 heal/ ─────────────────────┐   ┌─ 通知 notify/ ──────┐
│上下文包(脱敏,≤8k)    │   │Playbook.Match → 候选(规则产出,带ID)│   │确认期/合并/限速      │
│→ LLM → Schema/证据校验│──▶│→ Guard 闸链 → 定级 L0/L1/L2/影子  │──▶│夜间策略/重复/升级    │
│→ 诊断卡/排序/veto     │veto│→ 执行 → Verifier → 生效/自动回滚 │   │钉钉/飞书/企微/webhook│
│(失败→规则模板兜底)    │排序│→ 台账 plus_patrol_action          │   │/邮件 → notify_log    │
└──────────────────────┘   └──────────────┬───────────────────┘   └─────────────────────┘
                                          ▼ 执行器（全部是现有代码，经包装）
   dispatch: SheetPreview / SheetSave / UndoIfUnchanged / Retest(+Wake)   channelstatus: ProbeGroup
   bridge: PlusRunAccountTest / PlusRecoverAccount / PlusClearTempUnschedOnly / PlusAuditRecord / PlusSendEmail
   ops: CreateAlertRule / UpdateAlertRule
```

### 4.3 后端目录

```
backend/internal/plus/patrol/
├── module.go        Register(db, deps, admin)：装配，租约选主，启动循环，挂路由
├── config.go        单行 JSON + version 乐观锁（写法同 dispatch/channelstatus）
├── lease.go         plus_patrol_lease：TTL 90s，每 30s 续约
├── domain.go        Signal / Incident / Action / Severity / Level / Subject
├── check/           Registry + 各巡检项（ops_alert / ops_overview / availability / accounts /
│                    dispatch / channel / heartbeat / freshness / expiry / ingress / noise）
├── incident/        correlate.go（指纹、上卷、抑制）、state.go（状态机）、timeline.go
├── ai/              bundle.go（上下文与脱敏）、llm.go（iq.StreamClient）、schema.go、budget.go
├── heal/            playbook*.go、guard.go、policy.go（定级）、ledger.go、verify.go、reaper.go、
│                    exec_dispatch.go、exec_account.go、exec_probe.go、exec_ops.go
├── notify/          notifier.go、dingtalk.go、feishu.go、wecom.go、webhook.go、email.go、
│                    batcher.go、escalate.go
├── report.go        晨报 / 夜间预告 / 周报 / 事件复盘（DB claim 调度，last_run 落库）
├── repo_*.go        裸 SQL（plus 约定：取 service.PlusSQLDB 的 *sql.DB）
└── handler.go       /api/v1/plus/admin/patrol/*
```

### 4.4 运行模型
- **选主：** 每个实例都会 Register，但只有持有 `plus_patrol_lease` 的实例跑巡检、通知和自愈；API 由任何实例处理。之所以用 DB 租约，是因为 plus 模块拿不到 Redis，而 PG 会话级 advisory lock 需要独占连接，不适合 `*sql.DB` 连接池。
- **状态持久化：** 边沿检测需要的上一轮集合和连续命中计数存在 `plus_patrol_state`，通知限速和去重存在 `notify_log`，重启或换主都不丢。ops 评估器的 ruleStates（只有连续超阈计数）在内存里，重启或换主后清零，必须重新累计 sustained_minutes；patrol 不重复这个问题。
- **停机：** plus 后台任务用 `context.Background()` 启动，不参与优雅停机。对策是动作先落库再执行（幂等键唯一）；新 leader 上任时对账"执行中"和"TTL 已到期"的动作（§7.3）。
- **心跳：** 每轮 upsert `ops_job_heartbeats(job_name=plus_patrol)`；外部存活探针见 §8.9。

### 4.5 上游接触点（全部标 `// [plus]`，并写入 `docs-fork/upstream-sync.md` 的检查清单）

| # | 位置 | 改动 | 期 |
|---|---|---|---|
| U1 | `server/router.go:140` | `plus.RegisterRoutes(...)` 多传 `opsService`（作用域里已有；为 nil 时对应巡检项自动跳过） | P0 |
| U2 | 新文件 `service/plus_patrol_bridge.go` | 用包级 atomic 指针导出 `PlusSendEmail`、`PlusRunAccountTest`、`PlusRecoverAccount`、`PlusClearTempUnschedOnly`、`PlusAuditRecord`；未登记时返回"不可用"，调用方降级，不允许 panic | P0 只做邮件，P1 做其余 |
| U3 | EmailService、RateLimitService、AuditLogService、AccountTestService 的 provider | 各加一行，把实例登记给 bridge。若 provider 是手写的则不用重新生成 wire；否则执行 `go generate ./cmd/server` | P0 只登记邮件，P1 登记其余 |
| U4 | `ops_alert_evaluator_service.go`：CreateAlertEvent 之后、自动 resolve 之后 | `plusOpsAlertHook(rule, event, status)`，只入队，不阻塞 | P1 |
| U5 | `ratelimit_service.go:147/154`：notifyAccountSchedulingBlocked 和 Cleared | `plusAccountBlockHook(accountID, until, reason)`，拿到结构化 reason 码。**只覆盖 OpenAI 账号**：BlockAccountScheduling 对非 OpenAI 账号直接 return；runtimeBlocker 为 nil 时也不触发。其他平台继续用 accounts 差分；如需全平台覆盖，要在 `SetTempUnschedulable` / `SetError` 的调用点另加挂点（待评估） | P1 |
| U6 | 前端 `router/index.ts`、`AppSidebar.vue` | `[plus]` import 行 | P0 |

账号级写操作一律走 bridge，禁止用裸 SQL 改 accounts，否则会绕过 scheduler outbox、快照同步、temp_unsched Redis 缓存和 AccountRuntimeBlocker，造成调度不一致。

### 4.6 plus 内部改动

| 模块 | 改动 | 期 |
|---|---|---|
| `plus_routes.go` | `registerDispatch` 和 `registerChannelStatus` 改为返回 `*Service`；新增 `registerPatrol(...)` | P0 |
| dispatch | `Register` 返回 `*Service`，并暴露 `Scheduler.Wake` | P0 |
| dispatch | `SheetSave` 返回 record id（目前 service_weight.go:290 写的是 `_, err := s.Rules.ApplyRecord(...)`，把 id 丢掉了） | P0 |
| dispatch | 新增 `UndoIfUnchanged(recordID)`；`ApplyRecord` 事务里加 `pg_advisory_xact_lock`，对涉及的键 `SELECT … FOR UPDATE`，与期望的 From 不一致时返回 `ErrConflict` | P1 |
| dispatch | `Executor.OnFinish` 从 `func()` 改为可组合的 `func(Run)` | P1 |
| dispatch | `hook.Engine` 加原子计数（暂停剔除、抽签命中、名单被清空），保持零 IO | P1 |
| dispatch | Record/Audit 加 `source` 列（human/patrol），迁移 265 | P2 |
| channelstatus | 公开 `Verdicts(ctx, window)`（覆盖全部活跃分组）、`ProbeGroup(ctx, groupID)` | P0 |

### 4.7 数据表（264_plus_patrol.sql）

| 表 | 主要字段 | 保留期 |
|---|---|---|
| plus_patrol_config | id=1, data JSONB（阈值、通道、AI、各动作模式、值班人、夜间时段、报告时间、灰度分组）, version | — |
| plus_patrol_lease | name PK, holder, until | — |
| plus_patrol_state | check_name PK, data JSONB, updated_at | — |
| plus_patrol_signal | id, source, kind, subject, severity, value, threshold, fingerprint, evidence JSONB, created_at | 14 天 |
| plus_patrol_incident | id, parent_id, fingerprint, kind, subject, scope JSONB, severity, status, impact JSONB, first_seen, last_seen, acked_by/at, resolved_by/at, snooze_until, occurrences, flap_count, noise, source_ref（ops_alert_event_id）, ai JSONB, notify_count, last_notified_at；**部分唯一索引**：(fingerprint) WHERE status 未关闭 | 90 天 |
| plus_patrol_timeline | id, incident_id, kind, actor, text, ref JSONB, created_at | 90 天 |
| plus_patrol_action | id, incident_id, playbook, candidate_id, source(rule/ai/human), level, target JSONB, params JSONB, status, evidence JSONB, guard_result JSONB, preview JSONB, dispatch_record_id, expires_at, verify_due_at, verify_result JSONB, rollback_of, approver_id, ai_call_id, idem_key UNIQUE, cost_tokens, created_at/updated_at | 90 天 |
| plus_patrol_silence | id, matcher（subject 前缀/kind/severity）, mode(notify_only/notify_and_action), until, reason, created_by, ended_at | 90 天 |
| plus_patrol_notify_log | id, incident_id, channel, kind, status, error, sent_at | 30 天 |
| plus_patrol_report | id, kind(daily/night/weekly/postmortem), period, data JSONB, ai_text, sent_at | 90 天 |
| plus_patrol_series | bucket_start, subject, health_score, status, available, total, capacity_headroom, req, err_rate, r429, r529 | 90 天 |
| plus_patrol_ai_call | id, incident_id, purpose, model, prompt_hash, bundle JSONB, raw_output, parsed JSONB, valid, reject_reason, tokens, latency_ms | 原文 30 天 |

清理任务每 6 小时一次，写法同 dispatch 的 prune。

---

## 5. 巡检项清单

规则：
- 所有新增阈值都放在 `plus_patrol_config`，页面上可以修改，旁边注明来源（例如"沿用渠道状态规则 → 去修改"）。
- 上游已有同指标且已启用的告警规则时，B1 这类重复巡检项自动让位给 B6。
- 口径约定：**分组级结论只用 channelstatus，账号级结论只用 dispatch**，事件单上标明数据来源。

### A. 基础设施与巡检自身

| 编号 | 巡检项 | 指标与判定 | 数据源 | 默认阈值（来源） | 频率 | 级别 |
|---|---|---|---|---|---|---|
| A1 | DB/Redis 探活 | db_ok / redis_ok 连续 2 次为 false | `ops_system_metrics` ← GetLatestSystemMetrics（service/ops_metrics_collector.go） | 布尔（健康分存储项，ops_health_score.go） | 1 分钟 | P0 |
| A2 | CPU / 内存 | 超阈值持续 10 分钟 | 同上 | CPU >80%、内存 >85%（健康分扣分起点）；≥95% 升为 P1 | 1 分钟 | P2 |
| A3 | 后台任务停摆 | last_error_at 晚于 last_success_at，或距 last_success_at 超过 15 分钟 | `ops_job_heartbeats` ← ListJobHeartbeats | 15 分钟（健康分任务项口径） | 1 分钟 | P1；`ops_alert_evaluator` 或 `ops_metrics_collector` 停摆为 **P0**，因为此时告警本身已失效 |
| A4 | 数据新鲜度 | 聚合水位滞后 | `channel_monitor_v2_watermarks.data_through`（repository/channel_monitor_v2_aggregation.go）；dispatch traffic 最近刷新时间（plus/dispatch/traffic.go） | 滞后 >5 分钟（新默认） | 1 分钟 | P2；**滞后期间关闭全部自动动作** |
| A5 | 系统日志 sink | dropped_count 或 write_failed_count 增量 >0，或队列占用 >80% | `/admin/ops/system-logs/health`（service/ops_system_log_sink.go） | 新默认 | 5 分钟 | P2 |
| A6 | 巡检自身 | patrol 心跳；TTL 已到期但未处理的动作；AI 预算使用率；闸门拒绝率 | ops_job_heartbeats(plus_patrol)、plus_patrol_action | 心跳同 A3；预算 >80% 提醒 | 1 分钟 | P1；外部兜底见 §8.9 |

### B. 全局业务

| 编号 | 巡检项 | 指标与判定 | 数据源 | 默认阈值（来源） | 频率 | 级别 |
|---|---|---|---|---|---|---|
| B1 | SLA / 错误率 / 上游错误率 / TTFT p99 | 15 分钟窗口，先算整体，再按 platform 算 | `GetDashboardOverview`（QueryModeRaw，service/ops_dashboard.go） | `ops_metric_thresholds`：sla_percent_min、request_error_rate_percent_max、upstream_error_rate_percent_max、ttft_p99_ms_max（与仪表盘标红同口径） | 整体 1 分钟；按平台 5 分钟 | 整体 SLA 低于阈值为 P0；单平台为 P1；TTFT 为 P2 |
| B2 | 健康分 | <60 持续 10 分钟；<80 进日报 | overview.health_score（ops_health_score.go），写入 `plus_patrol_series` | 60 / 80（新默认，算法复用） | 5 分钟 | P1 / P3 |
| B3 | 429/529 突增 | 达到近 7 天同时段中位数的 3 倍，且每 5 分钟 ≥20 次 | overview.upstream_429_count / upstream_529_count；基线取 series | 新默认；基线不满 7 天时不判 | 5 分钟 | P2 |
| B4 | 流量异常 | 比上周同时段下降 >50%（入口故障），或上升到 3 倍以上（可能被刷） | overview.request_count_total / qps；基线取 series | 新默认 | 5 分钟 | 下降 P1；暴涨 P2 |
| B5 | 队列深度 | 达到基线 3 倍，且持续 2 个窗口 | overview.system_metrics.concurrency_queue_depth | 新默认 | 1 分钟 | P2 |
| B6 | 上游 ops 告警事件 | 新 firing 开单；resolved 时解除 | `ops_alert_events`（service/ops_alerts.go、repository/ops_repo_alerts.go）；P0 用 id 游标并回查已打开事件的 status，P1 改走 U4 回调 | 规则自身的 threshold / sustained / cooldown | 30 秒 → 实时 | 沿用规则 severity |
| B7 | 关键告警规则缺失 | success_rate、upstream_error_rate、account_error_ratio、concurrency_queue_depth、memory_usage_percent、group_available_accounts、proxy_expiring_soon_count 未建或未启用 | `ops_alert_rules` | 模板阈值取 `ops_metric_thresholds` | 每天 | P3（日报，可一键导入 H12） |

### C. 分组层

| 编号 | 巡检项 | 指标与判定 | 数据源 | 默认阈值（来源） | 频率 | 级别 |
|---|---|---|---|---|---|---|
| C1 | 分组状态迁移 | up→warn 持续 2 轮为 P2；→down 为 P1；核心分组 down 为 P0 | channelstatus `Verdicts`（新公开方法，包装 currentVerdicts），覆盖全部活跃分组 | `plus_channel_status_config.rules`（UpMin 80 / WarnMin 50 / MinSample 50）；"核心分组"在配置里勾选 | 1 分钟（判定窗口 JudgeWindowMinutes，配合 CachedSource） | P2 / P1 / P0 |
| C2 | 分组可用账号 | available=0；available ≤1 或比例 <50% | `OpsService.GetAccountAvailabilityStats`（ops_account_availability.go） | 已有 group_available_* 规则就用规则，否则用 0 / 1 / 50% | 1 分钟 | P0 / P1 |
| C3 | 容量余量 | Σ可用账号 max_capacity − current_in_use 低于 20% | `GetConcurrencyStats`（ops_concurrency.go） | 20%（新默认） | 1 分钟 | P2；同时作为闸门输入 |
| C4 | 静默分组 | 状态为 unknown 超过 30 分钟，且历史同时段有流量 | Verdicts + series | 30 分钟 | 15 分钟 | 不通知，触发 H3 主动探测 |
| C5 | 兜底探测失败 | `last_ok=false` 连续出现 | `plus_channel_status_probe_state`（plus/channelstatus/prober.go） | ProbePolicy（RetryMinutes 5） | 5 分钟 | P2 |
| C6 | 单点线路 | 某分组×模型上只有 1 个 Serving 账号 | dispatch lanes（plus/dispatch/lanes.go） | 结构性风险 | 每天 | P3（日报） |

### D. 账号层

| 编号 | 巡检项 | 指标与判定 | 数据源 | 默认阈值（来源） | 频率 | 级别 |
|---|---|---|---|---|---|---|
| D1 | 账号×模型调用失败多 | `FailModels` 命中或 status 为 Down，连续 2 轮 | `dispatch.Service.Board`（plus/dispatch/service_board.go）← plus_dispatch_traffic_hourly | channelstatus Rules（SameAsChannel）；failWindowHours=6 | 2 分钟（与 traffic 刷新对齐） | 按冗余度定 P2 / P1 |
| D1′ | 确认查询（只在生成自愈候选时执行） | 近 15 分钟该账号×分组×模型的失败率，只算 provider 侧失败（5xx、超时、断连），**不含 429** | usage_logs + ops_error_logs，走 (account_id, created_at) 索引 | 失败率 ≥50% 且 ≥20 次（新默认） | 触发时 | — |
| D2 | 模型身份检测不通过 | ProbeTag state=fail 且为 Identity 类，Verdict=bad | dispatch Verdict（verdict.go、detector_fingerprint.go） | 指纹检测项（FingerprintRule）默认值：window=10、min_valid=3、bad_min=3、min_confidence=60%；判分题用 bad_below=3、min_valid=5 | 2 分钟；P1 起改为 OnFinish 实时 | P1（疑似掺假，需人判断） |
| D3 | 检测持续报错 | ProbeTag state=fault | detector.go faultVerdict | `plus_dispatch_config.fault_runs=3` | 2 分钟 | P2 |
| D4 | 账号阻断事件 | 进入 error（永久禁用），或进入临时阻断（temp_unsched / overload / rate_limited） | P0：accounts 表差分（上一轮集合存在 plus_patrol_state）+ temp_unschedulable_reason；P1：U5 回调为 OpenAI 账号提供 reason 码，其他平台仍用差分 | auth_error、token_refresh_non_retryable 一出现即 P1（需要重新授权）；同一账号 1 小时内临时阻断 ≥3 次为"反复冷却" P2；同平台 10 分钟内 ≥3 个账号进入 error，合并为 P0"疑似批量封号或上游故障" | 1 分钟 → 实时 | P1 / P2 / P0 |
| D5 | 冷却风暴 | 分组内处于冷却的账号占比 >30%，连续 2 轮；同一 matched_keyword 15 分钟内影响 ≥3 个账号时合并 | accounts 冷却列 + TempUnschedState.matched_keyword（service/temp_unsched.go） | 30% / 3（新默认） | 1 分钟 | P1 |
| D6 | 调整后效果变差 | worse > better，或 dead 非空 | dispatch `Changes` → RecordView.effect（plus/dispatch/effect.go） | effect 内置（前后各 ≥30 次请求、变化 >5 个百分点） | 30 分钟 | P2（附"撤销"） |
| D7 | 暂停太久没恢复 | plus_dispatch_rule 中 share=0 持续 >24 小时 | plus_dispatch_rule | 24 小时 | 每天 | P3（日报） |

### E. 风险预告

| 编号 | 巡检项 | 指标与判定 | 数据源 | 默认阈值 | 频率 | 级别 |
|---|---|---|---|---|---|---|
| E1 | 代理到期 | 已过期 / 即将到期 | ProxyRepository（与 ops 指标 proxy_expired_count、proxy_expiring_soon_count 同源） | 上游口径 | 1 小时 | P2 / P3 |
| E2 | 账号到期 | expires_at 在 3 天内 | accounts | 3 天（新默认） | 1 小时 | P3 |
| E3 | 额度与余额 | codex_5h/7d_used_percent ≥90%，或距 account_scheduling_thresholds ≤5 个百分点；`{platform}_balance_low` | accounts.extra（service/account_scheduling_threshold_eval.go） | 复用平台阈值 | 15 分钟 | P3（进夜间预告） |

### F. 安全与 G. 噪声治理

| 编号 | 巡检项 | 指标与判定 | 数据源 | 默认阈值 | 频率 | 级别 |
|---|---|---|---|---|---|---|
| F1 | 入口拒绝突增 | invalid_api_key / invalid_auth_rate_limited 单 IP 超过 N 次/分钟；聚合溢出 overflowed_count 增量 >0 | 计数取 `ops_ingress_reject_aggregates` 表；overflowed_count **不在表里**，是内存健康结构 `OpsIngressRejectHealth` 的字段，在表里只体现为维度全为 other、client_ip=0.0.0.0 的聚合行（service/ops_ingress_reject.go） | N=60（新默认） | 5 分钟 | P2，只通知 |
| G1 | 告警噪声 | 一天触发 ≥5 次且平均 10 分钟内自行恢复；被标为噪声的比例 | 巡检自身统计 | — | 每天 | 降噪建议（L0，H13 一键应用） |

---

## 6. AI 调度

### 6.1 定位：AI 值班员，只诊断、不执行

| AI 负责 | AI 不负责（由确定性代码负责） |
|---|---|
| 归因：根因类别、置信度、证据引用、已排除项 | 是否触发剧本：由 `Playbook.Match` 用规则决定 |
| 在规则圈定的候选集里排序并说明理由 | 生成候选集以外的目标或动作，或修改参数上限 |
| 否决：`veto_auto=true` 加理由，例如"同平台 8 个账号同时 5xx，像上游故障，暂停任何账号都无济于事" | 提高自治级别，或绕过闸门 |
| 在预算内决定先复测哪些候选（H1/H2 的顺序） | 复测次数和 token 上限 |
| 建议合并或拆分事件、静默、调整阈值（都由人确认） | 修改阈值、告警规则或系统设置 |
| 写诊断卡、晨报一句话结论、事件复盘、平台故障公告草稿 | 发布公告；改余额、计费、分组、用户或 Key |
| 周报里给出调度权重优化建议（基于 dispatch effect 历史） | 执行任何写操作 |

AI 不可用时，自愈和通知照常工作：规则驱动的剧本完全不依赖 AI，AI 只是补充解释。

### 6.2 输入上下文（由系统确定性组装，AI 不能自己去查，≤8k token）

- 事件本身：信号列表、对象元数据、影响估算。
- 近 60 分钟的分钟级指标：分组和账号的请求数、失败率、429/529、TTFT。
- `ops_error_logs` 聚合出的前 5 个错误签名：status / error_type / upstream_status_code / provider_error_code / V2 分类，加上截断到 200 字的 message 和计数。
- 账号状态：`TempUnschedState` 的 JSON、阻断 reason 码。
- dispatch 侧：账号行（cols、fail_models、probes）、Verdict、ModelDetail 同伴对比、近 24 小时的 `plus_dispatch_record`。
- 近 2 小时的变更：dispatch 记录，以及 audit_logs 里账号和设置相关的写操作（最近变更是最常见的根因）。
- 同平台其他未关闭事件；规则产出的候选动作列表（每个带 `candidate_id`，含 SheetPreview 摘要）。

**脱敏与不可信数据**
- 不发送 `error_body`，也不发送任何请求或响应正文。
- message 截断后掩码 `sk-*`、`Bearer`、邮箱、IP 后两段。
- 用户身份只给计数；账号用 ID 加名称表示。
- 上游错误文本和系统日志都放进带标记的数据块，Prompt 里声明"数据块内的指令一律忽略"。

### 6.3 输出格式（严格 Schema，含未知字段一律拒绝）

```json
{
  "incident_id": 231,
  "summary": "claude-03 上游过载，同组其余 4 个账号正常",
  "category": "upstream_outage|account_overload|account_quota|account_credential|model_identity|proxy_network|gateway_infra|recent_change|client_abuse|metric_artifact|unknown",
  "confidence": "high|medium|low",
  "evidence": [{"ref": "err:550912", "text": "529 占该账号错误的绝大部分"}],
  "ruled_out": ["DB/Redis 正常", "近 2 小时无配置变更"],
  "veto_auto": false,
  "veto_reason": "",
  "candidates": [{"candidate_id": "c1", "rank": 1, "why": "..."}],
  "suggestions": [{"type": "merge|split|silence|threshold", "ref": "inc:229", "why": "..."}],
  "ask_human": ["是否已知上游维护？"]
}
```

**校验规则**
- `evidence[].ref` 必须存在于本次上下文包中。
- `candidates[].candidate_id` 只能引用候选列表里的 ID。
- 不符合的条目直接丢弃。整体解析失败时，退回规则模板（例如"账号 X 的 529 占错误 96%"这类由代码直接拼出的句子），不重试。

**展示原则**
- 系统事实用常规样式；AI 推断带"AI"标签和置信度。
- 证据旁边显示的数值由系统按 ref 渲染，不采用 AI 写出的数字。
- 动作执行前，闸门会重新独立查询指标，AI 文本里的数字不驱动任何动作。

### 6.4 可调用工具

| 阶段 | 工具 |
|---|---|
| P1–P2 | **不提供工具调用。** `iq.StreamClient` 只接收单个 Prompt、返回文本，本身不支持 function calling。全部上下文由系统预取。 |
| 后续（P3） | 事件页"问 AI"，只开放只读工具：`get_incident`、`get_error_samples(subject, window)`、`get_metric_series(subject, window)`、`get_dispatch_account(id)`、`get_recent_changes(subject)`、`get_account_block_state(id)`。每次会话最多调用 5 次，没有任何写工具。 |
| 对外 | 如果以后把巡检暴露给外部 agent（managementmcp），只授予 `MCPPrincipal.ReadOnly=true`。MCP 写工具里有 `admin_update_user_balance`，这条路必须封死。 |

### 6.5 权限分级

| 级别 | 含义 | 谁可以发起 | 执行条件 |
|---|---|---|---|
| **L0 只读建议** | 只记录、只推送，不动作 | 规则 / AI | 无 |
| **L1 需确认** | 一名管理员在后台点"确认执行"；确认时当场重跑闸门和 SheetPreview | 规则 / AI | 提议 30 分钟内有效，过期作废；必须登录后台，不能在 IM 里直接审批 |
| **L2 自动执行** | 低风险、可逆、有 TTL、有验证，事后汇报 | **只能由确定性规则发起** | 通过全部闸门，在预算内，全局模式为 `auto`，该动作已开启 L2，目标在灰度分组内 |
| **影子模式** | 完整跑完匹配和闸门，只记录"若开启将执行……" | 规则 | 新剧本上线默认处于影子模式 |
| **禁止** | 系统内不存在对应执行器，AI 也无从提议 | — | — |

**定级规则：只会降低自治程度，不会提高**
- 只有 AI 作为依据 → 最高 L1。
- AI 返回 `veto_auto=true` → L2 降为 L1，卡片上标注否决理由，并推送"疑似系统性故障"。
- 目标线路上有人工设置的 share/pause 规则，或目标 24 小时内被人改过 → 最高 L1。
- SheetPreview 返回 `Danger=true` 或 `Next.Dead` 非空 → **拦截**，L1 确认页也拦，只能去调度台人工操作。`Only=true` 时 `hard=false` 的暂停不会生效，因此不生成候选。
- 预演的 `Side` 非空（setShare 会连带调整其他账号）→ 最高 L1。
- 同一目标 24 小时内已被自动处理 2 次（抖动）→ 最高 L1。
- 数据陈旧（A4）、心跳异常（A3）、闸门依赖不可用、存在未关闭的基础设施或平台级事件、处于免动作维护窗口 → L0。
- 熔断已触发（§7.1 第 6 条）→ 全局最高 L1，人工复位后才恢复。
- 全局模式 `heal.mode = off | suggest | confirm | auto`，分别对应：不生成候选 / 最高 L0 / 最高 L1 / 允许 L2。每个动作另有自己的开关，并可设灰度分组名单。

### 6.6 防幻觉与安全
1. **候选集约束：** AI 只能在规则候选里选，无法扩大动作范围。
2. **证据绑定：** ref 必须属于本次上下文包；数值以系统渲染为准。
3. **双信号一致：** L2 自动执行要求被动流量信号和主动探测信号一致，AI 的判断不算一路信号。
4. **提示注入兜底：** 即使注入成功，最坏结果也只是一条错误建议，而且还要经过人审（第 1、3 条保证）。
5. **自指污染：** AI 走本站网关时，使用专用用户和专用 Key，并绑定与主业务不同平台的分组。巡检自己的查询按这个 api_key_id 排除，免得 AI 自己的失败污染巡检数据。
6. **审计：** 每次调用写入 `plus_patrol_ai_call`，包括证据包快照、原始输出、解析结果、是否被否决或丢弃及原因，便于复盘。

### 6.7 成本控制与降级
- **只按事件触发：** 新的 P0/P1 事件、级别上升、事件解决（写复盘）、晨报和周报才调 AI；巡检轮询本身不调。P2 不单独调用，只在晨报里批量调用一次。
- **复用与上限：** 同一指纹 30 分钟内复用上次结论；单个事件最多 2 次调用（开单时一次，解决复盘一次）。
- **预算：** 每小时调用次数上限、每日 token 上限（数值见 §11 开放问题）。超出后降为规则模板，发一条 P3，并在晨报里注明。
- **模型分工：** 诊断默认用小模型，晨报和周报可用大模型。
- **超时：** 普通事件最多等 20 秒，超时就先发不带 AI 的通知，AI 结果之后只更新到事件页。**P0 首推永远不等 AI。**
- **通道：** 在配置里填 endpoint、api_key_id、model 和 format，可选配备用通道（见开放问题）。

---

## 7. 自愈动作

### 7.1 通用闸门（每个动作执行前逐项检查，任何一项不过就降级，并写明原因）

1. **模式与开关：** `heal.mode`、该动作的开关、影子模式、灰度分组名单。dispatch `Config.apply=false` 时暂停规则不会生效，自动暂停没有意义，因此降为 L0。
2. **数据新鲜度：** 证据产生不超过 3 分钟；A4 正常（traffic 刷新 <5 分钟，水位滞后 <5 分钟）；采集器和评估器心跳正常。
3. **系统性故障熔断：** 同平台 ≥30% 的 Serving 账号被标记、同平台 ≥2 个分组异常，或存在未关闭的基础设施事件 → 冻结该平台（或全局）所有 L2。这一条防的是最糟糕的情况：上游整体故障或自家网络故障时把账号一个个停掉，结果雪崩。
4. **内置冷却去重：** 账号已处于 rate_limit_reset_at、overload_until 或 temp_unschedulable_until 冷却中，说明上游机制已在处理，不再叠加动作。
5. **爆炸半径：**
   - 单分组：同时最多 1 个自动暂停；自动暂停的账号不超过该分组 Serving 账号的 25%；动作后至少还有 2 个健康账号。
   - 容量：动作后剩余可用容量 Σmax_capacity 至少为当前 in_use 的 1.5 倍。ops 监控关闭导致取不到数据时，fail-closed。
   - 冻结期：某分组发生一次自动动作后，15 分钟内不再对该组发起新动作（回滚除外）。
   - 全局：每小时 ≤3 个自动动作，同时生效 ≤5 个，每天 ≤20 个（可配置）。
6. **熔断：** 1 小时内出现 2 次回滚或验证失败 → 全局降为 L1，推送 P1 说明原因，人工复位后才恢复。
7. **人工优先：** 目标线路 24 小时内被人改过（dispatch Record 里有非 system 操作人，或 audit_logs 里有相关写操作）→ 最高 L1。处于维护窗口（免动作模式）→ L0。
8. **多实例与幂等：** 只有租约 leader 执行。动作先写台账，`idem_key = incident:playbook:target:attempt` 唯一，然后才执行。

### 7.2 动作清单

执行身份：调用 dispatch 时传 `Actor{ID:0, Email:"system:patrol"}`，Note 写 `patrol#<incident> ttl=30m`。人工确认的动作，actor 记为该管理员。

| ID | 动作 | 触发 | 执行器（真实代码位置 / 新建） | 验证 | 自动回滚 | 默认级别 P0→P1→P2 | 爆炸半径与频率限制 |
|---|---|---|---|---|---|---|---|
| H1 | 复测（Direct 账号） | D1/D2/D3 命中；H4 暂停前确认；H5 到期复测 | 现有 `dispatch.Service.Retest` → EnqueueOne + `Scheduler.Wake`（plus/dispatch/scheduler.go）。结果 P0 轮询 Verdict，P1 起由 `OnFinish(func(Run))` 回传 | 新 Verdict 为 ok 或 bad | 不需要（只读，消耗 token） | L1 → L2 → L2 | 每账号每天 ≤6 次；全局 token 预算；Enqueue 自带去重 |
| H2 | 复测（非 Direct，如 OAuth 账号） | 同 H1，账号类型不在 apikey/upstream 内 | 新 bridge `PlusRunAccountTest` → 现有 `AccountTestService.RunTestBackground`（service/account_test_service.go）。**不走** `/admin/accounts/:id/test`，因为它成功后会在 handler 层顺带调用 `RecoverAccountAfterSuccessfulTest`。**注意：RunTestBackground 不是只读的**，它与管理端测试共用 `TestAccountConnection`，测试过程中可能 401/403 时 SetError、OpenAI 429 时 SetRateLimited（可能附带 ClearError）、Grok 写临时不可调度，还会 UpdateExtra 写额度快照。这些都是"如实反映上游状态"的副作用，可以接受，但要记进台账，H2 也不能当作零风险动作 | status=success | 不需要 | — → L2 → L2 | 同 H1；台账记录测试前后的账号状态差异 |
| H3 | 分组主动探测 | C4 静默分组；C1 事件恢复后确认 | channelstatus 新增 `ProbeGroup` → 现有 `Prober.RunNow`（plus/channelstatus/prober.go） | probe_state 更新且 last_ok=true | 不需要 | L1 → L2 → L2 | RunNow 绕过 DailyLimit，巡检自己限制每组每天 ≤12 次 |
| H4 | **线路软暂停**（账号×分组×模型，TTL 30 分钟） | D1 连续 2 轮 Down，**并且** D1′ 确认（≥20 次、失败率 ≥50%，以 provider 侧为主，排除 429，429 交给内置冷却），**并且** H1/H2 复测失败 | 现有 `SheetOpen` → `SheetPreview` → `SheetSave(mode=pause, hard=false)`（plus/dispatch/service_weight.go）。dispatch Rule 没有 TTL 字段，TTL 由台账 `expires_at` 加 Reaper 实现，不改 dispatch 表 | T+2、5、10 分钟检查五项：①执行 30 秒后起，目标线路新请求≈0（其他实例上 hook 最多 10 秒生效）；②分组错误率下降 ≥50% 或回到 WarnMin 对应水平；③同线路其他账号 429 率、队列深度升幅 ≤5 个百分点；④缓存命中率下降 ≤10 个百分点；⑤P1 起 Engine 暂停剔除计数 >0 | 验证失败 → `UndoIfUnchanged(record_id)`，并开 P1"自愈失败已回滚"。规则已被人改过 → 不回滚，标 `conflict_human` 并通知人 | 影子 → L1 → L2（先灰度分组） | §7.1 全部；Preview 要求 Danger=false、Dead 为空、Only=false、Side 为空；同一目标 24 小时内 ≤2 次 |
| H5 | 到期复测并恢复 | H4 TTL 到期 | H1/H2 复测：Direct 连续 2 次 pass，非 Direct 1 次 success → `UndoIfUnchanged(record_id)`。不通过就续期 30 分钟一次；仍不通过则保持暂停，转人工，事件升为 P1"需要你判断" | 恢复后 10 分钟内账号×模型失败率 <20%（≥10 次） | 恢复后又恶化 → 重新暂停一次（计入防抖）并发 P1，之后不再自动处理 | 影子 → L2 → L2 | 只能撤销本系统自己的记录；**恢复判断只依据主动探测**（暂停后被动样本会低于 MinSample，看起来像"恢复了"） |
| H6 | 金丝雀降权（share=5%） | 账号无法主动测试（H1/H2 都不可用） | 现有 `SheetSave(mode=share, pct=5)`；setShare 会连带调整其他账号，Record.Side 非空 | 30 分钟内承接比例≈5%，分组失败率回落 | `UndoIfUnchanged` | — → L1 → L1 | 同 H4；会让少量真实请求失败，需要业务方接受（开放问题） |
| H7 | 清除白名单内的临时冷却 | 分组可用 ≤1 或余量 <20%；冷却原因在白名单内（openai_apikey_health_breaker、stream_timeout 的 temp_unsched、internal500 第 1 档、529 overload）；剩余冷却 >10 分钟；H2 测试刚成功；没有生效中的模型级限流 | 新 bridge `PlusClearTempUnschedOnly`：只清 temp_unsched，删缓存，调用 notifyCleared，不碰模型限流。现有 `ClearTempUnschedulable` 会连带清掉全部模型限流，不直接使用 | 10 分钟内成功率不低于分组中位数，且没有马上重新进入冷却 | 不可逆，只能补偿：对该账号做 H4 软暂停；内置处理器再报错时会重新冷却 | — → L1 → L2（视 P1 数据决定） | 每账号每天 ≤1 次；**不适用于**带上游重置时间的 429、OpenAI 403、凭证类错误 |
| H8 | 恢复 error 账号（非凭证类） | status=error 持续 ≥30 分钟，原因不是凭证类（例如 internal500 第 3 轮、流超时 action=error；排除 401/403 鉴权和 OpenAI 403 达阈值）；H2 测试成功 | 新 bridge `PlusRecoverAccount` → 现有 `RateLimitService.RecoverAccountAfterSuccessfulTest` → `RecoverAccountState`（空 options，`InvalidateToken=false`，与 scheduled_test auto_recover 同一路径） | 30 分钟内没有再次 SetError | 补偿：H4 暂停 2 小时并发 P1 | — → L1 → L1 | 每账号每天 ≤1 次 |
| H9 | 给 error 账号挂定时探活 | error 超过 1 小时，且没有 scheduled test plan | 新 bridge 封装 ScheduledTestService 的创建（auto_recover=true，每 30 分钟） | 计划有结果写入 | 删除计划 | 后续 L1 | — |
| H10 | 跨分组暂停账号 | D2 身份不符；或账号在 ≥2 个分组都 Down | 现有 `SheetSave`，rows 覆盖该账号全部分组（不改 accounts） | 同 H4 | `UndoIfUnchanged` | — → L1 → L1 | 逐组按 H4 的限制检查 |
| H11 | 撤销一次调度调整 | D6 效果变差 | 现有 `Undo`，外面包 `UndoIfUnchanged` | 下一轮 D6 不再报 | 本身就是回滚 | 人工记录为 L1；本系统记录在 P2 为 L2 | — |
| H12 | 导入推荐告警规则 | B7 | 现有 `OpsService.CreateAlertRule` | 规则能列出来 | 删除规则 | L1 | 同名规则跳过 |
| H13 | 降噪：调整告警规则 | G1 | 现有 `OpsService.UpdateAlertRule`（sustained_minutes、notify_email） | 之后 3 天的触发次数 | 台账保存 before，一键改回 | L1（晨报里一键应用） | — |
| H14 | 静默 / 维护窗口 | 已知上游维护；人或 AI 提议 | 自有 `plus_patrol_silence` | — | 到期自动失效，或提前结束 | L1 | 单次最长 24 小时 |
| H15 | 平台级故障 | 平台级事件 | **不执行动作**；AI 生成公告草稿，人在现有公告页发布 | — | — | L0 | — |
| — | 人工处置入口 | D4 凭证失效、token 刷新失败等 | 事件抽屉跳转现有页面，或调用现有 admin 接口：`/admin/accounts/:id/{recover-state,test,refresh,schedulable}`、重新授权页（经上游审计中间件） | — | — | 人工 | — |

### 7.3 回滚与并发安全（读代码发现的必须修补点）

1. **Undo 不检查当前状态。** `Service.Undo` 直接把 From 写回，自动回滚可能覆盖人工后来的修改。修补：新增 `UndoIfUnchanged`，逐键比较当前规则与 `rec.To`（包括 Side.To），不一致就返回冲突，交给人。
2. **SheetSave 没有版本控制。** 它先 load 再 ApplyRecord，中间没有锁，人工和自动并发写会丢更新。修补：在 ApplyRecord 事务开头加 `pg_advisory_xact_lock`，对涉及的键 `FOR UPDATE`，与期望的 From 比较。人工写入同样受保护。
3. **SheetSave 不会因为 Danger 拦截。** 必须由 patrol 先调 SheetPreview，并且在确认时再预演一次。
4. **台账状态机：** `proposed → approved → executing → active → verifying → succeeded | rolled_back | conflict_human | expired | rejected | failed`；影子记录的状态为 `shadow`。
5. **启动与换主对账：** 新 leader 扫描 `executing` 超过 2 分钟、`active` 且 TTL 已过的条目，用 `ListRules` 对账。规则已不存在就标 `gone`；过期未处理的立即处理。dispatch 返回"没有改动"时视为已执行。
6. **一键止血：** 把 `heal.mode` 设为 off，然后对全部 active 的自动动作逐个执行 `UndoIfUnchanged`，冲突项列出来交给人。

### 7.4 留痕（四处，可以串联查询）
- `plus_patrol_action`：完整时间线，包括证据快照、闸门逐项结果、预演快照、验证结果、回滚原因。
- `plus_dispatch_record` / `plus_dispatch_audit`：操作人为 `system:patrol`，Note 写 `patrol#<id>`，不用改表就能在调度台"操作日志"里区分人工和自动；P2 再补 source 列。
- `audit_logs`：账号级动作和人工确认，经 `PlusAuditRecord` 写入（ActorRole=system、Method=SYSTEM、Action=`plus.patrol.<action>`、Path=`/system/plus/patrol/<id>`），在现有"审计日志"页可以统一查看。
- `plus_patrol_ai_call`：AI 依据。

---

## 8. 告警与降噪

### 8.1 分级：按"影响"定，而不是按"指标"定

冗余度来自 `GetAccountAvailabilityStats` 给出的分组可用数，加上 dispatch `lanes.GroupView` 预计的承接比例。

| 级别 | 定义 | 例子 |
|---|---|---|
| P0 | 用户大面积失败，或基础设施故障，或告警系统本身失效 | DB/Redis 探活失败；整体 SLA 低于阈值；分组可用账号为 0；平台级多分组 down；评估器或采集器停摆 |
| P1 | 单个分组受影响，或必须人处理，或自愈失败或已回滚 | 分组降级；分组只剩 1 个可用账号；凭证失效被永久停用；模型身份检测不通过；自愈已回滚 |
| P2 | 单个账号异常，但线路有冗余，或系统已处理 | 账号反复 529 冷却、已被自动暂停；429 较基线突增 |
| P3 | 风险或信息 | 代理或账号即将到期；余额低；用量 ≥90%；降噪建议 |

升级：P1 持续 30 分钟且影响扩大时升为 P0；P2 演变成线路无冗余时升为 P1。上游 ops 规则的 P0–P3 直接映射过来，只能被影响评估调高，不会调低。

### 8.2 聚合与降噪
1. **指纹去重：** 指纹 = kind 家族 + subject（例如 `dispatch:fail_model:account:345/model:x`）。事件未关闭期间，同一指纹的信号只追加到时间线；只有级别上升时才再推一次。
2. **层级上卷：** 同一分组 10 分钟内出现 ≥3 个账号级事件，就建一个分组级父事件，账号事件作为子项折叠。同一平台 ≥2 个分组异常，就建平台级事件，并禁止该平台的账号级自愈（换号无效）。一个账号属于多个分组时，挂在承接流量最多的分组下，其余分组列为影响范围。
3. **因果抑制：** 基础设施事件（A1/A3/A4）未关闭期间，新的业务信号都挂到它下面，标注"可能由 #N 引起"，不单独通知，也不做账号级动作。
4. **预期状态抑制：** 以下信号只记录，不建事件：人工暂停的（share=0 且操作人不是巡检）、schedulable=false、已过期、在维护窗口内、内置冷却已在处理且线路有冗余（例如 429 带上游重置时间，或 529 冷却后自行恢复）。这类只在晨报里计数，例如"自动冷却 37 次，均已自行恢复"。
5. **确认期与合并窗口：** P0 立即发；P1 需持续 2 分钟才发。同一渠道 60 秒内的多个事件合并成一条消息（钉钉机器人每分钟限 20 条，合并是硬性要求）。
6. **抖动抑制：** 事件解决后 30 分钟内复发，就重开原事件并让 flap+1，不新建。flap ≥3 时标"抖动"，只发一次汇总，后续进晨报，并生成降噪建议。
7. **恢复确认期：** 所有信号消失超过 10 分钟（P2 为 30 分钟），并且没有进行中的动作，才自动解决。
8. **避免双发：** 配置页检测到上游规则 `notify_email=true` 且巡检通知已启用时，提示会重复，并提供一键关闭这些规则的邮件（H13）。巡检里关闭 B6 镜像事件时，同步调用上游 `PUT /admin/ops/alert-events/:id/status manual_resolved`。

### 8.3 事件状态机

```
open ──认领──▶ acked ──有动作──▶ mitigating ──验证中──▶ monitoring ──恢复确认──▶ resolved
  │                │                                         │
  ├──静默──▶ snoozed（到点回到原状态）                         └─验证失败/升级─▶ escalated(→acked)
  └──关闭为噪声──▶ closed_noise        resolved ──30 分钟内复发──▶ 重开（flap+1）
```

### 8.4 通知矩阵与升级

| 级别 | 白天（默认 08:00–23:00） | 夜间 | 未认领时重复提醒 | 升级 |
|---|---|---|---|---|
| P0 | IM 立即发，@值班人，同时发邮件 | 同白天（夜里唯一会叫醒人的级别） | 每 15 分钟一次，最多 3 次 | 3 次后 @全部管理员，并邮件发给全部收件人 |
| P1 | IM，经确认期和合并后发 | 能自愈的：执行后记进晨报；不能自愈且已影响用户的：发消息但不 @（待拍板）；其余：记进晨报 | 每 60 分钟一次 | 持续 30 分钟且影响扩大，升为 P0 |
| P2 | 不推送；值班台和菜单红点提示；可选"整点摘要" | 记进晨报 | 无 | 线路冗余耗尽时升为 P1 |
| P3 | 只进晨报或夜间预告 | 同白天 | 无 | 无 |

后续只推关键变化：自愈失败或回滚、升级、解决。P0/P1 解决时发一行恢复通知，补上上游 `include_resolved_alerts` 没有生效的缺口（该字段在 `OpsEmailNotificationConfig.Alert` 里存着，但没有代码读取）。钉钉机器人消息不能编辑，所以不做"每一步都推"。

值班配置保持轻量：白天值班人和夜间值班人各填一个列表（手机号用于 @，加邮箱），不做轮换。

### 8.5 静默与维护窗口
- `plus_patrol_silence` 可以按 subject 前缀、kind 或级别匹配，支持列表、删除、提前结束，补上上游只能创建的缺口。
- 两种模式：**只免通知**；**免通知且免动作**（维护窗口，避免在别人维护时去自愈）。
- 上游 `ops_alert_silences` 和运行时邮件静默照旧生效，配置页展示两者的语义差异。

### 8.6 渠道与交互
- **通道：** 钉钉自定义机器人（markdown/actionCard，HMAC 加签）、飞书机器人（加签）、企微机器人、通用 JSON webhook（HMAC 签名头）、邮件（经 bridge）。可以配多个通道，每个通道设 min_severity；页面上有"发送测试"。
- **安全：** URL 和 secret 脱敏保存。机器人类型只允许官方域名；通用 webhook 只允许 https 公网地址，用已导出的 `internal/util/urlvalidator`（ValidateHTTPURL / ValidateResolvedIP / IsBlockedHost）做校验。channel_monitor_ssrf.go 里的规则是 service 包私有函数，plus 无法直接使用。不跟随重定向，超时 5 秒。每次投递写 notify_log。
- **一键确认 / 回滚：** 消息里只放深链 `/admin/patrol/incidents/:id?act=ack|snooze|approve|revert`，打开后需要管理员登录态。确认类操作在页面上当场重跑闸门和预演，再二次确认后执行。**IM 里不放可直接执行的签名链接**；是否给"认领 / 静默 ≤2 小时"开放免登录短链，见开放问题。
- **消息模板**（固定四段：影响 / 判断 / 系统已处理 / 你需要）：

```
### [P1] Claude/pro 分组降级  #231
- 影响：近 15 分钟 18% 请求失败，约 40 位用户
- 判断（AI·高）：账号 claude-03 上游 529 过载，同组其余 4 个账号正常
- 系统已处理：在 pro 分组暂停 claude-03 30 分钟（调度台记录 #5012），12:32 复测，通过后自动恢复
- 你需要：无需操作
[查看事件] [认领] [静默1小时] [撤销处理]
```
```
### [P1·需要你处理] 账号 openai-oauth-07 凭证失效  #232
- 影响：openai-plus 分组可用账号 3→2，暂无用户失败
- 原因（系统）：上游 401，已被永久停用（auth_error）
- 系统不能自动处理：需要重新授权
[去重新授权] [认领] [2小时后提醒]
```
```
### [P0] OpenAI 平台大面积失败  #240  @值班人
- 影响：近 5 分钟 3 个 OpenAI 分组错误率 64%
- 判断（AI·中）：6/6 账号同时返回 5xx，错误信息一致，像上游平台故障；换号无效
- 系统未执行任何动作（平台级故障不自愈）
- 建议：确认上游状态页；需要时发布公告（草稿已生成）
[查看事件] [认领] [查看公告草稿]
```

### 8.7 报告
- **晨报**（默认 09:00，必发，同时证明巡检还活着）。IM 发短版（≤12 行），邮件发完整 HTML，站内留档，三者从同一份结构化 `data` 渲染。内容包括：
  - 一句话结论；
  - 需要你处理（≤5 条）；
  - 系统自动处理了什么、效果如何；
  - 指标对比：SLA、错误率、上游错误率、p99 延迟和 TTFT、健康分，与前日和上周同日比较；
  - 风险预告（C6/D7/E 类）；
  - 降噪建议（附"一键应用"）。
- **夜间预告**（可选，21:00）：今晚可能出问题的项，以及今晚的自治级别，例如"今晚系统会自动处理 A/B 类问题，C 类会 @你"。
- **周报：** 自愈质量（准确率、回滚率、MTTR）、噪声排行、调度权重建议。
- **事件复盘：** 事件解决时 AI 写一句复盘，进入晨报和周报。
- 邮件主题固定前缀 `[巡检 #231]`，同一事件在邮件客户端里归成一串。

### 8.8 与上游邮件告警的关系
上游规则照常发邮件。巡检会检测重复并引导关闭；是自动关闭还是只提示，见开放问题。

### 8.9 巡检自身存活
- 心跳写入 `ops_job_heartbeats`，计入上游健康分的任务项。但任务项只占总分约 9%（基础设施占 30%，任务项又占其中 30%），没有流量时健康分直接返回 100，所以**不能**依赖健康分发现巡检挂掉，必须靠下面的外部手段。
- 外部存活探针 `GET /api/v1/plus/patrol/ping`：无鉴权，只返回最近一轮巡检的时间，供外部拨测服务使用。
- 可选的死人开关：每轮向配置好的外部 URL 推送一次心跳。
- 约定：晨报没到即视为异常。

---

## 9. 页面与交互

### 9.1 菜单（新侧边栏分组"巡检中心"，写法同 `dispatchAdminNav`，`expandOnly`）

| 菜单 | 路径 | 内容 |
|---|---|---|
| 值班台 | `/admin/patrol/desk` | 顶部一行状态（"一切正常"，或"2 个进行中事件，1 个等你批准"）和自愈总闸。下面三块 card：**需要你处理**（未认领的 P0/P1 和待批准动作）、**系统正在处理**（自愈中的事件，带倒计时，例如"12:32 复测"）、**今日事件条**（24 小时横轴，颜色表示级别）。右侧是健康分 24 小时曲线（来自 series） |
| 事件 | `/admin/patrol/incidents` | TablePageLayout + DataTable；用 Select 筛选状态、级别、范围、来源、是否自愈；游标分页 |
| 自愈动作 | `/admin/patrol/actions` | 全部动作流水，待批准的置顶；可查看预演快照、闸门逐项结果、验证结果、回滚记录 |
| 巡检报告 | `/admin/patrol/reports` | 晨报、周报、复盘列表；详情与邮件同源渲染；支持重新发送和测试发送 |
| 巡检配置 | `/admin/patrol/settings` | 两栏 grid 卡片：巡检项与阈值（标注来源，可跳转修改）、自愈剧本（各动作级别、灰度分组、预算、全局模式）、通知（通道、值班人、夜间时段、发送测试）、AI（通道、模型、预算）、静默与维护窗口 |

菜单红点复用现有 NavBadge，数值为未认领 P0/P1 数加上待批准动作数。

### 9.2 事件详情（核心页面）

```
#231 [P1 · 自愈中] Claude / 分组 pro 降级          持续 23 分钟 · 约 18% 请求受影响 · 约 40 位用户
[认领] [静默…] [批准/拒绝动作] [撤销自愈] [关闭为噪声] [备注]
┌─ 时间线（左，占 2/3）───────────────────────────────┐ ┌─ AI 诊断卡（右上）──────────────┐
│12:01 信号 渠道状态：pro 正常→降级（健康分 52）      │ │[AI·高] claude-03 上游过载，      │
│12:01 信号 账号 claude-03：529 冷却（1 小时内第 3 次）│ │同组其余 4 个账号正常             │
│12:02 合并 同组 3 条信号并入本事件                   │ │证据：①err:550912 ②run:88123     │
│12:02 AI   诊断完成 [展开证据]                       │ │已排除：DB/Redis 正常；近 2 小时  │
│12:02 提案 在 pro 暂停 claude-03 30 分钟              │ │  无配置变更                       │
│        预演：其余账号负载 62%→78%，无线路断流        │ │需要人判断：无                     │
│12:02 执行 L1 已确认（admin@…）· 调度台记录 #5012     │ │[诊断有用] [诊断不对]              │
│12:08 验证 分组 5 分钟错误率 18.2%→1.1%；剔除 312 次 ✓│ ├─ 影响范围 / 相关对象（右下）─────┤
│12:32 到期 复测 2/2 通过 → 撤销 #5012                 │ │分组 pro、平台 anthropic          │
│12:42 解决 恢复后 10 分钟错误率 0.9%                  │ │→ 调度台账号页 → Ops 错误(已带筛选)│
└────────────────────────────────────────────────────┘ │→ 渠道状态 → 检测记录 run#88123   │
                                                         └─────────────────────────────────┘
```

- **时间线条目类型：** signal / merge / severity / ai / proposal / approve / execute / verify / rollback / notify / ack / silence / comment / resolve / reopen。
- **跳转：** 每条证据都能点开，跳到已带好筛选条件的现有页面。例如 Ops 错误日志预填 account_id、group_id 和时间窗；调度台跳到 `/accounts/:id/model`。
- **确认动作弹窗：** 展示闸门逐项结果、SheetPreview（各账号承接比例前后对比、Danger/Dead）、预计转移流量（req/min）、动作后的容量余量、TTL 与回滚方式。被拦截时写明原因，不静默禁用按钮。

### 9.3 遵循项目约定
- 复用 TablePageLayout、DataTable、Select、card、badge；不加 max-w 容器；加载失败时显示"重试"；禁用的按钮要说明原因。
- 目录 `frontend/src/plus/patrol/{routes.ts, nav.ts, api.ts, views/, components/, __tests__/}`，在 `router/index.ts` 和 `AppSidebar.vue` 里各加一行 `[plus]`。
- 所有新增文案都要补齐全部 locale，否则 `pnpm build` 的 i18n 检查不通过。

### 9.4 接口（`/api/v1/plus/admin/patrol/*`，adminAuth）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `overview`、`badges`、`series` | 值班台数据、红点、健康分序列 |
| GET | `incidents`、`incidents/:id` | 列表（筛选加游标）；详情含时间线、AI 卡、动作 |
| POST | `incidents/:id/{ack,snooze,resolve,close-noise,comment,analyze}` | 事件操作；镜像单同步上游状态 |
| POST | `incidents/:id/actions/preview` | 传入 candidate_id，返回闸门结果和 SheetPreview |
| POST | `incidents/:id/actions` | 确认执行，当场重跑闸门 |
| GET / POST | `actions`、`actions/:id/rollback`、`heal/stop` | 动作流水、手动回滚、一键止血 |
| GET / PUT | `config` | 带 version 乐观锁，冲突返回 409 |
| POST | `channels/test` | 发送测试 |
| GET / POST / DELETE | `silences`、`silences/:id/end` | 静默管理 |
| GET / POST | `reports`、`reports/:id`、`reports/run`、`reports/:id/resend` | 报告 |
| GET | `/api/v1/plus/patrol/ping`（无鉴权） | 只返回最近一轮时间 |

人工确认操作经 `PlusAuditRecord` 显式写入 audit_logs，因为 plus 路由没挂上游审计中间件。

---

## 10. 分期计划

默认配置下所有自动化都关闭，需要管理员显式打开。级别矩阵和白名单的每次修改都写审计。

### P0（第 1–2 周）：看得见、少打扰，不做自动写动作

**交付物**
- 后端：
  - 迁移 264；租约、配置、Check 注册表，覆盖 A1–A6、B1–B7、C1–C6、D1–D7（D4 用差分实现）、E1–E3、F1；
  - 事件中心：指纹、上卷、抑制、静默、状态机、时间线；
  - 通知：钉钉、通用 webhook、邮件 bridge，包括合并、限速、夜间策略、重复提醒、恢复通知；
  - 规则模板版晨报；series 落库；心跳与 ping；
  - 剧本全部处于影子模式；人工 L1 动作 H1、H3、H11、H12、H14，加上人工处置入口；
  - plus 内部改动：dispatch 和 channelstatus 返回 `*Service`，SheetSave 返回 record id，新增 `Verdicts` 和 `ProbeGroup`。
- 上游接触点：U1、U2（只做邮件）、U3（只登记邮件）、U6。
- 前端：值班台、事件、报告、配置；菜单红点；补齐 i18n。

**验收标准**
- 在 dev compose 中人为制造 429 风暴、401 账号、分组无号、任务心跳停止：2 分钟内收到 IM 推送；条件消失并过了恢复确认期后收到恢复通知。
- 起两个实例：只有一个实例推送；杀掉 leader 后 ≤90 秒完成接管，且不重复推送。
- 晨报 09:00 准时送达，各段齐全。
- 影子记录可查，能人工复核。
- 生产观察 2 周（与 P1 开发并行）：日均推送 ≤5 条，可行动比例 ≥60%，被标为噪声的比例 <20%。

### P1（第 3–4 周）：AI 诊断加确认式处置

**交付物**
- AI 诊断卡、晨报结论、事件复盘、降噪建议；预算与兜底；`plus_patrol_ai_call`。
- 台账和确认弹窗（确认时实时重跑闸门和 SheetPreview）；自愈动作页。
- 级别：H4、H6、H7、H8、H10 为 L1；H1、H2、H3、H5 为 L2。
- `UndoIfUnchanged`、ApplyRecord 串行化、分钟级 Verifier、自动回滚、Engine 计数、OnFinish 改造。
- 上游接触点 U2–U5：ops 告警回调、账号阻断 reason 码、RateLimit/AccountTest/Audit bridge。
- 飞书、企微通道。
- H4 继续以影子模式记录"若为 L2 将执行"。

**验收标准**
- P0/P1 事件的 AI 卡 p95 在 60 秒内生成。AI 关闭、超时或报错时推送不受影响（自动化测试）。
- 非法 AI 输出被丢弃并显示兜底模板（单测覆盖 Schema、未知字段、伪造 ref）。
- 集成测试：自动动作之后有人改了同一个键，回滚不覆盖人工修改（标 `conflict_human`）。人工与自动并发写不丢更新。
- 闸门和定级规则用表驱动单测，覆盖 Danger、Dead、Only、Side、剩余号数、容量、平台熔断、防抖、数据陈旧、依赖为 nil。
- **进入 P2 的准入条件：** H4 影子样本 ≥20，规则判定准确率 ≥90%，通过闸门后的预演中 Dead 出现 0 次。

### P2（第 5–6 周）：受限自动执行与夜间自治

**交付物**
- H4、H5 开放 L2：第一周只对管理员指定的灰度分组开放，之后逐组扩大；H7 视 P1 数据决定是否升为 L2；H11 对本系统自己的记录开放 L2。
- 启用全部闸门：系统性熔断、爆炸半径、预算、熔断复位；一键止血。
- 认领、重复提醒和升级计时器；夜间预告；周报（含调度权重建议）。
- 外部死人开关；dispatch Record/Audit 加 source 列（迁移 265）；`docs-fork/patrol.md` 和 upstream-sync 检查清单。

**验收标准**
- 连续 2 周：自动动作回滚率 <10%，人工推翻率 <5%，自动动作导致"分组无号"或 Dead 的次数为 0，没有任何动作被人工评定为"造成损失"。
- 模拟平台级故障（同平台多账号 5xx）：没有任何自动动作，只推送一条父事件。
- 一键止血实测：所有 active 动作在 1 分钟内撤销完毕，冲突项列出。
- 夜间非 P0 的 @ 次数为 0。

### 后续（P3，按需）
H8、H9 自动化评估；"问 AI"只读工具；MCP 只读暴露巡检；IM 互动卡片回调（签名加一次性 token）；账号级可用率时间序列；统一 channelstatus 与 dispatch 的排除口径。

---

## 11. 风险与开放问题

### 11.1 风险

| # | 风险 | 影响 | 对策 |
|---|---|---|---|
| R1 | 自愈引发连锁过载：暂停后流量集中到剩余账号，引发 429/529，触发更多暂停，最后分组无号 | 小故障放大成事故 | SheetPreview 的 Danger/Dead 闸门；单分组同时只暂停 1 个、比例 ≤25%、剩余 ≥2 个；容量 ≥1.5 倍在用；15 分钟冻结期；验证时检查同伴账号；`hard=false` 保证线路只剩它时 fail-open；2 次回滚即熔断 |
| R2 | 系统性故障被误判为账号故障 | 逐个停号导致雪崩 | 同平台 ≥30% 账号异常或 ≥2 个分组异常就熔断；基础设施事件因果抑制；AI veto；平台级事件不自愈 |
| R3 | Undo 无条件覆盖、SheetSave 没有锁 | 自动回滚覆盖人工修改；并发写丢更新 | `UndoIfUnchanged`；ApplyRecord 加 advisory lock 和 FOR UPDATE 比较 From；人工优先 24 小时 |
| R4 | 暂停后被动信号失真（低于 MinSample 变成 unknown） | 过早恢复、反复抖动 | 恢复只看主动探测；防抖：同一目标 24 小时 ≤2 次；第二次恢复失败转人工 |
| R5 | 清冷却副作用：ClearTempUnschedulable 连带清模型限流；提前清 403 可能导致永久禁用 | 再次限流或被封 | 新建只清 temp_unsched 的 bridge；白名单排除 429 上游重置、403、凭证类；容量紧张时才清；每账号每天 ≤1 次 |
| R6 | 暂停永不恢复：dispatch Rule 没有 TTL；plus 任务不参与优雅停机；换主 | 账号长期被闲置 | 台账加 Reaper；换主时对账；A6 监控 TTL 过期未处理；D7 巡检兜底 |
| R7 | 数据陈旧或依赖缺失（水位滞后、ops 监控关闭、bridge 为 nil） | 基于过时数据误摘号 | 闸门 fail-closed 降为 L0；依赖为 nil 时对应执行器标为"不可用"，不 panic |
| R8 | 提示注入：错误文本来自上游或用户 | AI 提出恶意建议 | 不可信数据块、截断与掩码；AI 没有写工具；只能在候选集内选；L2 只由规则触发 |
| R9 | AI 幻觉，编造数值或证据 | 误导人工 | ref 校验；数值由系统渲染；只有 AI 依据的动作最高 L1；有"诊断不对"反馈 |
| R10 | AI 依赖本站网关，并污染巡检数据 | 故障时 AI 不可用 | P0 首推不等 AI；规则模板兜底；专用 Key 绑定其他平台的分组；巡检查询排除 AI 自己的 Key |
| R11 | 成本失控：事件风暴时 AI 和复测消耗大量 token 与上游额度 | 费用和额度 | 指纹 30 分钟复用；单事件 ≤2 次调用；日预算；复测每账号每天 ≤6 次；探测每组每天 ≤12 次；超出降级 |
| R12 | 粘性会话被打断：Engine 对老会话也会剔除被暂停账号 | 提示缓存失效，用户成本和延迟上升 | 预演展示转移流量；验证项包含缓存命中率；只暂停到模型粒度 |
| R13 | 审批通道被滥用（IM 链接泄露） | 他人执行写操作 | 写操作必须登录后台并二次确认；短链最多只开放认领和静默（待拍板） |
| R14 | 告警疲劳、定级不准 | 管理员重新陷入噪声 | 按影响定级、上卷、抑制、合并、抖动处理；影子期 2 周；用"关闭为噪声"的反馈校准；G1 降噪建议 |
| R15 | 双重通知（上游邮件加 IM） | 重复打扰 | 检测重复并一键关闭规则邮件；B6 镜像单同步状态 |
| R16 | 口径冲突：用户侧错误排除，channelstatus 是 AND 语义且受开关 `ExcludeUserSideErrors` 控制，请求数和错误数都扣；dispatch 是 OR 语义、始终生效、只扣错误数，并且错误另外限定在 upstream/network 阶段（≥500 或 429）；两边对 NULL 的处理也不同；channelstatus 疑似对属于忽略分类的用户侧错误重复扣减、低估错误率 | 分组和账号结论互相矛盾 | 分组级用 channelstatus，账号级用 dispatch，事件上标明来源；补测试确认是否重复扣减；统一口径另立项 |
| R17 | DB 压力：确认查询、overview raw 查询，以及窗口 <1 小时时 DBExcludedCounter 直扫 ops_error_logs | 慢查询 | 只对正在验证的对象查询；复用 CachedSource 和 Snapshotter；B1 按平台的查询降为 5 分钟；上线后观察 pg 慢查询 |
| R18 | 巡检与站点一起挂掉 | 夜间无人知道 | 外部拨测 ping；死人开关；"晨报没到即异常" |
| R19 | 钉钉限流（每分钟 20 条）且消息不能编辑 | 消息被丢 | 60 秒合并窗口；只推关键变化；投递失败重试并记录日志 |
| R20 | 上游同步冲突：P1 起 service 包里约 5 处 `[plus]` 行 | 合并上游时冲突 | 每处只改一行，写进 upstream-sync 检查清单；回调为 nil 时安全降级 |
| R21 | 隐私：error_body 可能包含用户内容 | 数据外泄到 LLM | 绝不发送 error_body；message 截断加掩码；用户只给计数 |
| R22 | 排期：6 周、1 人全栈偏紧 | 延期 | 优先保住 P0（推送、一键处理、晨报，已能取代每天手动看后台）；P2 可以顺延 |

### 11.2 需要拍板的问题

| # | 问题 | 选项 | 建议 |
|---|---|---|---|
| Q1 | 团队主用哪个 IM？ | 钉钉 / 飞书 / 企微 | P0 先做钉钉加通用 webhook，P1 补飞书和企微；如果主用飞书，就把飞书提到 P0 |
| Q2 | 夜间时段，以及"不能自愈但影响用户的 P1"夜里怎么处理？ | a. 发消息但不 @ ；b. 不发，记进晨报 | 23:00–08:00；选 a |
| Q3 | IM 里是否开放免登录短链（只限认领、静默 ≤2 小时，HMAC 签名、单次有效、15 分钟过期）？ | 开放 / 全部要求登录 | P0–P1 全部要求登录，P2 根据使用情况再评估 |
| Q4 | AI 通道：用哪个分组的 Key、哪个模型；是否允许外部备用通道；每日预算 | — | 专用用户和 Key，绑定与主业务不同平台的分组；诊断用小模型；初始上限每小时 20 次、每天 200 次调用，token 上限由你定 |
| Q5 | 阈值放在哪里？ | a. 推荐规则包一键创建到上游 ops 规则（复用上游阈值界面）；b. 巡检配置页另设阈值 | a；巡检只补上游没有的阈值 |
| Q6 | P2 是否允许夜间 L2 自动执行？全局上限默认值是否接受（3/小时、5 个同时生效、20/天）？ | — | 允许，但只限灰度分组；上限按建议值，运行 2 周后再调 |
| Q7 | 无法主动测试的账号，能否接受金丝雀降权（会让少量真实请求失败）？ | 接受（H6 为 L1） / 不接受（只通知） | 接受，但只作为 L1，由人确认 |
| Q8 | 人工优先窗口多长？ | 24 小时 / 30 分钟 | 24 小时（更安全）；调整频繁的分组可以单独配置 |
| Q9 | 口径统一和 channelstatus 疑似重复扣减，是否在本项目内处理？ | 本期处理 / 另立项 | 本期只标注来源并补测试确认，统一口径另立项 |
| Q10 | 是否接受 P1 起在 service 包加 `[plus]` 行（2 个回调加 3 个登记）？ | 接受 / 不接受 | 接受。不接受的话，P1 只能保持轮询，并放弃账号级动作（H2、H7、H8） |
| Q11 | 上游 ops 规则邮件与 IM 重复时怎么办？ | 自动关闭 / 只提示，由人一键关闭 | 只提示，由人一键关闭 |
| Q12 | 排期 | 6 周完整版 / 4 周压缩版（P0 加 P1 去掉 AI，H4 只到 L1，AI 和 L2 放到二期） | 6 周；如果必须 4 周就选压缩版 |
| Q13 | 值班人名单 | — | 请提供白天和夜间值班人的手机号与邮箱，以及"核心分组"名单（C1 用来判 P0） |

---

## 附录 A：评审需核对的关键代码事实

| 事实 | 位置 | 对方案的影响 |
|---|---|---|
| `SheetSave` 丢弃了 ApplyRecord 返回的 record id | plus/dispatch/service_weight.go:290 | 自动回滚要用 record id，必须改为返回 |
| `SheetSave` 不会因 Danger 拦截（全包只有 service_weight.go:50/192 两处出现 Danger）；`Undo` 不检查当前规则，Side 里连带改动的账号同样被直接写回 From | service_weight.go:232-304、:380-391 | 必须由 patrol 预演，并新增 `UndoIfUnchanged`（主键和 Side 都要比较） |
| Rule 没有 expires_at 或 TTL 字段 | plus/dispatch/ports.go | TTL 由台账加 Reaper 实现 |
| `hard=false` 时，线路上只剩被暂停账号就照常保留它（fail-open）；其他实例上 Runner 最多 10 秒生效 | hook/engine.go、hook/runner.go | 爆炸半径的天然边界；验证从执行 30 秒后起算 |
| dispatch 主动检测只支持 Direct（apikey/upstream） | plus/dispatch/catalog.go:106、:178-187 | 非 Direct 账号走 `RunTestBackground` bridge |
| `RunTestBackground` 不做测试成功后的统一恢复，但测试本身会写账号状态（SetError、SetRateLimited、UpdateExtra 等） | service/account_test_service.go:3309-3317、:618/:688/:979/:984 | H2 不是零副作用动作，台账要记录前后状态差异 |
| `notifyAccountSchedulingBlocked` 只对 OpenAI 账号生效，并且只在单进程内存里 | service/ratelimit_service.go:147/154、:342 | U5 回调覆盖不到其他平台 |
| `ops_ingress_reject_aggregates` 没有 overflowed_count 列 | migrations/183_ops_ingress_reject_aggregates.sql、service/ops_ingress_reject.go:60 | F1 的溢出数要从内存健康接口读取 |
| ops 告警邮件只有模板或配置错误才回退 SendEmail，SMTP 失败不回退 | service/ops_alert_evaluator_service.go:721-737 | patrol 的邮件通道自己负责重试 |
| `operator()` 在 Email 非空时直接取 Email；HTTP 入口 actorOf 只填 ID，所以人工操作走的都是按 ID 查邮箱 | plus/dispatch/service.go:142-150、handler.go:56-63 | patrol 在进程内调用时传 `Email:"system:patrol"` 即可区分，不用改表 |
| `Prober.RunNow` 的 force=true 会跳过 CAS 和每日上限 | plus/channelstatus/prober.go | 巡检自己限制探测次数 |
| channelstatus `Snapshot` 只算 visible 分组；`currentVerdicts` 是私有方法 | plus/channelstatus/service.go | 新增公开的 `Verdicts` |
| `channelstatus.Register` 已返回 `*Service`；`dispatch.Register` 没有返回值 | plus/*/module.go | dispatch 需要改签名 |
| plus 只拿得到 APIKey/Subscription/Setting 三个服务和裸 `*sql.DB`，没有 Redis；后台任务用 `context.Background()` 启动 | plus/plus_routes.go | 选主用 DB 租约；需要 bridge；动作先落库再执行，换主时对账 |
| `GetConcurrencyStats` 需要 ops 监控开启 | service/ops_concurrency.go | 监控关闭时容量闸门 fail-closed |
| `SetTempUnschedulable` 只在新 until 更晚时写入；`ClearTempUnschedulable` 连带清掉全部模型限流 | repository/account_repo.go:2524、ratelimit_service.go | 降流量不用它；清冷却改用新的细粒度 bridge |
| `iq.StreamClient` 只接收单个 Prompt、返回文本，不支持工具调用 | plus/iq | AI 天然没有写能力 |
| 最新迁移为 263_plus_feedback.sql | backend/migrations | 本期用 264，P2 用 265 |