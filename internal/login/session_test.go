package login_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/mahiro424/cbs/internal/login"
	"github.com/mahiro424/cbs/internal/network"
	"github.com/mahiro424/cbs/internal/storage"
)

func TestServiceNewinitAndHeartBeatUpdateWxidSessionStateSamplesAndStore(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	client, _, err := network.NewClient(network.Config{Mode: "mock"})
	if err != nil {
		t.Fatalf("创建 mock network client 失败：%v", err)
	}
	currentTime := time.Date(2026, 7, 7, 0, 1, 0, 0, time.UTC)
	service := login.NewService(login.Dependencies{
		States:    store,
		Network:   client,
		SampleDir: t.TempDir(),
		Now:       func() time.Time { return currentTime },
	})

	imported, err := service.ImportA16Data(context.Background(), login.ImportA16DataRequest{
		A16:        "session-a16-data",
		DeviceID:   "session-android-001",
		DeviceName: "业务层会话设备",
		Wxid:       "wxid_session_service",
	})
	if err != nil {
		t.Fatalf("导入 A16 登录态失败：%v", err)
	}

	initAt := time.Date(2026, 7, 7, 0, 2, 3, 456789000, time.UTC)
	currentTime = initAt
	initResult, err := service.Newinit(context.Background(), login.NewinitRequest{
		Wxid:           "wxid_session_service",
		MaxSyncKey:     "max-key-service",
		CurrentSyncKey: "current-key-service",
	})
	if err != nil {
		t.Fatalf("Newinit 返回错误：%v", err)
	}
	if initResult.Mode != "mock" || initResult.UUID != imported.UUID || initResult.Wxid != "wxid_session_service" || initResult.SessionState != "initialized" {
		t.Fatalf("Newinit result = %+v，期望初始化同一 wxid 登录态", initResult)
	}
	if !initResult.LastInitAt.Equal(initAt) || initResult.State.SessionState != "initialized" || !initResult.State.LastInitAt.Equal(initAt) {
		t.Fatalf("Newinit 时间/状态 = %+v，期望写入 last_init_at", initResult.State)
	}
	if !containsString(initResult.Stages, "mock_newinit_sync") || initResult.Stages[len(initResult.Stages)-1] != "write_sample" {
		t.Fatalf("Newinit stages = %+v，期望记录初始化阶段", initResult.Stages)
	}
	assertSessionSample(t, initResult.SamplePath, "wxid_session_service", "initialized", "", map[string]any{
		"max_synckey":     "max-key-service",
		"current_synckey": "current-key-service",
	})
	initResponse := initResult.ResponseData()
	for _, key := range []string{"mode", "uuid", "cache_key", "wxid", "session_state", "max_synckey", "current_synckey", "initialized_at", "mock_sync_status", "login_state", "sample_path", "stages"} {
		if _, ok := initResponse[key]; !ok {
			t.Fatalf("Newinit 响应缺少字段 %s：%+v", key, initResponse)
		}
	}

	beatAt := time.Date(2026, 7, 7, 0, 3, 4, 567890000, time.UTC)
	currentTime = beatAt
	beatResult, err := service.HeartBeat(context.Background(), login.HeartBeatRequest{Wxid: "wxid_session_service"})
	if err != nil {
		t.Fatalf("HeartBeat 返回错误：%v", err)
	}
	if beatResult.UUID != imported.UUID || beatResult.Wxid != "wxid_session_service" || beatResult.HeartbeatStatus != "alive" || beatResult.HeartbeatCount != 1 {
		t.Fatalf("HeartBeat result = %+v，期望 alive 且 heartbeat_count=1", beatResult)
	}
	if beatResult.State.SessionState != "initialized" || beatResult.State.HeartbeatStatus != "alive" || beatResult.State.HeartbeatCount != 1 || !beatResult.State.LastHeartbeatAt.Equal(beatAt) {
		t.Fatalf("HeartBeat 登录态 = %+v，期望保留 initialized 并记录心跳", beatResult.State)
	}
	if !containsString(beatResult.Stages, "mock_short_heartbeat") || beatResult.Stages[len(beatResult.Stages)-1] != "write_sample" {
		t.Fatalf("HeartBeat stages = %+v，期望记录短心跳阶段", beatResult.Stages)
	}
	assertSessionSample(t, beatResult.SamplePath, "wxid_session_service", "", "alive", nil)

	stored, ok, err := store.Get(context.Background(), imported.UUID, "")
	if err != nil || !ok {
		t.Fatalf("按 uuid 读取会话登录态 = %+v / %v / %v，期望已保存", stored, ok, err)
	}
	if stored.SamplePath != beatResult.SamplePath || stored.SessionState != "initialized" || stored.HeartbeatStatus != "alive" || stored.HeartbeatCount != 1 {
		t.Fatalf("存储登录态 = %+v，期望反映最近一次 HeartBeat", stored)
	}
	beatResponse := beatResult.ResponseData()
	for _, key := range []string{"mode", "uuid", "cache_key", "wxid", "heartbeat_status", "heartbeat_count", "heartbeat_at", "login_state", "sample_path", "stages"} {
		if _, ok := beatResponse[key]; !ok {
			t.Fatalf("HeartBeat 响应缺少字段 %s：%+v", key, beatResponse)
		}
	}
}

