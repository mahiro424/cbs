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

func TestServiceSendTextUsesLoginStateProtocolNetworkAndSample(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	state := storage.LoginState{
		UUID:         "mock-login-sendtxt",
		CacheKey:     "login:mock:sendtxt",
		Wxid:         "wxid_sender",
		DeviceID:     "android-msg-001",
		DeviceName:   "消息设备",
		Type:         "android",
		Mode:         "mock",
		LoginKind:    "a16_mock",
		SessionState: "initialized",
		CreatedAt:    time.Date(2026, 7, 6, 18, 0, 0, 0, time.UTC),
	}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("保存登录态失败：%v", err)
	}
	client, _, err := network.NewClient(network.Config{Mode: "mock"})
	if err != nil {
		t.Fatalf("创建 mock network client 失败：%v", err)
	}
	fixedNow := time.Date(2026, 7, 6, 23, 45, 0, 123456789, time.UTC)
	service := message.NewService(message.Dependencies{
		States:    store,
		Network:   client,
		SampleDir: t.TempDir(),
		Now:       func() time.Time { return fixedNow },
	})

	result, err := service.SendText(context.Background(), message.SendTextRequest{
		Wxid:    "wxid_sender",
		ToWxid:  "wxid_receiver",
		Content: "你好，mock 文本消息",
		Type:    1,
		At:      "wxid_at_001,wxid_at_002",
	})
	if err != nil {
		t.Fatalf("发送文本消息失败：%v", err)
	}
	if result.Status != "mock_sent" || result.MessageID == "" || result.SentAt != fixedNow {
		t.Fatalf("发送结果 = %+v，期望稳定 mock_sent/message_id/sent_at", result)
	}
	if result.Protocol["operation"] != "Msg.SendTxt" || result.Protocol["pack_kind"] != "business_packet_mock" {
		t.Fatalf("protocol = %+v，期望通过业务协议封包", result.Protocol)
	}
	if result.Network["mode"] != "mock" || result.Network["operation"] != "Msg.SendTxt" || result.Network["login_kind"] != "a16_mock" || result.Network["platform"] != "android" {
		t.Fatalf("network = %+v，期望 mock 网络摘要包含消息操作和登录类型", result.Network)
	}
	if result.LoginState.UUID != state.UUID || result.LoginState.Wxid != state.Wxid {
		t.Fatalf("login_state = %+v，期望来自存储中的发送方登录态", result.LoginState)
	}
	if result.ContentLength != len([]rune("你好，mock 文本消息")) {
		t.Fatalf("content_length = %d，期望按字符数统计", result.ContentLength)
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
	if request["wxid"] != "wxid_sender" || request["to_wxid"] != "wxid_receiver" || request["content"] != "你好，mock 文本消息" {
		t.Fatalf("样本 request = %+v，期望落盘发送请求", request)
	}

	responseData := result.ResponseData()
	for _, key := range []string{"status", "message_id", "wxid", "to_wxid", "content_length", "protocol", "network", "login_state", "sample_path", "stages"} {
		if _, ok := responseData[key]; !ok {
			t.Fatalf("响应缺少字段 %s：%+v", key, responseData)
		}
	}
}

func TestServiceSendTextReportsLoginStateErrors(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	service := message.NewService(message.Dependencies{States: store, SampleDir: t.TempDir()})

	_, err := service.SendText(context.Background(), message.SendTextRequest{Wxid: "wxid_missing", ToWxid: "wxid_receiver", Content: "hello"})
	if !errors.Is(err, message.ErrLoginStateNotFound) {
		t.Fatalf("不存在登录态错误 = %v，期望 ErrLoginStateNotFound", err)
	}

	loggedOut := storage.LoginState{
		UUID:         "mock-login-logged-out",
		CacheKey:     "login:mock:logged-out",
		Wxid:         "wxid_logged_out",
		LoginKind:    "a16_mock",
		SessionState: "logged_out",
		CreatedAt:    time.Date(2026, 7, 6, 19, 0, 0, 0, time.UTC),
	}
	if err := store.Save(context.Background(), loggedOut); err != nil {
		t.Fatalf("保存退出登录态失败：%v", err)
	}
	_, err = service.SendText(context.Background(), message.SendTextRequest{Wxid: "wxid_logged_out", ToWxid: "wxid_receiver", Content: "hello"})
	if !errors.Is(err, message.ErrSessionLoggedOut) {
		t.Fatalf("已退出登录态错误 = %v，期望 ErrSessionLoggedOut", err)
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
