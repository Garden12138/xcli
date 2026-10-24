import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { DateRangePicker } from "./DateRangePicker";
import type { RangePreset } from "../types";

const presets: Array<[RangePreset, string]> = [
  ["30d", "近 30 天"], ["1y", "近 1 年"], ["all", "全部"], ["custom", "自定义"],
];

describe("independent chat-insight date range acceptance", () => {
  it.each(presets)("exposes exactly the selected %s preset", (selected) => {
    render(<DateRangePicker preset={selected} range={{}} onPresetChange={vi.fn()} onRangeChange={vi.fn()} />);
    for (const [preset, label] of presets) {
      expect(screen.getByRole("button", { name: label })).toHaveAttribute("aria-pressed", String(preset === selected));
    }
  });

  it("reflects external props and preserves the selection callback", () => {
    const onPresetChange = vi.fn();
    const props = { range: {}, onPresetChange, onRangeChange: vi.fn() };
    const { rerender } = render(<DateRangePicker {...props} preset="30d" />);
    fireEvent.click(screen.getByRole("button", { name: "自定义" }));
    expect(onPresetChange).toHaveBeenLastCalledWith("custom");
    rerender(<DateRangePicker {...props} preset="custom" />);
    expect(screen.getByRole("button", { name: "自定义" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByLabelText("开始日期")).toBeVisible();
    rerender(<DateRangePicker {...props} preset="all" />);
    expect(screen.getByRole("button", { name: "全部" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.queryByLabelText("开始日期")).toBeNull();
  });

  it("keeps the other custom endpoint and permits clearing", () => {
    const onRangeChange = vi.fn();
    render(<DateRangePicker preset="custom" range={{ from: "2026-10-01", to: "2026-10-24" }} onPresetChange={vi.fn()} onRangeChange={onRangeChange} />);
    fireEvent.change(screen.getByLabelText("开始日期"), { target: { value: "2026-10-02" } });
    expect(onRangeChange).toHaveBeenLastCalledWith({ from: "2026-10-02", to: "2026-10-24" });
    fireEvent.change(screen.getByLabelText("开始日期"), { target: { value: "" } });
    expect(onRangeChange).toHaveBeenLastCalledWith({ from: undefined, to: "2026-10-24" });
    fireEvent.change(screen.getByLabelText("结束日期"), { target: { value: "" } });
    expect(onRangeChange).toHaveBeenLastCalledWith({ from: "2026-10-01", to: undefined });
  });
});
