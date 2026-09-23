// Package config 从环境变量读取全部配置。
// 设计约束（design.md D5）：密钥与凭据只来自环境变量，缺失即启动失败，禁止内置默认值。
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config 是应用的全部运行配置。
type Config struct {
	Port                string        // api 监听端口（Compose 内部，不暴露宿主）
	UploadDir           string        // 上传目录（只挂载到 api 容器）
	SessionSecret       string        // 会话 ID 的 HMAC 密钥（必填，无默认值）
	MySQLHost           string        // 数据库主机（Compose 内部网络服务名）
	MySQLPort           string        // 数据库端口
	MySQLDatabase       string        // 数据库名（必填）
	MySQLUser           string        // 普通数据库账号（必填；不下发 root 口令）
	MySQLPassword       string        // 普通账号口令（必填）
	SeedTeacherPassword string        // 预置教师口令（必填，bcrypt 入库）
	SeedStudentPassword string        // 预置学生口令（必填，bcrypt 入库）
	MaxUploadBytes      int64         // 上传大小上限
	SessionTTL          time.Duration // 会话有效期
	LoginFailLimit      int           // 登录失败限流阈值
	LoginFailWindow     time.Duration // 登录失败统计窗口
}

// required 返回环境变量值；缺失或为空时报错（失败即停，不回落默认值）。
func required(name string) (string, error) {
	v := os.Getenv(name)
	if v == "" {
		return "", fmt.Errorf("缺少必需环境变量 %s（请参照 .env.example 填写 .env）", name)
	}
	return v, nil
}

func getenvInt64(name string, def int64) (int64, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("环境变量 %s 必须为正整数，当前值 %q", name, v)
	}
	return n, nil
}

// Load 读取并校验配置；任一必填项缺失即返回错误，进程应退出。
func Load() (*Config, error) {
	c := &Config{}
	var err error

	if c.SessionSecret, err = required("SESSION_SECRET"); err != nil {
		return nil, err
	}
	if c.MySQLDatabase, err = required("MYSQL_DATABASE"); err != nil {
		return nil, err
	}
	if c.MySQLUser, err = required("MYSQL_USER"); err != nil {
		return nil, err
	}
	if c.MySQLPassword, err = required("MYSQL_PASSWORD"); err != nil {
		return nil, err
	}
	if c.SeedTeacherPassword, err = required("SEED_TEACHER_PASSWORD"); err != nil {
		return nil, err
	}
	if c.SeedStudentPassword, err = required("SEED_STUDENT_PASSWORD"); err != nil {
		return nil, err
	}

	// 非密钥项允许默认值（不构成安全问题）。
	c.Port = os.Getenv("API_PORT")
	if c.Port == "" {
		c.Port = "8081"
	}
	c.MySQLHost = os.Getenv("MYSQL_HOST")
	if c.MySQLHost == "" {
		c.MySQLHost = "db"
	}
	c.MySQLPort = os.Getenv("MYSQL_PORT")
	if c.MySQLPort == "" {
		c.MySQLPort = "3306"
	}
	c.UploadDir = os.Getenv("UPLOAD_DIR")
	if c.UploadDir == "" {
		c.UploadDir = "./uploads"
	}

	if c.MaxUploadBytes, err = getenvInt64("MAX_UPLOAD_BYTES", 1<<20); err != nil {
		return nil, err
	}
	ttlHours, err := getenvInt64("SESSION_TTL_HOURS", 24)
	if err != nil {
		return nil, err
	}
	c.SessionTTL = time.Duration(ttlHours) * time.Hour
	limit, err := getenvInt64("LOGIN_FAIL_LIMIT", 5)
	if err != nil {
		return nil, err
	}
	c.LoginFailLimit = int(limit)
	windowSec, err := getenvInt64("LOGIN_FAIL_WINDOW_SECONDS", 300)
	if err != nil {
		return nil, err
	}
	c.LoginFailWindow = time.Duration(windowSec) * time.Second

	return c, nil
}
