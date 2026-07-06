package storage_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mahiro424/cbs/internal/config"
	"github.com/mahiro424/cbs/internal/storage"
)

func TestLoginStateStoresImplementSharedInterface(t *testing.T) {
	var _ storage.LoginStateStore = storage.NewMemoryLoginStateStore()
	var _ storage.LoginStateStore = storage.NewRedisLoginStateStore(storage.RedisLoginStateStoreConfig{})
}

func TestRedisLoginStateStoreSavesAndReadsByUUIDCacheAndWxid(t *testing.T) {
	fake := newFakeRedis(t, "secret")
	defer fake.Close()

	store := storage.NewRedisLoginStateStoreFromConfig(config.Config{
		RedisLink:  fake.Addr(),
		RedisPass:  "secret",
		RedisDBNum: 7,
	})
	ctx := context.Background()
	createdAt := time.Date(2026, 7, 6, 21, 30, 0, 123456789, time.UTC)
	state := storage.LoginState{
		UUID:            "mock-redis-uuid",
		CacheKey:        "login:mock:mock-redis-uuid",
		DeviceID:        "device-redis",
		DeviceName:      "Redis 样本设备",
		Type:            "ipad",
		Wxid:            "wxid_redis",
		Data62:          "redis-62-data",
		A16:             "redis-a16-data",
		Mode:            "mock",
		LoginKind:       "data62_mock",
		QRStatus:        "waiting_scan",
		CheckCount:      2,
		SessionState:    "initialized",
		HeartbeatStatus: "alive",
		HeartbeatCount:  3,
		LastExportKind:  "mock_62data",
		CreatedAt:       createdAt,
		CheckedAt:       createdAt.Add(time.Minute),
		Protocol: map[string]any{
			"pack_kind": "hybrid_ecdh_ios_placeholder",
			"platform":  "ios",
		},
	}

	if err := store.Save(ctx, state); err != nil {
		t.Fatalf("Save 返回错误：%v", err)
	}

	byUUID, ok, err := store.Get(ctx, state.UUID, "")
	if err != nil || !ok {
		t.Fatalf("按 uuid 读取 = %+v / %v / %v，期望读回登录态", byUUID, ok, err)
	}
	assertRedisStateRoundTrip(t, state, byUUID)

	byCache, ok, err := store.Get(ctx, "", state.CacheKey)
	if err != nil || !ok {
		t.Fatalf("按 cache_key 读取 = %+v / %v / %v，期望读回登录态", byCache, ok, err)
	}
	assertRedisStateRoundTrip(t, state, byCache)

	byWxid, ok, err := store.GetByWxid(ctx, state.Wxid)
	if err != nil || !ok {
		t.Fatalf("按 wxid 读取 = %+v / %v / %v，期望读回登录态", byWxid, ok, err)
	}
	assertRedisStateRoundTrip(t, state, byWxid)

	missing, ok, err := store.Get(ctx, "missing-uuid", "")
	if err != nil || ok || missing.UUID != "" {
		t.Fatalf("缺失 uuid 读取 = %+v / %v / %v，期望稳定返回不存在", missing, ok, err)
	}

	log := strings.Join(fake.CommandLog(), "\n")
	for _, want := range []string{
		"AUTH secret",
		"SELECT 7",
		"SET login:state:mock-redis-uuid",
		"SET login:index:cache:login:mock:mock-redis-uuid mock-redis-uuid",
		"SET login:index:wxid:wxid_redis mock-redis-uuid",
		"GET login:state:mock-redis-uuid",
		"GET login:index:cache:login:mock:mock-redis-uuid",
		"GET login:index:wxid:wxid_redis",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("Redis 命令日志缺少 %q，完整日志：\n%s", want, log)
		}
	}
}

