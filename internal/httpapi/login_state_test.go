package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mahiro424/cbs/internal/config"
	"github.com/mahiro424/cbs/internal/httpapi"
)

func TestLoginGetQRPersistsStateAndSampleThenCacheInfoCanReadIt(t *testing.T) {
	cfg := config.Default()
	cfg.SampleDir = t.TempDir()
	h := httpapi.NewServer(cfg)

	payload := `{"DeviceID":"dev-002","DeviceName":"样本设备","Type":"ipad"}`
	qr := postJSON(t, h, "/Login/GetQR", payload)
	if !qr.Success || qr.Code != "ok" {
		t.Fatalf("GetQR 响应 = %+v，期望 ok", qr)
	}
	data := mustMap(t, qr.Data)
	uuid := mustString(t, data, "uuid")
	cacheKey := mustString(t, data, "cache_key")
	samplePath := mustString(t, data, "sample_path")
	if _, err := os.Stat(samplePath); err != nil {
		t.Fatalf("样本文件不存在：%s，错误：%v", samplePath, err)
	}
	protocol := mustMap(t, data["protocol"])
	if protocol["pack_kind"] != "hybrid_ecdh_ios_placeholder" {
		t.Fatalf("protocol.pack_kind = %v，期望 hybrid_ecdh_ios_placeholder", protocol["pack_kind"])
	}
	loginState := mustMap(t, data["login_state"])
	if loginState["cache_key"] != cacheKey || loginState["uuid"] != uuid {
		t.Fatalf("login_state = %+v，期望包含本次 uuid/cache_key", loginState)
	}

	sampleRaw, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatalf("读取样本失败：%v", err)
	}
	var sample map[string]any
	if err := json.Unmarshal(sampleRaw, &sample); err != nil {
		t.Fatalf("样本不是 JSON：%v", err)
	}
	for _, key := range []string{"request", "protocol", "mock_response", "login_state"} {
		if _, ok := sample[key]; !ok {
			t.Fatalf("样本缺少字段 %s：%+v", key, sample)
		}
	}

	byUUID := postJSON(t, h, "/Login/GetCacheInfo?uuid="+uuid, `{}`)
	if !byUUID.Success || byUUID.Code != "ok" {
		t.Fatalf("按 uuid 查询响应 = %+v，期望 ok", byUUID)
	}
	stateByUUID := mustMap(t, byUUID.Data)
	if stateByUUID["uuid"] != uuid || stateByUUID["cache_key"] != cacheKey {
		t.Fatalf("按 uuid 查询结果 = %+v，期望本次登录态", stateByUUID)
	}

	byCacheKey := postJSON(t, h, "/Login/GetCacheInfo?cache_key="+cacheKey, `{}`)
	if !byCacheKey.Success || byCacheKey.Code != "ok" {
		t.Fatalf("按 cache_key 查询响应 = %+v，期望 ok", byCacheKey)
	}
	stateByCacheKey := mustMap(t, byCacheKey.Data)
	if stateByCacheKey["uuid"] != uuid || stateByCacheKey["sample_path"] != samplePath {
		t.Fatalf("按 cache_key 查询结果 = %+v，期望本次登录态和样本路径", stateByCacheKey)
	}
}

func postJSON(t *testing.T, h *httpapi.Server, path string, payload string) httpapi.Envelope {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body httpapi.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v，正文：%s", err, rec.Body.String())
	}
	return body
}

func mustMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("值类型 = %T，期望对象：%+v", v, v)
	}
	return m
}

func mustString(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	v, ok := m[key].(string)
	if !ok || v == "" {
		t.Fatalf("字段 %s = %#v，期望非空字符串", key, m[key])
	}
	return v
}

