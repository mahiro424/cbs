package login

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mahiro424/cbs/internal/network"
	protocolpkg "github.com/mahiro424/cbs/internal/protocol"
	"github.com/mahiro424/cbs/internal/storage"
)

var (
	ErrProtocolPack = errors.New("login protocol pack failed")
	ErrSamplePath   = errors.New("login sample path failed")
	ErrSampleWrite  = errors.New("login sample write failed")
	ErrStateStore   = errors.New("login state store failed")
)

type Dependencies struct {
	States    storage.LoginStateStore
	Network   network.Client
	SampleDir string
	Now       func() time.Time
}

type Service struct {
	states    storage.LoginStateStore
	network   network.Client
	sampleDir string
	now       func() time.Time
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	states := deps.States
	if states == nil {
		states = storage.NewMemoryLoginStateStore()
	}
	netClient := deps.Network
	if netClient == nil {
		netClient, _, _ = network.NewClient(network.Config{})
	}
	return &Service{
		states:    states,
		network:   netClient,
		sampleDir: deps.SampleDir,
		now:       now,
	}
}

type GetQRRequest struct {
	DeviceID   string
	DeviceName string
	Type       string
	Proxy      any
}

type GetQRResult struct {
	Mode         string
	UUID         string
	CacheKey     string
	DeviceID     string
	DeviceName   string
	Type         string
	QRURL        string
	QRStatus     string
	Protocol     map[string]any
	Network      map[string]any
	State        storage.LoginState
	SamplePath   string
	MockResponse map[string]any
	Stages       []string
}

type Import62DataRequest struct {
	Data62     string
	DeviceID   string
	DeviceName string
	Wxid       string
	Proxy      any
}

type ImportA16DataRequest struct {
	A16        string
	DeviceID   string
	DeviceName string
	Wxid       string
	Proxy      any
}

type ImportResult struct {
	Mode         string
	UUID         string
	CacheKey     string
	DeviceID     string
	DeviceName   string
	Type         string
	Wxid         string
	LoginKind    string
	Protocol     map[string]any
	Network      map[string]any
	State        storage.LoginState
	SamplePath   string
	MockResponse map[string]any
	Stages       []string
}

func (r GetQRResult) ResponseData() map[string]any {
	data := map[string]any{
		"mode":        r.Mode,
		"uuid":        r.UUID,
		"qr_url":      r.QRURL,
		"cache_key":   r.CacheKey,
		"device_id":   r.DeviceID,
		"device_name": r.DeviceName,
		"type":        r.Type,
		"protocol":    r.Protocol,
		"network":     r.Network,
		"login_state": r.State.ToMap(),
		"sample_path": r.SamplePath,
		"stages":      r.Stages,
	}
	for key, value := range r.MockResponse {
		data[key] = value
	}
	return data
}

func (r ImportResult) ResponseData() map[string]any {
	data := map[string]any{
		"mode":        r.Mode,
		"uuid":        r.UUID,
		"cache_key":   r.CacheKey,
		"device_id":   r.DeviceID,
		"device_name": r.DeviceName,
		"type":        r.Type,
		"protocol":    r.Protocol,
		"network":     r.Network,
		"login_state": r.State.ToMap(),
		"sample_path": r.SamplePath,
		"stages":      r.Stages,
	}
	for key, value := range r.MockResponse {
		data[key] = value
	}
	return data
}

