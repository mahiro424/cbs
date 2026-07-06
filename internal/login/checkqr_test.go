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

func TestServiceCheckQRUpdatesMockQRStateSampleAndStore(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	client, _, err := network.NewClient(network.Config{Mode: "mock"})
	if err != nil {
		t.Fatalf("创建 mock network client 失败：%v", err)
	}
	currentTime := time.Date(2026, 7, 6, 23, 30, 0, 0, time.UTC)
	service := login.NewService(login.Dependencies{
		States:    store,
		Network:   client,
		SampleDir: t.TempDir(),
		Now:       func() time.Time { return currentTime },
	})

	qr, err := service.GetQR(context.Background(), login.GetQRRequest{
		DeviceID:   "checkqr-dev-001",
		DeviceName: "业务层二维码检查设备",
		Type:       "ipad",
	})
	if err != nil {
		t.Fatalf("GetQR 返回错误：%v", err)
	}
	checkedAt := time.Date(2026, 7, 6, 23, 31, 2, 123456789, time.UTC)
	currentTime = checkedAt

	result, err := service.CheckQR(context.Background(), login.CheckQRRequest{UUID: qr.UUID})
	if err != nil {
		t.Fatalf("CheckQR 返回错误：%v", err)
	}
	if result.Mode != "mock" || result.UUID != qr.UUID || result.CacheKey != qr.CacheKey {
		t.Fatalf("CheckQR result = %+v，期望保留二维码 uuid/cache_key", result)
	}
	if result.QRStatus != "waiting_scan" || result.CheckCount != 1 || !result.CheckedAt.Equal(checkedAt) {
		t.Fatalf("CheckQR 状态 = %+v，期望 waiting_scan/check_count=1/固定检查时间", result)
	}
	if result.State.UUID != qr.UUID || result.State.LoginKind != "getqr_mock" || result.State.QRStatus != "waiting_scan" || result.State.CheckCount != 1 || !result.State.CheckedAt.Equal(checkedAt) {
		t.Fatalf("CheckQR 登录态 = %+v，期望更新二维码检查状态", result.State)
	}
	if !containsString(result.Stages, "load_qr_login_state") || !containsString(result.Stages, "mock_poll_qr_status") || result.Stages[len(result.Stages)-1] != "write_sample" {
		t.Fatalf("CheckQR stages = %+v，期望记录二维码检查阶段", result.Stages)
	}

	stored, ok, err := store.Get(context.Background(), qr.UUID, "")
	if err != nil || !ok {
		t.Fatalf("按 uuid 读取 CheckQR 登录态 = %+v / %v / %v，期望已保存", stored, ok, err)
	}
	if stored.CheckCount != 1 || stored.QRStatus != "waiting_scan" || stored.SamplePath != result.SamplePath || !stored.CheckedAt.Equal(checkedAt) {
		t.Fatalf("存储登录态 = %+v，期望反映最近一次 CheckQR", stored)
	}

	raw, err := os.ReadFile(result.SamplePath)
	if err != nil {
		t.Fatalf("读取 CheckQR 样本失败：%v", err)
	}
	var sample map[string]any
	if err := json.Unmarshal(raw, &sample); err != nil {
		t.Fatalf("CheckQR 样本不是 JSON：%v", err)
	}
	for _, key := range []string{"request", "mock_response", "login_state"} {
		if _, ok := sample[key]; !ok {
			t.Fatalf("CheckQR 样本缺少字段 %s：%+v", key, sample)
		}
	}
	request := sample["request"].(map[string]any)
	if request["uuid"] != qr.UUID {
		t.Fatalf("CheckQR 样本 request = %+v，期望记录 uuid", request)
	}
	mockResponse := sample["mock_response"].(map[string]any)
	if mockResponse["qr_status"] != "waiting_scan" || mockResponse["check_count"] != float64(1) {
		t.Fatalf("CheckQR 样本 mock_response = %+v，期望 waiting_scan/check_count=1", mockResponse)
	}

	responseData := result.ResponseData()
	for _, key := range []string{"mode", "uuid", "cache_key", "status", "qr_status", "checked_at", "check_count", "login_state", "sample_path", "stages"} {
		if _, ok := responseData[key]; !ok {
			t.Fatalf("CheckQR 响应缺少字段 %s：%+v", key, responseData)
		}
	}
	if responseData["status"] != "waiting_scan" || responseData["qr_status"] != "waiting_scan" || responseData["check_count"] != 1 {
		t.Fatalf("CheckQR 响应 = %+v，期望保留兼容状态字段", responseData)
	}
}

func TestServiceCheckQRReportsMissingAndUnsupportedLoginState(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	client, _, err := network.NewClient(network.Config{Mode: "mock"})
	if err != nil {
		t.Fatalf("创建 mock network client 失败：%v", err)
	}
	service := login.NewService(login.Dependencies{States: store, Network: client, SampleDir: t.TempDir()})

	_, err = service.CheckQR(context.Background(), login.CheckQRRequest{UUID: "mock-not-exists"})
	if !errors.Is(err, login.ErrLoginStateNotFound) {
		t.Fatalf("不存在 uuid 错误 = %v，期望 ErrLoginStateNotFound", err)
	}

	imported, err := service.Import62Data(context.Background(), login.Import62DataRequest{
		Data62:     "unsupported-checkqr-62",
		DeviceID:   "unsupported-checkqr-iphone",
		DeviceName: "非二维码登录态设备",
		Wxid:       "wxid_checkqr_unsupported",
	})
	if err != nil {
		t.Fatalf("导入 62 登录态失败：%v", err)
	}
	_, err = service.CheckQR(context.Background(), login.CheckQRRequest{UUID: imported.UUID})
	if !errors.Is(err, login.ErrUnsupportedLoginKind) {
		t.Fatalf("非二维码登录态错误 = %v，期望 ErrUnsupportedLoginKind", err)
	}
}
