package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/stats"
	"github.com/spf13/cobra"
)

func newStatsCommand() *cobra.Command {
	var since, until, app string
	var asJSON bool
	c := &cobra.Command{Use: "stats", Short: "查看接入应用调用统计", RunE: func(c *cobra.Command, args []string) error {
		if len(args) != 0 {
			return statsParameterError("请使用 xcli stats [--since 30d] [--until YYYY-MM-DD] [--app NAME] [--json]")
		}
		if c.Flags().Changed("until") && until == "" {
			return statsParameterError("请按 YYYY-MM-DD 填写 --until，例如 2026-11-23")
		}
		filter, err := statsRange(since, until, time.Now().In(statsLocation()))
		if err != nil {
			return err
		}
		if c.Flags().Changed("app") {
			var valid bool
			filter.App, valid = stats.ParseApp(app)
			if !valid {
				return statsParameterError("请将 --app 设为 1–40 位字母、数字、点、下划线或连字符，并以字母或数字开头")
			}
		}
		cfg, err := config.Load(rootOpts.home)
		if err != nil {
			return err
		}
		records, sources, err := stats.Collect(stats.Homes(rootOpts.home, cfg.Stats.Include))
		if err != nil {
			return err
		}
		report := stats.Aggregate(records, filter)
		report.Sources = sources
		if asJSON {
			enc := json.NewEncoder(c.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(report)
		}
		return writeStats(c.OutOrStdout(), report, since)
	}}
	c.Flags().StringVar(&since, "since", "30d", "起始时间，可选 7d / 30d / 90d / all / YYYY-MM-DD")
	c.Flags().StringVar(&until, "until", "", "结束日期，YYYY-MM-DD（含当天），默认现在")
	c.Flags().StringVar(&app, "app", "", "只查看指定应用")
	c.Flags().BoolVar(&asJSON, "json", false, "输出 JSON")
	c.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return statsParameterError("请检查统计参数并运行 xcli stats --help：" + err.Error())
	})
	return c
}

func statsParameterError(message string) error {
	return &ExitError{Code: 2, Message: message}
}

