// Package cron 实现标准 5 段 cron 表达式（分 时 日 月 周）的解析与调度计算。
// 支持 * / */n / a-b / a,b,c / 单值 及其组合。周字段 0-6，0=周日（7 亦视为周日）。
package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule 表示一个已解析的 cron 表达式。
type Schedule struct {
	minute map[int]bool
	hour   map[int]bool
	dom    map[int]bool
	month  map[int]bool
	dow    map[int]bool
	// domRestricted / dowRestricted 标记对应字段是否为 "*"，
	// 用于实现 cron 惯例：当日、周字段都被限定时取并集。
	domRestricted bool
	dowRestricted bool
}

// Parse 解析一个 5 段 cron 表达式。
func Parse(expr string) (*Schedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron 表达式需为 5 段（分 时 日 月 周），得到 %d 段", len(fields))
	}
	s := &Schedule{}
	var err error
	if s.minute, err = parseField(fields[0], 0, 59); err != nil {
		return nil, fmt.Errorf("分字段: %w", err)
	}
	if s.hour, err = parseField(fields[1], 0, 23); err != nil {
		return nil, fmt.Errorf("时字段: %w", err)
	}
	if s.dom, err = parseField(fields[2], 1, 31); err != nil {
		return nil, fmt.Errorf("日字段: %w", err)
	}
	if s.month, err = parseField(fields[3], 1, 12); err != nil {
		return nil, fmt.Errorf("月字段: %w", err)
	}
	if s.dow, err = parseDOW(fields[4]); err != nil {
		return nil, fmt.Errorf("周字段: %w", err)
	}
	s.domRestricted = fields[2] != "*"
	s.dowRestricted = fields[4] != "*"
	return s, nil
}

// Matches 判断给定时间是否命中该 schedule（秒级归零比较到分钟）。
func (s *Schedule) Matches(t time.Time) bool {
	if !s.minute[t.Minute()] || !s.hour[t.Hour()] || !s.month[int(t.Month())] {
		return false
	}
	dom := s.dom[t.Day()]
	dow := s.dow[int(t.Weekday())]
	// cron 惯例：日、周都被限定时命中其一即可；否则要求被限定的那个命中。
	switch {
	case s.domRestricted && s.dowRestricted:
		return dom || dow
	case s.domRestricted:
		return dom
	case s.dowRestricted:
		return dow
	default:
		return true
	}
}

// Next 返回 after 之后（不含 after 当分钟）的下一个命中时间；无解则返回零值。
func (s *Schedule) Next(after time.Time) time.Time {
	// 从下一分钟开始，逐分钟检查，最多向后一年。
	t := after.Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(1, 0, 0)
	for t.Before(limit) {
		if s.Matches(t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}
}

func parseField(field string, min, max int) (map[int]bool, error) {
	out := make(map[int]bool)
	for _, part := range strings.Split(field, ",") {
		step := 1
		if idx := strings.Index(part, "/"); idx != -1 {
			var err error
			if step, err = strconv.Atoi(part[idx+1:]); err != nil || step <= 0 {
				return nil, fmt.Errorf("非法步长 %q", part)
			}
			part = part[:idx]
		}

		lo, hi := min, max
		switch {
		case part == "*":
			// 保持全范围
		case strings.Contains(part, "-"):
			bounds := strings.SplitN(part, "-", 2)
			var err error
			if lo, err = strconv.Atoi(bounds[0]); err != nil {
				return nil, fmt.Errorf("非法范围 %q", part)
			}
			if hi, err = strconv.Atoi(bounds[1]); err != nil {
				return nil, fmt.Errorf("非法范围 %q", part)
			}
		default:
			v, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("非法值 %q", part)
			}
			lo, hi = v, v
		}

		if lo < min || hi > max || lo > hi {
			return nil, fmt.Errorf("值超出范围 [%d,%d]: %q", min, max, part)
		}
		for v := lo; v <= hi; v += step {
			out[v] = true
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("空字段")
	}
	return out, nil
}

// parseDOW 解析周字段，额外把 7 归一化为 0（周日）。
func parseDOW(field string) (map[int]bool, error) {
	m, err := parseField(field, 0, 7)
	if err != nil {
		return nil, err
	}
	if m[7] {
		m[0] = true
		delete(m, 7)
	}
	return m, nil
}
