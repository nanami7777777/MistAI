package main

import (
	"Mist/config"
	"Mist/database"
	"Mist/gui"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
)

func main() {
	// 加载配置文件
	if _, err := config.LoadConfig(); err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		return
	}
	// 初始化数据库
	if err := database.InitDB("./chat.db"); err != nil {
		fmt.Printf("数据库初始化失败: %v\n", err)
		return
	}
	defer database.CloseDB()

	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true, // 显示完整时间
	})

	file, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		log.SetOutput(file)
	}
	// 启动 GUI（所有 Fyne 相关逻辑已移入 gui 包）
	gui.Run()
}
