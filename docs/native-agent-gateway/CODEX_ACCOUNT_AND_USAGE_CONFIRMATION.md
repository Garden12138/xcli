# Codex 账号、用途与云端发送确认

G5A.2，2026-11-08。**以下六项当前全部待用户本人确认**。代码／测试／官方登录状态不能代答，历史剩余额度不构成本轮同意。不要向报告、聊天、Git 或脚本参数提交邮箱、Token、账号 ID、认证文件或其内容。

| 本人必须确认 | 私有 JSON 字段 |
| --- | --- |
| 使用本人拥有且有权使用的 ChatGPT/Codex 账号 | `own_authorized_account` |
| 当前计划与选定 Codex 能力可用；理解登录或模型目录不保证一次请求成功 | `plan_and_capability_available` |
| 仅本人本机开发与合成材料验收 | `personal_local_development_only` |
| 本次没有其他用户共享、商业推理服务或远程托管用途；如有则停止并另审供应商条件 | `no_sharing_commercial_or_hosting` |
| 同意将固定合成文本发送至该官方云端模型服务 | `allow_synthetic_cloud_request` |
| 理解最多一次顶层 CLI 启动不是内部模型请求／Token／费用的精确上限，并接受可能的额度或费用风险 | `understand_internal_calls_and_cost` |

计划、所有权和用途为本人声明，并非程序签发的供应商资格证明。若无法确认某项，不要改 true；保持 USER_CONFIRMATION_REQUIRED。若有精确金额硬限制而当前 route 无法强制，则不运行。

## 本地确认入口（零模型调用）

先按 USER_GUIDE 构建 `/tmp/xcli-nag`；`NAG_STATE` 是单独准备的新私有状态根，`NAG_CONFIG` 是本人选择的受限 YAML。Gateway 须停止。生成模板只读登记信息，六项默认 false，ref 空，不打开 native auth：

```bash
umask 077
/tmp/xcli-nag agent-gateway --state-dir "$NAG_STATE" --gateway-config "$NAG_CONFIG" \
  codex-limited confirmation-template > "$NAG_STATE/owner-confirmation.json"
```

本人审阅后手动编辑六项，`reference` 用不含身份资料的本次确认编号（8–128 字符），例如本人自定审批记录编号。不要让代理凭空填 true。模板绑定登记 SHA、策略 SHA 和 client_id；`recorded_at` 由导入命令本地记录，不能用模板时间延长有效期。

```bash
/tmp/xcli-nag agent-gateway --state-dir "$NAG_STATE" --gateway-config "$NAG_CONFIG" \
  codex-limited confirm --file "$NAG_STATE/owner-confirmation.json"
/tmp/xcli-nag agent-gateway --state-dir "$NAG_STATE" --gateway-config "$NAG_CONFIG" \
  codex-limited readiness
```

输入须为私有普通文件，未知字段、重复键、错误绑定、false／缺项被拒。记录只在私有状态根保留，readiness 公开摘要而非声明内容。有效期 24h；登记／策略／客户端变化、过期或移除 `codex-limited-confirmation.json` 即失效。本轮只测试构造的 owner fixture，没有导入本人的 true 声明。

## 独立门禁与授权

| 工程 | 本人声明 | 总体 state |
| --- | --- | --- |
| ENGINEERING_BLOCKED | 任意 | ENGINEERING_BLOCKED（另报声明轴） |
| ENGINEERING_READY | USER_CONFIRMATION_REQUIRED | USER_CONFIRMATION_REQUIRED |
| ENGINEERING_READY | CONFIRMED | READY_FOR_AUTHORIZATION |

ENGINEERING_READY 是独立工程轴；READY_FOR_AUTHORIZATION 不启动模型，也不代表调用许可。随后还需要本地显式启用、另行获得**本轮一次调用授权**并执行 authorize（15min），最后脚本显式 `--execute`。当前工程阻塞，导入声明也不能启用／授权。`provider_authorization_ref` 旧 YAML 字符串仅保留兼容，不能替代六项声明、供应商资格或调用授权。

个人本机 CLI 使用、应用 SIWC app-server、对其他应用提供通用推理是不同模式，见 [CODEX_LIMITED_PROFILE](CODEX_LIMITED_PROFILE.md)。本方案没有 SIWC 应用注册或 OAuth 授权；不能读／复用 Codex CLI 私有登录材料实现第三方 API。
