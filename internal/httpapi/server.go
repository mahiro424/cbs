package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mahiro424/cbs/internal/config"
	"github.com/mahiro424/cbs/internal/storage"
)

type Server struct {
	cfg       config.Config
	routes    map[string]Route
	pathIndex map[string]struct{}
	states    *loginStateStore
	seq       atomic.Uint64
}

func NewServer(cfg config.Config) *Server {
	s := &Server{cfg: cfg, routes: make(map[string]Route), pathIndex: make(map[string]struct{}), states: newLoginStateStore()}
	for _, route := range AllRoutes() {
		method := strings.ToUpper(route.Method)
		route.Method = method
		s.routes[method+" "+route.Path] = route
		s.pathIndex[route.Path] = struct{}{}
	}
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := s.requestID(r)
	if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
		s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "服务正常", RequestID: requestID, Data: map[string]any{
			"app":     s.cfg.AppName,
			"listen":  s.cfg.ListenAddress(),
			"redis":   storage.CheckRedis(context.Background(), s.cfg),
			"routes":  len(AllRoutes()),
			"mode":    "mock-first",
			"version": "0.1.0",
		}})
		return
	}
	key := strings.ToUpper(r.Method) + " " + r.URL.Path
	route, ok := s.routes[key]
	if !ok {
		if _, exists := s.pathIndex[r.URL.Path]; exists {
			s.write(w, http.StatusMethodNotAllowed, Envelope{Success: false, Code: "method_not_allowed", Message: "请求方法不匹配", RequestID: requestID, Data: map[string]any{"path": r.URL.Path, "method": r.Method}})
			return
		}
		s.write(w, http.StatusNotFound, Envelope{Success: false, Code: "route_not_found", Message: "路由不存在", RequestID: requestID, Data: map[string]any{"path": r.URL.Path, "method": r.Method}})
		return
	}
	if route.Path == "/Login/GetQR" {
		s.handleLoginGetQR(w, r, requestID)
		return
	}
	if route.Path == "/Login/GetCacheInfo" {
		s.handleLoginGetCacheInfo(w, r, requestID)
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: false, Code: "not_implemented", Message: "接口已接入但未实现", RequestID: requestID, Data: map[string]any{"path": route.Path, "method": route.Method, "module": route.Module, "operation": route.Operation}})
}

func (s *Server) requestID(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Request-Id")); v != "" {
		return v
	}
	n := s.seq.Add(1)
	return fmt.Sprintf("req-%d-%06d", time.Now().UnixNano(), n)
}

func (s *Server) write(w http.ResponseWriter, status int, body Envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type getQRRequest struct {
	DeviceID   string `json:"DeviceID"`
	DeviceName string `json:"DeviceName"`
	Type       string `json:"Type"`
	Proxy      any    `json:"Proxy,omitempty"`
}

func (s *Server) handleLoginGetQR(w http.ResponseWriter, r *http.Request, requestID string) {
	var req getQRRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: err.Error(), RequestID: requestID})
		return
	}
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
	protocol := map[string]any{
		"pack_kind":      "hybrid_ecdh_ios_placeholder",
		"platform":       "ios",
		"payload_sha256": hex.EncodeToString(sum[:]),
		"input_length":   len(seed),
	}
	mockResponse := map[string]any{
		"uuid":   uuid,
		"qr_url": "mock://login/" + uuid,
		"status": "waiting_scan",
	}
	state := loginState{
		UUID:       uuid,
		CacheKey:   cacheKey,
		DeviceID:   deviceID,
		DeviceName: deviceName,
		Type:       deviceType,
		Mode:       "mock",
		CreatedAt:  time.Now().UTC(),
		Protocol:   protocol,
	}
	samplePath, err := sampleFilePath(s.cfg.SampleDir, uuid)
	if err != nil {
		s.write(w, http.StatusInternalServerError, Envelope{Success: false, Code: "sample_path_error", Message: err.Error(), RequestID: requestID})
		return
	}
	state.SamplePath = samplePath
	sample := map[string]any{
		"request": map[string]any{
			"device_id":   deviceID,
			"device_name": deviceName,
			"type":        deviceType,
		},
		"protocol":      protocol,
		"mock_response": mockResponse,
		"login_state":   state.toMap(),
	}
	if err := writeSample(samplePath, sample); err != nil {
		s.write(w, http.StatusInternalServerError, Envelope{Success: false, Code: "sample_write_error", Message: err.Error(), RequestID: requestID})
		return
	}
	s.states.Save(state)

	data := map[string]any{
		"mode":        "mock",
		"uuid":        uuid,
		"qr_url":      mockResponse["qr_url"],
		"cache_key":   cacheKey,
		"device_id":   deviceID,
		"device_name": deviceName,
		"type":        deviceType,
		"protocol":    protocol,
		"login_state": state.toMap(),
		"sample_path": samplePath,
		"stages": []string{
			"parse_request",
			"build_login_context",
			"prepare_device_profile",
			"hybrid_ecdh_ios_pack_placeholder",
			"mock_network_response",
			"persist_login_state",
			"write_sample",
		},
	}
	s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "mock 二维码链路已跑通", RequestID: requestID, Data: data})
}

func (s *Server) handleLoginGetCacheInfo(w http.ResponseWriter, r *http.Request, requestID string) {
	uuid := strings.TrimSpace(r.URL.Query().Get("uuid"))
	cacheKey := strings.TrimSpace(r.URL.Query().Get("cache_key"))
	if uuid == "" && cacheKey == "" {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 uuid 或 cache_key", RequestID: requestID})
		return
	}
	state, ok := s.states.Get(uuid, cacheKey)
	if !ok {
		s.write(w, http.StatusOK, Envelope{Success: false, Code: "cache_not_found", Message: "未找到登录态", RequestID: requestID})
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "已读取登录态", RequestID: requestID, Data: state.toMap()})
}

func decodeJSON(body io.Reader, out any) error {
	if body == nil {
		return nil
	}
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("JSON 请求体无效：%w", err)
	}
	return nil
}

type loginState struct {
	UUID       string
	CacheKey   string
	DeviceID   string
	DeviceName string
	Type       string
	Mode       string
	SamplePath string
	CreatedAt  time.Time
	Protocol   map[string]any
}

func (s loginState) toMap() map[string]any {
	return map[string]any{
		"uuid":        s.UUID,
		"cache_key":   s.CacheKey,
		"device_id":   s.DeviceID,
		"device_name": s.DeviceName,
		"type":        s.Type,
		"mode":        s.Mode,
		"sample_path": s.SamplePath,
		"created_at":  s.CreatedAt.Format(time.RFC3339Nano),
		"protocol":    s.Protocol,
	}
}

type loginStateStore struct {
	mu      sync.RWMutex
	byUUID  map[string]loginState
	byCache map[string]loginState
}

func newLoginStateStore() *loginStateStore {
	return &loginStateStore{byUUID: make(map[string]loginState), byCache: make(map[string]loginState)}
}

func (s *loginStateStore) Save(state loginState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byUUID[state.UUID] = state
	s.byCache[state.CacheKey] = state
}

func (s *loginStateStore) Get(uuid, cacheKey string) (loginState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if uuid != "" {
		state, ok := s.byUUID[uuid]
		return state, ok
	}
	state, ok := s.byCache[cacheKey]
	return state, ok
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
