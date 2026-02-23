package mysql

import (
	"fmt"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"github.com/sirupsen/logrus"
)

const (
	DATABASE     string = "mysql"
	USER         string = "root"
	PASSWORD     string = "8888.216"
	PORT         string = "3306"
	DATABASENAME string = "local_review_go"
	ADDR         string = "127.0.0.1"
)

var _defalutDB *gorm.DB

func Init() {
	url := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8&parseTime=True&loc=Local",
		USER, PASSWORD, ADDR, PORT, DATABASENAME)
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
