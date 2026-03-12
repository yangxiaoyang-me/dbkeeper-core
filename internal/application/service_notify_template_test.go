package application

import "testing"

func TestBuildNotifyMessage_Default(t *testing.T) {
	got := buildNotifyMessage("", 0, 3, 0, 5, 2, "partial_failed", 12.3456)
	want := "success=0, failed=3, sync_failed=0"
	if got != want {
		t.Fatalf("unexpected default message: want=%q got=%q", want, got)
	}
}

func TestBuildNotifyMessage_Template(t *testing.T) {
	tpl := "结果: success={{success}}, failed={{failed}}, sync_failed={{sync_failed}}, status={status}, duration=${duration_s}"
	got := buildNotifyMessage(tpl, 7, 1, 2, 10, 4, "partial_failed", 9.8765)
	want := "结果: success=7, failed=1, sync_failed=2, status=partial_failed, duration=9.877"
	if got != want {
		t.Fatalf("unexpected template message: want=%q got=%q", want, got)
	}
}
