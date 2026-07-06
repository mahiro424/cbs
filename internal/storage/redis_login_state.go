package storage

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/mahiro424/cbs/internal/config"
)

var (
	// ErrRedisUnavailable 标识 Redis 网络连接、读写或超时不可用。
	ErrRedisUnavailable = errors.New("redis unavailable")
	// ErrRedisCommandFailed 标识 Redis 返回了 -ERR、-NOAUTH 等命令级错误。
	ErrRedisCommandFailed = errors.New("redis command failed")
)

// RedisLoginStateStoreConfig 是 Redis 登录态存储的最小配置。
type RedisLoginStateStoreConfig struct {
	Address        string
	Password       string
	Database       int
	DialTimeout    time.Duration
	CommandTimeout time.Duration
}

// RedisLoginStateStore 使用 Redis 保存 LoginState 主记录和索引记录。
type RedisLoginStateStore struct {
	cfg RedisLoginStateStoreConfig
}

func NewRedisLoginStateStore(cfg RedisLoginStateStoreConfig) *RedisLoginStateStore {
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 500 * time.Millisecond
	}
	if cfg.CommandTimeout <= 0 {
		cfg.CommandTimeout = 800 * time.Millisecond
	}
	cfg.Address = strings.TrimSpace(cfg.Address)
	return &RedisLoginStateStore{cfg: cfg}
}

func NewRedisLoginStateStoreFromConfig(cfg config.Config) *RedisLoginStateStore {
	return NewRedisLoginStateStore(RedisLoginStateStoreConfig{
		Address:  cfg.RedisLink,
		Password: cfg.RedisPass,
		Database: cfg.RedisDBNum,
	})
}

func (s *RedisLoginStateStore) Save(ctx context.Context, state LoginState) error {
	if err := contextError(ctx); err != nil {
		return fmt.Errorf("%w: %v", ErrRedisUnavailable, err)
	}
	uuid := strings.TrimSpace(state.UUID)
	if uuid == "" {
		return fmt.Errorf("%w: login state uuid 为空", ErrRedisCommandFailed)
	}
	payload, err := EncodeLoginState(state)
	if err != nil {
		return err
	}
	conn, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := conn.DoStatus(ctx, "SET", LoginStateRedisKey(uuid), string(payload)); err != nil {
		return err
	}
	if cacheKey := strings.TrimSpace(state.CacheKey); cacheKey != "" {
		if err := conn.DoStatus(ctx, "SET", LoginStateCacheIndexKey(cacheKey), uuid); err != nil {
			return err
		}
	}
	if wxid := strings.TrimSpace(state.Wxid); wxid != "" {
		if err := conn.DoStatus(ctx, "SET", LoginStateWxidIndexKey(wxid), uuid); err != nil {
			return err
		}
	}
	return nil
}

func (s *RedisLoginStateStore) Get(ctx context.Context, uuid string, cacheKey string) (LoginState, bool, error) {
	if err := contextError(ctx); err != nil {
		return LoginState{}, false, fmt.Errorf("%w: %v", ErrRedisUnavailable, err)
	}
	uuid = strings.TrimSpace(uuid)
	cacheKey = strings.TrimSpace(cacheKey)
	if uuid == "" && cacheKey == "" {
		return LoginState{}, false, nil
	}
	conn, err := s.open(ctx)
	if err != nil {
		return LoginState{}, false, err
	}
	defer conn.Close()

	if uuid == "" {
		indexValue, ok, err := conn.DoBulk(ctx, "GET", LoginStateCacheIndexKey(cacheKey))
		if err != nil || !ok {
			return LoginState{}, false, err
		}
		uuid = strings.TrimSpace(indexValue)
		if uuid == "" {
			return LoginState{}, false, nil
		}
	}
	return readLoginStateByUUID(ctx, conn, uuid)
}

func (s *RedisLoginStateStore) GetByWxid(ctx context.Context, wxid string) (LoginState, bool, error) {
	if err := contextError(ctx); err != nil {
		return LoginState{}, false, fmt.Errorf("%w: %v", ErrRedisUnavailable, err)
	}
	wxid = strings.TrimSpace(wxid)
	if wxid == "" {
		return LoginState{}, false, nil
	}
	conn, err := s.open(ctx)
	if err != nil {
		return LoginState{}, false, err
	}
	defer conn.Close()

	indexValue, ok, err := conn.DoBulk(ctx, "GET", LoginStateWxidIndexKey(wxid))
	if err != nil || !ok {
		return LoginState{}, false, err
	}
	uuid := strings.TrimSpace(indexValue)
	if uuid == "" {
		return LoginState{}, false, nil
	}
	return readLoginStateByUUID(ctx, conn, uuid)
}

func readLoginStateByUUID(ctx context.Context, conn *redisStateConn, uuid string) (LoginState, bool, error) {
	payload, ok, err := conn.DoBulk(ctx, "GET", LoginStateRedisKey(uuid))
	if err != nil || !ok {
		return LoginState{}, false, err
	}
	state, err := DecodeLoginState([]byte(payload))
	if err != nil {
		return LoginState{}, false, err
	}
	return state, true, nil
}

