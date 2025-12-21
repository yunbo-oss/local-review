package mysql

import (
	"fmt"
	"os"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"github.com/sirupsen/logrus"
)

const (
	DATABASE string = "mysql"

	DATABASENAME string = "local_review_go"
)

var _defalutDB *gorm.DB

func Init() {
	user := getEnv("MYSQL_USER", "root")
	password := getEnv("MYSQL_PASSWORD", "8888.216")
	addr := getEnv("MYSQL_ADDR", "127.0.0.1")
	port := getEnv("MYSQL_PORT", "3306")
	dbName := getEnv("MYSQL_DB", "local_review_go")

	url := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8&parseTime=True&loc=Local",
		user, password, addr, port, dbName)
	db, err := gorm.Open(DATABASE, url)
	if err != nil {
		logrus.Error("get mysql DB failed!")
		panic(err)
	}

	_defalutDB = db
}

func GetMysqlDB() *gorm.DB {
	return _defalutDB
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
