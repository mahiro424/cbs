package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mahiro424/cbs/internal/config"
	"github.com/mahiro424/cbs/internal/storage"
)

func TestLoginStateJSONRoundTripPreservesFieldsAndKeyPlan(t *testing.T) {
	createdAt := time.Date(2026, 7, 6, 12, 0, 0, 123456789, time.UTC)
	checkedAt := createdAt.Add(time.Minute)
	state := storage.LoginState{
		UUID:       "mock-uuid-001",
		CacheKey:   "login:mock:mock-uuid-001",
		DeviceID:   "device-001",
		DeviceName: "样本设备",
		Type:       "ipad",
		Wxid:       "wxid_sample",
		Data62:     "mock-62-data",
		A16:        "mock-a16-data",
		Mode:       "mock",
		LoginKind:  "getqr_mock",
		QRStatus:   "waiting_scan",
		CheckCount: 2,
		CreatedAt:  createdAt,
		CheckedAt:  checkedAt,
		Protocol: map[string]any{
			"pack_kind": "hybrid_ecdh_ios_placeholder",
			"platform":  "ios",
		},
	}

	encoded, err := storage.EncodeLoginState(state)
	if err != nil {
		t.Fatalf("EncodeLoginState 返回错误：%v", err)
	}
	decoded, err := storage.DecodeLoginState(encoded)
	if err != nil {
		t.Fatalf("DecodeLoginState 返回错误：%v", err)
	}
	if decoded.UUID != state.UUID || decoded.CacheKey != state.CacheKey || decoded.Wxid != state.Wxid || decoded.DeviceName != state.DeviceName || decoded.CheckCount != state.CheckCount {
		t.Fatalf("decoded = %+v，期望保留核心字段", decoded)
	}
	if !decoded.CreatedAt.Equal(createdAt) || !decoded.CheckedAt.Equal(checkedAt) {
		t.Fatalf("decoded 时间 = %s / %s，期望保留纳秒时间", decoded.CreatedAt, decoded.CheckedAt)
	}
	if decoded.Protocol["pack_kind"] != "hybrid_ecdh_ios_placeholder" || decoded.Protocol["platform"] != "ios" {
		t.Fatalf("decoded.Protocol = %+v，期望保留协议摘要", decoded.Protocol)
	}

	if got := storage.LoginStateRedisKey(state.UUID); got != "login:state:mock-uuid-001" {
		t.Fatalf("LoginStateRedisKey = %s，期望 login:state:mock-uuid-001", got)
	}
	if got := storage.LoginStateCacheIndexKey(state.CacheKey); got != "login:index:cache:login:mock:mock-uuid-001" {
		t.Fatalf("LoginStateCacheIndexKey = %s，期望 cache 索引 key", got)
	}
	if got := storage.LoginStateWxidIndexKey(state.Wxid); got != "login:index:wxid:wxid_sample" {
		t.Fatalf("LoginStateWxidIndexKey = %s，期望 wxid 索引 key", got)
	}

	m := decoded.ToMap()
	if m["uuid"] != state.UUID || m["cache_key"] != state.CacheKey || m["created_at"] == "" || m["checked_at"] == "" {
		t.Fatalf("ToMap = %+v，期望输出兼容 HTTP 响应字段", m)
	}
}

func TestMemoryLoginStateStoreSavesReadsAndUpdatesByIndexes(t *testing.T) {
	store := storage.NewMemoryLoginStateStore()
	ctx := context.Background()
	state := storage.LoginState{
		UUID:         "mock-uuid-002",
		CacheKey:     "login:mock:mock-uuid-002",
		Wxid:         "wxid_store",
		Mode:         "mock",
		LoginKind:    "a16_mock",
		SessionState: "initialized",
		CreatedAt:    time.Date(2026, 7, 6, 13, 0, 0, 0, time.UTC),
	}
	if err := store.Save(ctx, state); err != nil {
		t.Fatalf("Save 返回错误：%v", err)
	}

	byUUID, ok, err := store.Get(ctx, "mock-uuid-002", "")
	if err != nil || !ok || byUUID.CacheKey != state.CacheKey || byUUID.Wxid != state.Wxid {
		t.Fatalf("按 uuid 读取 = %+v / %v / %v，期望读回登录态", byUUID, ok, err)
	}
	byCache, ok, err := store.Get(ctx, "", state.CacheKey)
	if err != nil || !ok || byCache.UUID != state.UUID {
		t.Fatalf("按 cache_key 读取 = %+v / %v / %v，期望读回登录态", byCache, ok, err)
	}
	byWxid, ok, err := store.GetByWxid(ctx, state.Wxid)
	if err != nil || !ok || byWxid.UUID != state.UUID {
		t.Fatalf("按 wxid 读取 = %+v / %v / %v，期望读回登录态", byWxid, ok, err)
	}

	state.HeartbeatStatus = "alive"
	state.HeartbeatCount = 3
	if err := store.Save(ctx, state); err != nil {
		t.Fatalf("更新 Save 返回错误：%v", err)
	}
	updated, ok, err := store.GetByWxid(ctx, state.Wxid)
	if err != nil || !ok || updated.HeartbeatStatus != "alive" || updated.HeartbeatCount != 3 {
		t.Fatalf("更新后登录态 = %+v / %v / %v，期望覆盖旧状态", updated, ok, err)
	}
}

func TestLoginStateStoreFactorySelectsConfiguredBackend(t *testing.T) {
	memoryStore, memoryMode, err := storage.NewLoginStateStoreFromConfig(config.Config{LoginStateStore: ""})
	if err != nil || memoryMode != "memory" {
		t.Fatalf("默认 store = %T / %s / %v，期望 memory 且无错误", memoryStore, memoryMode, err)
	}
	if _, ok := memoryStore.(*storage.MemoryLoginStateStore); !ok {
		t.Fatalf("默认 store 类型 = %T，期望 MemoryLoginStateStore", memoryStore)
	}

	redisStore, redisMode, err := storage.NewLoginStateStoreFromConfig(config.Config{LoginStateStore: "redis", RedisLink: "127.0.0.1:6379", RedisDBNum: 7})
	if err != nil || redisMode != "redis" {
		t.Fatalf("Redis store = %T / %s / %v，期望 redis 且无配置错误", redisStore, redisMode, err)
	}
	if _, ok := redisStore.(*storage.RedisLoginStateStore); !ok {
		t.Fatalf("Redis store 类型 = %T，期望 RedisLoginStateStore", redisStore)
	}

	invalidStore, invalidMode, err := storage.NewLoginStateStoreFromConfig(config.Config{LoginStateStore: "sqlite"})
	if !errors.Is(err, storage.ErrLoginStateStoreConfig) || invalidMode != "sqlite" {
		t.Fatalf("非法 store = %T / %s / %v，期望 ErrLoginStateStoreConfig", invalidStore, invalidMode, err)
	}
	saveErr := invalidStore.Save(context.Background(), storage.LoginState{UUID: "invalid-store"})
	if !errors.Is(saveErr, storage.ErrLoginStateStoreConfig) {
		t.Fatalf("非法 store Save 错误 = %v，期望 ErrLoginStateStoreConfig", saveErr)
	}
}