func TestServiceSessionReportsMissingAndLoggedOutState(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	client, _, err := network.NewClient(network.Config{Mode: "mock"})
	if err != nil {
		t.Fatalf("创建 mock network client 失败：%v", err)
	}
	service := login.NewService(login.Dependencies{States: store, Network: client, SampleDir: t.TempDir()})

	_, err = service.Newinit(context.Background(), login.NewinitRequest{Wxid: "wxid_missing_session"})
	if !errors.Is(err, login.ErrWxidLoginStateNotFound) {
		t.Fatalf("Newinit 不存在 wxid 错误 = %v，期望 ErrWxidLoginStateNotFound", err)
	}
	_, err = service.HeartBeat(context.Background(), login.HeartBeatRequest{Wxid: "wxid_missing_session"})
	if !errors.Is(err, login.ErrWxidLoginStateNotFound) {
		t.Fatalf("HeartBeat 不存在 wxid 错误 = %v，期望 ErrWxidLoginStateNotFound", err)
	}

	imported, err := service.ImportA16Data(context.Background(), login.ImportA16DataRequest{
		A16:        "logged-out-a16",
		DeviceID:   "logged-out-android",
		DeviceName: "已退出会话设备",
		Wxid:       "wxid_session_logged_out",
	})
	if err != nil {
		t.Fatalf("导入 A16 登录态失败：%v", err)
	}
	state := imported.State
	state.SessionState = "logged_out"
	state.HeartbeatCount = 1
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("保存已退出登录态失败：%v", err)
	}

	result, err := service.HeartBeat(context.Background(), login.HeartBeatRequest{Wxid: "wxid_session_logged_out"})
	if !errors.Is(err, login.ErrSessionLoggedOut) {
		t.Fatalf("已退出心跳错误 = %v，期望 ErrSessionLoggedOut", err)
	}
	if result.State.UUID != imported.UUID || result.State.SessionState != "logged_out" || result.State.HeartbeatCount != 1 {
		t.Fatalf("已退出心跳 result = %+v，期望返回原登录态且不增加心跳", result)
	}
}

func assertSessionSample(t *testing.T, path string, wxid string, sessionState string, heartbeatStatus string, wantRequest map[string]any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取会话样本失败：%v", err)
	}
	var sample map[string]any
	if err := json.Unmarshal(raw, &sample); err != nil {
		t.Fatalf("会话样本不是 JSON：%v", err)
	}
	for _, key := range []string{"request", "mock_response", "login_state"} {
		if _, ok := sample[key]; !ok {
			t.Fatalf("会话样本缺少字段 %s：%+v", key, sample)
		}
	}
	request := sample["request"].(map[string]any)
	if request["wxid"] != wxid {
		t.Fatalf("会话样本 request = %+v，期望 wxid=%s", request, wxid)
	}
	for key, value := range wantRequest {
		if request[key] != value {
			t.Fatalf("会话样本 request[%s] = %#v，期望 %#v", key, request[key], value)
		}
	}
	state := sample["login_state"].(map[string]any)
	if sessionState != "" && state["session_state"] != sessionState {
		t.Fatalf("会话样本 login_state = %+v，期望 session_state=%s", state, sessionState)
	}
	if heartbeatStatus != "" && state["heartbeat_status"] != heartbeatStatus {
		t.Fatalf("会话样本 login_state = %+v，期望 heartbeat_status=%s", state, heartbeatStatus)
	}
}
