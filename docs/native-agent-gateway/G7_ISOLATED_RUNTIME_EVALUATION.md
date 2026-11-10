# G7：隔离运行时有界实验

2026-11-09 开始，2026-11-10 完成。起点 `99786e2`（G6 代码 `c9a8d88`），结论 **PARTIAL_HOST_ISOLATION_ONLY**。真实模型请求 0，真实账号认证材料读取、复制、使用均为 0。路线 B 已停止；不自动进入 G8。

## 环境及实验范围

实际使用已运行的 OrbStack 2.2.5（2020500）与 Docker 29.8.2，Linux arm64，内核 `7.2.12-orbstack-00394-gbecdd6130068`。没有安装、启动新虚拟化软件或修改全局安全设置。空 HOME 下 orbctl 的 Stopped 不能表示实际引擎停止；使用显式 Unix socket 的 Docker version 确认引擎已运行。未发现可用的 Podman、Lima、Colima、UTM、QEMU 命令。

Docker 使用新建空 `HOME`、`DOCKER_CONFIG` 和固定的本机 Unix socket，不读用户 Docker 登录配置。现有镜像缓存无可用基础镜像，拉取一次公开 Alpine 3.22 arm64，固定镜像 ID `sha256:49340b346798875b5592e5d420241f2f9cfba5f47e62ebb1446a1b34734e898f`；之后 `--pull=never`。镜像留在缓存，未删除共享镜像。

下载同版本官方 npm `@openai/codex` 的 `0.167.0-linux-arm64` 包，核对发布元数据的 SHA-512 integrity，仅提取两个明确命名的 ELF 普通文件。未执行包安装脚本。资产 URL、大小、SHA-256 和临时合成根见 [asset-and-runtime.json](evidence/g7/asset-and-runtime.json)。完整性匹配不等于额外签名验证；本轮不升级宿主 Codex。

只挂载临时合成 Git 项目的一个只读目录，包含固定材料、登记的可执行文件及两条故意逃逸的符号链接。宿主假长期凭据文件在该目录外，未挂载。未挂载宿主 HOME、原生配置、SSH、浏览器目录、Docker socket 或个人仓库；xcli 源码不作为 Agent 工作区。

固定 OS 约束：UID/GID 65532、只读根、只读工作区、cap-drop ALL、no-new-privileges、network none、独立 IPC、64 PID、512 MiB、1 CPU；仅 `/tmp` 和空 `/home/xcli` 为有界 noexec/nosuid/nodev tmpfs。容器启动和停止均经正常权限审批；没有放宽挂载、网络或沙箱来通过实验。

## A0–A5 结果

| 实验 | 结果 | 证据及结论范围 |
| --- | --- | --- |
| A0 程序启动退出 | PASS，真实系统实验 | [a0.json](evidence/g7/a0.json)：Linux/aarch64、UID 65532、固定材料、Codex 0.167.0、exit 0；只运行 `--version` |
| A1 非推理握手 | PASS，真实 Linux Codex | [最终实验](evidence/g7/linux-isolation-final.json)：initialize / initialized；无 thread、turn、account 请求 |
| A2 RPC 和停止 | PASS，真实进程 | stdio 请求有界、EOF exit 0，daemon 侧确认容器不存在；不是只确认 Docker 客户端退出 |
| A3 文件、链接、目录、网络 | PASS，限定的宿主隔离 | 真实 UID、工作区写入/目录替换拒绝；绝对/相对链接和宿主路径不可读，mock 控制面不可达，HostConfig 约束核实 |
| A4 app-server 直接文件接口 | PASS，限定的宿主隔离 | `fs/readFile` 能读指定材料；不能读宿主假凭据、链接目标、`/mnt/mac` 候选宿主映射、Docker socket；不证明整个 builtin 工具目录的行为 |
| A5 HTTP→NAG→容器→事件/结果 | PASS，**假 peer** | [a5-integration.log](evidence/g7/a5-integration.log)：成功任务、主动销毁运行环境后 failed、持久 SSE、幂等重放、重开 Store 后不重复执行；2 个假任务各扣 1 次 |
| B 凭据域分离 | **NOT_FEASIBLE_UNDER_CURRENT_CONSTRAINTS** | app-server 直接文件接口与代表性同 UID 子进程都能读取环境中的模拟 Access Token；见[凭据结论](G7_CREDENTIAL_BOUNDARY.md) |

网络证据包括实际 `network=none`、不可达的本机 mock 和无增加的访问计数。没有验证云服务连通；该策略刻意阻断云端。没有把一个端点的失败、模拟撤销或 Docker 配置字段推广为全部内置工具已经安全。没有执行模型触发的 shell、MCP、文件工具或推理。

本结论假设宿主/虚拟化引擎及 Gateway owner 可信；不涵盖内核漏洞、恶意 Docker 管理员或宿主 owner 主动替换挂载源。目录 inode/文件摘要预检与容器内拒绝替换分别有证据，不将预检描述为对宿主 owner 的持续强制隔离。

## 最小 NAG 增量

`internal/isolatedruntime/docker.go` 注册固定本地 socket、空 Docker 配置、私有父目录下的工作区、固定 digest 和 payload；保持登记的 multicall 入口，并检查目标身份不变。只有一个只读 bind，环境白名单，不接受 HTTP 路径、argv、env、容器/镜像参数。互斥锁使一次只有一个 workload。

