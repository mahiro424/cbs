package network_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/mahiro424/cbs/internal/network"
)

func TestMockClientReturnsObservableNetworkSummary(t *testing.T) {
	client, mode, err := network.NewClient(network.Config{Mode: ""})
	if err != nil || mode != "mock" {
		t.Fatalf("NewClient = %T / %s / %v，期望默认 mock", client, mode, err)
	}
	payload := []byte("mock-network-payload")
	resp, err := client.Send(context.Background(), network.Request{
		Operation: "Login.GetQR",
		LoginKind: "getqr_mock",
		Platform:  "ios",
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("mock Send 返回错误：%v", err)
	}
	sum := sha256.Sum256(payload)
	if resp.Mode != "mock" || resp.Operation != "Login.GetQR" || resp.LoginKind != "getqr_mock" || resp.Platform != "ios" {
		t.Fatalf("mock response = %+v，期望保留 mode/operation/login_kind/platform", resp)
	}
	if resp.Stage != "mock_network_response" || resp.PayloadSHA256 != hex.EncodeToString(sum[:]) || resp.PayloadLength != len(payload) {
		t.Fatalf("mock response 摘要 = %+v，期望稳定 payload 摘要", resp)
	}
	m := resp.ToMap()
	if m["mode"] != "mock" || m["stage"] != "mock_network_response" || m["payload_sha256"] == "" {
		t.Fatalf("ToMap = %+v，期望输出可落盘摘要", m)
	}
}

func TestRealClientReturnsStableNotReadyError(t *testing.T) {
	client, mode, err := network.NewClient(network.Config{Mode: "real"})
	if err != nil || mode != "real" {
		t.Fatalf("NewClient = %T / %s / %v，期望 real client", client, mode, err)
	}
	_, err = client.Send(context.Background(), network.Request{Operation: "Login.GetQR", LoginKind: "getqr_mock"})
	if !errors.Is(err, network.ErrRealNetworkNotReady) {
		t.Fatalf("real Send 错误 = %v，期望 ErrRealNetworkNotReady", err)
	}
}

func TestNetworkModeRejectsUnknownValue(t *testing.T) {
	client, mode, err := network.NewClient(network.Config{Mode: "mmttl"})
	if !errors.Is(err, network.ErrNetworkConfig) || mode != "mmttl" || client == nil {
		t.Fatalf("非法模式 client/mode/err = %T / %s / %v，期望 ErrNetworkConfig 且返回错误 client", client, mode, err)
	}
	_, sendErr := client.Send(context.Background(), network.Request{Operation: "Login.GetQR"})
	if !errors.Is(sendErr, network.ErrNetworkConfig) {
		t.Fatalf("错误 client Send = %v，期望 ErrNetworkConfig", sendErr)
	}
}
