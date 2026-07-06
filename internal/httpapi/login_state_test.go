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
	if mustString(t, protocol, "packed_hex") == "" {
		t.Fatalf("protocol.packed_hex 不能为空：%+v", protocol)
	}
	debug := mustMap(t, protocol["debug"])
	if debug["magic"] != "CBS1" || debug["payload_length"] == nil {
		t.Fatalf("protocol.debug = %+v，期望包含 CBS1 mock 帧摘要", debug)
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
			if mustString(t, protocol, "packed_hex") == "" {
				t.Fatalf("protocol.packed_hex 不能为空：%+v", protocol)
			}
			debug := mustMap(t, protocol["debug"])
			if debug["magic"] != "CBS1" || debug["payload_length"] == nil {
				t.Fatalf("protocol.debug = %+v，期望包含 CBS1 mock 帧摘要", debug)
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

func TestLoginExport62DataAndA16DataMockPathsUseWxidState(t *testing.T) {
	cfg := config.Default()
	cfg.SampleDir = t.TempDir()
	h := httpapi.NewServer(cfg)

	missing62 := postJSON(t, h, "/Login/Get62Data", `{}`)
	if missing62.Success || missing62.Code != "param_error" {
		t.Fatalf("Get62Data 缺少 wxid 响应 = %+v，期望 param_error", missing62)
	}
	missingA16 := postJSON(t, h, "/Login/GetA16Data", `{}`)
	if missingA16.Success || missingA16.Code != "param_error" {
		t.Fatalf("GetA16Data 缺少 wxid 响应 = %+v，期望 param_error", missingA16)
	}
	notFound := postJSON(t, h, "/Login/Get62Data?wxid=wxid_not_exists", `{}`)
	if notFound.Success || notFound.Code != "cache_not_found" {
		t.Fatalf("不存在 wxid 响应 = %+v，期望 cache_not_found", notFound)
	}

	login62 := postJSON(t, h, "/Login/62data", `{"Data62":"mock-62-export-source","DeviceID":"iphone-export","DeviceName":"62导出设备","Wxid":"wxid_export_62"}`)
	if !login62.Success || login62.Code != "ok" {
		t.Fatalf("62data 响应 = %+v，期望 ok", login62)
	}
	login62Data := mustMap(t, login62.Data)
	uuid62 := mustString(t, login62Data, "uuid")

	loginA16 := postJSON(t, h, "/Login/A16Data", `{"A16":"mock-a16-export-source","DeviceID":"android-export","DeviceName":"A16导出设备","Wxid":"wxid_export_a16"}`)
	if !loginA16.Success || loginA16.Code != "ok" {
		t.Fatalf("A16Data 响应 = %+v，期望 ok", loginA16)
	}
	loginA16Data := mustMap(t, loginA16.Data)
	uuidA16 := mustString(t, loginA16Data, "uuid")

	wrongA16 := postJSON(t, h, "/Login/GetA16Data?wxid=wxid_export_62", `{}`)
	if wrongA16.Success || wrongA16.Code != "unsupported_login_kind" {
		t.Fatalf("62 登录态导出 A16 响应 = %+v，期望 unsupported_login_kind", wrongA16)
	}
	wrong62 := postJSON(t, h, "/Login/Get62Data?wxid=wxid_export_a16", `{}`)
	if wrong62.Success || wrong62.Code != "unsupported_login_kind" {
		t.Fatalf("A16 登录态导出 62 响应 = %+v，期望 unsupported_login_kind", wrong62)
	}

	export62 := postJSON(t, h, "/Login/Get62Data?wxid=wxid_export_62", `{}`)
	if !export62.Success || export62.Code != "ok" {
		t.Fatalf("Get62Data 响应 = %+v，期望 ok", export62)
	}
	export62Data := mustMap(t, export62.Data)
	if export62Data["wxid"] != "wxid_export_62" || export62Data["export_kind"] != "mock_62data" || export62Data["data62"] != "mock-62-export-source" {
		t.Fatalf("Get62Data data = %+v，期望导出 62 数据", export62Data)
	}
	if _, ok := export62Data["stages"].([]any); !ok {
		t.Fatalf("Get62Data stages = %#v，期望数组", export62Data["stages"])
	}
	export62State := mustMap(t, export62Data["login_state"])
	if export62State["uuid"] != uuid62 || export62State["last_export_kind"] != "mock_62data" {
		t.Fatalf("Get62Data login_state = %+v，期望记录 last_export_kind", export62State)
	}
	export62SamplePath := mustString(t, export62Data, "sample_path")
	assertExportSample(t, export62SamplePath, "wxid_export_62", "mock_62data")

	exportA16 := postJSON(t, h, "/Login/GetA16Data?wxid=wxid_export_a16", `{}`)
	if !exportA16.Success || exportA16.Code != "ok" {
		t.Fatalf("GetA16Data 响应 = %+v，期望 ok", exportA16)
	}
	exportA16Data := mustMap(t, exportA16.Data)
	if exportA16Data["wxid"] != "wxid_export_a16" || exportA16Data["export_kind"] != "mock_a16data" || exportA16Data["a16"] != "mock-a16-export-source" {
		t.Fatalf("GetA16Data data = %+v，期望导出 A16 数据", exportA16Data)
	}
	if _, ok := exportA16Data["stages"].([]any); !ok {
		t.Fatalf("GetA16Data stages = %#v，期望数组", exportA16Data["stages"])
	}
	exportA16State := mustMap(t, exportA16Data["login_state"])
	if exportA16State["uuid"] != uuidA16 || exportA16State["last_export_kind"] != "mock_a16data" {
		t.Fatalf("GetA16Data login_state = %+v，期望记录 last_export_kind", exportA16State)
	}
	exportA16SamplePath := mustString(t, exportA16Data, "sample_path")
	assertExportSample(t, exportA16SamplePath, "wxid_export_a16", "mock_a16data")

	byUUID62 := postJSON(t, h, "/Login/GetCacheInfo?uuid="+uuid62, `{}`)
	if !byUUID62.Success || byUUID62.Code != "ok" {
		t.Fatalf("按 uuid 查询 62 响应 = %+v，期望 ok", byUUID62)
	}
	state62 := mustMap(t, byUUID62.Data)
	if state62["last_export_kind"] != "mock_62data" || state62["sample_path"] != export62SamplePath {
		t.Fatalf("按 uuid 查询 62 结果 = %+v，期望反映最近导出状态", state62)
	}

	byUUIDA16 := postJSON(t, h, "/Login/GetCacheInfo?uuid="+uuidA16, `{}`)
	if !byUUIDA16.Success || byUUIDA16.Code != "ok" {
		t.Fatalf("按 uuid 查询 A16 响应 = %+v，期望 ok", byUUIDA16)
	}
	stateA16 := mustMap(t, byUUIDA16.Data)
	if stateA16["last_export_kind"] != "mock_a16data" || stateA16["sample_path"] != exportA16SamplePath {
		t.Fatalf("按 uuid 查询 A16 结果 = %+v，期望反映最近导出状态", stateA16)
	}
}

func assertExportSample(t *testing.T, path string, wxid string, exportKind string) {
	t.Helper()
	sampleRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取导出样本失败：%v", err)
	}
	var sample map[string]any
	if err := json.Unmarshal(sampleRaw, &sample); err != nil {
		t.Fatalf("导出样本不是 JSON：%v", err)
	}
	for _, key := range []string{"request", "mock_response", "login_state"} {
		if _, ok := sample[key]; !ok {
			t.Fatalf("导出样本缺少字段 %s：%+v", key, sample)
		}
	}
	request := mustMap(t, sample["request"])
	if request["wxid"] != wxid {
		t.Fatalf("导出样本 request = %+v，期望 wxid=%s", request, wxid)
	}
	mockResponse := mustMap(t, sample["mock_response"])
	if mockResponse["export_kind"] != exportKind {
		t.Fatalf("导出样本 mock_response = %+v，期望 export_kind=%s", mockResponse, exportKind)
	}
}

func TestLoginLogOutMockPathMarksWxidStateLoggedOut(t *testing.T) {
	cfg := config.Default()
	cfg.SampleDir = t.TempDir()
	h := httpapi.NewServer(cfg)

	missing := postJSON(t, h, "/Login/LogOut", `{}`)
	if missing.Success || missing.Code != "param_error" {
		t.Fatalf("LogOut 缺少 wxid 响应 = %+v，期望 param_error", missing)
	}
	notFound := postJSON(t, h, "/Login/LogOut?wxid=wxid_not_exists", `{}`)
	if notFound.Success || notFound.Code != "cache_not_found" {
		t.Fatalf("LogOut 不存在 wxid 响应 = %+v，期望 cache_not_found", notFound)
	}

	login := postJSON(t, h, "/Login/A16Data", `{"A16":"mock-a16-logout","DeviceID":"android-logout","DeviceName":"退出样本设备","Wxid":"wxid_logout"}`)
	if !login.Success || login.Code != "ok" {
		t.Fatalf("A16Data 响应 = %+v，期望 ok", login)
	}
	loginData := mustMap(t, login.Data)
	uuid := mustString(t, loginData, "uuid")

	beatBefore := postJSON(t, h, "/Login/HeartBeat?wxid=wxid_logout", `{}`)
	if !beatBefore.Success || beatBefore.Code != "ok" {
		t.Fatalf("退出前 HeartBeat 响应 = %+v，期望 ok", beatBefore)
	}

	logout := postJSON(t, h, "/Login/LogOut?wxid=wxid_logout", `{}`)
	if !logout.Success || logout.Code != "ok" {
		t.Fatalf("LogOut 响应 = %+v，期望 ok", logout)
	}
	data := mustMap(t, logout.Data)
	if data["wxid"] != "wxid_logout" || data["logout_status"] != "logged_out" {
		t.Fatalf("LogOut data = %+v，期望 logged_out", data)
	}
	if _, ok := data["stages"].([]any); !ok {
		t.Fatalf("LogOut stages = %#v，期望数组", data["stages"])
	}
	state := mustMap(t, data["login_state"])
	if state["uuid"] != uuid || state["session_state"] != "logged_out" || state["logout_status"] != "logged_out" {
		t.Fatalf("LogOut login_state = %+v，期望记录退出状态", state)
	}
	samplePath := mustString(t, data, "sample_path")
	assertLogoutSample(t, samplePath, "wxid_logout")

	byUUID := postJSON(t, h, "/Login/GetCacheInfo?uuid="+uuid, `{}`)
	if !byUUID.Success || byUUID.Code != "ok" {
		t.Fatalf("按 uuid 查询响应 = %+v，期望 ok", byUUID)
	}
	stateByUUID := mustMap(t, byUUID.Data)
	if stateByUUID["session_state"] != "logged_out" || stateByUUID["logout_status"] != "logged_out" || stateByUUID["sample_path"] != samplePath {
		t.Fatalf("按 uuid 查询结果 = %+v，期望反映最近退出状态", stateByUUID)
	}
	if _, ok := stateByUUID["logged_out_at"].(string); !ok {
		t.Fatalf("按 uuid 查询 logged_out_at = %#v，期望时间字符串", stateByUUID["logged_out_at"])
	}

	beatAfter := postJSON(t, h, "/Login/HeartBeat?wxid=wxid_logout", `{}`)
	if beatAfter.Success || beatAfter.Code != "session_logged_out" {
		t.Fatalf("退出后 HeartBeat 响应 = %+v，期望 session_logged_out", beatAfter)
	}
	afterData := mustMap(t, beatAfter.Data)
	afterState := mustMap(t, afterData["login_state"])
	if afterState["heartbeat_count"] != float64(1) || afterState["session_state"] != "logged_out" {
		t.Fatalf("退出后 HeartBeat login_state = %+v，期望不增加心跳并保持 logged_out", afterState)
	}
}

func assertLogoutSample(t *testing.T, path string, wxid string) {
	t.Helper()
	sampleRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取退出样本失败：%v", err)
	}
	var sample map[string]any
	if err := json.Unmarshal(sampleRaw, &sample); err != nil {
		t.Fatalf("退出样本不是 JSON：%v", err)
	}
	for _, key := range []string{"request", "mock_response", "login_state"} {
		if _, ok := sample[key]; !ok {
			t.Fatalf("退出样本缺少字段 %s：%+v", key, sample)
		}
	}
	request := mustMap(t, sample["request"])
	if request["wxid"] != wxid {
		t.Fatalf("退出样本 request = %+v，期望 wxid=%s", request, wxid)
	}
	mockResponse := mustMap(t, sample["mock_response"])
	if mockResponse["logout_status"] != "logged_out" {
		t.Fatalf("退出样本 mock_response = %+v，期望 logged_out", mockResponse)
	}
}
