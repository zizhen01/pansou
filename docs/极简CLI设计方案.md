# PanSou 原理分析与极简 CLI 设计方案

> 目标：从当前 PanSou 的聚合搜索能力中提炼真正的核心，设计一个可维护、可组合、尽可能精简的命令行工具。

## 1. 结论先行

PanSou 的本质不是 Web 服务，而是一条聚合搜索数据管线：

```text
查询 → 多来源抓取 → 统一结果模型 → 去重 → 排序 / 过滤 → 输出
```

因此，新的 CLI 不应复制当前项目的 Gin、JWT、二级缓存、后台回填等服务端能力。第一版只需聚焦一件事：在一个明确的 deadline 内，从多个来源得到规范化、去重、可筛选的结果，并稳定地输出给人和脚本。

建议基线：Go 单二进制、一个 `search` 子命令、两个真实来源、无默认缓存、支持 `text`、`json` 和 `ndjson` 输出。

## 2. 当前项目的工作原理

### 2.1 启动与装配

`main.go` 完成以下工作：

1. 读取环境变量并构造全局 `config.AppConfig`。
2. 初始化 HTTP 客户端、二级缓存和延迟批量写入器。
3. 通过大量插件空导入触发各插件的 `init()`，将插件登记到全局注册表。
4. `PluginManager` 按 `ENABLED_PLUGINS` 筛选插件。
5. 创建 `SearchService`，并把它交给 Gin 路由。

现有代码的主要责任边界如下：

| 位置 | 职责 | CLI 的处理方式 |
| --- | --- | --- |
| `main.go` | 装配、服务器启动、关闭 | 改为装配命令与来源列表；不启动服务器 |
| `api/handler.go` | GET/POST 参数解析、默认值 | 收敛为 flags 到 `Query` 的一次转换 |
| `service/search_service.go` | 搜索调度、合并、排序、分类 | 保留其业务意图，重写为纯核心逻辑 |
| `plugin/plugin.go` | 插件接口、注册、异步基类、缓存 | 缩小为单个 `Source` 接口；去除全局状态 |
| `util/*` | HTTP、解析、URL/网盘识别 | 仅复用纯函数与经审计的链接规则 |

### 2.2 搜索请求的数据流

当前 API 接受 `kw`、`src`、`plugins`、`channels`、`res`、`cloud_types` 等参数。`SearchService.Search` 根据 `src` 并行启动 Telegram 和插件两条分支，然后统一整理结果：

```text
HTTP 请求
  → 参数默认化与互斥处理
  → TG 搜索 与 插件搜索并行
  → 每个来源抓取 / 解析
  → SearchResult 统一模型
  → mergeSearchResults 去重并保留更完整结果
  → 时间、关键词、来源等级综合排序
  → 链接类型识别与 merged_by_type 分组
  → JSON 响应
```

关键实现点：

- `service/search_service.go`：TG 与 plugin 分支各用 goroutine 并行；结果会合并、过滤、排序。
- `plugin/plugin.go`：插件通过 `AsyncSearchPlugin` 接口屏蔽来源差异。
- `model/response.go`：`SearchResult` 和 `Link` 是跨来源的统一数据契约。
- `mergeSearchResults`：以 `UniqueID`、`MessageID` 或标题/频道作为键，冲突时选择信息更完整的记录。
- `mergeResultsByType`：识别网盘 URL 类型，再按类型分组输出。

### 2.3 现有“异步插件”的真实语义

项目采用两阶段超时：短超时（默认 4 秒）优先返回已获得的内容，长超时（默认 30 秒）继续在后台获取，并回写缓存。它适合常驻 API：下一次相同请求可能命中更完整的缓存。

它不适合默认 CLI 语义：命令退出后，用户无法自然地观察“稍后补全”的结果；后台任务和缓存写入也增加了结果不确定性。

## 3. 什么应保留，什么应裁剪

| 能力 | 当前形态 | CLI v1 决策 | 原因 |
| --- | --- | --- | --- |
| HTTP/Gin/JWT/CORS | 路由、中间件、认证、压缩 | 删除 | CLI 的调用者是本地进程，stdout 是接口 |
| 插件全局注册 | `init` + 空导入 + 全局 map | 替换 | 显式来源列表更直观、更容易测试 |
| 二级缓存 | 内存 + 磁盘 + GOB | 删除 | 单次命令生命周期内收益低，复杂度高 |
| 两阶段异步回填 | 4 秒响应、30 秒后台更新 | 删除 | 退出后的后台结果对 CLI 用户没有明确语义 |
| 有界并发 | 工作池和背景槽位 | 保留但简化 | 多来源并发仍然必要；`errgroup.SetLimit` 足够 |
| 统一结果模型 | `SearchResult` / `Link` | 保留并瘦身 | 是跨来源聚合的核心边界 |
| 去重、链接类型识别 | service + util | 保留 | 是聚合器真正提供的价值 |
| 复杂评分 | 插件等级 + 时间 + 关键词 | 简化 | 先实现可解释、可配置的排序 |
| 链接可用性检测 | `CheckService` + 专用平台协议 | 延后 | 与搜索主路径无关，作为独立命令再评估 |

