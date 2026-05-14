# 参考 BetterAltay 用 Go 重写任务书

> 本文件是参考 BetterAltay 使用 Go 完整重写的长期任务台账。后续任何新对话开始前，应先阅读本文件，优先处理 `DOING`、`BLOCKED`、`NEEDS-SUPPLEMENT` 和最高优先级 `TODO` 任务，并在完成后更新状态、完成情况、备注和变更记录。

## 0. 当前基线

| 项目 | 内容 |
|---|---|
| 源项目 | `../BetterAltay` |
| 目标项目 | `.`，当前 Go module 为 `gucooing/bds` |
| 源项目类型 | PocketMine-MP/Altay 系 PHP Minecraft: Bedrock Edition 服务端 |
| 源项目版本 | `NAME=BetterAltay`，`BASE_VERSION=3.28.0`，`FORK_VERSION=1.39.3` |
| Minecraft 目标版本 | 当前实现跟随 `gophertunnel v1.56.1`，目标 Bedrock `1.26.20` |
| 协议号 | 当前实现 `protocol 975`；源项目 `944/1.26.10` 仅作历史参考 |
| 源码规模 | `src/pocketmine` 约 1470 个 PHP 文件 |
| 最大模块 | `network` 382、`block` 208、`item` 174、`level` 144、`event` 127、`entity` 119 |
| 当前仓库状态 | `.` 已初始化 git 仓库；`../BetterAltay` 当前不是 git 仓库 |
| 任务书创建日期 | 2026-05-05 |

## 1. 使用规则

### 1.1 状态枚举

| 状态 | 含义 |
|---|---|
| `TODO` | 尚未开始 |
| `DOING` | 正在进行，同一模块尽量只保留少量 `DOING` |
| `BLOCKED` | 被外部条件或决策阻塞，必须写明阻塞原因 |
| `NEEDS-SUPPLEMENT` | 已发现信息不足，需要补充资料、样例、协议说明或用户决策 |
| `REVIEW` | 已实现，等待代码审查、验收或补测 |
| `DONE` | 已完成并通过验收 |
| `DEFERRED` | 暂缓，不影响当前里程碑 |

### 1.2 任务记录要求

每次后续对话完成工作时，必须更新对应任务的这些字段：

| 字段 | 填写要求 |
|---|---|
| 状态 | 从 1.1 状态枚举中选择 |
| 完成情况 | 简述实际完成内容，不写空泛结论 |
| 备注 | 记录风险、取舍、已知问题、关联文件 |
| 需补充 | `否`、`是：原因` 或 `待确认：问题` |
| 验收证据 | 命令、测试、截图、客户端连接结果、性能数据或人工确认 |

### 1.3 新对话续接流程

1. 阅读本文件的 `0. 当前基线`、`2. 总目标`、`4. 当前决策`、`6. 分阶段任务总表`、`12. 变更记录`。
2. 找到第一个未完成里程碑中最高优先级的 `TODO`、`DOING`、`BLOCKED` 或 `NEEDS-SUPPLEMENT` 任务。
3. 开始改动前检查目标文件是否已有他人修改，避免覆盖并行工作。
4. 完成后更新任务表、模块台账、验收记录和变更记录。
5. 如果新增任务，使用已有 ID 前缀并追加新的编号，不复用旧 ID。

### 1.4 代码生成与实现规则

1. 禁止无意义硬编码：除协议号、版本号、默认配置、标准常量、测试 golden 数据等有明确来源和稳定语义的值以外，不得把可配置、可推导、可由数据表/注册表/协议定义生成的内容散落硬编码在业务逻辑中。
2. 禁止无意义小函数和中转函数：不得生成只做简单转发、改名、包一层、没有抽象收益、没有隔离外部依赖、没有复用价值的小函数或中转函数。
3. 允许必要抽象：当函数承担边界隔离、错误语义统一、并发所有权保护、协议/数据格式封装、测试替身注入或减少真实重复复杂度时，可以保留，但备注或代码结构应体现其必要性。
4. 发现硬编码或空转抽象时，优先回到数据表、注册表、配置、生成器或直接调用现有清晰接口。

## 2. 总目标

用 Go 在 `bds` 模块中重写 BetterAltay，目标是得到一个可运行、可维护、可测试的 Minecraft: Bedrock Edition 服务端。第一阶段不追求一次性覆盖全部 PocketMine API，而是先达到最小可玩闭环，再逐步补齐协议、世界、实体、物品、方块、插件和运维能力。

### 2.1 非目标与边界

| 编号 | 边界 | 状态 | 备注 |
|---|---|---|---|
| SCOPE-001 | 不直接机器翻译 PHP 文件生成 Go 代码 | TODO | 重写应以行为、数据结构、协议语义为目标 |
| SCOPE-002 | 不默认承诺 PHP 插件无缝兼容 | NEEDS-SUPPLEMENT | Go 无法原生执行 PHP 插件，需用户决策兼容策略 |
| SCOPE-003 | 协议基线按当前依赖显式记录 | DONE | 用户确认使用新版本；当前以 `gophertunnel v1.56.1` 的 `1.26.20/protocol 975` 为基线 |
| SCOPE-004 | 不在 MVP 中实现全部游戏机制 | TODO | MVP 先连入、出生、移动、聊天、基础方块交互 |

### 2.2 核心验收里程碑

| 里程碑 | 目标 | 验收标准 | 状态 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|
| M0 | 项目骨架可编译 | `go test ./...` 通过，`cmd/bds` 可启动并打印版本 | DONE | 已建立 `cmd/bds`、`internal/bootstrap`、版本信息和基础测试 | `go test ./...` 通过；`go run ./cmd/bds -version` 可输出版本 | 否 |
| M1 | RakNet ping 可见 | Bedrock 客户端局域网列表能看到 MOTD 或用测试工具获得 ping pong | DONE | 已接入 `go-raknet`，服务端可响应 BetterAltay 风格 `MCPE;...;` pong 数据，本地测试工具可获得 ping pong | 已通过 `internal/network/raknet` 适配层隔离第三方库；尚未用真实 Bedrock 客户端手工验证局域网列表 | 否 |
| M2 | 登录握手闭环 | 客户端能连接到服务器并进入资源包/StartGame 流程 | DOING | 已撤回 gophertunnel `minecraft.Listener` 托管完成结论；当前 `internal/network/mcpe` 只负责 RakNet/MCPE batch 连接适配，bootstrap 注入 `internal/server` client factory，由 `internal/server` 自有 MCPE 会话处理 NetworkSettings、离线 Login 解析、本地 ServerToClientHandshake/ClientToServerHandshake 加密握手、空资源包栈和 StartGame 包发送 | 真实 Bedrock 客户端登录、在线认证和完整 Spawn 流程仍需验收 | 需补充真实客户端验收 |
| M3 | 单人平坦世界可进入 | 客户端能出生在固定世界，能看到区块并移动 | DOING | 已新增 `internal/world` 可替换 `ChunkProvider` 和默认平坦世界生成器；`internal/server` 可在请求视距后发送 SetTime、SetSpawnPosition、NetworkChunkPublisherUpdate、LevelChunk 并响应 SubChunkRequest；测试可解码验证出生区块草地方块 | 真实客户端已能触发 StartGame 和初始区块发送；可见性、移动手感和玩家输入处理仍需后续验收/实现 | 需补充真实客户端验收 |
| M4 | 基础玩法闭环 | 聊天、命令、放置/破坏基础方块、背包热栏可用 | DOING | 聊天、客户端命令、控制台命令和最小权限系统已接入；背包/热栏初始同步、手持切换和基础 ItemStackRequest/Response 已接入；方块交互仍未完成 | D-008 已推进；H/I 方块交互、完整物品表、创造栏与掉落实体仍是 M4 风险 | 否 |
| M5 | 持久化世界 | 可加载/保存至少一种世界格式，重启后方块变化保留 | TODO |  |  | 需确认目标格式 |
| M6 | 多人稳定性 | 5-20 人测试，加入/离开/移动/聊天同步无明显崩溃 | TODO |  |  | 需准备测试方式 |
| M7 | 插件/扩展 API | Go 原生插件或脚本扩展能注册命令、监听事件、修改玩法 | TODO |  |  | 需用户决策 |
| M8 | 发布可用版本 | Windows/Linux 构建产物、配置、文档、迁移说明齐全 | TODO |  |  | 否 |

## 3. 推荐目录规划

此目录规划为初始建议，实际实现时可以按 Go 习惯调整，但调整必须记录在 `4. 当前决策`。

```text
bds/
  cmd/bds/                 # 服务端入口
  internal/bootstrap/      # 启动流程、关闭流程、版本信息
  internal/config/         # server.properties、yaml/json 配置、默认配置
  internal/logging/        # 日志、终端颜色、文件日志
  internal/server/         # Server 核心、tick loop、玩家列表、生命周期
  internal/network/raknet/ # RakNet 或其适配层
  internal/network/mcpe/   # MCPE 会话接入、压缩、加密、批处理
  internal/world/          # Level、Chunk、SubChunk、Provider、生成器
  internal/block/          # 方块状态、运行时 ID、碰撞箱、注册表
  internal/item/           # 物品、耐久、食物、附魔、注册表
  internal/entity/         # Entity、Player、Actor 属性、移动和伤害
  internal/inventory/      # 背包、容器、交易、合成
  internal/command/        # 命令系统、控制台命令、参数解析
  internal/permission/     # 权限、OP、BanList
  internal/event/          # 事件总线、监听器优先级、取消机制
  internal/plugin/         # 插件/扩展加载策略
  internal/scheduler/      # 主线程任务、异步任务、定时任务
  internal/resourcepack/   # 资源包校验、分块、发送
  internal/lang/           # 多语言文本与格式化
  internal/nbt/            # NBT 读写；可使用独立 package
  internal/math/           # 向量、AABB、方向、随机
  internal/testutil/       # 测试夹具、协议样例、虚拟客户端
  pkg/api/                 # 稳定公开 API，等 M7 后再承诺
  docs/                    # 架构、协议、迁移、运维文档
```

