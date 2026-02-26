package log

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Init 初始化日志：支持 JSON 格式（便于集中收集）和实例标识
// 环境变量：
//   - LOG_FORMAT=json 时输出 JSON，否则为文本
//   - INSTANCE_ID 或 HOSTNAME 用于标识实例，便于多实例日志区分
func Init() {
	format := os.Getenv("LOG_FORMAT")
	instanceID := os.Getenv("INSTANCE_ID")
	if instanceID == "" {
		instanceID = os.Getenv("HOSTNAME") // Docker 默认注入容器名
	}
	if instanceID == "" {
		instanceID = "local"
	}

	if format == "json" {
		logrus.SetFormatter(&logrus.JSONFormatter{
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyMsg:  "msg",
				logrus.FieldKeyTime: "time",
			},
		})
		// 为每条日志注入 instance_id，便于集中收集时区分实例
		logrus.AddHook(&instanceHook{instanceID: instanceID})
	} else {
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
		logrus.AddHook(&instanceHook{instanceID: instanceID})
	}

	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.InfoLevel)
}

// instanceHook 为每条日志注入 instance_id 字段
type instanceHook struct {
	instanceID string
}

func (h *instanceHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *instanceHook) Fire(entry *logrus.Entry) error {
	entry.Data["instance_id"] = h.instanceID
	return nil
}
