package riskalgo_test

import (
	"errors"
	"testing"

	"github.com/mahiro424/cbs/internal/riskalgo"
)

func TestBuildLoginRiskPlanMarksIOSAndAndroidSamplesRequired(t *testing.T) {
	cases := []struct {
		name           string
		request        riskalgo.LoginRiskRequest
		wantPlatform   string
		wantLoginKind  string
		wantComponents []string
	}{
		{
			name: "iOS 62data 高风险算法计划",
			request: riskalgo.LoginRiskRequest{
				Operation: "Login.62data",
				Platform:  "ios",
				LoginKind: "data62_mock",
				DeviceID:  "iphone-001",
				Wxid:      "wxid_62",
			},
			wantPlatform:   "ios",
			wantLoginKind:  "data62_mock",
			wantComponents: []string{"sae_encrypt_01", "sae_encrypt_06", "do_encrypt_input", "iphone_antispam"},
		},
		{
			name: "Android A16 高风险算法计划",
			request: riskalgo.LoginRiskRequest{
				Operation: "Login.A16Data",
				Platform:  "android",
				LoginKind: "a16_mock",
				DeviceID:  "android-001",
				Wxid:      "wxid_a16",
			},
			wantPlatform:   "android",
			wantLoginKind:  "a16_mock",
			wantComponents: []string{"zt_init", "zt_encrypt", "android_antispam", "android_cc_data"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := riskalgo.BuildLoginRiskPlan(tc.request)
			if err != nil {
				t.Fatalf("生成高风险算法计划失败：%v", err)
			}
			if plan.Status != riskalgo.StatusSampleRequired || plan.Ready {
				t.Fatalf("计划状态 = %s / ready=%v，期望 sample_required 且未就绪", plan.Status, plan.Ready)
			}
			if plan.Platform != tc.wantPlatform || plan.LoginKind != tc.wantLoginKind || plan.Operation != tc.request.Operation {
				t.Fatalf("计划上下文 = %+v，期望保留 operation/platform/login_kind", plan)
			}
			if len(plan.Components) != len(tc.wantComponents) {
				t.Fatalf("组件数量 = %d，期望 %d：%+v", len(plan.Components), len(tc.wantComponents), plan.Components)
			}
			for i, component := range plan.Components {
				if component.Name != tc.wantComponents[i] {
					t.Fatalf("组件[%d] = %s，期望 %s", i, component.Name, tc.wantComponents[i])
				}
				if component.Status != riskalgo.StatusSampleRequired || !component.Replaceable || len(component.MissingSamples) == 0 {
					t.Fatalf("组件[%d] = %+v，期望明确样本缺失且可替换", i, component)
				}
			}
			asMap := plan.ToMap()
			if asMap["status"] != string(riskalgo.StatusSampleRequired) || asMap["ready"] != false {
				t.Fatalf("ToMap = %+v，期望保留状态字段", asMap)
			}
			if _, ok := asMap["components"]; !ok {
				t.Fatalf("ToMap = %+v，期望包含 components", asMap)
			}
		})
	}
}

func TestBuildLoginRiskPlanRejectsUnsupportedProfile(t *testing.T) {
	_, err := riskalgo.BuildLoginRiskPlan(riskalgo.LoginRiskRequest{
		Operation: "Login.Unknown",
		Platform:  "web",
		LoginKind: "unknown",
	})
	if !errors.Is(err, riskalgo.ErrUnsupportedLoginRiskProfile) {
		t.Fatalf("错误 = %v，期望 ErrUnsupportedLoginRiskProfile", err)
	}
}