## 4. 当前决策

| ID | 决策项 | 当前结论 | 状态 | 备注 | 需补充 |
|---|---|---|---|---|---|
| DEC-001 | 当前协议基线 | 使用 `gophertunnel v1.56.1` 当前 `protocol 975`、`MC 1.26.20` | DONE | 用户确认不要回退到 944；源项目 `ProtocolInfo.php` 的 944 仅作历史参考 | 否 |
| DEC-002 | 初始目标 Go module | `gucooing/bds` | DONE | 来自 `go.mod` | 否 |
| DEC-003 | 插件兼容策略 | 未定 | NEEDS-SUPPLEMENT | 选项：Go 原生插件、脚本插件、PHP 进程桥接、只兼容配置和行为 | 待确认：是否必须兼容 API3 PHP 插件 |
| DEC-004 | RakNet 策略 | 初始采用 `github.com/sandertv/go-raknet`，通过 `internal/network/raknet` 适配层隔离 | DONE | 当前由依赖解析选用 `v1.15.1-0.20260112202637-beca0b10c217`；先适配成熟库以推进 M1/M2/M3，若 M6 暴露底层控制或稳定性问题，再 fork 或替换 | 否 |
| DEC-005 | 世界存储格式 | 未定 | NEEDS-SUPPLEMENT | 选项：PMMP Anvil/LevelDB 兼容、仅新格式、双格式 | 待确认：是否需要直接读取旧世界 |
| DEC-006 | 授权与署名 | 项目保持 LGPL-3.0，保留 `LICENSE`/`NOTICE`，上游来源和第三方依赖在发行前登记 | DONE | 默认不为每个新 Go 文件加版权头；非平凡移植上游行为、数据表或算法时在局部源码或文档加来源说明 | 否 |
| DEC-007 | 最小可玩目标 | 单人平坦世界 + 移动 + 聊天 + 基础方块交互 | TODO | 作为 M3/M4 的范围边界 | 否 |
| DEC-008 | 初始配置格式 | `server.properties` | DONE | 先用于 Go 服务端常规配置；PocketMine YAML 兼容后续作为迁移/兼容 Provider 处理 | 否 |
| DEC-009 | 协议与登录实现策略 | `gophertunnel` 直接用于 packet 定义、packet pool、字段编解码、batch 压缩和登录链解析；本项目不保留空转 `internal/protocol` 包，MCPE batch/encryption 放在 `internal/network/mcpe`，登录解析/握手/路由和业务流程放在 `internal/server` | DONE | 用户明确要求不能把实际逻辑交给 gophertunnel，也不能建立无意义协议包装层；保留 `go-raknet` 作为 RakNet 传输库，保留本地边界 | 否 |

## 5. 优先级规则

| 优先级 | 含义 |
|---|---|
| P0 | 阻塞主线或里程碑验收，优先处理 |
| P1 | 主线能力，按阶段推进 |
| P2 | 完整性、性能、兼容性增强 |
| P3 | 可延后优化、体验或维护项 |

## 6. 分阶段任务总表

### 6.1 阶段 A：项目治理与基线梳理

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| A-001 | 建立 Go 重写任务书 | P0 | DONE | 本文件存在并可持续更新 | 已创建 `REWRITE_TASKBOOK.md` | 后续所有对话先读本文件 | 否 |
| A-002 | 建立 git 仓库或确认版本管理方式 | P0 | DONE | `git status` 可用，或记录不用 git 的替代方案 | 已在 `bds` 目录执行 `git init` | 当前文件均为初始未跟踪状态，等待后续提交策略 | 否 |
| A-003 | 梳理 BetterAltay 模块清单和源码规模 | P0 | DONE | 完成模块数量基线 | 已记录主要模块和文件数 | 统计来自 `src/pocketmine` | 否 |
| A-004 | 明确重写级别：行为兼容/API 兼容/插件兼容 | P0 | NEEDS-SUPPLEMENT | 写入 `DEC-003` 和兼容矩阵 |  | 插件策略会影响架构 | 待用户确认 |
| A-005 | 建立设计文档目录 | P1 | DONE | `docs/architecture.md`、`docs/protocol.md` 等存在 | 已创建 `docs/architecture.md` 和 `docs/protocol.md` | 记录协议适配和模块边界 | 否 |
| A-006 | 建立编码规范 | P1 | DONE | `docs/coding-standards.md` 或 README 记录 | 已创建 `docs/coding-standards.md` | 已包含禁止无意义硬编码、小函数和中转函数，以及并发和数据来源规则 | 否 |
| A-007 | 建立许可证/NOTICE 策略 | P0 | DONE | `LICENSE`、`NOTICE`、源码头策略明确 | 已补充 `LICENSE`、`NOTICE` 和 `docs/license-notice.md` | 法律细节仍应在正式发布前人工复核；当前工程策略已明确 | 否 |
| A-008 | 建立迁移兼容矩阵 | P1 | TODO | 表格列出配置、世界、插件、资源包、玩家数据兼容性 |  |  | 待确认旧数据兼容目标 |

### 6.2 阶段 B：Go 项目骨架

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| B-001 | 创建 `cmd/bds` 入口 | P0 | DONE | `go run ./cmd/bds` 可启动 | 已创建 `cmd/bds/main.go`，调用 bootstrap 启动流程 | `go run ./cmd/bds -version` 通过 | 否 |
| B-002 | 建立基础 package 目录 | P0 | DONE | 目录与初始空测试存在 | 已建立 `cmd/bds`、`internal/bootstrap`、`docs`，并添加 bootstrap 测试 | 遵循 1.4，不创建无代码空包和空转中转层 | 否 |
| B-003 | 建立版本信息模块 | P0 | DONE | 输出 BetterAltay-Go 版本、协议号、MC 版本 | 已创建 `internal/bootstrap/version.go` | 常量对应 `DEC-001` 和源项目 VersionInfo/ProtocolInfo | 否 |
| B-004 | 建立配置加载 | P0 | DONE | 支持默认配置和本地覆盖 | 已实现 `internal/config`，首次启动生成 `server.properties`，支持本地覆盖和基础校验 | 初始采用 `server.properties`，见 `DEC-008` | 否 |
| B-005 | 建立日志系统 | P0 | DONE | 控制台彩色日志、文件日志、日志级别 | 已实现 `internal/logging`，支持 `debug/info/warn/error`、ANSI 彩色控制台输出和文件日志 | 文件路径由 `server.properties` 的 `log-file` 控制，空值可关闭文件日志 | 否 |
| B-006 | 建立优雅关闭 | P1 | REVIEW | Ctrl+C、命令 stop、异常退出路径可测 | 已在入口接入 `os.Interrupt`/`SIGTERM`，bootstrap 可等待 context 关闭并有测试覆盖 | 世界保存、踢出玩家、网络关闭需等对应子系统接入后补钩子 | 是：后续子系统接入关闭钩子 |
| B-007 | 建立基础测试框架 | P0 | DONE | `go test ./...` 通过 | 已添加 `internal/bootstrap/bootstrap_test.go` | 覆盖版本输出和 data path 输出 | 否 |
| B-008 | 建立 lint/format 脚本 | P1 | DONE | `gofmt`、`go test`、可选 lint 一键运行 | 已建立 `scripts/test.ps1`，支持格式化、`go test ./...`、可选 `-Race` 和 `-RunCheck` | Windows PowerShell 脚本已作为当前主入口；后续 CI 可复用 | 否 |
| B-009 | 建立错误与 panic 策略 | P1 | TODO | 文档和公共 helper |  | 区分配置错误、协议错误、内部 bug | 否 |