一个重要原则：**默认 CLI 必须在 deadline 到达时输出已完成的结果和明确 warning，而不是返回空数据后在后台静默“补全”。**

## 4. 极简 CLI 的领域模型

不要把 API 的 `SearchRequest` / `SearchResponse` 原样搬过来。CLI 应采用与传输层无关的领域对象。

```go
type Query struct {
    Text     string
    Sources  []string
    Types    []string
    Limit    int
    Timeout  time.Duration
    NoRank   bool
    NoDedupe bool
}

type Item struct {
    Title       string
    URL         string
    Password    string
    Source      string
    PublishedAt time.Time
    Tags        []string
    Type        string // 由 URL 识别出的 quark / aliyun / ...
}

type Source interface {
    Name() string
    Search(ctx context.Context, q Query) ([]Item, error)
}

type Result struct {
    Items    []Item
    Warnings []Warning
    Elapsed  time.Duration
}
```

边界应保持严格：

- `Source` 只负责“请求来源并产出候选项”。
- 核心层负责规范化 URL、识别类型、去重、排序、过滤和截断。
- 命令层负责 flags、stdout/stderr、退出码。
- 核心层不读取环境变量、不打印日志、不直接发 HTTP、不自行启动 goroutine。

## 5. 命令体验

第一版只提供一个主命令：

```bash
pansou search <query>
```

查看当前默认来源与 Telegram 频道：

```bash
pansou sources
```

查看公开频道最近消息：

```bash
pansou recent Aliyun_4K_Movies --limit 5
```

推荐参数：

| 参数 | 默认值 | 行为 |
| --- | --- | --- |
| `--source a,b` | `tg` | 只运行指定来源；未知来源直接报参数错误。`pan666` 仅在显式指定时调用，因为其公开接口可能拒绝自动化请求。 |
| `--type quark,aliyun` | 不过滤 | 只保留指定网盘类型 |
| `--limit 20` | `20` | 去重、排序后截断 |
| `--timeout 8s` | `8s` | 整个命令的 deadline |
| `--parallel 4` | `min(4, 来源数)` | 同时执行的来源上限 |
| `--format text|json|ndjson` | `text` | 面向人、程序、流式处理的三种输出 |
| `--no-rank` | `false` | 保留合并后的来源返回顺序，便于调试 |
| `--no-dedupe` | `false` | 保留重复项，便于定位来源问题 |
| `--no-progress` | `false` | 关闭 stderr 上的实时搜索计时 |
| `--verbose` | `false` | 展开逐来源的失败原因；默认只显示失败/超时汇总 |

所有已注册插件均可通过 `all` 选择：

```bash
pansou search "三体" --source all --parallel 8 --timeout 15s
```

这会调用所有现有插件；推荐只在扩展搜索时使用，因为外站失败、限流与超时会显著增多。

示例：

```bash
pansou search "三体" \
  --source tg,forum \
  --type quark,aliyun \
  --limit 10 \
  --format text
```

输出契约：

- stdout 只写结果，尤其是 JSON/NDJSON 时绝不夹杂日志。
- stderr 写来源失败、超时与诊断信息。
- 退出码 `0`：至少一个来源成功；`1`：参数/配置错误；`2`：所有所选来源都失败或超时。

## 6. 推荐目录结构

```text
.
├── cmd/pansou/                 # 入口：flags、stdout/stderr、退出码
├── internal/app/               # Runner、deadline、并发、错误汇总
├── internal/core/              # Query、Item、URL 规范化、去重、排序、类型识别
├── internal/sources/
│   ├── tg/                     # 一个 Telegram / HTML 适配器
│   └── forum/                  # 一个 JSON / 论坛 API 适配器
├── internal/output/            # text、json、ndjson 格式化
├── configs/sources.example.yaml
└── testdata/                   # 每个来源的离线解析 fixture
```

依赖方向必须单向：

```text
cmd → app → core
           ↑
        sources
           ↓
         output
```

更准确地说，`sources` 和 `output` 都依赖 `core`，但 `core` 不依赖它们。这样可在无网络环境下测试所有去重、排序和输出逻辑。

## 7. 并发、错误和排序策略

### 7.1 并发

使用一个请求级 `context.WithTimeout` 与 `errgroup.Group`：

