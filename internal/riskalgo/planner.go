// Package riskalgo 定义登录链路中高风险算法的可替换边界。
package riskalgo

import (
	"errors"
	"fmt"
	"strings"
)

var ErrUnsupportedLoginRiskProfile = errors.New("unsupported login risk profile")

type Status string

const (
	StatusSampleRequired Status = "sample_required"
)

type LoginRiskRequest struct {
	Operation string
	Platform  string
	LoginKind string
	DeviceID  string
	Wxid      string
}

type ComponentPlan struct {
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Status         Status   `json:"status"`
	Replaceable    bool     `json:"replaceable"`
	MissingSamples []string `json:"missing_samples,omitempty"`
	NextStep       string   `json:"next_step"`
}

type LoginRiskPlan struct {
	Operation      string          `json:"operation"`
	Platform       string          `json:"platform"`
	LoginKind      string          `json:"login_kind"`
	DeviceID       string          `json:"device_id,omitempty"`
	Wxid           string          `json:"wxid,omitempty"`
	Status         Status          `json:"status"`
	Ready          bool            `json:"ready"`
	Components     []ComponentPlan `json:"components"`
	MissingSamples []string        `json:"missing_samples"`
	Stages         []string        `json:"stages"`
}

type Planner interface {
	BuildLoginRiskPlan(req LoginRiskRequest) (LoginRiskPlan, error)
}

type DefaultPlanner struct{}

func BuildLoginRiskPlan(req LoginRiskRequest) (LoginRiskPlan, error) {
	return DefaultPlanner{}.BuildLoginRiskPlan(req)
}

func (DefaultPlanner) BuildLoginRiskPlan(req LoginRiskRequest) (LoginRiskPlan, error) {
	normalized := normalizeRequest(req)
	components, err := componentsFor(normalized.Platform, normalized.LoginKind)
	if err != nil {
		return LoginRiskPlan{}, err
	}
	missing := collectMissingSamples(components)
	return LoginRiskPlan{
		Operation:      normalized.Operation,
		Platform:       normalized.Platform,
		LoginKind:      normalized.LoginKind,
		DeviceID:       normalized.DeviceID,
		Wxid:           normalized.Wxid,
		Status:         StatusSampleRequired,
		Ready:          false,
		Components:     components,
		MissingSamples: missing,
		Stages: []string{
			"select_login_risk_profile",
			"mark_sample_required",
			"expose_replaceable_boundary",
		},
	}, nil
}

func (p LoginRiskPlan) ToMap() map[string]any {
	components := make([]map[string]any, 0, len(p.Components))
	for _, component := range p.Components {
		m := map[string]any{
			"name":        component.Name,
			"category":    component.Category,
			"status":      string(component.Status),
			"replaceable": component.Replaceable,
			"next_step":   component.NextStep,
		}
		if len(component.MissingSamples) > 0 {
			missing := append([]string(nil), component.MissingSamples...)
			m["missing_samples"] = missing
		}
		components = append(components, m)
	}
	return map[string]any{
		"operation":       p.Operation,
		"platform":        p.Platform,
		"login_kind":      p.LoginKind,
		"device_id":       p.DeviceID,
		"wxid":            p.Wxid,
		"status":          string(p.Status),
		"ready":           p.Ready,
		"components":      components,
		"missing_samples": append([]string(nil), p.MissingSamples...),
		"stages":          append([]string(nil), p.Stages...),
	}
}

func normalizeRequest(req LoginRiskRequest) LoginRiskRequest {
	req.Operation = strings.TrimSpace(req.Operation)
	req.Platform = strings.ToLower(strings.TrimSpace(req.Platform))
	req.LoginKind = strings.ToLower(strings.TrimSpace(req.LoginKind))
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	req.Wxid = strings.TrimSpace(req.Wxid)
	if req.Platform == "" {
		switch req.LoginKind {
		case "data62_mock", "getqr_mock":
			req.Platform = "ios"
		case "a16_mock":
			req.Platform = "android"
		}
	}
	return req
}

func componentsFor(platform, loginKind string) ([]ComponentPlan, error) {
	switch {
	case platform == "ios" && loginKind == "data62_mock":
		return []ComponentPlan{
			sampleRequired("sae_encrypt_01", "sae", "fixtures/riskalgo/ios/sae_encrypt_01/*.json"),
			sampleRequired("sae_encrypt_06", "sae", "fixtures/riskalgo/ios/sae_encrypt_06/*.json"),
			sampleRequired("do_encrypt_input", "sae", "fixtures/riskalgo/ios/do_encrypt_input/*.json"),
			sampleRequired("iphone_antispam", "antispam", "fixtures/riskalgo/ios/iphone_antispam/*.json"),
		}, nil
	case platform == "android" && loginKind == "a16_mock":
		return []ComponentPlan{
			sampleRequired("zt_init", "zt", "fixtures/riskalgo/android/zt_init/*.json"),
			sampleRequired("zt_encrypt", "zt", "fixtures/riskalgo/android/zt_encrypt/*.json"),
			sampleRequired("android_antispam", "antispam", "fixtures/riskalgo/android/antispam/*.json"),
			sampleRequired("android_cc_data", "cc_data", "fixtures/riskalgo/android/cc_data/*.json"),
		}, nil
	default:
		return nil, fmt.Errorf("%w: platform=%q login_kind=%q", ErrUnsupportedLoginRiskProfile, platform, loginKind)
	}
}

func sampleRequired(name, category, samplePattern string) ComponentPlan {
	return ComponentPlan{
		Name:           name,
		Category:       category,
		Status:         StatusSampleRequired,
		Replaceable:    true,
		MissingSamples: []string{samplePattern},
		NextStep:       "补充真实输入输出样本后替换占位实现并进行字节级对拍",
	}
}

func collectMissingSamples(components []ComponentPlan) []string {
	var missing []string
	for _, component := range components {
		missing = append(missing, component.MissingSamples...)
	}
	return missing
}