`nativeadapter.IsolatedAppServer` 是 Go owner 显式 `Enabled` 的实验 transport，零值关闭，**生产 serve 不选择它**；Native Profile.Check 原有拒绝保持有效。只复用 G6 stdio Adapter 的内部启动接口和事件映射；HTTP、Session、Task、SSE、预算、幂等、所有权、Model Gateway 均未改契约。实际 A5 使用明确的 `Native=false` **假 ELF** 夹具，不伪装成真实 Profile 验收。

停止通过本次随机 name + owner label 查 daemon，且仅删除对应 ID。不能确认 daemon 侧停止时 `local_stop_confirmed=false`、`execution_state_unknown`，交给现有 `needs_reconcile` 封存机制；不自动重试。控制输出 4 KiB、控制超时 1s。模拟 daemon 失联回归不停止全局引擎。上游撤销始终 nullable/未知。

该 transport 尚无生产注册入口、认证、云端 egress、完整远程权限分离或原生 Thread 数据持久卷。A5 的重启证据是 **NAG 结果/预算不重执行**，不代表跨销毁容器的真实 Codex Thread 恢复已实现。没有增加生产级容器平台。

## 失败记录与修复

1. Python urllib 下载遇到本机 TLS 证书链错误。改用系统 curl 的正常 HTTPS 验证；没有 `--insecure`、安装 CA 或更改信任设置。首次提取时旧包路径断言失败，改为包内实际 `bin/codex`，仍只提取明确普通文件；没有扩大 200 MiB 下载界限，二进制使用已有 512 MiB 上限。
2. 首次 A1 提前 EOF 保留在 [linux-isolation-initial.json](evidence/g7/linux-isolation-initial.json)。[一次启动诊断](evidence/g7/linux-isolation-diagnostic.json)明确为 `CODEX_HOME=/home/xcli/codex` 不存在，exit 1；改为已存在的私有 tmpfs `/home/xcli` 后通过。没有扩大权限。`linux-isolation.json` 保留初次输出副本。
3. 诊断首次因自动审批审查额度错误在执行前中断；11-10 正常重新审批后执行，没有绕过审查。普通沙箱下 Docker socket/loopback 不可用的检查没有当作产品失败或 PASS。
4. [A5 初次失败](evidence/g7/a5-initial-failure.log)：将 OrbStack docker symlink 解析为 `docker-tools` 后改变 multicall 入口名称，CLI exit 127，未启动假 peer，daemon 停止无法确认，任务 `needs_reconcile`。保留登记入口并校验其目标，补 retarget 攻击测试后通过；不退款、不重跑原 Task，使用独立测试根。

## G6 SIGABRT：保留原结论

G6 的实际证据为 [protocol-diagnostics.json](evidence/g6/protocol-diagnostics.json)：严格 deny-default Seatbelt 下 initialize 前 `SIGABRT`（-6）、stderr 空。当前尝试对**同一策略**取栈，固定 Darwin 0.167.0 二进制、假 HOME、initialize-only 输入；未增加 Mach、Keychain、个人文件或网络权限。

[LLDB 日志](evidence/g7/g6-crash-lldb.log)显示 OS 拒绝 debugger attach，目标未成功启动；[复现记录](evidence/g7/g6-crash-reproduction.json)保留二进制/策略/原始日志摘要与私有复现目录。它不是本轮再次观察到 SIGABRT。最小判断仍为 macOS 严格进程边界下的启动失败，具体 syscall/library 根因 **未定位**，诊断状态 `BLOCKED_DIAGNOSTIC_PERMISSION`。没有改 SIP、签名、entitlement 或系统策略。Linux 路线正常运行是替代运行时证据，**不是已解决 macOS 崩溃**。

## 复验、证据与清理

所有普通离线测试入口：

```bash
bash tests/native_agent_gateway/verify-offline.sh
```

完整结果、源码摘要及启动账本见 [validation.json](evidence/g7/validation.json)、[offline.log](evidence/g7/offline.log)、[non-model-ledger.json](evidence/g7/non-model-ledger.json)。离线 gate 不自动启动容器；A5 外部系统实验必须另行显式 opt-in，准确命令见 [G7_DECISION](G7_DECISION.md)。

独立脚本默认 dry-run，未带执行参数时连资产/引擎都不打开：

```bash
python3 tests/native_agent_gateway/g7_isolation_spike.py \
  --metadata docs/native-agent-gateway/evidence/g7/asset-and-runtime.json \
  --socket unix:///Users/cengjiada/.orbstack/run/docker.sock \
  --out /private/tmp/g7-isolation-dry-run.json
```

只有 owner 明确 `--execute-non-model` 才会运行有限的真实无推理套件；RPC allowlist 固定 initialize/initialized/fs/readFile，不能调用 turn、thread、auth、command。已有证据足够，本轮不再追加执行。

所有实验 workload 最终均确认消失，mock 已关闭，私有 Docker 临时配置自动删除。临时合成根/公开包保留便于复核，不含真实凭据；不需停止 OrbStack、删除共享缓存或扫描其他容器。临时资产若已清理，报告不再可直接执行；不能改挂宿主 HOME 来恢复。
