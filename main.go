package main

import (
	"Mist/config"
	"Mist/database"
	"Mist/gui"
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
)

func main() {
	// 加载配置文件
	fmt.Printf("加载配置文件...")
	starttime := time.Now()
	if _, err := config.LoadConfig(); err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		return
	}
	// 初始化数据库
	cfg := config.GetConfig()
	if err := database.InitDB(cfg.MongoDB.ConnectionString); err != nil {
		fmt.Printf("数据库初始化失败: %v\n", err)
		return
	}
	defer database.CloseDB()
	
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true, // 显示完整时间
	})

	// 设置日志级别为Debug以便调试流式响应
	log.SetLevel(log.DebugLevel)

	file, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		log.SetOutput(file)
	}
	endtime := time.Now()
	fmt.Println("启动时间:", endtime.Sub(starttime))
	// 启动 GUI（所有 Fyne 相关逻辑已移入 gui 包）
	gui.Run()
}
