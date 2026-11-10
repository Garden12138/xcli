# R4.1 实现与验收进度

2026-11-10。起点 `ae0b4bee9498149b741756deb7a6213bd9c9f2a7`，核心 `d247a9485b86eafe53b5b670fcbe862146847c4c`，初始工作区干净。候选 `0.5.0-r4.1-dev`，未 Push/发布。

## 实现

- 统一 `launch.Start`：先预留、再持久启动意图、再 OS Start；唯一 ID，窗口不可变，幂等请求重放拒绝新增启动。
- 提取既有 OS 私有文件/锁工具为 `localstate`，预算以一个校验快照为事实源，不读取模型供应商凭据，不依赖 Gateway 服务。
- run/use/resume、Task/Workflow/重试、后台与监督 attempt、版本/help/status、配置别名和兼容原生启动路径接入；保存 Task/Run/Session/阶段/证据关联。
- 100 路原子预算/并发/幂等竞争、进程立即退出、取消/超时、Start 失败、真实 OS 子进程模拟 owner 崩溃及重启恢复均用 Fake 验证。
- 外部基线必须显式：未知 count 为 null，未确认的窗口不派发。R4 历史 UNKNOWN 不重建，旧配置和数据保留。
- `launches window create/budget/windows/list/show/record-legacy/confirm-stopped/probe` 可供查询和明确操作；新手册说明新门禁，旧 R0 smoke 探针改走受控入口并输出前后账本统计。

## 回归记录

首次全量回归发现把严格私有目录规则套到旧 RunStore 后，会拒绝已有宽权限测试目录。修复保留 RunStore 原有写入兼容，严格权限仅用于新账本与本来要求私有状态的设施；未通过改变用户目录权限解决。

首次核心回归又发现旧 owner-kill 测试只等原生 PID 消失就立即 reconcile，未等待 attempt 监督者完成持久化并释放既有锁。生产入口正确返回 still exiting，测试已改为等待该锁失活后再检查恢复；保留原生产门禁，不自动重试原生调用。

完整 `verify-offline.sh` 已为 exit 0 / `R4_FULL_REGRESSION_PASS`：全仓库 `go test -race -p 1 -count=1 -timeout 240s ./...`、`go vet ./...`、build、go mod tidy -diff、gofmt/diff check 全通过。Python R0 11、Native Gateway 9、R3 25、Model Gateway 9（合计 54）通过；假上游 lifecycle 与 12 项隔离安装检查通过；Darwin amd64/arm64、Linux amd64/arm64、Windows amd64 构建通过。独立 `verify-core.sh` 最终 exit 0 / `R4_CORE_REGRESSION_PASS`：核心 Go race/vet/build、R0/R3 Python 36 项、隔离安装 12 项全部通过。owner-kill 定向 race 连续 10 次通过；最后改动只涉及该测试的同步，生产二进制散列不变。

保留 [首次失败日志](evidence/r4_1/initial-regression.log)、[最终全量日志](evidence/r4_1/full-regression.log)、[启动路径源码审查](evidence/r4_1/source-audit.json)、[二进制 Fake 黑盒证据](evidence/r4_1/blackbox.json)。历史 Python 日志中的 Worklog 是临时合成仓库；fake token/HTTP 都属于本机假服务，未修改真实四应用或访问普通模型服务。

黑盒验收用新构建 `xcli version 0.5.0-r4.1-dev`，`go build -trimpath`。SHA-256：

```text
c2d7a445df8da0dc4613c1091971d133306b15482f7c5c8f1e8845d8e02efc6f
```

临时二进制 `/tmp/xcli-r4_1-acceptance/bin/xcli`；它与最终全量 control smoke 二进制散列相同。临时文件可能清理，源码与脱敏证据纳入本地提交；不是发布包。额外黑盒用了 3 次 fake 启动：run、同 Session resume、status。验证 list/show、重放不启动、耗尽拒绝、旧 UNKNOWN 与账号原文不出输出/账本。此处的 2 次扣额/剩余 0 仅属于 fake-contract 窗口，不能当成真实验收余额。

最终证据：[核心日志](evidence/r4_1/core-regression.log)、[首次核心时序失败](evidence/r4_1/initial-core-regression.log)、[十次恢复回归](evidence/r4_1/owner-recovery-repeat.log)、[机器汇总](evidence/r4_1/validation.json)。最终 16 份活跃文档本地链接检查、源码审查散列复核、脚本语法与差异检查均通过。未新增依赖，go.mod/go.sum 不变。

## 边界与后续门槛

本轮真实模型请求/新增真实 CLI 启动为 0，不改原生登录配置、Trust、四应用，不启用模型/NAG/SIWC。测试中的冻结模块只运行既有假上游契约，真实 Profile 仍关闭。

xcli 受控直接原生入口可计账；任意包装脚本/工具内部后代、供应商内部模型轮数/费用与外部终端不能精确自动监控。旧 R3 研究 `calls.jsonl` 和 Task.Calls 是分派/研究统计，不是 R4.1 的启动预算事实源；不能拿其默认值或历史总数替代新的显式窗口。

旧 R4 真实结论仍 **NEEDS_FIXES / UNKNOWN**。下一轮必须用户重新授权新窗口，并先审计全部 scoped 外部启动；若控制器历史无法确认，继续 BLOCKED，不能声称还有五次。不在本轮启动真实验收或 R5。