func (s *RedisLoginStateStore) open(ctx context.Context) (*redisStateConn, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: RedisLoginStateStore 为空", ErrRedisUnavailable)
	}
	if strings.TrimSpace(s.cfg.Address) == "" {
		return nil, fmt.Errorf("%w: 未配置 Redis 地址", ErrRedisUnavailable)
	}
	if err := contextError(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRedisUnavailable, err)
	}
	dialer := net.Dialer{Timeout: s.cfg.DialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", s.cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("%w: dial %s: %v", ErrRedisUnavailable, s.cfg.Address, err)
	}
	stateConn := &redisStateConn{conn: conn, reader: bufio.NewReader(conn), timeout: s.cfg.CommandTimeout}
	if strings.TrimSpace(s.cfg.Password) != "" {
		if err := stateConn.DoStatus(ctx, "AUTH", s.cfg.Password); err != nil {
			_ = stateConn.Close()
			return nil, err
		}
	}
	if s.cfg.Database != 0 {
		if err := stateConn.DoStatus(ctx, "SELECT", strconv.Itoa(s.cfg.Database)); err != nil {
			_ = stateConn.Close()
			return nil, err
		}
	}
	return stateConn, nil
}

type redisStateConn struct {
	conn    net.Conn
	reader  *bufio.Reader
	timeout time.Duration
}

func (c *redisStateConn) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *redisStateConn) DoStatus(ctx context.Context, args ...string) error {
	resp, err := c.do(ctx, args...)
	if err != nil {
		return err
	}
	if resp.kind != '+' {
		return fmt.Errorf("%w: %s 返回类型 %q", ErrRedisCommandFailed, args[0], resp.kind)
	}
	return nil
}

func (c *redisStateConn) DoBulk(ctx context.Context, args ...string) (string, bool, error) {
	resp, err := c.do(ctx, args...)
	if err != nil {
		return "", false, err
	}
	if resp.kind != '$' {
		return "", false, fmt.Errorf("%w: %s 返回类型 %q", ErrRedisCommandFailed, args[0], resp.kind)
	}
	if resp.nil {
		return "", false, nil
	}
	return resp.text, true, nil
}

func (c *redisStateConn) do(ctx context.Context, args ...string) (redisResponse, error) {
	if c == nil || c.conn == nil {
		return redisResponse{}, fmt.Errorf("%w: Redis 连接为空", ErrRedisUnavailable)
	}
	if err := contextError(ctx); err != nil {
		return redisResponse{}, fmt.Errorf("%w: %v", ErrRedisUnavailable, err)
	}
	if err := c.setDeadline(ctx); err != nil {
		return redisResponse{}, err
	}
	if err := writeRESPCommand(c.conn, args...); err != nil {
		return redisResponse{}, fmt.Errorf("%w: write: %v", ErrRedisUnavailable, err)
	}
	resp, err := readRESPResponse(c.reader)
	if err != nil {
		return redisResponse{}, fmt.Errorf("%w: read: %v", ErrRedisUnavailable, err)
	}
	if resp.kind == '-' {
		return redisResponse{}, fmt.Errorf("%w: %s", ErrRedisCommandFailed, resp.text)
	}
	return resp, nil
}

func (c *redisStateConn) setDeadline(ctx context.Context) error {
	if c.timeout <= 0 {
		return nil
	}
	deadline := time.Now().Add(c.timeout)
	if ctx != nil {
		if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
			deadline = ctxDeadline
		}
	}
	if err := c.conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("%w: set deadline: %v", ErrRedisUnavailable, err)
	}
	return nil
}

func writeRESPCommand(w io.Writer, args ...string) error {
	if _, err := fmt.Fprintf(w, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, arg := range args {
		if _, err := fmt.Fprintf(w, "$%d\r\n%s\r\n", len(arg), arg); err != nil {
			return err
		}
	}
	return nil
}

type redisResponse struct {
	kind byte
	text string
	nil  bool
}

func readRESPResponse(reader *bufio.Reader) (redisResponse, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return redisResponse{}, err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return redisResponse{}, fmt.Errorf("空 RESP 响应")
	}
	switch line[0] {
	case '+':
		return redisResponse{kind: '+', text: line[1:]}, nil
	case '-':
		return redisResponse{kind: '-', text: line[1:]}, nil
	case ':':
		return redisResponse{kind: ':', text: line[1:]}, nil
	case '$':
		size, err := strconv.Atoi(line[1:])
		if err != nil {
			return redisResponse{}, err
		}
		if size < 0 {
			return redisResponse{kind: '$', nil: true}, nil
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return redisResponse{}, err
		}
		return redisResponse{kind: '$', text: string(buf[:size])}, nil
	default:
		return redisResponse{}, fmt.Errorf("不支持的 RESP 响应：%q", line)
	}
}
