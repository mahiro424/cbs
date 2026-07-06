package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mahiro424/cbs/internal/config"
	"github.com/mahiro424/cbs/internal/httpapi"
)

func TestSwaggerRoutesAreLoaded(t *testing.T) {
	routes := httpapi.AllRoutes()
	if len(routes) != 142 {
		t.Fatalf("路由数量 = %d，期望 142", len(routes))
	}
	for _, path := range []string{"/Login/GetQR", "/Login/62data", "/Login/A16Data", "/Msg/SendTxt", "/Friend/Search", "/Group/CreateChatRoom", "/Tools/setproxy"} {
		if !httpapi.HasRoute("POST", path) {
			t.Fatalf("缺少 Swagger 路由：POST %s", path)
		}
	}
}

func TestUnknownRouteReturnsUnifiedJSON404(t *testing.T) {
	h := httpapi.NewServer(config.Default())
	req := httptest.NewRequest(http.MethodPost, "/not-exists", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d，期望 404", rec.Code)
	}
	var body httpapi.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v，正文：%s", err, rec.Body.String())
	}
	if body.Success || body.Code != "route_not_found" {
		t.Fatalf("响应 = %+v，期望 route_not_found", body)
	}
}

func TestImplementedSwaggerRouteReturnsNotImplementedEnvelope(t *testing.T) {
	h := httpapi.NewServer(config.Default())
	req := httptest.NewRequest(http.MethodPost, "/Msg/SendTxt", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200", rec.Code)
	}
	var body httpapi.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v", err)
	}
	if body.Success || body.Code != "not_implemented" {
		t.Fatalf("响应 = %+v，期望 not_implemented", body)
	}
}

func TestLoginGetQRMockPathReturnsStableEnvelope(t *testing.T) {
	h := httpapi.NewServer(config.Default())
	payload := `{"DeviceID":"dev-001","DeviceName":"测试设备","Type":"ipad"}`
	req := httptest.NewRequest(http.MethodPost, "/Login/GetQR", strings.NewReader(payload))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200，正文：%s", rec.Code, rec.Body.String())
	}
	var body httpapi.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v", err)
	}
	if !body.Success || body.Code != "ok" {
		t.Fatalf("响应 = %+v，期望 ok", body)
	}
	data, ok := body.Data.(map[string]any)
	if !ok {
		t.Fatalf("data 类型 = %T，期望对象", body.Data)
	}
	if data["mode"] != "mock" {
		t.Fatalf("mode = %v，期望 mock", data["mode"])
	}
	if data["uuid"] == "" || data["qr_url"] == "" || data["cache_key"] == "" {
		t.Fatalf("缺少二维码 mock 必要字段：%+v", data)
	}
}