func (s *Service) GetQR(ctx context.Context, req GetQRRequest) (GetQRResult, error) {
	seed := strings.Join([]string{req.DeviceID, req.DeviceName, req.Type}, "|")
	if strings.Trim(seed, "|") == "" {
		seed = "anonymous-device"
	}
	sum := sha256.Sum256([]byte(seed))
	uuid := "mock-" + hex.EncodeToString(sum[:])[:24]
	deviceID := strings.TrimSpace(req.DeviceID)
	if deviceID == "" {
		deviceID = "mock-device"
	}
	deviceName := strings.TrimSpace(req.DeviceName)
	if deviceName == "" {
		deviceName = "mock-device-name"
	}
	deviceType := strings.TrimSpace(req.Type)
	if deviceType == "" {
		deviceType = "ipad"
	}
	cacheKey := "login:mock:" + uuid
	hybrid, _, err := protocolpkg.HybridECDHPackIOS(protocolpkg.HybridRequest{
		Operation: "Login.GetQR",
		Payload:   []byte(seed),
		DeviceID:  deviceID,
		LoginKind: "getqr_mock",
	})
	if err != nil {
		return GetQRResult{}, fmt.Errorf("%w: %v", ErrProtocolPack, err)
	}
	protocol := protocolTraceFromHybrid(hybrid, "getqr_mock")
	networkTrace, err := s.sendNetwork(ctx, "Login.GetQR", "getqr_mock", hybrid.Platform, hybrid, map[string]string{
		"device_id": deviceID,
		"type":      deviceType,
	})
	if err != nil {
		return GetQRResult{}, err
	}
	mockResponse := map[string]any{
		"uuid":      uuid,
		"qr_url":    "mock://login/" + uuid,
		"status":    "waiting_scan",
		"qr_status": "waiting_scan",
	}
	state := storage.LoginState{
		UUID:       uuid,
		CacheKey:   cacheKey,
		DeviceID:   deviceID,
		DeviceName: deviceName,
		Type:       deviceType,
		Mode:       "mock",
		LoginKind:  "getqr_mock",
		QRStatus:   "waiting_scan",
		CreatedAt:  s.now().UTC(),
		Protocol:   protocol,
	}
	samplePath, err := sampleFilePath(s.sampleDir, uuid)
	if err != nil {
		return GetQRResult{}, fmt.Errorf("%w: %v", ErrSamplePath, err)
	}
	state.SamplePath = samplePath
	sample := map[string]any{
		"request": map[string]any{
			"device_id":   deviceID,
			"device_name": deviceName,
			"type":        deviceType,
		},
		"protocol":      protocol,
		"network":       networkTrace,
		"mock_response": mockResponse,
		"login_state":   state.ToMap(),
	}
	if err := writeSample(samplePath, sample); err != nil {
		return GetQRResult{}, fmt.Errorf("%w: %v", ErrSampleWrite, err)
	}
	if err := s.states.Save(ctx, state); err != nil {
		return GetQRResult{}, fmt.Errorf("%w: %v", ErrStateStore, err)
	}
	return GetQRResult{
		Mode:         "mock",
		UUID:         uuid,
		CacheKey:     cacheKey,
		DeviceID:     deviceID,
		DeviceName:   deviceName,
		Type:         deviceType,
		QRURL:        "mock://login/" + uuid,
		QRStatus:     "waiting_scan",
		Protocol:     protocol,
		Network:      networkTrace,
		State:        state,
		SamplePath:   samplePath,
		MockResponse: mockResponse,
		Stages: []string{
			"parse_request",
			"build_login_context",
			"prepare_device_profile",
			"hybrid_ecdh_ios_pack_placeholder",
			"mock_network_response",
			"persist_login_state",
			"write_sample",
		},
	}, nil
}

func (s *Service) Import62Data(ctx context.Context, req Import62DataRequest) (ImportResult, error) {
	deviceID := strings.TrimSpace(req.DeviceID)
	if deviceID == "" {
		deviceID = "mock-iphone"
	}
	deviceName := strings.TrimSpace(req.DeviceName)
	if deviceName == "" {
		deviceName = "mock-iphone-name"
	}
	wxid := strings.TrimSpace(req.Wxid)
	if wxid == "" {
		wxid = "wxid_mock_data62"
	}
	request := map[string]any{
		"data62":      req.Data62,
		"device_id":   deviceID,
		"device_name": deviceName,
		"type":        "iphone",
		"wxid":        wxid,
	}
	if req.Proxy != nil {
		request["proxy"] = req.Proxy
	}
	return s.importMock(ctx, importSpec{
		SeedParts:  []string{"data62_mock", req.Data62, deviceID, deviceName, wxid},
		Request:    request,
		DeviceID:   deviceID,
		DeviceName: deviceName,
		Type:       "iphone",
		Wxid:       wxid,
		Data62:     req.Data62,
		LoginKind:  "data62_mock",
		Operation:  "Login.62data",
		Platform:   "ios",
		Payload:    req.Data62,
		MockResponse: map[string]any{
			"status": "mock_login_ready",
			"wxid":   wxid,
		},
		Stages: []string{
			"parse_request",
			"build_login_context",
			"load_62data_fixture",
			"hybrid_ecdh_ios_pack_placeholder",
			"mock_network_response",
			"persist_login_state",
			"write_sample",
		},
	})
}

func (s *Service) ImportA16Data(ctx context.Context, req ImportA16DataRequest) (ImportResult, error) {
	deviceID := strings.TrimSpace(req.DeviceID)
	if deviceID == "" {
		deviceID = "mock-android"
	}
	deviceName := strings.TrimSpace(req.DeviceName)
	if deviceName == "" {
		deviceName = "mock-android-name"
	}
	wxid := strings.TrimSpace(req.Wxid)
	if wxid == "" {
		wxid = "wxid_mock_a16"
	}
	request := map[string]any{
		"a16":         req.A16,
		"device_id":   deviceID,
		"device_name": deviceName,
		"type":        "android",
		"wxid":        wxid,
	}
	if req.Proxy != nil {
		request["proxy"] = req.Proxy
	}
	return s.importMock(ctx, importSpec{
		SeedParts:  []string{"a16_mock", req.A16, deviceID, deviceName, wxid},
		Request:    request,
		DeviceID:   deviceID,
		DeviceName: deviceName,
		Type:       "android",
		Wxid:       wxid,
		A16:        req.A16,
		LoginKind:  "a16_mock",
		Operation:  "Login.A16Data",
		Platform:   "android",
		Payload:    req.A16,
		MockResponse: map[string]any{
			"status": "mock_login_ready",
			"wxid":   wxid,
		},
		Stages: []string{
			"parse_request",
			"build_login_context",
			"load_a16_fixture",
			"hybrid_ecdh_android_pack_placeholder",
			"mock_network_response",
			"persist_login_state",
			"write_sample",
		},
	})
}

