package storage

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// LoginState 是登录态在 storage 层的稳定结构。
// 后续 Redis backend 应复用此结构做 JSON 序列化和索引维护。
type LoginState struct {
	UUID            string         `json:"uuid"`
	CacheKey        string         `json:"cache_key"`
	DeviceID        string         `json:"device_id"`
	DeviceName      string         `json:"device_name"`
	Type            string         `json:"type"`
	Wxid            string         `json:"wxid"`
	Data62          string         `json:"data62,omitempty"`
	A16             string         `json:"a16,omitempty"`
	Mode            string         `json:"mode"`
	LoginKind       string         `json:"login_kind"`
	QRStatus        string         `json:"qr_status"`
	CheckCount      int            `json:"check_count"`
	SessionState    string         `json:"session_state"`
	HeartbeatStatus string         `json:"heartbeat_status"`
	HeartbeatCount  int            `json:"heartbeat_count"`
	SamplePath      string         `json:"sample_path"`
	LogoutStatus    string         `json:"logout_status"`
	CreatedAt       time.Time      `json:"created_at"`
	CheckedAt       time.Time      `json:"checked_at,omitempty"`
	LastInitAt      time.Time      `json:"last_init_at,omitempty"`
	LastHeartbeatAt time.Time      `json:"last_heartbeat_at,omitempty"`
	LastExportKind  string         `json:"last_export_kind"`
	LastExportAt    time.Time      `json:"last_export_at,omitempty"`
	LoggedOutAt     time.Time      `json:"logged_out_at,omitempty"`
	Protocol        map[string]any `json:"protocol,omitempty"`
}

func EncodeLoginState(state LoginState) ([]byte, error) {
	return json.Marshal(state)
}

func DecodeLoginState(payload []byte) (LoginState, error) {
	var state LoginState
	if err := json.Unmarshal(payload, &state); err != nil {
		return LoginState{}, err
	}
	return state, nil
}

// LoginStateStore 是登录态持久化后端的公共接口。
// HTTP、mock-first 链路和后续真实 Redis backend 都应依赖该接口，而不是依赖具体实现。
type LoginStateStore interface {
	Save(ctx context.Context, state LoginState) error
	Get(ctx context.Context, uuid string, cacheKey string) (LoginState, bool, error)
	GetByWxid(ctx context.Context, wxid string) (LoginState, bool, error)
}

func LoginStateRedisKey(uuid string) string {
	return "login:state:" + strings.TrimSpace(uuid)
}

func LoginStateCacheIndexKey(cacheKey string) string {
	return "login:index:cache:" + strings.TrimSpace(cacheKey)
}

func LoginStateWxidIndexKey(wxid string) string {
	return "login:index:wxid:" + strings.TrimSpace(wxid)
}

func (s LoginState) ToMap() map[string]any {
	m := map[string]any{
		"uuid":             s.UUID,
		"cache_key":        s.CacheKey,
		"device_id":        s.DeviceID,
		"device_name":      s.DeviceName,
		"type":             s.Type,
		"wxid":             s.Wxid,
		"mode":             s.Mode,
		"login_kind":       s.LoginKind,
		"qr_status":        s.QRStatus,
		"check_count":      s.CheckCount,
		"session_state":    s.SessionState,
		"heartbeat_status": s.HeartbeatStatus,
		"heartbeat_count":  s.HeartbeatCount,
		"last_export_kind": s.LastExportKind,
		"logout_status":    s.LogoutStatus,
		"sample_path":      s.SamplePath,
		"created_at":       s.CreatedAt.Format(time.RFC3339Nano),
		"protocol":         s.Protocol,
	}
	if !s.CheckedAt.IsZero() {
		m["checked_at"] = s.CheckedAt.Format(time.RFC3339Nano)
	}
	if !s.LastInitAt.IsZero() {
		m["last_init_at"] = s.LastInitAt.Format(time.RFC3339Nano)
	}
	if !s.LastHeartbeatAt.IsZero() {
		m["last_heartbeat_at"] = s.LastHeartbeatAt.Format(time.RFC3339Nano)
	}
	if !s.LastExportAt.IsZero() {
		m["last_export_at"] = s.LastExportAt.Format(time.RFC3339Nano)
	}
	if !s.LoggedOutAt.IsZero() {
		m["logged_out_at"] = s.LoggedOutAt.Format(time.RFC3339Nano)
	}
	return m
}

// MemoryLoginStateStore 是 Redis backend 接入前的进程内登录态仓库。
type MemoryLoginStateStore struct {
	mu      sync.RWMutex
	byUUID  map[string]LoginState
	byCache map[string]LoginState
	byWxid  map[string]LoginState
}

func NewMemoryLoginStateStore() *MemoryLoginStateStore {
	return &MemoryLoginStateStore{byUUID: make(map[string]LoginState), byCache: make(map[string]LoginState), byWxid: make(map[string]LoginState)}
}

func (s *MemoryLoginStateStore) Save(ctx context.Context, state LoginState) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(state.UUID) != "" {
		s.byUUID[state.UUID] = state
	}
	if strings.TrimSpace(state.CacheKey) != "" {
		s.byCache[state.CacheKey] = state
	}
	if strings.TrimSpace(state.Wxid) != "" {
		s.byWxid[state.Wxid] = state
	}
	return nil
}

func (s *MemoryLoginStateStore) Get(ctx context.Context, uuid string, cacheKey string) (LoginState, bool, error) {
	if err := contextError(ctx); err != nil {
		return LoginState{}, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if strings.TrimSpace(uuid) != "" {
		state, ok := s.byUUID[uuid]
		return state, ok, nil
	}
	if strings.TrimSpace(cacheKey) == "" {
		return LoginState{}, false, nil
	}
	state, ok := s.byCache[cacheKey]
	return state, ok, nil
}

func (s *MemoryLoginStateStore) GetByWxid(ctx context.Context, wxid string) (LoginState, bool, error) {
	if err := contextError(ctx); err != nil {
		return LoginState{}, false, err
	}
	if strings.TrimSpace(wxid) == "" {
		return LoginState{}, false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.byWxid[wxid]
	return state, ok, nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
