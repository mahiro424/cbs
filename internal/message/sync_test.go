package message_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/mahiro424/cbs/internal/message"
	"github.com/mahiro424/cbs/internal/network"
	"github.com/mahiro424/cbs/internal/storage"
)

func TestServiceSyncMessagesUsesLoginStateProtocolNetworkAndSample(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	state := storage.LoginState{
		UUID:         "mock-login-sync",
		CacheKey:     "login:mock:sync",
		Wxid:         "wxid_sync_sender",
		DeviceID:     "iphone-sync-001",
		DeviceName:   "同步设备",
		Type:         "iphone",
		Mode:         "mock",
		LoginKind:    "data62_mock",
		SessionState: "initialized",
		CreatedAt:    time.Date(2026, 7, 6, 18, 30, 0, 0, time.UTC),
	}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("保存登录态失败：%v", err)
	}
	client, _, err := network.NewClient(network.Config{Mode: "mock"})
	if err != nil {
		t.Fatalf("创建 mock network client 失败：%v", err)
	}
	fixedNow := time.Date(2026, 7, 7, 0, 10, 0, 987654321, time.UTC)
	service := message.NewService(message.Dependencies{
		States:    store,
		Network:   client,
		SampleDir: t.TempDir(),
		Now:       func() time.Time { return fixedNow },
	})

	result, err := service.Sync(context.Background(), message.SyncRequest{
		Wxid:    "wxid_sync_sender",
		Scene:   0,
		Synckey: "current-sync-key",
	})
	if err != nil {
		t.Fatalf("同步消息失败：%v", err)
	}
	if result.Status != "mock_synced" || result.SyncID == "" || result.NextSyncKey == "" || result.SyncedAt != fixedNow {
		t.Fatalf("同步结果 = %+v，期望稳定 mock_synced/sync_id/next_synckey/synced_at", result)
	}
	if result.Wxid != state.Wxid || result.Scene != 0 || result.SyncKey != "current-sync-key" {
		t.Fatalf("同步上下文 = %+v，期望保留 wxid/scene/synckey", result)
	}
	if result.Protocol["operation"] != "Msg.Sync" || result.Protocol["pack_kind"] != "business_packet_mock" {
		t.Fatalf("protocol = %+v，期望通过业务协议封包", result.Protocol)
	}
	if result.Network["mode"] != "mock" || result.Network["operation"] != "Msg.Sync" || result.Network["login_kind"] != "data62_mock" || result.Network["platform"] != "ios" {
		t.Fatalf("network = %+v，期望 mock 网络摘要包含同步操作和登录类型", result.Network)
	}
	if result.LoginState.UUID != state.UUID || result.LoginState.Wxid != state.Wxid {
		t.Fatalf("login_state = %+v，期望来自存储中的同步方登录态", result.LoginState)
	}
	if !containsString(result.Stages, "business_packet_pack") || result.Stages[len(result.Stages)-1] != "write_sample" {
		t.Fatalf("stages = %+v，期望经过封包、网络和样本落盘", result.Stages)
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
	if request["wxid"] != "wxid_sync_sender" || request["synckey"] != "current-sync-key" || request["scene"] != float64(0) {
		t.Fatalf("样本 request = %+v，期望落盘同步请求", request)
	}

	responseData := result.ResponseData()
	for _, key := range []string{"status", "sync_id", "wxid", "scene", "synckey", "next_synckey", "protocol", "network", "login_state", "sample_path", "stages"} {
		if _, ok := responseData[key]; !ok {
			t.Fatalf("响应缺少字段 %s：%+v", key, responseData)
		}
	}
}

func TestServiceSyncReportsLoginStateErrors(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	service := message.NewService(message.Dependencies{States: store, SampleDir: t.TempDir()})

	_, err := service.Sync(context.Background(), message.SyncRequest{Wxid: "wxid_missing", Scene: 0})
	if !errors.Is(err, message.ErrLoginStateNotFound) {
		t.Fatalf("不存在登录态错误 = %v，期望 ErrLoginStateNotFound", err)
	}

	loggedOut := storage.LoginState{
		UUID:         "mock-sync-logged-out",
		CacheKey:     "login:mock:sync-logged-out",
		Wxid:         "wxid_sync_logged_out",
		LoginKind:    "a16_mock",
		SessionState: "logged_out",
		CreatedAt:    time.Date(2026, 7, 6, 19, 30, 0, 0, time.UTC),
	}
	if err := store.Save(context.Background(), loggedOut); err != nil {
		t.Fatalf("保存退出登录态失败：%v", err)
	}
	_, err = service.Sync(context.Background(), message.SyncRequest{Wxid: "wxid_sync_logged_out", Scene: 0})
	if !errors.Is(err, message.ErrSessionLoggedOut) {
		t.Fatalf("已退出登录态错误 = %v，期望 ErrSessionLoggedOut", err)
	}
}
