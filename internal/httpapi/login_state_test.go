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

func TestLoginCheckQRMockPathReadsAndUpdatesGetQRState(t *testing.T) {
	cfg := config.Default()
	cfg.SampleDir = t.TempDir()
	h := httpapi.NewServer(cfg)

	qr := postJSON(t, h, "/Login/GetQR", `{"DeviceID":"qr-dev-001","DeviceName":"扫码样本设备","Type":"ipad"}`)
	if !qr.Success || qr.Code != "ok" {
		t.Fatalf("GetQR 响应 = %+v，期望 ok", qr)
	}
	qrData := mustMap(t, qr.Data)
	uuid := mustString(t, qrData, "uuid")
	cacheKey := mustString(t, qrData, "cache_key")

	missingUUID := postJSON(t, h, "/Login/CheckQR", `{}`)
	if missingUUID.Success || missingUUID.Code != "param_error" {
		t.Fatalf("缺少 uuid 响应 = %+v，期望 param_error", missingUUID)
	}

	notFound := postJSON(t, h, "/Login/CheckQR?uuid=mock-not-exists", `{}`)
	if notFound.Success || notFound.Code != "cache_not_found" {
		t.Fatalf("不存在 uuid 响应 = %+v，期望 cache_not_found", notFound)
	}

	check := postJSON(t, h, "/Login/CheckQR?uuid="+uuid, `{}`)
	if !check.Success || check.Code != "ok" {
		t.Fatalf("CheckQR 响应 = %+v，期望 ok", check)
	}
	data := mustMap(t, check.Data)
	if data["uuid"] != uuid || data["cache_key"] != cacheKey || data["qr_status"] != "waiting_scan" {
		t.Fatalf("CheckQR data = %+v，期望包含本次 uuid/cache_key/waiting_scan", data)
	}
	if _, ok := data["stages"].([]any); !ok {
		t.Fatalf("CheckQR stages = %#v，期望数组", data["stages"])
	}
	samplePath := mustString(t, data, "sample_path")
	loginState := mustMap(t, data["login_state"])
	if loginState["uuid"] != uuid || loginState["login_kind"] != "getqr_mock" || loginState["qr_status"] != "waiting_scan" {
		t.Fatalf("CheckQR login_state = %+v，期望包含 getqr_mock waiting_scan 状态", loginState)
	}

	sampleRaw, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatalf("读取 CheckQR 样本失败：%v", err)
	}
	var sample map[string]any
	if err := json.Unmarshal(sampleRaw, &sample); err != nil {
		t.Fatalf("CheckQR 样本不是 JSON：%v", err)
	}
	for _, key := range []string{"request", "mock_response", "login_state"} {
		if _, ok := sample[key]; !ok {
			t.Fatalf("CheckQR 样本缺少字段 %s：%+v", key, sample)
		}
	}
	request := mustMap(t, sample["request"])
	if request["uuid"] != uuid {
		t.Fatalf("CheckQR 样本 request = %+v，期望记录 uuid", request)
	}
	mockResponse := mustMap(t, sample["mock_response"])
	if mockResponse["qr_status"] != "waiting_scan" {
		t.Fatalf("CheckQR 样本 mock_response = %+v，期望 waiting_scan", mockResponse)
	}

	byUUID := postJSON(t, h, "/Login/GetCacheInfo?uuid="+uuid, `{}`)
	if !byUUID.Success || byUUID.Code != "ok" {
		t.Fatalf("按 uuid 查询响应 = %+v，期望 ok", byUUID)
	}
	stateByUUID := mustMap(t, byUUID.Data)
	if stateByUUID["uuid"] != uuid || stateByUUID["qr_status"] != "waiting_scan" || stateByUUID["sample_path"] != samplePath {
		t.Fatalf("按 uuid 查询结果 = %+v，期望反映最近一次 CheckQR 状态", stateByUUID)
	}
}

