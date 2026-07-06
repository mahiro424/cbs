package storage

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/mahiro424/cbs/internal/config"
)

type RedisStatus struct {
	Available bool   `json:"available"`
	Address   string `json:"address"`
	Database  int    `json:"database"`
	Message   string `json:"message"`
}

func CheckRedis(ctx context.Context, cfg config.Config) RedisStatus {
	status := RedisStatus{Address: cfg.RedisLink, Database: cfg.RedisDBNum}
	if strings.TrimSpace(cfg.RedisLink) == "" {
		status.Message = "未配置 Redis 地址"
		return status
	}
	dialer := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", cfg.RedisLink)
	if err != nil {
		status.Message = err.Error()
		return status
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(800 * time.Millisecond))
	reader := bufio.NewReader(conn)
	if cfg.RedisPass != "" {
		if _, err := fmt.Fprintf(conn, "*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(cfg.RedisPass), cfg.RedisPass); err != nil {
			status.Message = err.Error()
			return status
		}
		line, _ := reader.ReadString('\n')
		if !strings.HasPrefix(line, "+OK") {
			status.Message = "Redis AUTH 失败：" + strings.TrimSpace(line)
			return status
		}
	}
	if cfg.RedisDBNum != 0 {
		db := fmt.Sprintf("%d", cfg.RedisDBNum)
		if _, err := fmt.Fprintf(conn, "*2\r\n$6\r\nSELECT\r\n$%d\r\n%s\r\n", len(db), db); err != nil {
			status.Message = err.Error()
			return status
		}
		line, _ := reader.ReadString('\n')
		if !strings.HasPrefix(line, "+OK") {
			status.Message = "Redis SELECT 失败：" + strings.TrimSpace(line)
			return status
		}
	}
	if _, err := fmt.Fprint(conn, "*1\r\n$4\r\nPING\r\n"); err != nil {
		status.Message = err.Error()
		return status
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		status.Message = err.Error()
		return status
	}
	if strings.HasPrefix(line, "+PONG") {
		status.Available = true
		status.Message = "PONG"
		return status
	}
	status.Message = strings.TrimSpace(line)
	return status
}