### 6.3 阶段 C：协议与 RakNet 网络层

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| C-001 | RakNet 方案调研与决策 | P0 | DONE | 写入 `DEC-004`，列出取舍和压测结果 | 已决定初始适配 `github.com/sandertv/go-raknet v1.15.0`，并记录到 `docs/network-raknet.md` | 当前完成 smoke test，尚未进行多人压测 | 否 |
| C-002 | 实现或接入 UDP 监听 | P0 | DONE | 绑定端口，处理启动失败 | 已新增 `internal/network/raknet` 低层适配，并在运行时通过 `internal/network/mcpe` 按 `server-address/server-port` 启动本地 MCPE 会话入口 | 默认 19132；`-check` 不启动网络监听 | 否 |
| C-003 | 实现 RakNet ping/pong | P0 | DONE | 客户端列表可看到 MOTD | 已实现 BetterAltay 风格 pong 数据并用 `raknet.PingTimeout` 本地验证 | M1 已可由测试工具验收；真实 Bedrock 客户端列表待手工补测 | 否 |
| C-004 | 实现 RakNet session 生命周期 | P0 | DONE | 连接、断开、超时、MTU、可靠包 | 已为 `go-raknet` 会话增加可注入处理器、活跃连接跟踪和关闭等待；MTU/可靠传输由底层库负责 | 当前只做会话生命周期边界，MCPE 登录和包分发仍待实现 | 否 |
| C-005 | 实现 MCPE batch/压缩层 | P0 | DONE | packet batch 编解码测试通过 | 已由 `internal/network/mcpe/codec.go` 接入 gophertunnel 的 packet pool、字段编解码和 batch 压缩/解压 | 使用 `packet.DefaultCompression` 和 256 阈值 | 否 |
| C-006 | 实现登录链解析 | P0 | DONE | LoginPacket JWT/skin/device 信息可解析 | 已由 `internal/server/mcpe_login.go` 接入 gophertunnel 登录解析，并补充离线登录测试；运行时由 `internal/server` 保存连接身份信息 | 在线认证仍需真实账号/客户端验收 | 否 |
| C-007 | 实现加密握手 | P1 | REVIEW | 支持 ServerToClientHandshake/ClientToServerHandshake | 已在本地 MCPE session 中实现 P-384 ECDH、ServerToClientHandshake JWT salt、batch AES-CTR 加密/校验和 ClientToServerHandshake 确认；测试覆盖本地虚拟客户端加密登录 | 尚未用真实 Bedrock 客户端验证；不能交给 `minecraft.Conn` | 否 |
| C-008 | 实现包分发器 | P0 | DONE | 按 packet ID 路由到 handler | 已移除无意义 `internal/protocol` dispatcher；当前由 `internal/server/mcpe_router.go` 对 gophertunnel `packet.Packet` 做服务端会话路由 | 路由归属业务会话，协议包定义仍直接使用 gophertunnel | 否 |
| C-009 | 建立协议 fuzz/边界测试 | P1 | TODO | 畸形包不导致崩溃 |  |  | 否 |
| C-010 | 建立虚拟客户端测试工具 | P1 | TODO | 可自动 ping、握手、登录 smoke test | 已撤回基于 gophertunnel `minecraft.Dialer` 的完成结论；当前仅有本地 raw session 测试和 RakNet ping smoke test | 后续虚拟客户端也必须只使用 packet 编解码/打包能力，不使用 gophertunnel 托管连接逻辑 | 否 |

### 6.4 阶段 D：协议包定义与生成

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| D-001 | 抽取 `ProtocolInfo.php` 常量到 Go | P0 | TODO | Go 常量覆盖协议号与 packet ID |  | 可手工或生成 | 否 |
| D-002 | 建立协议读写基础类型 | P0 | DONE | varint、zigzag、little endian、string、UUID 测试通过 | 已直接使用 gophertunnel Reader/Writer 和 packet header；MCPE batch codec 位于 `internal/network/mcpe` | 遵循 `DEC-009`，不保留自写二进制流实现，也不保留空转 `internal/protocol` 包 | 否 |
| D-003 | 建立 Packet interface | P0 | DONE | `ID()`、`Marshal`、`Unmarshal` 或等价接口 | 已直接复用 gophertunnel 的 `packet.Packet` 接口 | 不建立本仓库 Packet 别名/包装接口 | 否 |
| D-004 | 实现登录阶段核心包 | P0 | REVIEW | Login、PlayStatus、Handshake、Disconnect、ResourcePack、StartGame | `internal/server` MCPE 会话已处理 NetworkSettings、Login 解析、本地加密 Handshake、PlayStatus、空 ResourcePack 和 StartGame 包构造；`internal/network/mcpe` 仅做连接适配 | packet 定义/编解码可复用 gophertunnel，流程逻辑必须本地实现；真实客户端验收仍属 M2 风险 | 否 |
| D-005 | 实现世界同步核心包 | P0 | REVIEW | LevelChunk、SubChunk、UpdateBlock、SetTime、SetSpawnPosition | 已在 `internal/server` 接入 RequestChunkRadius/SubChunkRequest；初始世界同步发送 SetTime、SetSpawnPosition、NetworkChunkPublisherUpdate 和 limited LevelChunk；SubChunk 响应补齐 height map/render height map 并覆盖 flat spawn 编码测试；UpdateBlock 包构造测试已补充 | packet 定义/编解码复用 gophertunnel；LevelChunk/SubChunk 直接按当前协议编码，本地使用 BDS hash block network ID；完整方块/物品玩法交互仍留给后续 M4 路由真实实现 | 需补充真实客户端可见性验收 |
| D-006 | 实现玩家同步核心包 | P0 | REVIEW | AddPlayer、MovePlayer、PlayerList、SetActorData、SetActorMotion | 已在 `internal/server` 新增本地玩家列表和玩家状态；StartGame 后发送 PlayerList/SetActorData/SetActorMotion，SetLocalPlayerAsInitialised 后向已生成玩家互发 AddPlayer/SetActorData/SetActorMotion；接入 PlayerAuthInput 与 MovePlayer 路由并广播 MovePlayer/SetActorData/SetActorMotion；补齐真实客户端进服时的 ServerBoundLoadingScreen、Interact 与 ContainerClose 路由：加载屏 start/end 状态记录与 ID 校验、鼠标悬停实体记录、自带背包 ContainerOpen/Close 窗口生命周期 | 参考 BetterAltay 的 PlayerList、AddPlayer 和 MovePlayer 流程；自带背包打开只处理窗口生命周期，物品内容/热栏/StackRequest 仍属 D-008；碰撞、反作弊、离线移除和多人压测留给后续 E/J/M6 | 需补充真实客户端多人验收 |
| D-007 | 实现聊天与命令包 | P1 | REVIEW | Text、CommandRequest、AvailableCommands、CommandOutput | 已在 `internal/server` 路由 Text 与 CommandRequest；StartGame 后发送 AvailableCommands；CommandRequest 用 CommandOutput 回包；普通 Text 聊天广播给已生成玩家 | 命令解析/权限/控制台输入采用本地最小实现，packet 定义仍复用 gophertunnel；真实客户端命令 UI 仍需验收 | 需补充真实客户端命令 UI 验收 |
| D-008 | 实现背包与物品核心包 | P1 | REVIEW | InventoryContent、InventorySlot、MobEquipment、ItemStackRequest/Response | 已在 `internal/server` 新增服务端权威背包状态：请求视距后发送主背包/副手 InventoryContent 与当前手持 MobEquipment；AddPlayer 带当前 HeldItem；接入 MobEquipment 路由并广播热栏切换；接入独立 ItemStackRequest 和 PlayerAuthInput 内嵌 ItemStackRequest，支持 Take/Place/Swap/Drop/Destroy/MineBlock 的服务端校验、状态提交、ItemStackResponse OK/Error 与失败后的权威背包回同步；InventorySlot 用于热栏物品不一致时回补 | 当前初始背包仍为空，创造栏内容、配方合成、真实掉落实体、完整物品 runtime/ItemRegistry 生成留给 I/H 后续；需真实 Bedrock 客户端验收背包 UI 与热栏手感 | 需补充真实客户端背包/热栏验收 |
| D-009 | 实现资源包包组 | P1 | REVIEW | ResourcePacksInfo/ResourcePackStack/ResourcePackChunkData/ResourcePackChunkRequest | 已在 `internal/resourcepack` 建立资源包 Pack/Queue 模型，`internal/server` 发送资源包信息、处理 `ResourcePackClientResponse`、`ResourcePackChunkRequest` 与 `ResourcePacksReadyForValidation`，支持空包和内存资源包逐块下载 | 当前默认仍可空资源包，亦可通过 `MCPEOptions.ResourcePacks` 注入内存资源包；真实客户端资源包下载流程仍待验收 | 否 |
| D-010 | 建立协议包覆盖率表 | P1 | TODO | 列出 PHP 包、Go 包、测试状态 |  | 建议生成到 `docs/protocol-packets.md` | 否 |
| D-011 | 建立协议样例数据目录 | P1 | TODO | golden packet 测试数据可复用 |  | 来自源测试、抓包或手写 | 需补充抓包方式 |