func TestRedisLoginStateStoreReturnsStableUnavailableError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建临时监听失败：%v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	store := storage.NewRedisLoginStateStore(storage.RedisLoginStateStoreConfig{
		Address:        addr,
		DialTimeout:    50 * time.Millisecond,
		CommandTimeout: 50 * time.Millisecond,
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err = store.Save(ctx, storage.LoginState{UUID: "unavailable-uuid", CacheKey: "login:mock:unavailable-uuid"})
	if !errors.Is(err, storage.ErrRedisUnavailable) {
		t.Fatalf("Save 错误 = %v，期望 errors.Is ErrRedisUnavailable", err)
	}
}

func TestRedisLoginStateStoreReturnsStableCommandError(t *testing.T) {
	fake := newFakeRedis(t, "secret")
	defer fake.Close()

	store := storage.NewRedisLoginStateStore(storage.RedisLoginStateStoreConfig{
		Address:        fake.Addr(),
		Password:       "wrong-secret",
		Database:       7,
		DialTimeout:    time.Second,
		CommandTimeout: time.Second,
	})

	err := store.Save(context.Background(), storage.LoginState{UUID: "command-error-uuid", CacheKey: "login:mock:command-error-uuid"})
	if !errors.Is(err, storage.ErrRedisCommandFailed) {
		t.Fatalf("Save 错误 = %v，期望 errors.Is ErrRedisCommandFailed", err)
	}
}

func assertRedisStateRoundTrip(t *testing.T, want, got storage.LoginState) {
	t.Helper()
	if got.UUID != want.UUID || got.CacheKey != want.CacheKey || got.Wxid != want.Wxid || got.DeviceName != want.DeviceName {
		t.Fatalf("读回登录态 = %+v，期望保留 uuid/cache_key/wxid/device_name", got)
	}
	if got.Data62 != want.Data62 || got.A16 != want.A16 || got.SessionState != want.SessionState || got.HeartbeatCount != want.HeartbeatCount {
		t.Fatalf("读回登录态业务字段 = %+v，期望保留导入、会话和心跳字段", got)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || !got.CheckedAt.Equal(want.CheckedAt) {
		t.Fatalf("读回时间 = %s / %s，期望保留纳秒时间", got.CreatedAt, got.CheckedAt)
	}
	if got.Protocol["pack_kind"] != want.Protocol["pack_kind"] || got.Protocol["platform"] != want.Protocol["platform"] {
		t.Fatalf("读回协议摘要 = %+v，期望保留 protocol 字段", got.Protocol)
	}
}

type fakeRedis struct {
	t        *testing.T
	listener net.Listener
	wg       sync.WaitGroup
	password string

	mu       sync.Mutex
	values   map[string]string
	commands []string
}

func newFakeRedis(t *testing.T, password string) *fakeRedis {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("启动 fake Redis 失败：%v", err)
	}
	f := &fakeRedis{t: t, listener: ln, password: password, values: make(map[string]string)}
	f.wg.Add(1)
	go f.accept()
	return f
}

func (f *fakeRedis) Addr() string {
	return f.listener.Addr().String()
}

func (f *fakeRedis) Close() {
	_ = f.listener.Close()
	f.wg.Wait()
}

func (f *fakeRedis) CommandLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.commands))
	copy(out, f.commands)
	return out
}

func (f *fakeRedis) accept() {
	defer f.wg.Done()
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		f.wg.Add(1)
		go func() {
			defer f.wg.Done()
			f.handle(conn)
		}()
	}
}

func (f *fakeRedis) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	authed := f.password == ""
	for {
		cmd, err := readRESPArray(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			return
		}
		if len(cmd) == 0 {
			_, _ = conn.Write([]byte("-ERR empty command\r\n"))
			continue
		}
		name := strings.ToUpper(cmd[0])
		f.record(cmd)
		if !authed && name != "AUTH" {
			_, _ = conn.Write([]byte("-NOAUTH Authentication required\r\n"))
			continue
		}
		switch name {
		case "AUTH":
			if len(cmd) != 2 || cmd[1] != f.password {
				_, _ = conn.Write([]byte("-ERR invalid password\r\n"))
				continue
			}
			authed = true
			_, _ = conn.Write([]byte("+OK\r\n"))
		case "SELECT":
			if len(cmd) != 2 {
				_, _ = conn.Write([]byte("-ERR wrong number of arguments for SELECT\r\n"))
				continue
			}
			_, _ = conn.Write([]byte("+OK\r\n"))
		case "SET":
			if len(cmd) != 3 {
				_, _ = conn.Write([]byte("-ERR wrong number of arguments for SET\r\n"))
				continue
			}
			f.mu.Lock()
			f.values[cmd[1]] = cmd[2]
			f.mu.Unlock()
			_, _ = conn.Write([]byte("+OK\r\n"))
		case "GET":
			if len(cmd) != 2 {
				_, _ = conn.Write([]byte("-ERR wrong number of arguments for GET\r\n"))
				continue
			}
			f.mu.Lock()
			value, ok := f.values[cmd[1]]
			f.mu.Unlock()
			if !ok {
				_, _ = conn.Write([]byte("$-1\r\n"))
				continue
			}
			_, _ = fmt.Fprintf(conn, "$%d\r\n%s\r\n", len(value), value)
		case "PING":
			_, _ = conn.Write([]byte("+PONG\r\n"))
		default:
			_, _ = fmt.Fprintf(conn, "-ERR unsupported command %s\r\n", name)
		}
	}
}

func (f *fakeRedis) record(cmd []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = append(f.commands, strings.Join(cmd, " "))
}

func readRESPArray(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(line, "*") {
		return nil, fmt.Errorf("RESP 数组格式无效：%q", line)
	}
	count, err := strconv.Atoi(strings.TrimPrefix(line, "*"))
	if err != nil {
		return nil, err
	}
	parts := make([]string, 0, count)
	for i := 0; i < count; i++ {
		header, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		header = strings.TrimRight(header, "\r\n")
		if !strings.HasPrefix(header, "$") {
			return nil, fmt.Errorf("RESP bulk string 格式无效：%q", header)
		}
		size, err := strconv.Atoi(strings.TrimPrefix(header, "$"))
		if err != nil {
			return nil, err
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return nil, err
		}
		parts = append(parts, string(buf[:size]))
	}
	return parts, nil
}
