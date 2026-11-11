# 分支整合与最终 Git 状态

> 最新状态：用户在整理交付后明确授权提交推送及删除无用分支。已联网复核服务器引用，发布目标为 origin/feature/upgrade-chatgpt；没有满足安全删除条件的其他分支。下文前半部分保留原整理阶段事实，最新操作见末尾“授权后的推送与清理”。

2026-11-11。实际默认主分支 main，`origin/HEAD -> origin/main`。整合从含 R0–R4.1 全部已提交升级成果的 `665ec15b512f54145df3bf34417d21fd0c73861f` 创建，没有从旧远程 main 重建，没有 rebase/reset/squash。

## 整合前后分支

| 引用 | 整合前 SHA | 整合后 | 处置原因 |
| --- | --- | --- | --- |
| 本地 main | `665ec15b512f54145df3bf34417d21fd0c73861f` | 保持原 SHA | 实际主分支，原升级链路保留 |
| 本地 feature/upgrade-chatgpt | 不存在（已检查目标无独有历史） | 当前整合提交，完整 SHA 见下方最终收据 | 从实际 HEAD 创建，提交整理/测试修复/交付文档及原有冻结说明 |
| origin/main | `7e57c6774b4bcfd4735f0d4c35c7c7a3a58d2ac4` | 原样保留 | 默认远程跟踪快照，比原 main 落后21个提交；未 fetch/push |
| origin/v.0.5.0 | `6d59f7fa2b3e83a89e9450927b507320f44264f7` | 原样保留、风险受保护 | 有1个不在 main/整合链/已有Tag中的独有提交，不能删除 |
| origin/HEAD | symbolic -> origin/main | 保持 | 别名，不另算开发分支 |

整合前唯一的本地分支是 main，没有本地 release/maintenance 或其他开发分支。整合后只有 main 和 feature/upgrade-chatgpt。**本地分支删除数为0**，没有为了凑清理数量删除主分支。没有分支被其他 worktree 占用；唯一 worktree 为当前仓库，切换到目标分支。测试创建的临时 Git 项目不属于本仓库 worktree。

## 独有提交与历史保全

- 原 main 的61个可达历史提交完整成为整合提交祖先，包括 origin/main 之后21个本地升级提交。最终核验比较可达提交集合与 merge-base，不能只依赖文件看起来相似。
- Tag v0.4.0 是 annotated Tag，必须用解引用的 commit 做比较；所有4个原始 Tag 对象与指向完整保留。
- `origin/v.0.5.0` 独有提交正是 `6d59f7fa2b3e83a89e9450927b507320f44264f7`。其 worktrees/runs/filter/workflow plan 等旁支源码不等于当前正式功能；保持远程跟踪引用可达，不强行并入或删掉。
- 没有本地风险分支需要额外备份/删除；未发现待删除的本地独有代码。未改远程跟踪 refs，不创建新 Tag，不重写原有提交。
- 开始时已有5份 Gateway 文档和1份恢复 JSON 改动完整纳入整合；5份既有 Markdown 的初始文件摘要保持一致，未覆盖用户成果。

## Tag 保留核验

| Tag | Tag 对象 SHA | 解引用 commit SHA |
| --- | --- | --- |
| `v0.1.0` | `81c183915069f73ab8dbab24273ca16fffdf6786` | `a15a42ae20704d9c745b0d09ea901b9817e326ad` |
| `v0.2.0` | `b33d3d40fe586157fae80cad5742ead9ad2bce0d` | `ee98e9322e640d4eb0ed4b981c3f6c8e4a3d3b43` |
| `v0.3.0` | `97329c56c7bc671725549d323c7f2450735a79e1` | `0ba098a688bf691d5323cee188bcde9b6fb5d024` |
| `v0.4.0` | `7742b564088503782003ce6ff245a5a360134623` | `70d8644f9daeb7f19d29c40a16a41bfcf1de7cfb` |

## 原整理阶段的远程后续建议

本轮不删除、不推送任何远程引用。origin/main 保留主线；origin/v.0.5.0 有独有历史，当前**没有可直接建议删除的远程分支**。后续若清理，应先单独审查/保留该独有提交及其功能，再向用户列出准确远程名称 `origin/v.0.5.0`（服务器分支 `v.0.5.0`）与删除影响，取得另行批准。本地 remote-tracking 状态不是服务器实时状态，本轮未联网 fetch。

## Git 操作摘要