### 6.5 阶段 E：服务器核心生命周期

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| E-001 | 实现 Server 核心对象 | P0 | REVIEW | 管理配置、日志、网络、世界、玩家、命令 | 新增 `internal/server.Server`，由核心对象持有配置、日志、MCPE handler、网络 listener、世界 provider、玩家表和命令入口；`bootstrap` 退回到参数/配置/日志和进程关闭外壳 | 对标 `Server.php` 的最小核心；插件、事件和完整持久化后续阶段继续扩展 | 否 |
| E-002 | 实现 tick loop | P0 | REVIEW | 固定 20 TPS，能统计 tick time | `Server.Start` 启动 20 TPS tick loop，记录 tick 数、最后 tick 耗时和 TPS 平滑值，测试覆盖 tick 推进 | 当前 tick loop 先承载生命周期和任务队列，世界/实体 tick 会在 G/H/J 后续接入 | 否 |
| E-003 | 实现主线程任务队列 | P1 | REVIEW | 跨 goroutine 操作回到主 loop | 新增 `Server.Submit` 有界队列并在核心 loop 中执行，任务 panic 会记录错误而不直接杀死 runtime；测试覆盖提交任务被执行 | 后续世界、实体、插件 API 需要统一走此队列收束并发所有权 | 否 |
| E-004 | 实现 Player session 状态机 | P0 | REVIEW | Handshaking/Login/Spawned/Disconnected | MCPE 状态机补齐 `stateDisconnected`，网络 session 结束时通过 `DisconnectAware` 回调到 server 会话；断开后忽略后续包 | Handshaking/Login/Spawned 仍沿用本地 MCPE state machine；真实客户端异常断线仍需压测 | 需补充真实客户端断线验收 |
| E-005 | 实现玩家加入/离开流程 | P0 | REVIEW | 广播加入离开、保存基础数据 | 玩家生成后向已生成玩家广播加入翻译消息；session 断开会从玩家表移除，广播 PlayerList remove、RemoveActor 和离开翻译消息；新增 `PlayerStore` 钩子保存 UUID/name/XUID/位置/旋转/速度/lastSeen 快照 | 磁盘玩家数据格式尚未决定，当前只提供生命周期保存钩子；多人真实断线仍需验收 | 需补充真实客户端多人验收 |
| E-006 | 实现 MOTD 与 Query 基础信息 | P1 | REVIEW | 在线人数、版本、协议、世界名 | RakNet/MCPE listener 支持运行时刷新 pong 数据；玩家加入/离开时更新 MOTD 在线人数；`Server.Info()` 提供服务器名、品牌、协议、MC 版本、游戏模式、在线人数、最大人数和地址快照 | 当前覆盖 Bedrock server list/MOTD 和本地 Query 信息快照，尚未实现额外 Query 协议扩展 | 否 |
| E-007 | 实现崩溃报告/错误诊断 | P2 | REVIEW | panic 时输出版本、goroutine、玩家数 | 核心 runtime panic 时记录版本、uptime、tick、在线人数和 goroutine 数；主线程任务 panic 独立记录，避免单个任务直接击穿 loop | 这是最小诊断；完整 CrashDump 文件、堆栈归档和崩溃保存留给后续 P/O | 否 |
| E-008 | 实现内存/性能守护 | P2 | REVIEW | TPS、内存、goroutine、网络队列指标 | `Server.Stats()` 暴露 uptime、tick、last tick time、TPS、在线人数、goroutine、Alloc、HeapInuse 和任务队列长度/容量；测试覆盖指标可用 | 目前是基础观测面，不含阈值告警、自动 GC 策略或长稳趋势记录 | 否 |

### 6.6 阶段 F：配置、语言和资源

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| F-001 | 梳理源项目资源文件 | P1 | TODO | 资源清单与复制/转换策略 |  | `src/pocketmine/resources`、`lang/locale` | 否 |
| F-002 | 实现配置读写 | P0 | TODO | 首次启动生成默认配置，配置错误可读 |  | server.properties/yaml/json 待定 | 需确认格式 |
| F-003 | 实现语言文件加载 | P1 | TODO | 可读取 locale ini，支持格式化文本 |  | 对标 BaseLang/TextContainer | 否 |
| F-004 | 实现文本颜色/格式码 | P1 | TODO | MC 颜色码、控制台格式转换测试通过 |  | 对标 TextFormat | 否 |
| F-005 | 实现 ops、whitelist、banlist 数据 | P1 | TODO | 可加载/保存，命令能修改 |  | 对标 permission/BanList | 否 |
| F-006 | 实现玩家数据目录策略 | P1 | TODO | 玩家 UUID/name 数据可保存 |  |  | 需确认格式 |

### 6.7 阶段 G：NBT、世界、区块和生成器

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| G-001 | NBT 方案决策 | P0 | TODO | 选择自研/库，读写测试通过 |  | 源项目使用 benedikt05/nbt | 需调研 Go NBT 库或自研 |
| G-002 | 实现 NBT 基础读写 | P0 | TODO | compound/list/number/string/byte array 测试通过 |  | 世界与物品均依赖 | 否 |
| G-003 | 决定世界存储格式 | P0 | NEEDS-SUPPLEMENT | 写入 `DEC-005` |  | 是否读取旧世界是关键 | 待用户确认 |
| G-004 | 实现 Level/World 抽象 | P0 | TODO | 世界加载、卸载、tick、玩家加入 |  | 对标 `level` 模块 | 否 |
| G-005 | 实现 Chunk/SubChunk 数据结构 | P0 | REVIEW | 坐标、区块状态、方块读写测试通过 | 已新增 `internal/world.Chunk` 和 `ChunkProvider`，通过本地 chunk 数据结构保存方块和 biome | 需要后续补世界加载、保存和更多读写测试 | 否 |
| G-006 | 实现方块 palette/runtime ID 映射 | P0 | REVIEW | 能编码客户端可识别区块 | 已按官方 BDS `block-network-ids-are-hashes=true` 行为计算 air/bedrock/dirt/grass 等 hash block network ID，并能编码网络区块 payload | 后续补完整 block registry/生成器，避免手写更多方块状态 | 否 |
| G-007 | 实现空世界/平坦世界生成器 | P0 | REVIEW | 客户端能看到地面 | 已新增默认平坦世界生成器，测试可解码出生区块 y=63 草地方块 | 真实客户端可见性仍依赖 M2 加密/Spawn 闭环 | 需补充真实客户端验收 |
| G-008 | 实现区块发送调度 | P0 | DOING | 按玩家视距发送/卸载区块 | 已有初始视距内 chunk packet publisher，可生成 `NetworkChunkPublisherUpdate`、`SetTime`、`SetSpawnPosition`、`LevelChunk`，并可响应 `SubChunkRequest` | 还缺动态视距、卸载、缓存和真实客户端可见性验收 | 否 |
| G-009 | 实现区块保存 | P1 | TODO | 变更方块后重启仍存在 |  | M5 必需 | 否 |
| G-010 | 实现实体/Tile 持久化挂钩 | P1 | TODO | Tile 和 Entity 可随区块保存 |  | 后续箱子/告示牌依赖 | 否 |
| G-011 | 实现世界时间/天气/难度 | P2 | TODO | 协议同步、配置可控 |  |  | 否 |
| G-012 | 建立世界格式兼容测试 | P1 | TODO | 样例世界可加载/保存/回归 |  | 需准备样例世界 | 需补充测试素材 |

### 6.8 阶段 H：方块系统

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| H-001 | 设计方块注册表 | P0 | TODO | ID、名称、状态、硬度、碰撞箱可查询 |  | 对标 `block` 208 文件 | 否 |
| H-002 | 建立基础方块集合 | P0 | TODO | air、stone、grass、dirt、bedrock、water 等可用 |  | M3/M4 必需 | 否 |
| H-003 | 实现方块状态与 meta 转换 | P1 | TODO | 源项目常见 meta 逻辑有测试 |  | 旧 API 兼容点 | 需补充映射表 |
| H-004 | 实现碰撞箱/AABB | P0 | TODO | 玩家不能穿过实体方块 |  | M4 必需 | 否 |
| H-005 | 实现破坏逻辑 | P1 | TODO | 硬度、工具、掉落、事件钩子 |  | M4 基础可简化 | 否 |
| H-006 | 实现放置逻辑 | P1 | TODO | 朝向、替换、水、相邻更新 |  | M4 基础可简化 | 否 |
| H-007 | 实现方块更新系统 | P1 | TODO | 随机 tick、计划更新、邻居更新 |  | 农作物/流体/红石基础 | 否 |
| H-008 | 实现 Tile 方块入口 | P1 | TODO | 箱子、熔炉、告示牌、床等可挂 Tile |  | 与阶段 M 关联 | 否 |
| H-009 | 建立方块覆盖率表 | P2 | TODO | PHP 方块到 Go 方块状态映射 |  | 可先覆盖常用方块 | 否 |

### 6.9 阶段 I：物品、背包和合成

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| I-001 | 设计物品注册表 | P0 | TODO | ID、名称、最大堆叠、耐久、组件可查询 |  | 对标 `item` 174 文件 | 否 |
| I-002 | 实现 ItemStack 数据结构 | P0 | TODO | 物品 ID、数量、meta/NBT、比较、序列化 |  | 协议和库存依赖 | 否 |
| I-003 | 建立基础物品集合 | P0 | TODO | 方块物品、基础工具、食物可编码 |  | M4 必需 | 否 |
| I-004 | 实现玩家背包与热栏 | P0 | TODO | 客户端能看到并切换热栏 |  | M4 必需 | 否 |
| I-005 | 实现物品使用流程 | P1 | TODO | 右键/长按、食物、桶、弓等逐步支持 |  | 可分批 | 否 |
| I-006 | 实现容器抽象 | P1 | TODO | chest/furnace 等容器可打开/同步 |  |  | 否 |
| I-007 | 实现合成数据发送 | P1 | TODO | 客户端能看到基础配方 |  | CraftingData/UnlockedRecipes | 需补充配方资源 |
| I-008 | 实现 ItemStackRequest/Response | P0 | TODO | 移动物品、丢弃、创造拿取基本正确 |  | 新协议关键 | 需补充协议测试 |
| I-009 | 建立物品覆盖率表 | P2 | TODO | PHP 物品到 Go 物品映射 |  |  | 否 |

