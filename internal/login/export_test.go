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

func TestServiceExports62DataAndA16DataSamplesAndStore(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	client, _, err := network.NewClient(network.Config{Mode: "mock"})
	if err != nil {
		t.Fatalf("创建 mock network client 失败：%v", err)
	}
	currentTime := time.Date(2026, 7, 7, 1, 1, 0, 0, time.UTC)
	service := login.NewService(login.Dependencies{
		States:    store,
		Network:   client,
		SampleDir: t.TempDir(),
		Now:       func() time.Time { return currentTime },
	})

	login62, err := service.Import62Data(context.Background(), login.Import62DataRequest{
		Data62:     "service-export-62",
		DeviceID:   "service-export-iphone",
		DeviceName: "业务层 62 导出设备",
		Wxid:       "wxid_service_export_62",
	})
	if err != nil {
		t.Fatalf("导入 62 登录态失败：%v", err)
	}
	loginA16, err := service.ImportA16Data(context.Background(), login.ImportA16DataRequest{
		A16:        "service-export-a16",
		DeviceID:   "service-export-android",
		DeviceName: "业务层 A16 导出设备",
		Wxid:       "wxid_service_export_a16",
	})
	if err != nil {
		t.Fatalf("导入 A16 登录态失败：%v", err)
	}

	cases := []struct {
		name         string
		call         func() (login.ExportLoginDataResult, error)
		wantUUID     string
		wantWxid     string
		wantKind     string
		wantField    string
		wantPayload  string
		wantStage    string
		wantExported time.Time
	}{
		{
			name: "导出 62 数据",
			call: func() (login.ExportLoginDataResult, error) {
				currentTime = time.Date(2026, 7, 7, 1, 2, 3, 0, time.UTC)
				return service.Get62Data(context.Background(), login.ExportLoginDataRequest{Wxid: "wxid_service_export_62"})
			},
			wantUUID:     login62.UUID,
			wantWxid:     "wxid_service_export_62",
			wantKind:     "mock_62data",
			wantField:    "data62",
			wantPayload:  "service-export-62",
			wantStage:    "mock_export_62data",
			wantExported: time.Date(2026, 7, 7, 1, 2, 3, 0, time.UTC),
		},
		{
			name: "导出 A16 数据",
			call: func() (login.ExportLoginDataResult, error) {
				currentTime = time.Date(2026, 7, 7, 1, 3, 4, 0, time.UTC)
				return service.GetA16Data(context.Background(), login.ExportLoginDataRequest{Wxid: "wxid_service_export_a16"})
			},
			wantUUID:     loginA16.UUID,
			wantWxid:     "wxid_service_export_a16",
			wantKind:     "mock_a16data",
			wantField:    "a16",
			wantPayload:  "service-export-a16",
			wantStage:    "mock_export_a16data",
			wantExported: time.Date(2026, 7, 7, 1, 3, 4, 0, time.UTC),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.call()
			if err != nil {
				t.Fatalf("导出返回错误：%v", err)
			}
			if result.Mode != "mock" || result.UUID != tc.wantUUID || result.Wxid != tc.wantWxid || result.ExportKind != tc.wantKind {
				t.Fatalf("导出 result = %+v，期望同一登录态和导出类型", result)
			}
			if result.ResponseField != tc.wantField || result.Payload != tc.wantPayload || result.PayloadSize != len(tc.wantPayload) || !result.ExportedAt.Equal(tc.wantExported) {
				t.Fatalf("导出 payload = %+v，期望字段/长度/时间正确", result)
			}
			if result.State.LastExportKind != tc.wantKind || !result.State.LastExportAt.Equal(tc.wantExported) || result.State.SamplePath != result.SamplePath {
				t.Fatalf("导出登录态 = %+v，期望记录最近导出", result.State)
			}
			if !containsString(result.Stages, tc.wantStage) || result.Stages[len(result.Stages)-1] != "write_sample" {
				t.Fatalf("导出 stages = %+v，期望记录导出阶段", result.Stages)
			}

			stored, ok, err := store.Get(context.Background(), tc.wantUUID, "")
			if err != nil || !ok {
				t.Fatalf("按 uuid 读取导出登录态 = %+v / %v / %v，期望已保存", stored, ok, err)
			}
			if stored.LastExportKind != tc.wantKind || stored.SamplePath != result.SamplePath {
				t.Fatalf("存储登录态 = %+v，期望反映最近导出", stored)
			}

			raw, err := os.ReadFile(result.SamplePath)
			if err != nil {
				t.Fatalf("读取导出样本失败：%v", err)
			}
			var sample map[string]any
			if err := json.Unmarshal(raw, &sample); err != nil {
				t.Fatalf("导出样本不是 JSON：%v", err)
			}
			for _, key := range []string{"request", "mock_response", "login_state"} {
				if _, ok := sample[key]; !ok {
					t.Fatalf("导出样本缺少字段 %s：%+v", key, sample)
				}
			}
			request := sample["request"].(map[string]any)
			if request["wxid"] != tc.wantWxid {
				t.Fatalf("导出样本 request = %+v，期望 wxid=%s", request, tc.wantWxid)
			}
			mockResponse := sample["mock_response"].(map[string]any)
			if mockResponse["export_kind"] != tc.wantKind || mockResponse[tc.wantField] != tc.wantPayload || mockResponse["payload_size"] != float64(len(tc.wantPayload)) {
				t.Fatalf("导出样本 mock_response = %+v，期望导出摘要", mockResponse)
			}

			responseData := result.ResponseData()
			for _, key := range []string{"mode", "uuid", "cache_key", "wxid", "export_kind", "exported_at", "payload_size", tc.wantField, "login_state", "sample_path", "stages"} {
				if _, ok := responseData[key]; !ok {
					t.Fatalf("导出响应缺少字段 %s：%+v", key, responseData)
				}
			}
		})
	}
}

func TestServiceExportReportsMissingAndUnsupportedLoginKind(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	client, _, err := network.NewClient(network.Config{Mode: "mock"})
	if err != nil {
		t.Fatalf("创建 mock network client 失败：%v", err)
	}
	service := login.NewService(login.Dependencies{States: store, Network: client, SampleDir: t.TempDir()})

	_, err = service.Get62Data(context.Background(), login.ExportLoginDataRequest{Wxid: "wxid_missing_export"})
	if !errors.Is(err, login.ErrWxidLoginStateNotFound) {
		t.Fatalf("不存在 wxid 错误 = %v，期望 ErrWxidLoginStateNotFound", err)
	}

	login62, err := service.Import62Data(context.Background(), login.Import62DataRequest{
		Data62:     "wrong-export-62",
		DeviceID:   "wrong-export-iphone",
		DeviceName: "错误导出类型设备",
		Wxid:       "wxid_wrong_export_62",
	})
	if err != nil {
		t.Fatalf("导入 62 登录态失败：%v", err)
	}
	result, err := service.GetA16Data(context.Background(), login.ExportLoginDataRequest{Wxid: "wxid_wrong_export_62"})
	if !errors.Is(err, login.ErrUnsupportedLoginKind) {
		t.Fatalf("错误导出类型 = %v，期望 ErrUnsupportedLoginKind", err)
	}
	if result.State.UUID != login62.UUID || result.State.LoginKind != "data62_mock" {
		t.Fatalf("错误导出 result = %+v，期望返回原登录态", result)
	}
}