func statsLocation() *time.Location {
	local := time.Local
	if local.String() != "Local" {
		return local
	}
	path, explicit := os.LookupEnv("TZ")
	if !explicit {
		path, _ = os.Readlink("/etc/localtime")
	}
	if _, name, ok := strings.Cut(path, "/zoneinfo/"); ok {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	return local
}

func statsRange(since, until string, now time.Time) (stats.Filter, error) {
	loc := now.Location()
	f := stats.Filter{Until: now, Location: loc}
	var err error
	if until != "" {
		date, err := time.ParseInLocation(time.DateOnly, until, loc)
		if err != nil {
			return f, statsParameterError("请按 YYYY-MM-DD 填写 --until，例如 2026-11-23")
		}
		f.Until = date.AddDate(0, 0, 1).Add(-time.Nanosecond)
	}
	switch since {
	case "7d", "30d", "90d":
		days, _ := strconv.Atoi(strings.TrimSuffix(since, "d"))
		y, m, d := f.Until.Date()
		f.Since = time.Date(y, m, d-days+1, 0, 0, 0, 0, loc)
	case "all":
	default:
		f.Since, err = time.ParseInLocation(time.DateOnly, since, loc)
		if err != nil {
			return f, statsParameterError("请将 --since 设为 7d、30d、90d、all 或 YYYY-MM-DD")
		}
	}
	if f.Since.After(f.Until) {
		return f, statsParameterError("请将 --since 设为不晚于 --until 的日期")
	}
	return f, nil
}

func writeStats(w io.Writer, report stats.Report, since string) error {
	var out strings.Builder
	zone := report.Range.Timezone
	switch since {
	case "7d", "30d", "90d":
		zone = fmt.Sprintf("近 %s 天，%s", strings.TrimSuffix(since, "d"), zone)
	}
	fmt.Fprintf(&out, "xcli 应用统计 · %s ～ %s（%s）\n", report.Range.Since.Format(time.DateOnly), report.Range.Until.Format(time.DateOnly), zone)
	o := report.Overview
	if o.Tasks == 0 {
		fmt.Fprintln(&out, "时间范围内没有任务")
	} else {
		fmt.Fprintf(&out, "任务 %d · 成功率 %s（成功 %d / 需关注 %d / 失败 %d）· 取消 %d\n",
			o.Tasks, statsRate(o.SuccessRate), o.Succeeded, o.NeedsAttention, o.Failed, o.Cancelled)
		fmt.Fprintf(&out, "Token：输入 %s / 输出 %s（%d/%d 个步骤有用量数据）\n",
			statsTokens(o.InputTokens), statsTokens(o.OutputTokens), o.StepsWithUsage, o.Steps)
		writeStatsTables(&out, report)
	}
	writeStatsDays(&out, report.Daily)
	s := report.Sources
	fmt.Fprintf(&out, "\n数据来源：账本 %d 条，补录 %d 条，跳过损坏 %d 行；数据目录 %d 个\n",
		s.LedgerRecords, s.BackfilledTasks, s.SkippedLines, len(s.Homes))
	if s.ApproximateTasks > 0 {
		fmt.Fprintf(&out, "其中 %d 个任务的 CLI 调用及耗时按可用记录估算\n", s.ApproximateTasks)
	}
	_, err := io.WriteString(w, out.String())
	return err
}

func writeStatsTables(out *strings.Builder, report stats.Report) {
	apps := [][]string{{"应用", "任务", "成功率", "失败", "取消", "平均耗时", "P50", "P90", "最近使用"}}
	for _, a := range report.Apps {
		apps = append(apps, []string{a.App, strconv.Itoa(a.Tasks), statsRate(a.SuccessRate), strconv.Itoa(a.Failed), strconv.Itoa(a.Cancelled),
			statsDuration(a.AvgMS, a.HasDuration), statsDuration(a.P50MS, a.HasDuration), statsDuration(a.P90MS, a.HasDuration), a.LastUsed.Format("01-02 15:04")})
	}
	writeStatsTable(out, "按应用", apps)

	agents := [][]string{{"CLI", "调用", "成功率", "平均耗时", "P90", "被切走", "接手", "Token 输入/输出"}}
	for _, a := range report.Agents {
		usage := "—"
		if a.HasUsage {
			usage = statsTokens(a.InputTokens) + " / " + statsTokens(a.OutputTokens)
		}
		agents = append(agents, []string{a.Agent, strconv.Itoa(a.Calls), statsRate(a.SuccessRate), statsDuration(a.AvgMS, a.HasDuration),
			statsDuration(a.P90MS, a.HasDuration), strconv.Itoa(a.FallbackFrom), strconv.Itoa(a.FallbackTo), usage})
	}
	writeStatsTable(out, "按 CLI（含审查和规划）", agents)

	failures := [][]string{{"原因", "次数", "占比", "涉及应用", "涉及 CLI"}}
	for _, e := range report.Errors {
		failures = append(failures, []string{statsValue(e.Kind), strconv.Itoa(e.Count), statsRate(&e.Share),
			statsValue(strings.Join(e.Apps, ", ")), statsValue(strings.Join(e.Agents, ", "))})
	}
	writeStatsTable(out, "失败原因", failures)

}

func writeStatsDays(out *strings.Builder, daily []stats.DayReport) {
	busiest := 0
	for _, d := range daily {
		busiest = max(busiest, d.Tasks)
	}
	days := [][]string{{"日期", "任务", "成功", "失败", "平均耗时", ""}}
	week := [...]string{"日", "一", "二", "三", "四", "五", "六"}
	for _, d := range daily {
		date, _ := time.Parse(time.DateOnly, d.Date)
		bar := ""
		if d.Tasks > 0 {
			bar = strings.Repeat("█", max(1, (20*d.Tasks+busiest/2)/busiest))
		}
		days = append(days, []string{date.Format("01-02") + " 周" + week[date.Weekday()], strconv.Itoa(d.Tasks),
			strconv.Itoa(d.Succeeded), strconv.Itoa(d.Failed), statsDuration(d.AvgMS, d.HasDuration), bar})
	}
	writeStatsTable(out, "按天", days)
}

func writeStatsTable(out *strings.Builder, title string, rows [][]string) {
	fmt.Fprint(out, "\n", title, "\n")
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], statsDisplayWidth(cell))
		}
	}
	for _, row := range rows {
		last := len(row) - 1
		if row[last] == "" {
			last--
		}
		for i, cell := range row[:last+1] {
			out.WriteString(cell)
			if i < last {
				out.WriteString(strings.Repeat(" ", widths[i]-statsDisplayWidth(cell)+2))
			}
		}
		out.WriteByte('\n')
	}
}

func statsDisplayWidth(s string) int {
	width := 0
	for _, r := range s {
		width++
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) ||
			(r >= 0x3000 && r <= 0x303f) || (r >= 0xff01 && r <= 0xff60) {
			width++
		}
	}
	return width
}

func statsValue(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func statsRate(rate *float64) string {
	if rate == nil {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", *rate*100)
}

func statsDuration(ms int64, present bool) string {
	if !present {
		return "—"
	}
	seconds := ms / 1000
	switch {
	case seconds >= 3600:
		return fmt.Sprintf("%dh%02dm", seconds/3600, seconds/60%60)
	case seconds >= 60:
		return fmt.Sprintf("%dm%02ds", seconds/60, seconds%60)
	case seconds >= 1:
		return fmt.Sprintf("%ds", seconds)
	default:
		return fmt.Sprintf("%dms", ms)
	}
}

func statsTokens(n int64) string {
	format := func(divisor float64, suffix string) string {
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/divisor), ".0") + suffix
	}
	if n >= 1_000_000 {
		return format(1_000_000, "M")
	}
	if n >= 1_000 {
		return format(1_000, "K")
	}
	return strconv.FormatInt(n, 10)
}