### 6.10 阶段 J：实体、玩家和移动

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| J-001 | 设计 Entity/Actor 基类 | P0 | TODO | runtime ID、位置、速度、metadata、tick |  | 对标 `entity` 模块 | 否 |
| J-002 | 实现 Player 实体 | P0 | TODO | 名称、UUID、游戏模式、权限、皮肤、连接状态 |  | M3 必需 | 否 |
| J-003 | 实现移动接收与广播 | P0 | TODO | 单人移动不回弹，多人可见 |  | M3/M6 必需 | 否 |
| J-004 | 实现碰撞与落地基础判断 | P1 | TODO | 方块碰撞、防穿墙、落地状态 |  | 可先服务端宽松校验 | 否 |
| J-005 | 实现生命值/饥饿/经验属性 | P1 | TODO | 协议同步，命令可修改 |  | UpdateAttributes/SetHealth | 否 |
| J-006 | 实现伤害系统 | P1 | TODO | 实体/方块/摔落/火焰伤害事件 |  | 对标 EntityDamageEvent | 否 |
| J-007 | 实现物品实体 | P1 | TODO | 掉落物生成、拾取、过期 |  | M4/M5 后增强 | 否 |
| J-008 | 实现基础生物框架 | P2 | TODO | 被动/怪物实体可 spawn、tick、同步 |  | AI 可后置 | 否 |
| J-009 | 实现传送、维度与出生点 | P1 | TODO | teleport/respawn/change dimension 流程 |  |  | 否 |

### 6.11 阶段 K：事件、命令、权限

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| K-001 | 设计事件总线 | P0 | TODO | 事件注册、优先级、取消机制、同步派发 |  | 对标 `event` 127 文件 | 否 |
| K-002 | 实现核心事件 | P0 | TODO | PlayerJoin/Quit/Move/Chat、BlockBreak/Place、Command |  | M4/M7 必需 | 否 |
| K-003 | 设计命令框架 | P0 | REVIEW | 命令注册、权限、参数、帮助 | 新增 `internal/command` Registry、命令分发、参数描述和 `/help` 输出 | 当前为最小框架；未实现事件、插件注册和复杂参数解析 | 否 |
| K-004 | 实现控制台输入 | P0 | REVIEW | 可输入 stop、list、say、op 等 | bootstrap 启动控制台输入循环，控制台 sender 拥有全部权限，`stop` 可触发 shutdown cancel | stdin 在测试中未做交互式端到端验证；命令执行单元测试已覆盖 stop | 否 |
| K-005 | 实现基础命令集 | P1 | DOING | stop、help、list、say、gamemode、tp、give、time、op、ban | 已实现 help、list、say、stop、op、deop | gamemode、tp、give、time、ban 等留给后续玩法/库存/实体阶段 | 否 |
| K-006 | 实现权限系统 | P1 | REVIEW | OP、permission node、attachment 或 Go 等价机制 | 新增最小权限管理：玩家默认 help/list，控制台全权限，op/deop 在线玩家后刷新 AvailableCommands | 尚无持久化 ops、permission attachment 或插件权限树 | 否 |
| K-007 | 实现客户端命令枚举同步 | P1 | REVIEW | AvailableCommands 正确显示 | StartGame 后按玩家权限发送 AvailableCommands；op/deop 后重发命令列表 | 需真实客户端验证命令 UI 和自动补全显示 | 需补充真实客户端命令 UI 验收 |

### 6.12 阶段 L：插件/扩展系统

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| L-001 | 决定插件兼容路线 | P0 | NEEDS-SUPPLEMENT | 更新 `DEC-003`，写出不兼容清单 |  | 这是架构级决策 | 待用户确认 |
| L-002 | 设计 Go 原生扩展 API | P1 | TODO | `pkg/api` 草案，事件/命令/调度接口 |  | 不过早承诺稳定 | 依赖 L-001 |
| L-003 | 设计插件元数据格式 | P1 | TODO | plugin.yml 兼容或新 manifest |  | 可读取旧 plugin.yml 但不执行 PHP | 依赖 L-001 |
| L-004 | 实现插件加载器 | P1 | TODO | 加载、启用、禁用、依赖排序 |  | 对标 PluginManager | 依赖 L-002 |
| L-005 | 实现脚本扩展可行性调研 | P2 | TODO | Lua/JS/WASM/外部进程对比 |  | 可作为 PHP 插件替代路线 | 待确认 |
| L-006 | 实现示例插件 | P1 | TODO | 注册命令、监听事件、发送消息 |  | M7 验收用 | 依赖 L-004 |
| L-007 | 建立 API 稳定性政策 | P2 | TODO | SemVer、废弃策略、兼容层 |  | 发布前需要 | 否 |

### 6.13 阶段 M：Tile、容器、地图、表单、资源包

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| MISC-001 | 设计 Tile 抽象 | P1 | TODO | Tile 注册、加载、保存、网络同步 |  | 对标 `tile` 23 文件 | 否 |
| MISC-002 | 实现 Sign | P1 | TODO | 放置、编辑、保存、显示文本 |  |  | 否 |
| MISC-003 | 实现 Chest | P1 | TODO | 打开、同步、保存物品 |  | 依赖库存系统 | 否 |
| MISC-004 | 实现 Furnace/Hopper 等容器 | P2 | TODO | 基础 tick 和配方处理 |  | 可后置 | 否 |
| MISC-005 | 实现 Form API | P2 | TODO | ModalFormRequest/Response 可用 |  | BetterAltay 有 `form` 模块 | 否 |
| MISC-006 | 实现 Map 系统 | P3 | TODO | 地图数据、渲染器、同步 |  | 对标 `maps` | 否 |
| MISC-007 | 实现 ResourcePack 管理 | P1 | TODO | 资源包校验、发送、客户端响应 |  | 可先空包栈 | 否 |
| MISC-008 | 实现 Scoreboard/Bossbar/Title 基础 API | P2 | TODO | 常用显示功能可用 |  | 协议包逐步补齐 | 否 |

### 6.14 阶段 N：调度、并发和异步任务

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| N-001 | 设计 goroutine 所有权模型 | P0 | TODO | 文档说明哪些数据只能主线程访问 |  | 防止 data race | 否 |
| N-002 | 实现 Scheduler | P1 | TODO | 延迟任务、重复任务、取消任务 |  | 对标 `scheduler` | 否 |
| N-003 | 实现异步任务接口 | P1 | TODO | 后台 IO/压缩/生成后回主线程回调 |  | 类似 AsyncTask | 否 |
| N-004 | 加入 race 测试流程 | P1 | TODO | 关键 package 可 `go test -race` |  | Windows 下可能较慢 | 否 |
| N-005 | 建立网络背压策略 | P1 | TODO | 玩家发送队列满时限流/断开/降级 |  | 多人稳定性关键 | 否 |

### 6.15 阶段 O：测试、工具链和 CI

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| O-001 | 建立单元测试基线 | P0 | TODO | 核心 package 均有 smoke test |  | 先保持 `go test ./...` 快速 | 否 |
| O-002 | 建立协议 golden tests | P0 | TODO | 样例包 round-trip 通过 |  | D 阶段关键 | 需补充样例 |
| O-003 | 建立集成测试 | P1 | TODO | 虚拟客户端完成 ping/login/spawn | 已撤回依赖 gophertunnel `minecraft.Dialer` 的集成测试完成结论；当前保留 RakNet ping smoke test 和本地 raw MCPE session 测试 | 后续虚拟客户端必须走本项目自有会话逻辑，不使用 gophertunnel 托管连接 | 否 |
| O-004 | 建立性能基准 | P1 | TODO | packet 编解码、chunk 编码、tick loop benchmark |  |  | 否 |
| O-005 | 建立兼容性测试清单 | P1 | TODO | 用真实客户端逐项验收并记录 |  | 需要手工测试 | 需准备客户端版本 |
| O-006 | 建立 CI 脚本 | P2 | TODO | Windows/Linux 至少 test/build |  | 当前没有 git/CI | 依赖 A-002 |
| O-007 | 建立代码生成工具 | P2 | TODO | 协议常量、包清单、方块/物品注册表可生成 |  | 降低手工错误 | 否 |

### 6.16 阶段 P：性能、观测和稳定性

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| P-001 | 实现 TPS/内存/玩家数指标 | P1 | TODO | 命令或日志可查看 |  | M6/M8 需要 | 否 |
| P-002 | 实现 profiling 开关 | P2 | TODO | 可按配置启用 pprof 或等价能力 |  | 注意默认关闭 | 否 |
| P-003 | 优化 packet 编解码分配 | P2 | TODO | benchmark 有基线和优化记录 |  |  | 否 |
| P-004 | 优化 chunk 编码缓存 | P2 | TODO | 玩家视距内重复发送成本可控 |  |  | 否 |
| P-005 | 实现崩溃自动保存 | P1 | TODO | panic/stop 前尽力保存玩家和世界 |  |  | 否 |
| P-006 | 建立长稳测试 | P2 | TODO | 空服/假人/真实客户端运行 2-24 小时 |  | 记录内存和 goroutine 增长 | 需准备测试工具 |

### 6.17 阶段 Q：发布、文档和迁移

| ID | 任务 | 优先级 | 状态 | 产出/验收 | 完成情况 | 备注 | 需补充 |
|---|---|---|---|---|---|---|---|
| Q-001 | 编写 README | P1 | TODO | 构建、运行、配置、版本说明 |  | 发布前必须 | 否 |
| Q-002 | 编写管理员文档 | P2 | TODO | 配置、命令、权限、备份、故障处理 |  |  | 否 |
| Q-003 | 编写开发者文档 | P2 | TODO | 架构、插件 API、事件、命令、贡献指南 |  |  | 否 |
| Q-004 | 编写旧项目迁移说明 | P1 | TODO | 配置、世界、玩家数据、插件迁移路径 |  | 依赖兼容决策 | 待确认 |
| Q-005 | 建立构建产物 | P1 | TODO | Windows/Linux 可执行文件，默认配置 |  |  | 否 |
| Q-006 | 建立发布检查清单 | P1 | TODO | 版本号、协议号、测试、文档、许可证齐全 |  |  | 否 |
| Q-007 | 编写已知问题清单 | P1 | TODO | 明确未实现功能和兼容限制 |  | 避免误用 | 否 |

