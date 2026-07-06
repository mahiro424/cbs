package httpapi_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/mahiro424/cbs/internal/config"
	"github.com/mahiro424/cbs/internal/httpapi"
)

func TestMsgSendTxtMockPathUsesLoginStateProtocolNetworkAndSample(t *testing.T) {
	cfg := config.Default()
	cfg.SampleDir = t.TempDir()
	h := httpapi.NewServer(cfg)

	missing := postJSON(t, h, "/Msg/SendTxt", `{}`)
	if missing.Success || missing.Code != "param_error" {
		t.Fatalf("缺少参数响应 = %+v，期望 param_error", missing)
	}

	notFound := postJSON(t, h, "/Msg/SendTxt", `{"Wxid":"wxid_not_exists","ToWxid":"wxid_receiver","Content":"hello"}`)
	if notFound.Success || notFound.Code != "cache_not_found" {
		t.Fatalf("不存在登录态响应 = %+v，期望 cache_not_found", notFound)
	}

	login := postJSON(t, h, "/Login/A16Data", `{"A16":"mock-a16-msg","DeviceID":"android-msg","DeviceName":"消息设备","Wxid":"wxid_msg_sender"}`)
	if !login.Success || login.Code != "ok" {
		t.Fatalf("A16Data 响应 = %+v，期望 ok", login)
	}
	init := postJSON(t, h, "/Login/Newinit?wxid=wxid_msg_sender", `{}`)
	if !init.Success || init.Code != "ok" {
		t.Fatalf("Newinit 响应 = %+v，期望 ok", init)
	}

	resp := postJSON(t, h, "/Msg/SendTxt", `{"Wxid":"wxid_msg_sender","ToWxid":"wxid_receiver","Content":"你好，HTTP 文本消息","Type":1,"At":"wxid_at"}`)
	if !resp.Success || resp.Code != "ok" {
		t.Fatalf("SendTxt 响应 = %+v，期望 ok", resp)
	}
	data := mustMap(t, resp.Data)
	if data["status"] != "mock_sent" || data["wxid"] != "wxid_msg_sender" || data["to_wxid"] != "wxid_receiver" {
		t.Fatalf("SendTxt data = %+v，期望 mock_sent 和发送对象", data)
	}
	messageID := mustString(t, data, "message_id")
	samplePath := mustString(t, data, "sample_path")
	protocol := mustMap(t, data["protocol"])
	if protocol["operation"] != "Msg.SendTxt" || protocol["pack_kind"] != "business_packet_mock" || mustString(t, protocol, "packed_hex") == "" {
		t.Fatalf("protocol = %+v，期望文本消息协议封包摘要", protocol)
	}
	network := mustMap(t, data["network"])
	if network["mode"] != "mock" || network["operation"] != "Msg.SendTxt" || network["login_kind"] != "a16_mock" {
		t.Fatalf("network = %+v，期望 mock 文本消息网络摘要", network)
	}
	loginState := mustMap(t, data["login_state"])
	if loginState["wxid"] != "wxid_msg_sender" || loginState["login_kind"] != "a16_mock" {
		t.Fatalf("login_state = %+v，期望读取发送方登录态", loginState)
	}

	raw, err := os.ReadFile(samplePath)
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
	mockResponse := mustMap(t, sample["mock_response"])
	if mockResponse["message_id"] != messageID || mockResponse["status"] != "mock_sent" {
		t.Fatalf("样本 mock_response = %+v，期望记录 message_id/status", mockResponse)
	}
}

func TestMsgSyncMockPathUsesLoginStateProtocolNetworkAndSample(t *testing.T) {
	cfg := config.Default()
	cfg.SampleDir = t.TempDir()
	h := httpapi.NewServer(cfg)

	missing := postJSON(t, h, "/Msg/Sync", `{}`)
	if missing.Success || missing.Code != "param_error" {
		t.Fatalf("缺少参数响应 = %+v，期望 param_error", missing)
	}

	notFound := postJSON(t, h, "/Msg/Sync", `{"Wxid":"wxid_not_exists","Scene":0}`)
	if notFound.Success || notFound.Code != "cache_not_found" {
		t.Fatalf("不存在登录态响应 = %+v，期望 cache_not_found", notFound)
	}

	login := postJSON(t, h, "/Login/62data", `{"Data62":"mock-62-sync","DeviceID":"iphone-sync","DeviceName":"同步设备","Wxid":"wxid_sync_sender"}`)
	if !login.Success || login.Code != "ok" {
		t.Fatalf("62data 响应 = %+v，期望 ok", login)
	}
	init := postJSON(t, h, "/Login/Newinit?wxid=wxid_sync_sender&CurrentSynckey=current-sync-key", `{}`)
	if !init.Success || init.Code != "ok" {
		t.Fatalf("Newinit 响应 = %+v，期望 ok", init)
	}

	resp := postJSON(t, h, "/Msg/Sync", `{"Wxid":"wxid_sync_sender","Scene":0,"Synckey":"current-sync-key"}`)
	if !resp.Success || resp.Code != "ok" {
		t.Fatalf("Sync 响应 = %+v，期望 ok", resp)
	}
	data := mustMap(t, resp.Data)
	if data["status"] != "mock_synced" || data["wxid"] != "wxid_sync_sender" || data["scene"] != float64(0) || data["synckey"] != "current-sync-key" {
		t.Fatalf("Sync data = %+v，期望 mock_synced 和同步上下文", data)
	}
	syncID := mustString(t, data, "sync_id")
	if mustString(t, data, "next_synckey") == "" {
		t.Fatalf("Sync data = %+v，期望 next_synckey 非空", data)
	}
	samplePath := mustString(t, data, "sample_path")
	protocol := mustMap(t, data["protocol"])
	if protocol["operation"] != "Msg.Sync" || protocol["pack_kind"] != "business_packet_mock" || mustString(t, protocol, "packed_hex") == "" {
		t.Fatalf("protocol = %+v，期望同步消息协议封包摘要", protocol)
	}
	network := mustMap(t, data["network"])
	if network["mode"] != "mock" || network["operation"] != "Msg.Sync" || network["login_kind"] != "data62_mock" {
		t.Fatalf("network = %+v，期望 mock 同步消息网络摘要", network)
	}
	loginState := mustMap(t, data["login_state"])
	if loginState["wxid"] != "wxid_sync_sender" || loginState["login_kind"] != "data62_mock" {
		t.Fatalf("login_state = %+v，期望读取同步方登录态", loginState)
	}

	raw, err := os.ReadFile(samplePath)
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
	mockResponse := mustMap(t, sample["mock_response"])
	if mockResponse["sync_id"] != syncID || mockResponse["status"] != "mock_synced" {
		t.Fatalf("样本 mock_response = %+v，期望记录 sync_id/status", mockResponse)
	}
}

