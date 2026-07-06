package httpapi_test

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mahiro424/cbs/internal/config"
	"github.com/mahiro424/cbs/internal/httpapi"
)

func TestHTTPLoginStateStoreUsesRedisBackendAcrossServerInstances(t *testing.T) {
	fake := newHTTPFakeRedis(t, "secret")
	defer fake.Close()

	cfg := config.Default()
	cfg.LoginStateStore = "redis"
	cfg.RedisLink = fake.Addr()
	cfg.RedisPass = "secret"
	cfg.RedisDBNum = 7
	cfg.SampleDir = t.TempDir()

	writer := httpapi.NewServer(cfg)
	login := postJSON(t, writer, "/Login/62data", `{"Data62":"redis-http-62","DeviceID":"iphone-http-redis","DeviceName":"HTTP Redis 设备","Wxid":"wxid_http_redis"}`)
	if !login.Success || login.Code != "ok" {
		t.Fatalf("Redis 模式 62data 响应 = %+v，期望 ok", login)
	}
	loginData := mustMap(t, login.Data)
	uuid := mustString(t, loginData, "uuid")
	cacheKey := mustString(t, loginData, "cache_key")

	readerCfg := cfg
	readerCfg.SampleDir = t.TempDir()
	reader := httpapi.NewServer(readerCfg)
	init := postJSON(t, reader, "/Login/Newinit?wxid=wxid_http_redis&MaxSynckey=max-redis&CurrentSynckey=current-redis", `{}`)
	if !init.Success || init.Code != "ok" {
		t.Fatalf("新的 Server 读取 Redis 登录态 Newinit 响应 = %+v，期望 ok", init)
	}
	initData := mustMap(t, init.Data)
	if initData["uuid"] != uuid || initData["wxid"] != "wxid_http_redis" || initData["session_state"] != "initialized" {
		t.Fatalf("Newinit data = %+v，期望跨 Server 读回 Redis 登录态并更新 initialized", initData)
	}

	byUUID := postJSON(t, reader, "/Login/GetCacheInfo?uuid="+uuid, `{}`)
	if !byUUID.Success || byUUID.Code != "ok" {
		t.Fatalf("按 uuid 从 Redis 查询响应 = %+v，期望 ok", byUUID)
	}
	stateByUUID := mustMap(t, byUUID.Data)
	if stateByUUID["cache_key"] != cacheKey || stateByUUID["session_state"] != "initialized" {
		t.Fatalf("Redis 查询结果 = %+v，期望保留 cache_key 并反映 Newinit 更新", stateByUUID)
	}

	byCacheKey := postJSON(t, reader, "/Login/GetCacheInfo?cache_key="+cacheKey, `{}`)
	if !byCacheKey.Success || byCacheKey.Code != "ok" {
		t.Fatalf("按 cache_key 从 Redis 查询响应 = %+v，期望 ok", byCacheKey)
	}
	stateByCacheKey := mustMap(t, byCacheKey.Data)
	if stateByCacheKey["uuid"] != uuid || stateByCacheKey["wxid"] != "wxid_http_redis" {
		t.Fatalf("Redis cache_key 查询结果 = %+v，期望读回同一登录态", stateByCacheKey)
	}

	health := getJSON(t, reader, "/healthz")
	if !health.Success || health.Code != "ok" {
		t.Fatalf("healthz 响应 = %+v，期望 ok", health)
	}
	healthData := mustMap(t, health.Data)
	store := mustMap(t, healthData["login_state_store"])
	if store["mode"] != "redis" {
		t.Fatalf("healthz login_state_store = %+v，期望 mode=redis", store)
	}
}

func TestHTTPRedisLoginStateStoreUnavailableReturnsStableEnvelope(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建临时监听失败：%v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	cfg := config.Default()
	cfg.LoginStateStore = "redis"
	cfg.RedisLink = addr
	cfg.SampleDir = t.TempDir()
	h := httpapi.NewServer(cfg)

	status, body := postJSONWithStatus(t, h, "/Login/GetQR", `{"DeviceID":"redis-down","DeviceName":"Redis 不可用设备","Type":"ipad"}`)
	if status != http.StatusInternalServerError {
		t.Fatalf("状态码 = %d，期望 500，响应：%+v", status, body)
	}
	if body.Success || body.Code != "login_state_store_error" || !strings.Contains(body.Message, "redis unavailable") {
		t.Fatalf("Redis 不可用响应 = %+v，期望 login_state_store_error 且包含 redis unavailable", body)
	}
}

func getJSON(t *testing.T, h *httpapi.Server, path string) httpapi.Envelope {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body httpapi.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v，正文：%s", err, rec.Body.String())
	}
	return body
}

func postJSONWithStatus(t *testing.T, h *httpapi.Server, path string, payload string) (int, httpapi.Envelope) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body httpapi.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v，正文：%s", err, rec.Body.String())
	}
	return rec.Code, body
}

type httpFakeRedis struct {
	t        *testing.T
	listener net.Listener
	wg       sync.WaitGroup
	password string

	mu     sync.Mutex
	values map[string]string
}

func newHTTPFakeRedis(t *testing.T, password string) *httpFakeRedis {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("启动 HTTP fake Redis 失败：%v", err)
	}
	f := &httpFakeRedis{t: t, listener: ln, password: password, values: make(map[string]string)}
	f.wg.Add(1)
	go f.accept()
	return f
}

func (f *httpFakeRedis) Addr() string {
	return f.listener.Addr().String()
}

func (f *httpFakeRedis) Close() {
	_ = f.listener.Close()
	f.wg.Wait()
}

func (f *httpFakeRedis) accept() {
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

func (f *httpFakeRedis) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	authed := f.password == ""
	for {
		cmd, err := readHTTPFakeRESPArray(reader)
		if err != nil {
			return
		}
		if len(cmd) == 0 {
			_, _ = conn.Write([]byte("-ERR empty command\r\n"))
			continue
		}
		name := strings.ToUpper(cmd[0])
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
			_, _ = conn.Write([]byte("+OK\r\n"))
		case "PING":
			_, _ = conn.Write([]byte("+PONG\r\n"))
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
		default:
			_, _ = fmt.Fprintf(conn, "-ERR unsupported command %s\r\n", name)
		}
	}
}

func readHTTPFakeRESPArray(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, err
		}
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