## 7. 源模块迁移台账

| 源模块 | 文件数 | Go 目标模块 | 状态 | 完成情况 | 备注 | 需补充 |
|---|---:|---|---|---|---|---|
| root constants/classes | 13 | `internal/bootstrap`、`internal/server` | DOING | 已建立版本信息、bootstrap 外壳和 `internal/server.Server` 核心生命周期对象；Server 负责 listener、tick loop、任务队列、玩家/命令入口、Stats/Info 与最小崩溃诊断 | Player 公共 API、PocketMine 风格静态入口和完整 Server API 仍待后续设计 | 否 |
| `network` | 382 | `internal/network`、`internal/server` | DOING | 已完成 RakNet 初始适配、UDP 监听、unconnected ping/pong、session 生命周期、packet/batch codec、本地 MCPE session 初版、本地加密握手、玩家同步核心包路由、session 断开回调和 MOTD 在线人数刷新；已移除 gophertunnel `minecraft.Listener` 运行时路径、旧 debug session 路径和空转 `internal/protocol` 包 | 真实 Bedrock 客户端验收、多玩家稳定性和后续玩法包处理仍未完成 | 需补充真实客户端测试 |
| `block` | 208 | `internal/block` | TODO |  | 方块状态、碰撞、更新、掉落 | 需补充 runtime ID 映射 |
| `item` | 174 | `internal/item` | TODO |  | 物品注册表、工具、食物、附魔 | 需补充物品映射 |
| `level` | 144 | `internal/world` | DOING | 已建立可替换 `ChunkProvider`、默认平坦世界生成器、基础 Chunk 包装和区块网络编码测试 | 世界加载/保存、tick、动态区块调度和旧格式兼容仍未实现 | 需确认存储格式 |
| `event` | 127 | `internal/event` | TODO |  | 事件总线和核心事件 | 否 |
| `entity` | 119 | `internal/entity`、`internal/server` | DOING | 已在 `internal/server` 建立 MCPE 玩家同步状态和 AddPlayer/MovePlayer/PlayerList/SetActorData/SetActorMotion 发包；完整 Entity/Player 领域模块仍未抽取 | 玩家、移动、伤害、生物 | 否 |
| `command` | 65 | `internal/command`、`internal/server` | DOING | 已建立最小命令注册表、权限过滤、控制台入口、Text/CommandRequest/AvailableCommands/CommandOutput 路由和 `help/list/say/stop/op/deop` 默认命令 | 完整命令参数类型、选择器、权限树和插件命令仍待后续 | 否 |
| `lang` | 46 | `internal/lang` | TODO |  | locale ini、文本容器 | 否 |
| `inventory` | 44 | `internal/inventory` | TODO |  | 背包、窗口、交易、合成 | 需补充协议测试 |
| `resources` | 25 | `internal/resources` 或 `assets` | TODO |  | 方块/物品/配方/实体资源 | 需盘点 |
| `tile` | 23 | `internal/tile` 或 `internal/world/tile` | TODO |  | 容器方块、告示牌等 | 否 |
| `utils` | 21 | `internal/math`、`internal/config`、`internal/logging` | TODO |  | UUID、Config、Random、Internet 等 | 否 |
| `form` | 18 | `internal/form` 或 `pkg/api/form` | TODO |  | ModalForm API | 否 |
| `plugin` | 14 | `internal/plugin`、`pkg/api` | TODO |  | 需先决定兼容策略 | 待确认 |
| `scheduler` | 12 | `internal/scheduler` | TODO |  | 主线程和异步任务 | 否 |
| `permission` | 11 | `internal/permission` | TODO |  | OP、Ban、Permission | 否 |
| `metadata` | 7 | `internal/entity/metadata` | TODO |  | Actor metadata | 否 |
| `maps` | 5 | `internal/maps` | TODO |  | 可后置 | 否 |
| `resourcepacks` | 5 | `internal/resourcepack`、`internal/server` | REVIEW | 已建立资源包 Pack/Queue 模型，支持 ResourcePacksInfo/Stack、ChunkRequest 和 ReadyForValidation 流程 | 真实客户端下载链路仍待验收 | 否 |
| `timings` | 2 | `internal/server`、后续 `internal/metrics` | DOING | `Server.Stats()` 已暴露 uptime、tick、last tick time、TPS、在线人数、goroutine、内存和任务队列指标 | 完整 Timings/指标导出、阈值告警和长稳记录仍待后续 | 否 |
| `updater` | 2 | `internal/updater` 或删除 | DEFERRED |  | Go 重写初期不需要自动更新 | 待确认 |
| `wizard` | 1 | `internal/bootstrap` | DEFERRED |  | 初期可用默认配置替代 | 否 |

## 8. 功能兼容矩阵

| 功能 | MVP | 完整目标 | 状态 | 备注 | 需补充 |
|---|---|---|---|---|---|
| 客户端 ping | 必须 | MOTD、人数、协议、世界名完整 | DONE | M1；本地 RakNet ping 测试通过，真实客户端列表待手工补测 | 否 |
| 登录 | 必须 | 在线/离线验证、皮肤、设备信息、封禁检查 | DOING | M2；`internal/server` MCPE 会话已能解析离线 Login、完成本地加密握手、发送 PlayStatus、空资源包栈和 StartGame 包 | 需真实客户端和在线认证验收 |
| 世界 | 平坦世界 | 读取/保存兼容世界、多个世界、生成器 | DOING | M3/M5；已具备默认平坦世界、出生点和世界时间/出生点同步包 | 需确认持久化格式 |
| 区块 | 固定视距发送 | 动态视距、缓存、压缩优化 | DOING | M3；已覆盖初始 LevelChunk、SubChunk 响应和 UpdateBlock 包构造 | 需真实客户端可见性验收 |
| 移动 | 单人不回弹 | 多人同步、碰撞、反作弊基础 | DOING | M3/M6；已接入 PlayerAuthInput 与 MovePlayer，能记录玩家位置/旋转并向已生成玩家广播 MovePlayer、SetActorData、SetActorMotion | 需真实客户端移动手感、碰撞和多人压测 |
| 聊天 | 必须 | 格式化、权限、事件、日志 | TODO | M4 | 否 |
| 命令 | stop/list/say | PMMP 常用命令、参数提示 | TODO | M4 | 否 |
| 方块交互 | 基础破坏/放置 | 掉落、更新、流体、红石、Tile | TODO | M4+ | 否 |
| 背包 | 热栏和基础物品 | 容器、合成、附魔、耐久 | TODO | M4+ | 需协议测试 |
| 实体 | Player/Item | 生物、AI、投射物、载具 | DOING | M6+；已完成玩家同步核心包，完整实体系统仍未抽取 | 否 |
| 插件 | 可延后 | Go API 或其他扩展策略 | NEEDS-SUPPLEMENT | M7 | 待用户确认 |
| 资源包 | 空栈 | 完整发送、校验、分块 | TODO | 可先最小实现 | 否 |
| 表单 | 可延后 | Modal/Custom/Simple Form | TODO | 插件常用 | 否 |
| 权限 | OP 基础 | Permission node、默认权限、附件 | TODO | M4/M7 | 否 |
| 数据迁移 | 可延后 | 配置/世界/玩家/插件迁移说明 | NEEDS-SUPPLEMENT | 发布前必须 | 待确认 |

## 9. 风险与补充需求

| ID | 风险 | 影响 | 状态 | 缓解方式 | 需补充 |
|---|---|---|---|---|---|
| R-001 | PHP 插件生态无法直接在 Go 中运行 | 插件兼容承诺可能无法兑现 | OPEN | 早期明确兼容边界；考虑脚本/外部进程桥接 | 用户决策 |
| R-002 | Bedrock 协议复杂且变化快 | 登录、背包、区块容易不兼容 | OPEN | 用户确认随当前 `gophertunnel v1.56.1` 使用 `protocol 975/MC 1.26.20`；建立 golden tests 和真实客户端验收 | 样例包 |
| R-003 | RakNet 实现质量决定稳定性 | 掉线、卡顿、吞包 | OPEN | 初始采用 `go-raknet` 适配并通过本地 ping smoke test；M2/M6 阶段继续压测，必要时 fork 或替换 | 多人压测结果 |
| R-004 | 世界格式兼容难度高 | 旧服迁移受阻 | OPEN | 先平坦新世界，后做 Provider 适配 | 目标格式 |
| R-005 | 方块/物品 runtime ID 映射庞大 | 客户端显示错误或崩溃 | OPEN | 当前 flat world 已切到官方 BDS hash block network ID；后续使用生成器和资源表补完整方块/物品数据，避免手写状态 | runtime 数据 |
| R-006 | Go 并发误用导致竞态 | 随机崩溃或数据损坏 | OPEN | 主线程所有权模型，race 测试 | 文档和测试 |
| R-007 | 一次性追求完整导致进度失控 | 长期无可运行版本 | OPEN | 以 M0-M4 为第一可玩闭环 | 否 |