func TestMsgRevokeMockPathUsesLoginStateProtocolNetworkAndSample(t *testing.T) {
	cfg := config.Default()
	cfg.SampleDir = t.TempDir()
	h := httpapi.NewServer(cfg)

	missing := postJSON(t, h, "/Msg/Revoke", `{}`)
	if missing.Success || missing.Code != "param_error" {
		t.Fatalf("缺少参数响应 = %+v，期望 param_error", missing)
	}

	notFound := postJSON(t, h, "/Msg/Revoke", `{"Wxid":"wxid_not_exists","ToUserName":"wxid_receiver","NewMsgId":1,"ClientMsgId":2,"CreateTime":1783340000}`)
	if notFound.Success || notFound.Code != "cache_not_found" {
		t.Fatalf("不存在登录态响应 = %+v，期望 cache_not_found", notFound)
	}

	login := postJSON(t, h, "/Login/A16Data", `{"A16":"mock-a16-revoke","DeviceID":"android-revoke","DeviceName":"撤回设备","Wxid":"wxid_revoke_sender"}`)
	if !login.Success || login.Code != "ok" {
		t.Fatalf("A16Data 响应 = %+v，期望 ok", login)
	}
	init := postJSON(t, h, "/Login/Newinit?wxid=wxid_revoke_sender", `{}`)
	if !init.Success || init.Code != "ok" {
		t.Fatalf("Newinit 响应 = %+v，期望 ok", init)
	}

	resp := postJSON(t, h, "/Msg/Revoke", `{"Wxid":"wxid_revoke_sender","ToUserName":"wxid_receiver","NewMsgId":900000000001,"ClientMsgId":700000000002,"CreateTime":1783340000}`)
	if !resp.Success || resp.Code != "ok" {
		t.Fatalf("Revoke 响应 = %+v，期望 ok", resp)
	}
	data := mustMap(t, resp.Data)
	if data["status"] != "mock_revoked" || data["wxid"] != "wxid_revoke_sender" || data["to_user_name"] != "wxid_receiver" {
		t.Fatalf("Revoke data = %+v，期望 mock_revoked 和撤回上下文", data)
	}
	revokeID := mustString(t, data, "revoke_id")
	if data["new_msg_id"] != float64(900000000001) || data["client_msg_id"] != float64(700000000002) || data["create_time"] != float64(1783340000) {
		t.Fatalf("Revoke data = %+v，期望消息 ID 和创建时间", data)
	}
	samplePath := mustString(t, data, "sample_path")
	protocol := mustMap(t, data["protocol"])
	if protocol["operation"] != "Msg.Revoke" || protocol["pack_kind"] != "business_packet_mock" || mustString(t, protocol, "packed_hex") == "" {
		t.Fatalf("protocol = %+v，期望撤回消息协议封包摘要", protocol)
	}
	network := mustMap(t, data["network"])
	if network["mode"] != "mock" || network["operation"] != "Msg.Revoke" || network["login_kind"] != "a16_mock" {
		t.Fatalf("network = %+v，期望 mock 撤回消息网络摘要", network)
	}
	loginState := mustMap(t, data["login_state"])
	if loginState["wxid"] != "wxid_revoke_sender" || loginState["login_kind"] != "a16_mock" {
		t.Fatalf("login_state = %+v，期望读取撤回方登录态", loginState)
	}

	raw, err := os.ReadFile(samplePath)
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
	mockResponse := mustMap(t, sample["mock_response"])
	if mockResponse["revoke_id"] != revokeID || mockResponse["status"] != "mock_revoked" {
		t.Fatalf("样本 mock_response = %+v，期望记录 revoke_id/status", mockResponse)
	}
}