type importSpec struct {
	SeedParts    []string
	Request      map[string]any
	DeviceID     string
	DeviceName   string
	Type         string
	Wxid         string
	Data62       string
	A16          string
	LoginKind    string
	Operation    string
	Platform     string
	Payload      string
	MockResponse map[string]any
	Stages       []string
}

func (s *Service) importMock(ctx context.Context, spec importSpec) (ImportResult, error) {
	seed := strings.Join(spec.SeedParts, "|")
	if strings.Trim(seed, "|") == "" {
		seed = spec.LoginKind + "|anonymous-device"
	}
	sum := sha256.Sum256([]byte(seed))
	uuid := "mock-" + hex.EncodeToString(sum[:])[:24]
	cacheKey := "login:mock:" + uuid
	operation := strings.TrimSpace(spec.Operation)
	if operation == "" {
		operation = spec.LoginKind
	}
	hybrid, _, err := protocolpkg.HybridECDHPack(protocolpkg.HybridRequest{
		Platform:  spec.Platform,
		Operation: operation,
		Payload:   []byte(spec.Payload),
		DeviceID:  spec.DeviceID,
		LoginKind: spec.LoginKind,
	})
	if err != nil {
		return ImportResult{}, fmt.Errorf("%w: %v", ErrProtocolPack, err)
	}
	protocol := protocolTraceFromHybrid(hybrid, spec.LoginKind)
	networkTrace, err := s.sendNetwork(ctx, operation, spec.LoginKind, hybrid.Platform, hybrid, map[string]string{
		"device_id": spec.DeviceID,
		"type":      spec.Type,
		"wxid":      spec.Wxid,
	})
	if err != nil {
		return ImportResult{}, err
	}
	mockResponse := map[string]any{
		"uuid":      uuid,
		"cache_key": cacheKey,
	}
	for key, value := range spec.MockResponse {
		mockResponse[key] = value
	}
	state := storage.LoginState{
		UUID:       uuid,
		CacheKey:   cacheKey,
		DeviceID:   spec.DeviceID,
		DeviceName: spec.DeviceName,
		Type:       spec.Type,
		Wxid:       spec.Wxid,
		Data62:     spec.Data62,
		A16:        spec.A16,
		Mode:       "mock",
		LoginKind:  spec.LoginKind,
		CreatedAt:  s.now().UTC(),
		Protocol:   protocol,
	}
	samplePath, err := sampleFilePath(s.sampleDir, uuid)
	if err != nil {
		return ImportResult{}, fmt.Errorf("%w: %v", ErrSamplePath, err)
	}
	state.SamplePath = samplePath
	sample := map[string]any{
		"request":       spec.Request,
		"protocol":      protocol,
		"network":       networkTrace,
		"mock_response": mockResponse,
		"login_state":   state.ToMap(),
	}
	if err := writeSample(samplePath, sample); err != nil {
		return ImportResult{}, fmt.Errorf("%w: %v", ErrSampleWrite, err)
	}
	if err := s.states.Save(ctx, state); err != nil {
		return ImportResult{}, fmt.Errorf("%w: %v", ErrStateStore, err)
	}
	return ImportResult{
		Mode:         "mock",
		UUID:         uuid,
		CacheKey:     cacheKey,
		DeviceID:     spec.DeviceID,
		DeviceName:   spec.DeviceName,
		Type:         spec.Type,
		Wxid:         spec.Wxid,
		LoginKind:    spec.LoginKind,
		Protocol:     protocol,
		Network:      networkTrace,
		State:        state,
		SamplePath:   samplePath,
		MockResponse: mockResponse,
		Stages:       spec.Stages,
	}, nil
}

func (s *Service) sendNetwork(ctx context.Context, operation string, loginKind string, platform string, hybrid protocolpkg.HybridPacket, metadata map[string]string) (map[string]any, error) {
	resp, err := s.network.Send(ctx, network.Request{
		Operation: operation,
		LoginKind: loginKind,
		Platform:  platform,
		Payload:   []byte(hybrid.PackedHex),
		Metadata:  metadata,
	})
	if err != nil {
		return nil, err
	}
	return resp.ToMap(), nil
}

func protocolTraceFromHybrid(hybrid protocolpkg.HybridPacket, loginKind string) map[string]any {
	return map[string]any{
		"pack_kind":      hybrid.PackKind,
		"platform":       hybrid.Platform,
		"login_kind":     loginKind,
		"operation":      hybrid.Operation,
		"payload_sha256": hybrid.PayloadSHA256,
		"payload_length": hybrid.PayloadLength,
		"input_length":   hybrid.PayloadLength,
		"packed_hex":     hybrid.PackedHex,
		"debug":          hybrid.Debug,
	}
}

func sampleFilePath(sampleDir, uuid string) (string, error) {
	if strings.TrimSpace(sampleDir) == "" {
		sampleDir = ".scratch/samples"
	}
	absDir, err := filepath.Abs(sampleDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(absDir, uuid+".json"), nil
}

func writeSample(path string, sample map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(sample, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(payload, '\n'), 0o644)
}
