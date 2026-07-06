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

func TestServiceGetQRBuildsMockLoginContextSampleAndState(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	client, _, err := network.NewClient(network.Config{Mode: "mock"})
	if err != nil {
		t.Fatalf("创建 mock network client 失败：%v", err)
	}
	fixedNow := time.Date(2026, 7, 6, 22, 10, 0, 123456789, time.UTC)
	service := login.NewService(login.Dependencies{
		States:    store,
		Network:   client,
		SampleDir: t.TempDir(),
		Now:       func() time.Time { return fixedNow },
	})

	result, err := service.GetQR(context.Background(), login.GetQRRequest{
		DeviceID:   "svc-dev-001",
		DeviceName: "业务层设备",
		Type:       "ipad",
	})
	if err != nil {
		t.Fatalf("GetQR 返回错误：%v", err)
	}
	if result.UUID == "" || result.CacheKey != "login:mock:"+result.UUID || result.QRURL != "mock://login/"+result.UUID {
		t.Fatalf("GetQR result = %+v，期望稳定 uuid/cache_key/qr_url", result)
	}
	if result.DeviceID != "svc-dev-001" || result.DeviceName != "业务层设备" || result.Type != "ipad" {
		t.Fatalf("设备字段 = %+v，期望保留请求设备信息", result)
	}
	if result.Protocol["pack_kind"] != "hybrid_ecdh_ios_placeholder" || result.Protocol["operation"] != "Login.GetQR" {
		t.Fatalf("protocol = %+v，期望 iOS Hybrid GetQR 摘要", result.Protocol)
	}
	if result.Network["mode"] != "mock" || result.Network["operation"] != "Login.GetQR" || result.Network["login_kind"] != "getqr_mock" {
		t.Fatalf("network = %+v，期望 mock GetQR 网络摘要", result.Network)
	}
	if len(result.Stages) == 0 || result.Stages[len(result.Stages)-1] != "write_sample" {
		t.Fatalf("stages = %+v，期望记录到 write_sample", result.Stages)
	}
	if result.State.UUID != result.UUID || !result.State.CreatedAt.Equal(fixedNow) || result.State.SamplePath != result.SamplePath {
		t.Fatalf("state = %+v，期望保存业务层登录态", result.State)
	}

	stored, ok, err := store.Get(context.Background(), result.UUID, "")
	if err != nil || !ok {
		t.Fatalf("按 uuid 读取登录态 = %+v / %v / %v，期望业务层已保存", stored, ok, err)
	}
	if stored.CacheKey != result.CacheKey || stored.Protocol["pack_kind"] != "hybrid_ecdh_ios_placeholder" {
		t.Fatalf("存储登录态 = %+v，期望包含 cache_key 与协议摘要", stored)
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
	sampleNetwork := sample["network"].(map[string]any)
	if sampleNetwork["mode"] != "mock" || sampleNetwork["operation"] != "Login.GetQR" {
		t.Fatalf("样本 network = %+v，期望落盘网络摘要", sampleNetwork)
	}
}

func TestServiceGetQRUsesDefaultDeviceValues(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	client, _, err := network.NewClient(network.Config{Mode: "mock"})
	if err != nil {
		t.Fatalf("创建 mock network client 失败：%v", err)
	}
	service := login.NewService(login.Dependencies{States: store, Network: client, SampleDir: t.TempDir()})

	result, err := service.GetQR(context.Background(), login.GetQRRequest{})
	if err != nil {
		t.Fatalf("GetQR 返回错误：%v", err)
	}
	if result.DeviceID != "mock-device" || result.DeviceName != "mock-device-name" || result.Type != "ipad" {
		t.Fatalf("默认设备字段 = %+v，期望 mock-device/mock-device-name/ipad", result)
	}
}
