package driver

import (
	"strings"

	"github.com/Garden12138/xcli/internal/api"
)

func (k ErrorKind) Switchable(kind api.Kind) bool {
	switch k {
	case ErrNotInstalled, ErrAuth, ErrQuota, ErrTrust, ErrUnavailable:
		return true
	case ErrPermissionDenied:
		return kind == api.KindAsk || kind == api.KindReview
	case ErrNoArtifact:
		return kind == api.KindImage
	}
	return false
}

func ClassifyText(stderrTail, text string) ErrorKind {
	s := strings.ToLower(stderrTail + "\n" + text)
	for _, rule := range []struct {
		kind ErrorKind
		keys []string
	}{
		{ErrQuota, []string{"usage_limit_exceeded", "quota_exceeded", "usage limit", "quota", "rate limit", "rate_limit", "429", "too many requests"}},
		{ErrUnavailable, []string{"503", "unavailable", "no capacity", "overloaded"}},
		{ErrAuth, []string{"not logged", "not authenticated", "unauthorized", "401", "authentication", "login", "sign in"}},
		{ErrTrust, []string{"workspace trust", "trust required", "工作区信任"}},
		{ErrPermissionDenied, []string{"permission denied", "permission check failed", "sandbox denied"}},
	} {
		for _, key := range rule.keys {
			if strings.Contains(s, key) {
				return rule.kind
			}
		}
	}
	return ErrNone
}

func errorMessage(name string, kind ErrorKind) string {
	command := map[string]string{"codex": "codex", "cursor": "cursor-agent", "antigravity": "agy"}[name]
	switch kind {
	case ErrAuth:
		if name == "antigravity" {
			return "请在终端运行 agy 完成登录后重试"
		}
		return "请在终端运行 " + command + " login 登录后重试"
	case ErrQuota:
		return name + " 额度暂不可用，请稍后重试或选择其他 CLI"
	case ErrUnavailable:
		return name + " 服务暂不可用，请稍后重试或选择其他 CLI；详情见步骤日志 stdout.jsonl 和 stderr.log"
	case ErrTrust:
		return "请检查 Cursor 的 --trust 和工作目录设置后重试"
	case ErrPermissionDenied:
		return "请检查任务所需权限和 CLI 权限设置，调整后重试"
	case ErrProtocol:
		return "请查看步骤日志中的 stdout.jsonl 和 stderr.log，检查 CLI 输出格式后重试"
	default:
		return "请查看步骤日志中的 stdout.jsonl 和 stderr.log，检查 CLI 参数、模型和运行环境后重试"
	}
}
