package mysql

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	DATABASE string = "mysql"
)

var _defalutDB *gorm.DB

// getEnv 获取环境变量，如果不存在则返回默认值（避免循环导入）
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func Init() {
	// 从环境变量读取配置，支持灵活部署
	user := getEnv("MYSQL_USER", "root")
	password := getEnv("MYSQL_PASSWORD", "8888.216")
	addr := getEnv("MYSQL_ADDR", "127.0.0.1")
	port := getEnv("MYSQL_PORT", "3306")
	database := getEnv("MYSQL_DATABASE", "local_review_go")

	// 构建 DSN
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		user, password, addr, port, database)

	// 使用新的 GORM v2
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // 生产环境可以设置为 Silent
	})
	if err != nil {
		logrus.Errorf("Failed to connect to MySQL: %v", err)
		panic(err)
	}

	// 配置连接池参数
	sqlDB, err := db.DB()
	if err != nil {
		logrus.Errorf("Failed to get underlying sql.DB: %v", err)
		panic(err)
	}

	// 设置连接池参数（分布式部署时 3 实例会放大连接数，单实例默认 100，多实例建议 30/实例）
	// MYSQL_MAX_OPEN_CONNS：可通过环境变量覆盖，默认单机 100、多实例场景建议 30
	maxOpen := 100
	if v := getEnv("MYSQL_MAX_OPEN_CONNS", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxOpen = n
		}
	}
	sqlDB.SetMaxOpenConns(maxOpen)

	// MaxIdleConns: 最大空闲连接数，建议为 MaxOpenConns 的 1/4 到 1/2
	maxIdle := maxOpen / 4
	if maxIdle < 5 {
		maxIdle = 5
	}
	sqlDB.SetMaxIdleConns(maxIdle)

	// ConnMaxLifetime: 连接的最大生存时间
	// 设置为 1 小时，避免长时间空闲连接占用资源，同时允许连接复用
	sqlDB.SetConnMaxLifetime(time.Hour)

	// ConnMaxIdleTime: 连接的最大空闲时间
	// 设置为 15 分钟，空闲连接超过此时间会被关闭，释放资源
	sqlDB.SetConnMaxIdleTime(15 * time.Minute)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		logrus.Errorf("Failed to ping MySQL: %v", err)
		panic(err)
	}

	logrus.Info("MySQL connection pool configured successfully")
	logrus.Infof("MySQL connection pool: MaxOpen=%d, MaxIdle=%d, OpenConnections=%d, InUse=%d",
		sqlDB.Stats().MaxOpenConnections, sqlDB.Stats().Idle, sqlDB.Stats().OpenConnections, sqlDB.Stats().InUse)

	_defalutDB = db
}

func GetMysqlDB() *gorm.DB {
	return _defalutDB
}

// GetMysqlDBStats 获取数据库连接池统计信息（用于监控）
func GetMysqlDBStats() interface{} {
	if _defalutDB == nil {
		return nil
	}
	sqlDB, err := _defalutDB.DB()
	if err != nil {
		return nil
	}
	return sqlDB.Stats()
}
