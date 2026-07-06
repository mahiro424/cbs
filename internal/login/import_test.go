package login_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mahiro424/cbs/internal/login"
	"github.com/mahiro424/cbs/internal/network"
	"github.com/mahiro424/cbs/internal/storage"
)

func TestServiceImportsLoginMaterialBuildMockContextSampleAndWxidState(t *testing.T) {
	cases := []struct {
		name             string
		call             func(*login.Service) (login.ImportResult, error)
		wantDeviceID     string
		wantDeviceName   string
		wantType         string
		wantWxid         string
		wantLoginKind    string
		wantOperation    string
		wantPackKind     string
		wantPlatform     string
		wantStage        string
		wantRequestField string
		wantRequestValue string
	}{
		{
			name: "62data 导入登录",
			call: func(service *login.Service) (login.ImportResult, error) {
				return service.Import62Data(context.Background(), login.Import62DataRequest{
					Data62:     "svc-62-data",
					DeviceID:   "svc-iphone-001",
					DeviceName: "业务层 62 设备",
					Wxid:       "wxid_svc_62",
				})
			},
			wantDeviceID:     "svc-iphone-001",
			wantDeviceName:   "业务层 62 设备",
			wantType:         "iphone",
			wantWxid:         "wxid_svc_62",
			wantLoginKind:    "data62_mock",
			wantOperation:    "Login.62data",
			wantPackKind:     "hybrid_ecdh_ios_placeholder",
			wantPlatform:     "ios",
			wantStage:        "load_62data_fixture",
			wantRequestField: "data62",
			wantRequestValue: "svc-62-data",
		},
		{
			name: "A16 导入登录",
			call: func(service *login.Service) (login.ImportResult, error) {
				return service.ImportA16Data(context.Background(), login.ImportA16DataRequest{
					A16:        "svc-a16-data",
					DeviceID:   "svc-android-001",
					DeviceName: "业务层 A16 设备",
					Wxid:       "wxid_svc_a16",
				})
			},
			wantDeviceID:     "svc-android-001",
			wantDeviceName:   "业务层 A16 设备",
			wantType:         "android",
			wantWxid:         "wxid_svc_a16",
			wantLoginKind:    "a16_mock",
			wantOperation:    "Login.A16Data",
			wantPackKind:     "hybrid_ecdh_android_placeholder",
			wantPlatform:     "android",
			wantStage:        "load_a16_fixture",
			wantRequestField: "a16",
			wantRequestValue: "svc-a16-data",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := storage.NewMemoryLoginStateStore()
			client, _, err := network.NewClient(network.Config{Mode: "mock"})
			if err != nil {
				t.Fatalf("创建 mock network client 失败：%v", err)
			}
			fixedNow := time.Date(2026, 7, 6, 23, 5, 0, 987654321, time.UTC)
			service := login.NewService(login.Dependencies{
				States:    store,
				Network:   client,
				SampleDir: t.TempDir(),
				Now:       func() time.Time { return fixedNow },
			})

			result, err := tc.call(service)
			if err != nil {
				t.Fatalf("导入登录返回错误：%v", err)
			}
			if result.Mode != "mock" || result.UUID == "" || result.CacheKey != "login:mock:"+result.UUID {
				t.Fatalf("导入结果 = %+v，期望稳定 mock uuid/cache_key", result)
			}
			if result.DeviceID != tc.wantDeviceID || result.DeviceName != tc.wantDeviceName || result.Type != tc.wantType || result.Wxid != tc.wantWxid {
				t.Fatalf("导入设备字段 = %+v，期望保留请求上下文", result)
			}
			if result.LoginKind != tc.wantLoginKind {
				t.Fatalf("login_kind = %s，期望 %s", result.LoginKind, tc.wantLoginKind)
			}
			if result.Protocol["pack_kind"] != tc.wantPackKind || result.Protocol["operation"] != tc.wantOperation || result.Protocol["platform"] != tc.wantPlatform {
				t.Fatalf("protocol = %+v，期望对应平台 Hybrid 摘要", result.Protocol)
			}
			if result.Network["mode"] != "mock" || result.Network["operation"] != tc.wantOperation || result.Network["login_kind"] != tc.wantLoginKind || result.Network["platform"] != tc.wantPlatform {
				t.Fatalf("network = %+v，期望 mock 导入网络摘要", result.Network)
			}
			if !containsString(result.Stages, tc.wantStage) || result.Stages[len(result.Stages)-1] != "write_sample" {
				t.Fatalf("stages = %+v，期望包含 %s 并写入样本", result.Stages, tc.wantStage)
			}
			if result.State.UUID != result.UUID || result.State.Wxid != tc.wantWxid || result.State.LoginKind != tc.wantLoginKind || !result.State.CreatedAt.Equal(fixedNow) {
				t.Fatalf("state = %+v，期望保存导入登录态", result.State)
			}
			if tc.wantRequestField == "data62" && result.State.Data62 != tc.wantRequestValue {
				t.Fatalf("state.Data62 = %q，期望导入 62 数据", result.State.Data62)
			}
			if tc.wantRequestField == "a16" && result.State.A16 != tc.wantRequestValue {
				t.Fatalf("state.A16 = %q，期望导入 A16 数据", result.State.A16)
			}

			stored, ok, err := store.GetByWxid(context.Background(), tc.wantWxid)
			if err != nil || !ok {
				t.Fatalf("按 wxid 读取登录态 = %+v / %v / %v，期望业务层已保存", stored, ok, err)
			}
			if stored.UUID != result.UUID || stored.Protocol["pack_kind"] != tc.wantPackKind {
				t.Fatalf("按 wxid 读取登录态 = %+v，期望包含同一 uuid 与协议摘要", stored)
			}

			raw, err := os.ReadFile(result.SamplePath)
			if err != nil {
				t.Fatalf("读取样本失败：%v", err)
			}
			var sample map[string]any
			if err := json.Unmarshal(raw, &sample); err != nil {
				t.Fatalf("样本不是 JSON：%v", err)
			}
			for _, key := range []string{"request", "protocol", "network", "mock_response", "login_state"} {
				if _, ok := sample[key]; !ok {
					t.Fatalf("样本缺少字段 %s：%+v", key, sample)
				}
			}
			request := sample["request"].(map[string]any)
			if request[tc.wantRequestField] != tc.wantRequestValue || request["wxid"] != tc.wantWxid || request["type"] != tc.wantType {
				t.Fatalf("样本 request = %+v，期望落盘导入请求", request)
			}
			sampleNetwork := sample["network"].(map[string]any)
			if sampleNetwork["mode"] != "mock" || sampleNetwork["operation"] != tc.wantOperation {
				t.Fatalf("样本 network = %+v，期望落盘网络摘要", sampleNetwork)
			}

			responseData := result.ResponseData()
			for _, key := range []string{"mode", "uuid", "cache_key", "device_id", "device_name", "type", "protocol", "network", "login_state", "sample_path", "stages", "status", "wxid"} {
				if _, ok := responseData[key]; !ok {
					t.Fatalf("响应缺少字段 %s：%+v", key, responseData)
				}
			}
			if responseData["status"] != "mock_login_ready" || responseData["wxid"] != tc.wantWxid {
				t.Fatalf("响应 mock 字段 = %+v，期望保留 status/wxid", responseData)
			}
		})
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
