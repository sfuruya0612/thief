package aws

import (
	"testing"
	"time"
)

// TestTerminateSessionGracePeriodValue は TerminateSessionGracePeriod の値を固定する。
// この定数は internal/session, internal/api, internal/cli の 4 箇所が共有する唯一の
// 定義であり、値が意図せず変わっても他のテストがそれを検知しないため、ここで直接検証する。
func TestTerminateSessionGracePeriodValue(t *testing.T) {
	want := 5 * time.Second
	if TerminateSessionGracePeriod != want {
		t.Errorf("TerminateSessionGracePeriod = %v, want %v", TerminateSessionGracePeriod, want)
	}
}
