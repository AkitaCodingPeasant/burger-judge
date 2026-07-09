package domain

// 此檔案測試角色階層與提交狀態機。

import "testing"

// TestTerminalRejectsUnknownStatus 驗證未知狀態不會被視為完成。
func TestTerminalRejectsUnknownStatus(t *testing.T) {
	if SubmissionStatus("UNKNOWN").Terminal() {
		t.Fatal("an unknown status must not be treated as a completed result")
	}
	if !StatusAC.Terminal() || StatusRunning.Terminal() {
		t.Fatal("known terminal and active statuses were classified incorrectly")
	}
}

// TestRoleAllowsOnlyKnownRequirements 驗證角色只能滿足已定義的權限需求。
func TestRoleAllowsOnlyKnownRequirements(t *testing.T) {
	if !RoleAdmin.Allows(RoleUser) || RoleUser.Allows(RoleAdmin) {
		t.Fatal("role hierarchy is incorrect")
	}
	if RoleAdmin.Allows(Role("unknown")) {
		t.Fatal("admin must not satisfy an unknown authorization requirement")
	}
}