## 10. 验收记录模板

后续每次完成里程碑或关键任务时，在这里追加记录。

| 日期 | 任务/里程碑 | 验收方式 | 结果 | 证据/命令 | 备注 |
|---|---|---|---|---|---|
| 2026-05-05 | A-001 | 文件创建检查 | 通过 | `bds/REWRITE_TASKBOOK.md` | 初始任务书 |
| 2026-05-05 | M0/B-001/B-003/B-007 | Go 测试与启动命令 | 通过 | `go test ./...`；`go run ./cmd/bds -version` | Go 环境提示 GOPATH 与 GOROOT 相同，但命令成功 |
| 2026-05-05 | B-004/B-005/B-006 | 配置、日志、关闭骨架 | 通过 | `go test ./...`；`go run ./cmd/bds -data-path .runtime -check` | 覆盖默认配置生成、配置覆盖、日志级别、彩色控制台、文件日志和信号关闭骨架；`.runtime` 已清理并加入 `.gitignore` |
| 2026-05-05 | A-007/B-008 | 文档与脚本检查 | 通过 | `powershell -ExecutionPolicy Bypass -File scripts\test.ps1` | 许可证/NOTICE 策略已写入 `docs/license-notice.md`；脚本执行 gofmt 和测试 |
| 2026-05-05 | M1/C-001/C-002/C-003 | RakNet 本地 ping smoke test | 通过 | `go test ./...`；`powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` | `internal/network/raknet` 测试会启动本地监听并用 `raknet.PingTimeout` 验证 `MCPE;...;` pong |
| 2026-05-05 | C-004 | RakNet session 生命周期 | 通过 | `go test ./internal/network/raknet -count=1 -v`；`go test ./...` | 新增 session handler 注入、活跃会话跟踪和关闭等待；真实连接由 `raknet.DialTimeout` 验证 |
| 2026-05-05 | D-002 | 协议读写基础类型 | 通过 | `go test ./internal/protocol -count=1 -v`；`go test ./...`；`powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` | 覆盖 varint golden、zigzag round-trip、little-endian 数值、string、UUID 网络顺序和 EOF/overflow 边界 |
| 2026-05-05 | DEC-009/C-005/C-006/C-008/D-003 | gophertunnel 协议适配与 debug 网络日志 | 通过 | `go test ./internal/protocol -count=1 -v`；`go test ./internal/bootstrap -count=1 -v`；`go test ./...`；`powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` | 覆盖 batch 压缩、Packet interface、按 ID 分发、离线 Login 解析、NetworkSettings 回复和 debug 日志输出 |
| 2026-05-05 | M2/C-007/C-010/D-004/O-003 | MCPE 登录与 StartGame 集成测试 | 撤回完成结论 | 先前证据基于 gophertunnel `minecraft.Listener`/`minecraft.Dialer` 托管流程 | 用户要求 gophertunnel 只做数据包解析/打包，因此 M2、C-007、C-010、D-004、O-003 已按需降级为 DOING/TODO |
| 2026-05-05 | DEC-009/M2/D-004 | 自有 MCPE session smoke test | 通过 | `go test ./internal/network/mcpe -count=1 -v`；`go test ./internal/protocol -count=1 -v`；`go test ./...`；`powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` | 覆盖本地 RakNet ping、本地 MCPE session 的 NetworkSettings、离线 Login 解析、空资源包栈、StartGame 包发送和 LevelChunk 编码；真实客户端与加密握手仍未完成 |
| 2026-05-06 | C-007/D-004/M2 | 本地 MCPE 加密握手 smoke test | 通过 | `go test ./internal/protocol -count=1 -v`；`go test ./internal/network/mcpe -count=1 -v`；`go test ./...`；`powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` | 覆盖 ServerToClientHandshake JWT salt、P-384 ECDH key 一致性、batch AES-CTR 加密/校验、加密 ClientToServerHandshake、加密后的 PlayStatus/ResourcePacksInfo 和 StartGame；真实 Bedrock 客户端仍待验收 |
| 2026-05-06 | D-005/G-008/M3 | 世界同步核心包 smoke test | 通过 | `go test ./internal/server -count=1 -v`；`go test ./internal/network/mcpe -count=1 -v`；`go test ./...`；`powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` | 覆盖 `internal/server` MCPE 路由和世界同步逻辑、`internal/network/mcpe` batch codec、RequestChunkRadius 后的 SetTime/SetSpawnPosition/NetworkChunkPublisherUpdate/LevelChunk 顺序、SubChunk 响应、UpdateBlock 包构造和 LevelChunk 解码；真实客户端可见性仍待验收 |
| 2026-05-06 | D-006/M3/M6 | 玩家同步核心包 smoke test | 通过 | `go test ./internal/server -count=1 -v`；`go test ./internal/network/mcpe -count=1 -v`；`go test ./... -count=1`；`powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` | 覆盖 PlayerList 自身/全量同步、AddPlayer 同步到双方、SetActorData、SetActorMotion、PlayerAuthInput 位置/旋转/冲刺元数据广播和 legacy MovePlayer 广播；真实 Bedrock 客户端多人验收仍待补充 |
| 2026-05-06 | D-006/真实客户端未处理包补齐 | 真实日志回归与 Go 测试 | 通过 | `go test ./internal/server -count=1`；`go test ./... -count=1`；`powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` | 针对真实客户端日志中的 `ServerBoundLoadingScreen` state 5、`Interact` state 5/6 补齐本地路由：加载屏 start/end 状态记录与可选 ID 校验、鼠标悬停玩家目标记录、自带背包 ContainerOpen、重复打开防抖、ContainerClose/0xff 关闭处理和 close ack；D-008 仍负责物品内容、热栏与 StackRequest |
| 2026-05-06 | D-008/M4 | 背包与物品核心包 smoke test | 通过 | `go test ./internal/server -count=1`；`go test ./... -count=1`；`powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` | 覆盖初始 InventoryContent/MobEquipment 同步、MobEquipment 热栏切换广播、ItemStackRequest Take/Place/Swap/Drop/Destroy/MineBlock OK 响应、StackNetworkID 不一致和未支持 Consume 的 Error 响应、PlayerAuthInput 内嵌 ItemStackRequest；完整物品表/创造栏/合成/掉落实体仍待后续 |
| 2026-05-14 | E-001/E-002/E-003/E-004/E-005/E-006/E-007/E-008 | 服务器核心生命周期 Go 测试与脚本验收 | 通过 | `go test ./internal/server -count=1 -v`；`go test ./internal/network/mcpe ./internal/network/raknet ./internal/bootstrap -count=1`；`go test ./... -count=1`；`powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` | 覆盖 `Server` 核心对象、20 TPS tick loop、主线程任务队列、stop 命令、session 断开清理、加入/离开广播、PlayerStore 快照钩子、MOTD 在线人数刷新、Info/Stats 指标；真实客户端断线和多人压测仍待补充 |

## 11. 后续对话交接模板

后续对话结束时，可以复制并更新这个小节，方便下一次快速恢复。

```text
当前阶段：
本次完成：
仍在进行：
阻塞项：
需要用户确认：
建议下一步：
关键文件：
已运行验证：
未运行验证：
```

当前交接：

| 字段 | 内容 |
|---|---|
| 当前阶段 | 阶段 E：服务器核心生命周期已进入 REVIEW；下一步可继续 M3/M4/M6 验收与玩法闭环 |
| 本次完成 | E-001 到 E-008：新增 `internal/server.Server` 核心对象，收束 bootstrap 临时 wiring；实现 20 TPS tick loop、主线程任务队列、Stats/Info、stop 命令关闭、session 断开回调、玩家加入/离开广播、PlayerStore 快照钩子和 MOTD 在线人数刷新 |
| 仍在进行 | M2 登录握手闭环的真实客户端验收、M3 世界可见性与移动手感验收、M4 方块交互、完整物品表/创造栏/合成/掉落实体、M6 多人压测、玩家数据磁盘格式 |
| 阻塞项 | 真实 Bedrock 客户端验收、在线认证验收、插件兼容策略、世界/玩家存储格式、多人压测 |
| 需要用户确认 | 是否必须兼容 PHP API3 插件；是否需要读取旧 BetterAltay 世界和玩家数据 |
| 建议下一步 | 用 Bedrock 1.26.20/protocol 975 客户端重新验收登录、加载完成、移动、断线重连、背包打开/关闭、热栏切换、聊天和命令 UI；随后推进 H/I 方块交互、完整物品表/创造栏、合成和掉落实体 |
| 关键文件 | `REWRITE_TASKBOOK.md`、`go.mod`、`go.sum`、`scripts/test.ps1`、`LICENSE`、`NOTICE`、`cmd/bds/main.go`、`internal/bootstrap/bootstrap.go`、`internal/config/config.go`、`internal/server/server.go`、`internal/server/*`、`internal/network/mcpe/*`、`internal/network/raknet/*`、`docs/architecture.md`、`docs/protocol.md`、`docs/license-notice.md`、`docs/network-raknet.md` |
| 已运行验证 | `go test ./internal/server -count=1 -v`；`go test ./internal/network/mcpe ./internal/network/raknet ./internal/bootstrap -count=1`；`go test ./... -count=1`；`powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` |
| 未运行验证 | 尚未进行真实 Bedrock 客户端局域网列表、登录连接和多人压测；`go test -race` 因当前环境缺少 `gcc` 未运行 |

