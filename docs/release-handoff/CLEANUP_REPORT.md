# 仓库清理报告

日期：2026-11-11。整理前 HEAD：`665ec15b512f54145df3bf34417d21fd0c73861f`。没有 AGENTS.md 附加规则；仅操作本仓库和本轮临时验收目录。

## 删除前审查及结果

删除前建立逐文件 [cleanup-manifest.json](cleanup-manifest.json)，记录路径、大小、SHA-256、原因、引用情况、能否再生成、测试影响和最终结果。核对摘要后逐文件 unlink；空缓存/构建目录仅用 rmdir 收尾。未执行 git clean、目录级 rm -rf、用户数据删除或历史重写。

| 类别 | 文件数 | 原因与引用审查 | 再生成/测试影响 | 结果 |
| --- | ---: | --- | --- | --- |
| Finder `.DS_Store` | 2 | 无程序/测试依赖 | Finder 再生成，无影响 | 已删除 |
| 根目录 `xcli` | 1 | 旧 R3 编译产物，文档曾将其作为构建目标 | 当前源码可构建；本轮重新构建到 `bin/xcli` | 已删除旧文件 |
| `dist/` 内旧 R3 二进制、tar.gz、校验和、元数据 | 12 | `tests/r3/package.py` 显式消费 dist；不属于源码输入 | 现有打包脚本可再生成；本轮重新交叉构建并验包 | 已删除 |
| `__pycache__/*.pyc` | 40 | 自动编译字节码，无源码依赖 | Python 再生成；本轮禁写字节码验证 | 已删除 |

合计 **55 文件、55,462,900 字节（约 52.9 MiB）**，全部未跟踪且可再生成。新 `bin/xcli` 是本次本地交付产物，已忽略，不纳入 Git；不会使用旧根目录二进制。

## 保留、归类与归档

- `docs/r0`–`docs/r3`、`docs/phase0`、`docs/cli-first/evidence`、`docs/product-realignment/evidence`：历史验收及产品决策资料，按既有阶段目录保留。图片、视频、补丁、失败日志和 source manifest 不删除。
- 重复内容审查：发现 10 组相同文本，主要是 before-reset/after-reset 证据、预算最终快照及失败日志；所属实验时点不同且被报告引用，保留对应目录以免破坏历史证据关系。未新建副本。
- `docs/model-gateway`、`docs/native-agent-gateway`、`docs/model-access-gateway` 及对应代码/测试：现有入口已有冻结标识，保持默认关闭；依赖见 [模块处置](../product-realignment/MODULE_DISPOSITION.md)。cmd 仍链接实验包，不能按目录名删除。旧 examples 增加分类入口，避免将 workflow/Gateway 示例当作当前命令。
- 开始时已有 5 份 Gateway Markdown 改动和 `recovery-20261111.json`：全部保留并纳入整合提交；是此前中断恢复/冻结说明，本轮没有重做其外部仓库检查或实验。
- 未删除任何真实配置、原生凭据、数据库、Run/Session、工作区、Git Tag 或外部目录。没有凭目录陈旧而清除历史报告。

## 敏感内容审查

对全部原有跟踪文件及初始未跟踪恢复 JSON 进行文件名和文本特征审查：未发现匹配真实私钥头、长 sk-/GitHub Token 的内容。发现 47 处用户绝对路径和 40 处邮箱格式匹配，集中在历史 R0 体验/原生探针证据及合成测试/补丁。测试身份和补丁上下文不是新的认证材料；已入库历史资料不改写，其摘要及复现关系保留。此项是特征扫描，不能证明历史绝无其他形态秘密。

新交付只记录 `~` 路径、公开版本/摘要和合成测试结果；不读取原生认证文件或导出账号。初始新增恢复记录未检出上述敏感特征。没有将原始 stdout、历史配置副本、完整环境或新 Token 纳入交付。

`.gitignore` 保留现有规则，并增加 `/bin/`、`/build/`、根目录 `.cache`/`.pytest_cache`、`*.py[cod]`、覆盖率输出、`.env*`（允许 `.env.example`）及明确的 `config.local.yaml/json`。不使用会误忽略历史证据的全局 `*.log`、`*.json` 或 `*.yaml`。

## 清理后验证

完整 Go/Fake CLI、Python、安装、兼容、交叉构建与文件产物验证见 [AUTOMATED_ACCEPTANCE.md](AUTOMATED_ACCEPTANCE.md)。静态格式、模块、依赖、构建及 diff 检查通过；测试问题的初始失败与修复均保留在验收摘要。没有因清理删除测试输入或放宽断言。
