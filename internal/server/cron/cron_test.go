package cron

import (
	"testing"
	"time"
)

func TestNext(t *testing.T) {
	cases := []struct {
		expr string
		from string
		want string
	}{
		{"0 2 * * *", "2026-07-02T01:00:00Z", "2026-07-02T02:00:00Z"},
		{"0 2 * * *", "2026-07-02T03:00:00Z", "2026-07-03T02:00:00Z"},
		{"*/15 * * * *", "2026-07-02T10:05:00Z", "2026-07-02T10:15:00Z"},
		{"30 9 * * 1", "2026-07-02T00:00:00Z", "2026-07-06T09:30:00Z"}, // 下一个周一
		{"0 0 1 * *", "2026-07-02T00:00:00Z", "2026-08-01T00:00:00Z"},
		{"0 0 * * 0", "2026-07-02T00:00:00Z", "2026-07-05T00:00:00Z"}, // 周日
	}
	for _, c := range cases {
		s, err := Parse(c.expr)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.expr, err)
		}
		from, _ := time.Parse(time.RFC3339, c.from)
		want, _ := time.Parse(time.RFC3339, c.want)
		got := s.Next(from)
		if !got.Equal(want) {
			t.Errorf("Next(%q, %s) = %s, want %s", c.expr, c.from, got.Format(time.RFC3339), c.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	bad := []string{"", "* * * *", "60 * * * *", "* 24 * * *", "* * 0 * *", "a * * * *", "*/0 * * * *"}
	for _, expr := range bad {
		if _, err := Parse(expr); err == nil {
			t.Errorf("Parse(%q) 应报错但通过了", expr)
		}
	}
}

func TestDOMOrDOWUnion(t *testing.T) {
	// 日=15 或 周=1(周一) 都应命中
	s, _ := Parse("0 0 15 * 1")
	mustMatch := []string{"2026-07-15T00:00:00Z", "2026-07-06T00:00:00Z"} // 15号 / 周一
	for _, ts := range mustMatch {
		tm, _ := time.Parse(time.RFC3339, ts)
		if !s.Matches(tm) {
			t.Errorf("%s 应命中", ts)
		}
	}
	tm, _ := time.Parse(time.RFC3339, "2026-07-07T00:00:00Z") // 周二且非15号
	if s.Matches(tm) {
		t.Errorf("2026-07-07 不应命中")
	}
}