## 12. 变更记录

| 日期 | 对话/操作者 | 变更 | 关联任务 | 备注 |
|---|---|---|---|---|
| 2026-05-05 | Codex | 补充代码生成与实现规则：禁止无意义硬编码、无意义小函数和无意义中转函数 | A-006 | 用户明确要求 |
| 2026-05-05 | Codex | 正式开始 Go 重写：初始化 git，建立 `cmd/bds`、`internal/bootstrap`、版本信息、基础测试和编码规范 | A-002、A-006、B-001、B-002、B-003、B-007、M0 | `go test ./...` 和 `go run ./cmd/bds -version` 通过 |
| 2026-05-05 | Codex | 增加 `server.properties` 配置加载、彩色/文件日志、信号关闭骨架和相关测试 | B-004、B-005、B-006、DEC-008 | `go test ./...` 和 `go run ./cmd/bds -data-path .runtime -check` 通过 |
| 2026-05-05 | Codex | 创建任务书，建立状态规则、里程碑、分阶段任务、模块台账、风险和交接模板 | A-001 | 初始版本 |
| 2026-05-05 | Codex | 补齐许可证/NOTICE 策略与测试脚本验收记录；新增 RakNet 决策文档、适配层、UDP 监听和 unconnected ping/pong smoke test | A-007、B-008、DEC-004、DEC-006、C-001、C-002、C-003、M1 | `go test ./...`、`scripts/test.ps1` 和 `scripts/test.ps1 -RunCheck` 通过；真实 Bedrock 客户端待测 |
| 2026-05-05 | Codex | 完善 RakNet session 生命周期：支持 session handler 注入、活跃连接跟踪、关闭取消和等待退出，并补充真实 RakNet dial 测试 | C-004 | `go test ./internal/network/raknet -count=1 -v` 和 `go test ./...` 通过；`go test -race` 因当前环境缺少 `gcc` 未运行 |
| 2026-05-05 | Codex | 建立 `internal/protocol` 基础 Reader/Writer 和 UUID 类型，覆盖 varint、zigzag、little-endian、string、UUID 网络顺序及边界测试 | D-002 | `go test ./internal/protocol -count=1 -v`、`go test ./...` 和 `scripts/test.ps1 -RunCheck` 通过 |
| 2026-05-05 | Codex | 将协议层切换到开源优先适配：接入 gophertunnel 的 batch/compression、login parsing、packet interface 与 dispatcher，并补充 debug 网络日志 | DEC-009、C-005、C-006、C-008、D-003、A-005 | `go test ./internal/protocol -count=1 -v`、`go test ./internal/bootstrap -count=1 -v`、`go test ./...`、`powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` 通过 |
| 2026-05-05 | Codex | 历史记录：曾新增 gophertunnel Minecraft listener 路线并用虚拟客户端验证登录/StartGame；该完成结论已在后续按用户要求撤回 | M2、C-002、C-007、C-010、D-004、O-003 | 不再作为当前完成状态依据 |
| 2026-05-05 | Codex | 按用户要求撤回 gophertunnel 托管运行时路线：删除 `internal/protocol/session.go` debug 会话，`internal/network/mcpe` 改为自有 MCPE session，gophertunnel 仅保留 packet/batch/login 解析打包用途 | DEC-009、M2、C-007、C-010、D-004、O-003 | `go test ./...` 和 `scripts/test.ps1 -RunCheck` 通过；加密握手和真实客户端验收仍待做 |
| 2026-05-06 | Codex | 实现本地 MCPE 加密握手：新增 batch AES-CTR 加密/校验、ServerToClientHandshake JWT salt 生成、P-384 ECDH key 推导和 ClientToServerHandshake 状态推进 | C-007、D-004、M2 | `go test ./internal/protocol -count=1 -v`、`go test ./internal/network/mcpe -count=1 -v`、`go test ./...`、`scripts/test.ps1 -RunCheck` 通过；真实 Bedrock 客户端验收仍待做 |
| 2026-05-06 | Codex | 实现 D-005 世界同步核心包并整理目录边界：`internal/network/mcpe` 仅保留 MCPE 连接适配，bootstrap 负责注入 `internal/server` client factory；初始同步发送 SetTime/SetSpawnPosition/LevelChunk，支持 SubChunkRequest 响应和 UpdateBlock 包构造 | D-005、G-008、M3 | `go test ./internal/server -count=1 -v`、`go test ./internal/network/mcpe -count=1 -v`、`go test ./...`、`scripts/test.ps1 -RunCheck` 通过；玩家输入/交互包未做空处理，留给 D-006/M4 |
| 2026-05-06 | Codex | 移除空转 `internal/protocol` 包：MCPE batch codec/encryption 移入 `internal/network/mcpe`，Login 解析和 ServerToClientHandshake key/JWT 移入 `internal/server`，packet 路由改为 server 会话本地路由，所有包类型直接使用 gophertunnel `packet.Packet` | DEC-009、C-005、C-006、C-008、D-002、D-003 | `go test ./internal/network/mcpe ./internal/server -count=1 -v`、`go test ./...`、`scripts/test.ps1 -RunCheck` 通过 |
| 2026-05-06 | Codex | 实现 D-006 玩家同步核心包：新增本地玩家状态/列表、PlayerList 全量与增量同步、AddPlayer 双向生成、SetActorData/SetActorMotion 初始化与广播，并处理 PlayerAuthInput/MovePlayer 输入广播 | D-006、M3、M6 | `go test ./internal/server -count=1 -v`、`go test ./... -count=1`、`go test ./internal/network/mcpe -count=1 -v`、`go test ./...`、`scripts/test.ps1 -RunCheck` 通过；真实 Bedrock 客户端多人验收仍待做 |
| 2026-05-06 | Codex | 按用户确认将协议基线更新到 `gophertunnel v1.56.1` 的 `protocol 975/MC 1.26.20`，并完成 D-007 聊天与命令包：Text 聊天广播、CommandRequest、AvailableCommands、CommandOutput、控制台输入、最小命令框架和 OP 权限 | DEC-001、SCOPE-003、D-007、K-003、K-004、K-005、K-006、K-007、M4 | `go test ./...` 和 `powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` 通过；真实 Bedrock 客户端命令 UI 和聊天验收仍待做 |
| 2026-05-06 | Codex | 排查 D-006 后世界显示成告示牌的问题：确认 gophertunnel v1.56.1 不提供方块 runtime 映射；补齐 StartGame 前 JigsawStructureData/VoxelShapes，LevelChunk 改为 limited sub-chunk request mode，SubChunk 响应补 height map/render height map，并增加 flat spawn sub-chunk 编码回归测试 | D-005、D-006、DEC-001、R-005、M3 | `go test ./internal/server ./internal/world -count=1`、`go test ./...` 和 `powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` 通过；仍需用户用真实 Bedrock 1.26.20/protocol 975 客户端确认世界可见性 |
| 2026-05-06 | Codex | 针对真实客户端日志补齐 D-006 后续路由：ServerBoundLoadingScreen 记录加载屏 start/end 与可选 ID，Interact 处理鼠标悬停实体、自带背包打开和离开载具位置，ContainerClose 处理自带背包/聊天混合关闭并回 close ack | D-006、D-008、M3、M4 | `go test ./internal/server -count=1`、`go test ./... -count=1`、`powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` 通过；物品内容、热栏和 StackRequest 留给 D-008 |
| 2026-05-06 | Codex | 实现 D-008 背包与物品核心包：新增本地权威背包状态，初始 InventoryContent/MobEquipment 同步、AddPlayer HeldItem、MobEquipment 热栏切换广播、InventorySlot 不一致回补，以及独立/PlayerAuthInput 内嵌 ItemStackRequest 的 Take/Place/Swap/Drop/Destroy/MineBlock OK/Error 响应和拒绝后背包回同步 | D-008、M4、I-008 | `go test ./internal/server -count=1`、`go test ./... -count=1`、`powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` 通过；完整物品表/创造栏/合成/真实掉落实体仍待后续 |
| 2026-05-10 | Codex | 实现 D-009 资源包包组：新增 `internal/resourcepack` 的 Pack/Queue 模型，`internal/server` 侧发送 ResourcePacksInfo/ResourcePackStack 并处理 ResourcePackClientResponse、ResourcePackChunkRequest 与 ResourcePacksReadyForValidation；补充空包与内存资源包测试 | D-009 | `go test ./internal/resourcepack ./internal/server -count=1`、`go test ./... -count=1` | 默认仍可空资源包；真实 Bedrock 客户端下载链路仍待验收 |
| 2026-05-14 | Codex | 实现阶段 E 服务器核心生命周期：新增 `internal/server.Server` 核心对象并由 bootstrap 调用；实现 listener 生命周期、20 TPS tick loop、主线程任务队列、Stats/Info、崩溃诊断、stop 命令关闭、session 断开回调、玩家加入/离开广播、PlayerStore 快照钩子和 MOTD 在线人数刷新 | E-001、E-002、E-003、E-004、E-005、E-006、E-007、E-008、M6 | `go test ./internal/server -count=1 -v`、`go test ./internal/network/mcpe ./internal/network/raknet ./internal/bootstrap -count=1`、`go test ./... -count=1`、`powershell -ExecutionPolicy Bypass -File scripts\test.ps1 -RunCheck` 通过；真实客户端断线和多人压测仍待做 |