1. 基于选中的来源构造任务。
2. `SetLimit(parallel)` 控制并发数。
3. 每个 `Source.Search` 使用同一个总 deadline；若来源需要更小超时，可在内部再收紧。
4. 单个来源失败记录为 `Warning`，而不是取消所有来源。
5. deadline 后汇总已完成结果；如果全部失败，再返回退出码 `2`。

不要实现常驻 WorkerPool、全局背景队列或后台回填。

### 7.2 去重

建议的去重键优先级：

1. 规范化后的分享 URL（去掉无意义查询参数、统一编码）。
2. URL 与提取出的访问密码组合。
3. 来源名 + 标题的规范化文本，作为无 URL 时的保守兜底。

冲突时选择“信息更完整”的项：有 URL、密码、发布时间、较长标题、较多标签者优先。该规则沿用当前 `mergeSearchResults` 的思想，但避免依赖 Telegram 专有 ID。

### 7.3 排序

第一版应使用可解释分数：

```text
score = source_weight + recency_score + exact_title_match_bonus
```

- `source_weight`：来自配置，默认都为 `0`；不把质量判断写死在代码里。
- `recency_score`：只对存在 `PublishedAt` 的项生效。
- `exact_title_match_bonus`：标题包含完整查询时加分。

不要一开始复刻现有 1–4 级插件体系和隐式关键词表；等有真实使用数据后再校准。

## 8. 分阶段实施计划

| 阶段 | 范围 | 完成标准 |
| --- | --- | --- |
| 0. 契约 | 定义 `Query`、`Item`、`Source`、输出格式 | 用纯单元测试验证 JSON、去重、排序 |
| 1. 单源骨架 | 命令入口、静态 Mock Source、text/json 输出 | `pansou search x` 可稳定运行和报错 |
| 2. 聚合核心 | `Runner`、有界并发、超时、部分失败、去重、类型过滤 | 慢源不阻塞；同 URL 只输出一次 |
| 3. 两个真实来源 | 迁移两个最稳定且许可明确的来源 | 每个来源有 fixture 解析测试和可选集成测试 |
| 4. 可选缓存 | 显式 `--cache` 的小型 TTL 文件缓存 | 默认关闭；缓存故障不影响搜索 |
| 5. 扩展评估 | 新来源、链接检查、配置增强 | 只按真实需求增加复杂度 |

## 9. 从当前仓库迁移什么

| 当前资产 | 处理方式 | 说明 |
| --- | --- | --- |
| `model/response.go` 的 `Link` / `SearchResult` | 抽取并瘦身 | 迁为 `core.Item`，只留跨来源字段 |
| `service/search_service.go` 的合并逻辑 | 重写为纯 `Dedupe` | 保留“选择更完整记录”的原则 |
| `mergeResultsByType` 与 URL 正则 | 审计后抽取 | 变成可单测的 `DetectType` / `FilterTypes` |
| 单个插件的抓取/解析代码 | 每次迁移一个 | 先补 fixture，再接入 `Source` |
| `BaseAsyncPlugin` | 不迁移 | 它绑定了全局缓存、后台任务和服务端语义 |
| `config.AppConfig` | 不迁移 | flags 优先；可选 YAML 配置替代全局环境变量 |
| `CheckService` | 暂不迁移 | 后续若需要，做独立 `check` 子命令 |

## 10. 风险与边界

- 来源稳定性：每个来源都是不可靠边界；必须有单独超时、有限重试和 fixture。
- 合规性：只接入允许访问和自动化调用的来源；不要把绕过限制变成通用功能。
- 解析脆弱性：HTML 来源尤其需要离线 fixture，避免每次改解析器都依赖线上站点。
- 凭据和代理：只从环境变量或系统密钥环读取，不能写入 stdout、日志或默认缓存。
- 输出兼容性：JSON 字段应在 v1 就稳定下来，避免脚本用户被频繁破坏。

## 11. 推荐 MVP

第一个可交付版本应严格限制为：

```text
一个 Go 二进制
一个 search 子命令
两个真实来源
8 秒总超时、4 路并发
text / json / ndjson
URL 去重与网盘类型过滤
无认证、无服务、无默认缓存、无后台任务
```

开始实现前，仍需确认三件产品决策：

1. 首批接入哪两个合法且稳定的来源？
2. 是只用 flags，还是同时提供可选 YAML 配置？
3. 二进制是否与原项目同名 `pansou`，还是使用 `pansou-cli` 避免混淆？

## 12. 分析依据

本文件基于以下代码和项目内设计文档的静态分析：

- `main.go`
- `api/router.go`、`api/handler.go`
- `service/search_service.go`、`service/check_service.go`
- `plugin/plugin.go`
- `model/request.go`、`model/response.go`
- `config/config.go`
- `util/pool/worker_pool.go`
- `docs/系统开发设计文档.md`
- `docs/插件开发指南.md`

分析过程未对任何外部来源发起搜索请求，也没有修改现有业务代码。
