package agent

import "math"

type Usage struct {
	InputTokens      int64    `json:"input_tokens"`
	CacheReadTokens  int64    `json:"cache_read_tokens"`
	CacheWriteTokens int64    `json:"cache_write_tokens"`
	OutputTokens     int64    `json:"output_tokens"`
	ReasoningTokens  int64    `json:"reasoning_tokens"`
	TotalTokens      int64    `json:"total_tokens"`
	EstimatedCostUSD *float64 `json:"estimated_cost_usd,omitempty"`
}

func (u *Usage) Add(other *Usage) {
	if other == nil {
		return
	}
	u.InputTokens = addTokenCounts(u.InputTokens, other.InputTokens)
	u.CacheReadTokens = addTokenCounts(u.CacheReadTokens, other.CacheReadTokens)
	u.CacheWriteTokens = addTokenCounts(u.CacheWriteTokens, other.CacheWriteTokens)
	u.OutputTokens = addTokenCounts(u.OutputTokens, other.OutputTokens)
	u.ReasoningTokens = addTokenCounts(u.ReasoningTokens, other.ReasoningTokens)
	u.TotalTokens = addTokenCounts(u.TotalTokens, other.TotalTokens)
	if other.EstimatedCostUSD != nil {
		cost := *other.EstimatedCostUSD
		if u.EstimatedCostUSD != nil {
			if cost > math.MaxFloat64-*u.EstimatedCostUSD {
				cost = math.MaxFloat64
			} else {
				cost += *u.EstimatedCostUSD
			}
		}
		u.EstimatedCostUSD = &cost
	}
}

type usageAccumulator struct {
	usage           Usage
	seen            bool
	opencodePartIDs map[string]bool
}

func newUsageAccumulator() *usageAccumulator {
	return &usageAccumulator{opencodePartIDs: map[string]bool{}}
}

func (a *usageAccumulator) capture(adapter string, event map[string]interface{}) {
	switch adapter {
	case "codex":
		a.captureCodex(event)
	case "claude":
		a.captureClaude(event)
	case "gemini":
		a.captureGemini(event)
	case "opencode":
		a.captureOpenCode(event)
	}
}

func (a *usageAccumulator) result() *Usage {
	if !a.seen {
		return nil
	}
	result := a.usage
	if a.usage.EstimatedCostUSD != nil {
		cost := *a.usage.EstimatedCostUSD
		result.EstimatedCostUSD = &cost
	}
	return &result
}

func (a *usageAccumulator) captureCodex(event map[string]interface{}) {
	if stringValue(event, "type") != "turn.completed" {
		return
	}
	values, ok := mapValue(event, "usage")
	if !ok {
		return
	}
	input, hasInput := tokenValue(values, "input_tokens")
	cacheRead, hasCache := tokenValue(values, "cached_input_tokens")
	output, hasOutput := tokenValue(values, "output_tokens")
	reasoning, hasReasoning := tokenValue(values, "reasoning_output_tokens")
	total, hasTotal := tokenValue(values, "total_tokens")
	if !hasInput && !hasCache && !hasOutput && !hasReasoning && !hasTotal {
		return
	}
	usage := Usage{
		InputTokens:     subtractFloor(input, cacheRead),
		CacheReadTokens: cacheRead,
		OutputTokens:    subtractFloor(output, reasoning),
		ReasoningTokens: reasoning,
	}
	usage.TotalTokens = totalOrSum(total, hasTotal, usage)
	a.add(&usage)
}

func (a *usageAccumulator) captureClaude(event map[string]interface{}) {
	if stringValue(event, "type") != "result" {
		return
	}
	usage := Usage{}
	seen := false
	if values, ok := mapValue(event, "usage"); ok {
		seen = assignToken(values, "input_tokens", &usage.InputTokens) || seen
		seen = assignToken(values, "cache_read_input_tokens", &usage.CacheReadTokens) || seen
		seen = assignToken(values, "cache_creation_input_tokens", &usage.CacheWriteTokens) || seen
		seen = assignToken(values, "output_tokens", &usage.OutputTokens) || seen
	}
	if cost, ok := nonNegativeNumber(event, "total_cost_usd"); ok {
		usage.EstimatedCostUSD = &cost
		seen = true
	}
	if !seen {
		return
	}
	usage.TotalTokens = componentTotal(usage)
	a.add(&usage)
}

