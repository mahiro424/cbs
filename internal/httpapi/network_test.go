package httpapi_test

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/mahiro424/cbs/internal/config"
	"github.com/mahiro424/cbs/internal/httpapi"
)

func TestLoginMockPathsIncludeNetworkSummaryAndSamples(t *testing.T) {
	cfg := config.Default()
	cfg.NetworkMode = "mock"
	cfg.SampleDir = t.TempDir()
	h := httpapi.NewServer(cfg)

	cases := []struct {
		name      string
		path      string
		payload   string
		operation string
		loginKind string
		platform  string
	}{
		{
			name:      "二维码登录",
			path:      "/Login/GetQR",
			payload:   `{"DeviceID":"network-qr","DeviceName":"网络二维码设备","Type":"ipad"}`,
			operation: "Login.GetQR",
			loginKind: "getqr_mock",
			platform:  "ios",
		},
		{
			name:      "62 登录",
			path:      "/Login/62data",
			payload:   `{"Data62":"network-62","DeviceID":"network-iphone","DeviceName":"网络 62 设备","Wxid":"wxid_network_62"}`,
			operation: "Login.62data",
			loginKind: "data62_mock",
			platform:  "ios",
		},
		{
			name:      "A16 登录",
			path:      "/Login/A16Data",
			payload:   `{"A16":"network-a16","DeviceID":"network-android","DeviceName":"网络 A16 设备","Wxid":"wxid_network_a16"}`,
			operation: "Login.A16Data",
			loginKind: "a16_mock",
			platform:  "android",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postJSON(t, h, tc.path, tc.payload)
			if !resp.Success || resp.Code != "ok" {
				t.Fatalf("%s 响应 = %+v，期望 ok", tc.path, resp)
			}
			data := mustMap(t, resp.Data)
			networkSummary := mustMap(t, data["network"])
			if networkSummary["mode"] != "mock" || networkSummary["operation"] != tc.operation || networkSummary["login_kind"] != tc.loginKind || networkSummary["platform"] != tc.platform {
				t.Fatalf("network = %+v，期望包含 mock/%s/%s/%s", networkSummary, tc.operation, tc.loginKind, tc.platform)
			}
			if networkSummary["stage"] != "mock_network_response" || networkSummary["payload_sha256"] == "" {
				t.Fatalf("network 摘要 = %+v，期望包含 mock 阶段和 payload 摘要", networkSummary)
			}

			samplePath := mustString(t, data, "sample_path")
			raw, err := os.ReadFile(samplePath)
			if err != nil {
				t.Fatalf("读取样本失败：%v", err)
			}
			var sample map[string]any
			if err := json.Unmarshal(raw, &sample); err != nil {
				t.Fatalf("样本不是 JSON：%v", err)
			}
			sampleNetwork := mustMap(t, sample["network"])
			if sampleNetwork["mode"] != "mock" || sampleNetwork["operation"] != tc.operation {
				t.Fatalf("样本 network = %+v，期望落盘同一网络摘要", sampleNetwork)
			}
		})
	}
}

func TestRealNetworkModeReturnsStableNetworkError(t *testing.T) {
	cfg := config.Default()
	cfg.NetworkMode = "real"
	cfg.SampleDir = t.TempDir()
	h := httpapi.NewServer(cfg)

	status, body := postJSONWithStatus(t, h, "/Login/GetQR", `{"DeviceID":"real-network","DeviceName":"真实网络未就绪设备","Type":"ipad"}`)
	if status != http.StatusBadGateway {
		t.Fatalf("状态码 = %d，期望 502，响应：%+v", status, body)
	}
	if body.Success || body.Code != "network_error" || body.Message == "" {
		t.Fatalf("real network 响应 = %+v，期望 network_error", body)
	}
}

func TestHealthzReportsNetworkMode(t *testing.T) {
	cfg := config.Default()
	cfg.NetworkMode = "mock"
	h := httpapi.NewServer(cfg)

	health := getJSON(t, h, "/healthz")
	if !health.Success || health.Code != "ok" {
		t.Fatalf("healthz 响应 = %+v，期望 ok", health)
	}
	data := mustMap(t, health.Data)
	networkSummary := mustMap(t, data["network"])
	if networkSummary["mode"] != "mock" || networkSummary["available"] != true {
		t.Fatalf("healthz network = %+v，期望 mode=mock 且 available=true", networkSummary)
	}
}