func TestLoginData62AndA16MockPathsPersistStateAndSamples(t *testing.T) {
	cfg := config.Default()
	cfg.SampleDir = t.TempDir()
	h := httpapi.NewServer(cfg)

	cases := []struct {
		name       string
		path       string
		payload    string
		packKind   string
		loginKind  string
		platform   string
		deviceName string
	}{
		{
			name:       "62 数据登录",
			path:       "/Login/62data",
			payload:    `{"Data62":"mock-62-data","DeviceID":"iphone-001","DeviceName":"62样本设备","Wxid":"wxid_62"}`,
			packKind:   "hybrid_ecdh_ios_placeholder",
			loginKind:  "data62_mock",
			platform:   "ios",
			deviceName: "62样本设备",
		},
		{
			name:       "A16 数据登录",
			path:       "/Login/A16Data",
			payload:    `{"A16":"mock-a16-data","DeviceID":"android-001","DeviceName":"A16样本设备","Wxid":"wxid_a16"}`,
			packKind:   "hybrid_ecdh_android_placeholder",
			loginKind:  "a16_mock",
			platform:   "android",
			deviceName: "A16样本设备",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postJSON(t, h, tc.path, tc.payload)
			if !resp.Success || resp.Code != "ok" {
				t.Fatalf("%s 响应 = %+v，期望 ok", tc.path, resp)
			}
			data := mustMap(t, resp.Data)
			uuid := mustString(t, data, "uuid")
			cacheKey := mustString(t, data, "cache_key")
			samplePath := mustString(t, data, "sample_path")
			protocol := mustMap(t, data["protocol"])
			if protocol["pack_kind"] != tc.packKind || protocol["platform"] != tc.platform || protocol["login_kind"] != tc.loginKind {
				t.Fatalf("protocol = %+v，期望 pack/platform/login kind 匹配", protocol)
			}
			loginState := mustMap(t, data["login_state"])
			if loginState["uuid"] != uuid || loginState["cache_key"] != cacheKey || loginState["login_kind"] != tc.loginKind {
				t.Fatalf("login_state = %+v，期望包含本次 uuid/cache_key/login_kind", loginState)
			}

			sampleRaw, err := os.ReadFile(samplePath)
			if err != nil {
				t.Fatalf("读取样本失败：%v", err)
			}
			var sample map[string]any
			if err := json.Unmarshal(sampleRaw, &sample); err != nil {
				t.Fatalf("样本不是 JSON：%v", err)
			}
			for _, key := range []string{"request", "protocol", "mock_response", "login_state"} {
				if _, ok := sample[key]; !ok {
					t.Fatalf("样本缺少字段 %s：%+v", key, sample)
				}
			}
			request := mustMap(t, sample["request"])
			if request["device_name"] != tc.deviceName {
				t.Fatalf("样本 request.device_name = %v，期望 %s", request["device_name"], tc.deviceName)
			}

			byUUID := postJSON(t, h, "/Login/GetCacheInfo?uuid="+uuid, `{}`)
			if !byUUID.Success || byUUID.Code != "ok" {
				t.Fatalf("按 uuid 查询响应 = %+v，期望 ok", byUUID)
			}
			stateByUUID := mustMap(t, byUUID.Data)
			if stateByUUID["uuid"] != uuid || stateByUUID["cache_key"] != cacheKey || stateByUUID["login_kind"] != tc.loginKind {
				t.Fatalf("按 uuid 查询结果 = %+v，期望本次登录态", stateByUUID)
			}

			byCacheKey := postJSON(t, h, "/Login/GetCacheInfo?cache_key="+cacheKey, `{}`)
			if !byCacheKey.Success || byCacheKey.Code != "ok" {
				t.Fatalf("按 cache_key 查询响应 = %+v，期望 ok", byCacheKey)
			}
			stateByCacheKey := mustMap(t, byCacheKey.Data)
			if stateByCacheKey["uuid"] != uuid || stateByCacheKey["sample_path"] != samplePath {
				t.Fatalf("按 cache_key 查询结果 = %+v，期望本次登录态和样本路径", stateByCacheKey)
			}
		})
	}
}