func (a *usageAccumulator) captureGemini(event map[string]interface{}) {
	if stringValue(event, "type") != "result" {
		return
	}
	stats, ok := mapValue(event, "stats")
	if !ok {
		return
	}
	input, hasInput := tokenValue(stats, "input")
	inputTotal, hasInputTotal := tokenValue(stats, "input_tokens")
	cacheRead, hasCache := tokenValue(stats, "cached")
	output, hasOutput := tokenValue(stats, "output_tokens")
	total, hasTotal := tokenValue(stats, "total_tokens")
	if !hasInput && !hasInputTotal && !hasCache && !hasOutput && !hasTotal {
		return
	}
	if !hasInput && hasInputTotal {
		input = subtractFloor(inputTotal, cacheRead)
	}
	usage := Usage{InputTokens: input, CacheReadTokens: cacheRead, OutputTokens: output}
	if hasTotal {
		accounted := addTokenCounts(input, cacheRead)
		accounted = addTokenCounts(accounted, output)
		usage.ReasoningTokens = subtractFloor(total, accounted)
	}
	usage.TotalTokens = totalOrSum(total, hasTotal, usage)
	a.add(&usage)
}

func (a *usageAccumulator) captureOpenCode(event map[string]interface{}) {
	if stringValue(event, "type") != "step_finish" {
		return
	}
	part, ok := mapValue(event, "part")
	if !ok {
		return
	}
	if id := stringValue(part, "id"); id != "" {
		if a.opencodePartIDs[id] {
			return
		}
		a.opencodePartIDs[id] = true
	}
	usage := Usage{}
	seen := false
	total := int64(0)
	hasTotal := false
	if values, ok := mapValue(part, "tokens"); ok {
		seen = assignToken(values, "input", &usage.InputTokens) || seen
		seen = assignToken(values, "output", &usage.OutputTokens) || seen
		seen = assignToken(values, "reasoning", &usage.ReasoningTokens) || seen
		total, hasTotal = tokenValue(values, "total")
		seen = seen || hasTotal
		if cache, ok := mapValue(values, "cache"); ok {
			seen = assignToken(cache, "read", &usage.CacheReadTokens) || seen
			seen = assignToken(cache, "write", &usage.CacheWriteTokens) || seen
		}
	}
	if cost, ok := nonNegativeNumber(part, "cost"); ok {
		usage.EstimatedCostUSD = &cost
		seen = true
	}
	if !seen {
		return
	}
	usage.TotalTokens = totalOrSum(total, hasTotal, usage)
	a.add(&usage)
}

func (a *usageAccumulator) add(usage *Usage) {
	a.usage.Add(usage)
	a.seen = true
}

func assignToken(values map[string]interface{}, key string, destination *int64) bool {
	value, ok := tokenValue(values, key)
	if ok {
		*destination = value
	}
	return ok
}

func mapValue(values map[string]interface{}, key string) (map[string]interface{}, bool) {
	value, ok := values[key].(map[string]interface{})
	return value, ok
}

func stringValue(values map[string]interface{}, key string) string {
	value, _ := values[key].(string)
	return value
}

func tokenValue(values map[string]interface{}, key string) (int64, bool) {
	value, ok := values[key].(float64)
	if !ok || value < 0 || value > math.MaxInt64 || math.Trunc(value) != value {
		return 0, false
	}
	return int64(value), true
}

func nonNegativeNumber(values map[string]interface{}, key string) (float64, bool) {
	value, ok := values[key].(float64)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, false
	}
	return value, true
}

func subtractFloor(total, part int64) int64 {
	if part >= total {
		return 0
	}
	return total - part
}

func componentTotal(usage Usage) int64 {
	total := addTokenCounts(usage.InputTokens, usage.CacheReadTokens)
	total = addTokenCounts(total, usage.CacheWriteTokens)
	total = addTokenCounts(total, usage.OutputTokens)
	return addTokenCounts(total, usage.ReasoningTokens)
}

func totalOrSum(total int64, hasTotal bool, usage Usage) int64 {
	if hasTotal {
		return total
	}
	return componentTotal(usage)
}

func addTokenCounts(left, right int64) int64 {
	if right > math.MaxInt64-left {
		return math.MaxInt64
	}
	return left + right
}
