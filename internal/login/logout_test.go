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

func TestServiceLogOutMarksStateSampleAndBlocksFutureHeartBeat(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	client, _, err := network.NewClient(network.Config{Mode: "mock"})
	if err != nil {
		t.Fatalf("创建 mock network client 失败：%v", err)
	}
	currentTime := time.Date(2026, 7, 7, 2, 1, 0, 0, time.UTC)
	service := login.NewService(login.Dependencies{
		States:    store,
		Network:   client,
		SampleDir: t.TempDir(),
		Now:       func() time.Time { return currentTime },
	})

	imported, err := service.ImportA16Data(context.Background(), login.ImportA16DataRequest{
		A16:        "logout-a16-data",
		DeviceID:   "logout-android",
		DeviceName: "业务层退出设备",
		Wxid:       "wxid_logout_service",
	})
	if err != nil {
		t.Fatalf("导入 A16 登录态失败：%v", err)
	}
	_, err = service.HeartBeat(context.Background(), login.HeartBeatRequest{Wxid: "wxid_logout_service"})
	if err != nil {
		t.Fatalf("退出前 HeartBeat 失败：%v", err)
	}

	logoutAt := time.Date(2026, 7, 7, 2, 2, 3, 123456789, time.UTC)
	currentTime = logoutAt
	result, err := service.LogOut(context.Background(), login.LogOutRequest{Wxid: "wxid_logout_service"})
	if err != nil {
		t.Fatalf("LogOut 返回错误：%v", err)
	}
	if result.Mode != "mock" || result.UUID != imported.UUID || result.Wxid != "wxid_logout_service" || result.LogoutStatus != "logged_out" {
		t.Fatalf("LogOut result = %+v，期望标记同一登录态为 logged_out", result)
	}
	if result.State.SessionState != "logged_out" || result.State.LogoutStatus != "logged_out" || !result.State.LoggedOutAt.Equal(logoutAt) {
		t.Fatalf("LogOut 登录态 = %+v，期望记录退出状态和时间", result.State)
	}
	if result.State.HeartbeatCount != 1 {
		t.Fatalf("LogOut 后 heartbeat_count = %d，期望保留退出前次数", result.State.HeartbeatCount)
	}
	if !containsString(result.Stages, "mock_logout") || result.Stages[len(result.Stages)-1] != "write_sample" {
		t.Fatalf("LogOut stages = %+v，期望记录退出阶段", result.Stages)
	}

	stored, ok, err := store.Get(context.Background(), imported.UUID, "")
	if err != nil || !ok {
		t.Fatalf("按 uuid 读取退出登录态 = %+v / %v / %v，期望已保存", stored, ok, err)
	}
	if stored.SessionState != "logged_out" || stored.LogoutStatus != "logged_out" || stored.SamplePath != result.SamplePath || !stored.LoggedOutAt.Equal(logoutAt) {
		t.Fatalf("存储登录态 = %+v，期望反映最近退出", stored)
	}

	raw, err := os.ReadFile(result.SamplePath)
	if err != nil {
		t.Fatalf("读取退出样本失败：%v", err)
	}
	var sample map[string]any
	if err := json.Unmarshal(raw, &sample); err != nil {
		t.Fatalf("退出样本不是 JSON：%v", err)
	}
	for _, key := range []string{"request", "mock_response", "login_state"} {
		if _, ok := sample[key]; !ok {
			t.Fatalf("退出样本缺少字段 %s：%+v", key, sample)
		}
	}
	request := sample["request"].(map[string]any)
	if request["wxid"] != "wxid_logout_service" {
		t.Fatalf("退出样本 request = %+v，期望记录 wxid", request)
	}
	mockResponse := sample["mock_response"].(map[string]any)
	if mockResponse["logout_status"] != "logged_out" || mockResponse["logged_out_at"] == "" {
		t.Fatalf("退出样本 mock_response = %+v，期望退出状态和时间", mockResponse)
	}

	responseData := result.ResponseData()
	for _, key := range []string{"mode", "uuid", "cache_key", "wxid", "logout_status", "logged_out_at", "login_state", "sample_path", "stages"} {
		if _, ok := responseData[key]; !ok {
			t.Fatalf("LogOut 响应缺少字段 %s：%+v", key, responseData)
		}
	}

	beatAfter, err := service.HeartBeat(context.Background(), login.HeartBeatRequest{Wxid: "wxid_logout_service"})
	if !errors.Is(err, login.ErrSessionLoggedOut) {
		t.Fatalf("退出后 HeartBeat 错误 = %v，期望 ErrSessionLoggedOut", err)
	}
	if beatAfter.State.HeartbeatCount != 1 || beatAfter.State.SessionState != "logged_out" {
		t.Fatalf("退出后 HeartBeat result = %+v，期望不增加心跳并保持 logged_out", beatAfter)
	}
}

func TestServiceLogOutReportsMissingWxidState(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	client, _, err := network.NewClient(network.Config{Mode: "mock"})
	if err != nil {
		t.Fatalf("创建 mock network client 失败：%v", err)
	}
	service := login.NewService(login.Dependencies{States: store, Network: client, SampleDir: t.TempDir()})

	_, err = service.LogOut(context.Background(), login.LogOutRequest{Wxid: "wxid_missing_logout"})
	if !errors.Is(err, login.ErrWxidLoginStateNotFound) {
		t.Fatalf("LogOut 不存在 wxid 错误 = %v，期望 ErrWxidLoginStateNotFound", err)
	}
}
