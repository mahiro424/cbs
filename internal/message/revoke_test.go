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

func TestServiceRevokeUsesLoginStateProtocolNetworkAndSample(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	state := storage.LoginState{
		UUID:         "mock-login-revoke",
		CacheKey:     "login:mock:revoke",
		Wxid:         "wxid_revoke_sender",
		DeviceID:     "android-revoke-001",
		DeviceName:   "撤回设备",
		Type:         "android",
		Mode:         "mock",
		LoginKind:    "a16_mock",
		SessionState: "initialized",
		CreatedAt:    time.Date(2026, 7, 6, 20, 0, 0, 0, time.UTC),
	}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("保存登录态失败：%v", err)
	}
	client, _, err := network.NewClient(network.Config{Mode: "mock"})
	if err != nil {
		t.Fatalf("创建 mock network client 失败：%v", err)
	}
	fixedNow := time.Date(2026, 7, 7, 0, 35, 0, 123456789, time.UTC)
	service := message.NewService(message.Dependencies{
		States:    store,
		Network:   client,
		SampleDir: t.TempDir(),
		Now:       func() time.Time { return fixedNow },
	})

	result, err := service.Revoke(context.Background(), message.RevokeRequest{
		Wxid:        "wxid_revoke_sender",
		ToUserName:  "wxid_receiver",
		NewMsgID:    900000000001,
		ClientMsgID: 700000000002,
		CreateTime:  1783340000,
	})
	if err != nil {
		t.Fatalf("撤回消息失败：%v", err)
	}
	if result.Status != "mock_revoked" || result.RevokeID == "" || result.RevokedAt != fixedNow {
		t.Fatalf("撤回结果 = %+v，期望稳定 mock_revoked/revoke_id/revoked_at", result)
	}
	if result.Wxid != state.Wxid || result.ToUserName != "wxid_receiver" || result.NewMsgID != 900000000001 || result.ClientMsgID != 700000000002 || result.CreateTime != 1783340000 {
		t.Fatalf("撤回上下文 = %+v，期望保留请求字段", result)
	}
	if result.Protocol["operation"] != "Msg.Revoke" || result.Protocol["pack_kind"] != "business_packet_mock" {
		t.Fatalf("protocol = %+v，期望通过业务协议封包", result.Protocol)
	}
	if result.Network["mode"] != "mock" || result.Network["operation"] != "Msg.Revoke" || result.Network["login_kind"] != "a16_mock" || result.Network["platform"] != "android" {
		t.Fatalf("network = %+v，期望 mock 网络摘要包含撤回操作和登录类型", result.Network)
	}
	if result.LoginState.UUID != state.UUID || result.LoginState.Wxid != state.Wxid {
		t.Fatalf("login_state = %+v，期望来自存储中的撤回方登录态", result.LoginState)
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
	if request["wxid"] != "wxid_revoke_sender" || request["to_user_name"] != "wxid_receiver" || request["new_msg_id"] != float64(900000000001) {
		t.Fatalf("样本 request = %+v，期望落盘撤回请求", request)
	}

	responseData := result.ResponseData()
	for _, key := range []string{"status", "revoke_id", "wxid", "to_user_name", "new_msg_id", "client_msg_id", "create_time", "protocol", "network", "login_state", "sample_path", "stages"} {
		if _, ok := responseData[key]; !ok {
			t.Fatalf("响应缺少字段 %s：%+v", key, responseData)
		}
	}
}

func TestServiceRevokeReportsLoginStateErrors(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	service := message.NewService(message.Dependencies{States: store, SampleDir: t.TempDir()})

	_, err := service.Revoke(context.Background(), message.RevokeRequest{Wxid: "wxid_missing", ToUserName: "wxid_receiver", NewMsgID: 1})
	if !errors.Is(err, message.ErrLoginStateNotFound) {
		t.Fatalf("不存在登录态错误 = %v，期望 ErrLoginStateNotFound", err)
	}

	loggedOut := storage.LoginState{
		UUID:         "mock-revoke-logged-out",
		CacheKey:     "login:mock:revoke-logged-out",
		Wxid:         "wxid_revoke_logged_out",
		LoginKind:    "a16_mock",
		SessionState: "logged_out",
		CreatedAt:    time.Date(2026, 7, 6, 21, 0, 0, 0, time.UTC),
	}
	if err := store.Save(context.Background(), loggedOut); err != nil {
		t.Fatalf("保存退出登录态失败：%v", err)
	}
	_, err = service.Revoke(context.Background(), message.RevokeRequest{Wxid: "wxid_revoke_logged_out", ToUserName: "wxid_receiver", NewMsgID: 1})
	if !errors.Is(err, message.ErrSessionLoggedOut) {
		t.Fatalf("已退出登录态错误 = %v，期望 ErrSessionLoggedOut", err)
	}
}