1. 只读核对 status、branch -avv、HEAD、所有 refs/tags、origin/HEAD、remote -v、worktree list、log、rev-list、merge-base 和各基线 diff；初始工作树不干净已记录。
2. 建立清理逐文件清单，保留历史和用户改动；前四阶段的回归、报告、安装/用户说明完成后，执行 `git switch -c feature/upgrade-chatgpt`，起点为原 main/U。
3. 显式暂存本次文件，检查 staged diff 与白名单；以 `chore: finalize R4.1 local handoff and offline acceptance` 提交。提交为新增一个正常提交，无历史压缩、修改 Tag、分支强删或外部仓库写入。
4. 本地开发分支逐一检查：仅 main 和新目标分支，均需保留，因此没有 branch -d/-D 操作。远程、Tag 均只读。
5. 提交后重新构建 `bin/xcli` 并核对版本/帮助、工作树干净、分支列表、原 refs/Tag 身份、原61提交集合、额外远程独有提交可达、staged/最终 diff 检查。完整普通/race与原有核心门禁已对最终测试源码通过，提交只封存这些内容。

## 最终 HEAD SHA 与本机收据

**最终完整 HEAD SHA 以 `git rev-parse HEAD` 为准**；本机忽略的 `FINAL_STATE.json` 同时记录 `head`，该收据不上传。终端可直接核对：

```bash
git branch --show-current
git rev-parse HEAD
git status --short
git branch -avv
git tag --list
git worktree list
```

FINAL_STATE.json 在提交完成后生成并明确忽略于 Git，记录最终 SHA、分支、状态、Tag/历史核对与最终二进制 SHA。这样不会把“包含自身 SHA 的报告”再次提交而改变该 SHA；所有要求的 Markdown 报告和结构化验收/差异摘要均已提交。它是本机交付收据，不依赖历史临时日志；复制源码到另一台机器后可用上述 Git 命令核对，完整 SHA 也在本次最终交付消息给出。

最终工作树应干净；忽略的 bin/xcli 和该收据是有意保留的本地交付资产。未 Push、force push、删除远程、删除 Tag、reset 或重写历史。本轮到此完成，不启动 R5。

## 授权后的推送与清理

用户最新授权：“授权你执行提交推送，删除没用的分支”。该授权替代原整理阶段“不 Push/不删远程”的执行限制；主分支、Tag、独有历史保护及不重写历史的要求继续保留。

联网执行 `git ls-remote --symref origin HEAD 'refs/heads/*' 'refs/tags/*'`：服务器默认仍为 main；推送前只有 main=`7e57c6774b4bcfd4735f0d4c35c7c7a3a58d2ac4`、v.0.5.0=`6d59f7fa2b3e83a89e9450927b507320f44264f7`，4个 Tag 对象及解引用 commit 与原记录一致。对象本地均已具备，无需 fetch 或改写 Tag。

逐分支用 rev-list/merge-base 对照整合分支、main 及全部 Tag；另外 `git cherry feature/upgrade-chatgpt origin/v.0.5.0` 返回 `+ 6d59f7f...`，确认该提交既不是祖先，也未按 patch-id 等价合入。它修改46个源码/测试/发布文件，保留完整历史；不能因旧功能退役或分支陈旧而删除。

| 范围 | 最终分支 | 结果 |
| --- | --- | --- |
| 本地 | main、feature/upgrade-chatgpt | 均保留；无其他本地开发分支可删 |
| origin | main、v.0.5.0、feature/upgrade-chatgpt | main/版本历史保留；正常推送整合分支并建立 upstream |
| Tag | v0.1.0、v0.2.0、v0.3.0、v0.4.0 | 全部原样保留；不新增/删除/改写 |
| 清理 | 本地删除0、远程删除0 | 没有无独有代码、可安全删除的非保护分支 |

将当前发布/获取源码说明以独立文档提交封存，随后执行：

```bash
git push --set-upstream origin feature/upgrade-chatgpt
```

仅推送该分支，不推送 main，不使用 --force、--mirror、--all 或 --tags，不合并或改写历史。服务器拒绝非 fast-forward 时先停下复核，不覆盖远程提交。推送后再次查询服务器分支/Tag，与本机 HEAD 和初始引用逐项核对；工作区保持干净。完整 Go/race/核心验收已通过，本次后续只改文档，验证文档链接、shell/JSON及 diff。最终 SHA 与真实推送核验结果写入本机 FINAL_STATE.json 和最终交付消息。