func TestLoginNewinitAndHeartBeatMockPathsUpdateWxidState(t *testing.T) {
	cfg := config.Default()
	cfg.SampleDir = t.TempDir()
	h := httpapi.NewServer(cfg)

	missingNewinit := postJSON(t, h, "/Login/Newinit", `{}`)
	if missingNewinit.Success || missingNewinit.Code != "param_error" {
		t.Fatalf("Newinit 缺少 wxid 响应 = %+v，期望 param_error", missingNewinit)
	}
	missingHeartBeat := postJSON(t, h, "/Login/HeartBeat", `{}`)
	if missingHeartBeat.Success || missingHeartBeat.Code != "param_error" {
		t.Fatalf("HeartBeat 缺少 wxid 响应 = %+v，期望 param_error", missingHeartBeat)
	}
	notFound := postJSON(t, h, "/Login/Newinit?wxid=wxid_not_exists", `{}`)
	if notFound.Success || notFound.Code != "cache_not_found" {
		t.Fatalf("不存在 wxid 响应 = %+v，期望 cache_not_found", notFound)
	}
	heartBeatNotFound := postJSON(t, h, "/Login/HeartBeat?wxid=wxid_not_exists", `{}`)
	if heartBeatNotFound.Success || heartBeatNotFound.Code != "cache_not_found" {
		t.Fatalf("HeartBeat 不存在 wxid 响应 = %+v，期望 cache_not_found", heartBeatNotFound)
	}

	login := postJSON(t, h, "/Login/A16Data", `{"A16":"mock-a16-post-login","DeviceID":"android-post-login","DeviceName":"登录后样本设备","Wxid":"wxid_post_login"}`)
	if !login.Success || login.Code != "ok" {
		t.Fatalf("A16Data 响应 = %+v，期望 ok", login)
	}
	loginData := mustMap(t, login.Data)
	uuid := mustString(t, loginData, "uuid")

	init := postJSON(t, h, "/Login/Newinit?wxid=wxid_post_login&MaxSynckey=max-key-1&CurrentSynckey=current-key-1", `{}`)
	if !init.Success || init.Code != "ok" {
		t.Fatalf("Newinit 响应 = %+v，期望 ok", init)
	}
	initData := mustMap(t, init.Data)
	if initData["wxid"] != "wxid_post_login" || initData["session_state"] != "initialized" {
		t.Fatalf("Newinit data = %+v，期望 initialized", initData)
	}
	if _, ok := initData["stages"].([]any); !ok {
		t.Fatalf("Newinit stages = %#v，期望数组", initData["stages"])
	}
	initState := mustMap(t, initData["login_state"])
	if initState["uuid"] != uuid || initState["wxid"] != "wxid_post_login" || initState["session_state"] != "initialized" {
		t.Fatalf("Newinit login_state = %+v，期望包含 uuid/wxid/initialized", initState)
	}
	initSamplePath := mustString(t, initData, "sample_path")
	assertLoginStageSample(t, initSamplePath, "wxid_post_login", "initialized", "")

	beat := postJSON(t, h, "/Login/HeartBeat?wxid=wxid_post_login", `{}`)
	if !beat.Success || beat.Code != "ok" {
		t.Fatalf("HeartBeat 响应 = %+v，期望 ok", beat)
	}
	beatData := mustMap(t, beat.Data)
	if beatData["wxid"] != "wxid_post_login" || beatData["heartbeat_status"] != "alive" || beatData["heartbeat_count"] != float64(1) {
		t.Fatalf("HeartBeat data = %+v，期望 alive 且 heartbeat_count=1", beatData)
	}
	if _, ok := beatData["stages"].([]any); !ok {
		t.Fatalf("HeartBeat stages = %#v，期望数组", beatData["stages"])
	}
	beatState := mustMap(t, beatData["login_state"])
	if beatState["uuid"] != uuid || beatState["session_state"] != "initialized" || beatState["heartbeat_status"] != "alive" {
		t.Fatalf("HeartBeat login_state = %+v，期望保留 initialized 并进入 alive", beatState)
	}
	beatSamplePath := mustString(t, beatData, "sample_path")
	assertLoginStageSample(t, beatSamplePath, "wxid_post_login", "", "alive")

	byUUID := postJSON(t, h, "/Login/GetCacheInfo?uuid="+uuid, `{}`)
	if !byUUID.Success || byUUID.Code != "ok" {
		t.Fatalf("按 uuid 查询响应 = %+v，期望 ok", byUUID)
	}
	stateByUUID := mustMap(t, byUUID.Data)
	if stateByUUID["wxid"] != "wxid_post_login" || stateByUUID["session_state"] != "initialized" || stateByUUID["heartbeat_status"] != "alive" || stateByUUID["sample_path"] != beatSamplePath {
		t.Fatalf("按 uuid 查询结果 = %+v，期望反映最近一次 HeartBeat 状态", stateByUUID)
	}
}

func assertLoginStageSample(t *testing.T, path string, wxid string, sessionState string, heartbeatStatus string) {
	t.Helper()
	sampleRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取登录阶段样本失败：%v", err)
	}
	var sample map[string]any
	if err := json.Unmarshal(sampleRaw, &sample); err != nil {
		t.Fatalf("登录阶段样本不是 JSON：%v", err)
	}
	for _, key := range []string{"request", "mock_response", "login_state"} {
		if _, ok := sample[key]; !ok {
			t.Fatalf("登录阶段样本缺少字段 %s：%+v", key, sample)
		}
	}
	request := mustMap(t, sample["request"])
	if request["wxid"] != wxid {
		t.Fatalf("登录阶段样本 request = %+v，期望 wxid=%s", request, wxid)
	}
	mockResponse := mustMap(t, sample["mock_response"])
	if sessionState != "" && mockResponse["session_state"] != sessionState {
		t.Fatalf("登录阶段样本 mock_response = %+v，期望 session_state=%s", mockResponse, sessionState)
	}
	if heartbeatStatus != "" && mockResponse["heartbeat_status"] != heartbeatStatus {
		t.Fatalf("登录阶段样本 mock_response = %+v，期望 heartbeat_status=%s", mockResponse, heartbeatStatus)
	}
}
