package database

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Conversation 对话模型
type Conversation struct {
	ID        uint   `gorm:"primaryKey;autoIncrement"`
	Name      string `gorm:"not null"`
	CreatedAt string `gorm:"type:text;not null"`
	IsDeleted bool   `gorm:"default:0;not null"` // 软删除标记
}

// TableName 指定表名
func (Conversation) TableName() string {
	return "conversations"
}

// ChatMessage 聊天消息模型
type ChatMessage struct {
	ID        uint   `gorm:"primaryKey;autoIncrement"`
	Role      string `gorm:"not null"`
	Content   string `gorm:"not null;type:text"`
	CreatedAt string `gorm:"type:text;not null"`
	IsDeleted bool   `gorm:"default:0;not null"` // 软删除标记
}

var db *gorm.DB
var currentConversationID uint = 0

// InitDB 初始化数据库连接
func InitDB(dbPath string) error {
	var err error
	db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("打开数据库失败: %v", err)
	}

	// 自动迁移表结构
	err = db.AutoMigrate(&Conversation{})
	if err != nil {
		return fmt.Errorf("迁移对话表结构失败: %v", err)
	}

	// 如果没有对话，创建默认对话
	var count int64
	db.Model(&Conversation{}).Where("is_deleted = ?", false).Count(&count)
	if count == 0 {
		conv, err := CreateConversation("新对话")
		if err != nil {
			return fmt.Errorf("创建默认对话失败: %v", err)
		}
		currentConversationID = conv.ID
	} else {
		// 加载第一个对话
		var firstConv Conversation
		db.Where("is_deleted = ?", false).Order("created_at ASC").First(&firstConv)
		currentConversationID = firstConv.ID
		// 确保该对话的消息表存在
		ensureMessageTable(currentConversationID)
	}

	return nil
}

// CloseDB 关闭数据库连接
func CloseDB() error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// ensureMessageTable 确保对话的消息表存在
func ensureMessageTable(conversationID uint) error {
	tableName := fmt.Sprintf("messages_%d", conversationID)

	// 使用原生SQL创建表
	sql := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at TEXT NOT NULL,
			is_deleted INTEGER NOT NULL DEFAULT 0
		)
	`, tableName)

	return db.Exec(sql).Error
}

// GetCurrentConversationID 获取当前对话ID
func GetCurrentConversationID() uint {
	return currentConversationID
}

// SetCurrentConversationID 设置当前对话ID
func SetCurrentConversationID(id uint) error {
	// 确保该对话的消息表存在
	if err := ensureMessageTable(id); err != nil {
		return fmt.Errorf("确保消息表存在失败: %v", err)
	}
	currentConversationID = id
	return nil
}

// CreateConversation 创建新对话
func CreateConversation(name string) (*Conversation, error) {
	conv := Conversation{
		Name:      name,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
		IsDeleted: false,
	}

	result := db.Create(&conv)
	if result.Error != nil {
		return nil, fmt.Errorf("创建对话失败: %v", result.Error)
	}

	// 创建该对话的消息表
	if err := ensureMessageTable(conv.ID); err != nil {
		return nil, fmt.Errorf("创建消息表失败: %v", err)
	}

	return &conv, nil
}

// GetAllConversations 获取所有对话
func GetAllConversations() ([]Conversation, error) {
	var conversations []Conversation
	result := db.Where("is_deleted = ?", false).Order("created_at DESC").Find(&conversations)
	if result.Error != nil {
		return nil, fmt.Errorf("查询对话失败: %v", result.Error)
	}
	return conversations, nil
}

// DeleteConversation 软删除对话
func DeleteConversation(id uint) error {
	result := db.Model(&Conversation{}).Where("id = ?", id).Update("is_deleted", true)
	if result.Error != nil {
		return fmt.Errorf("删除对话失败: %v", result.Error)
	}
	return nil
}

// UpdateConversationName 更新对话名称
func UpdateConversationName(id uint, name string) error {
	result := db.Model(&Conversation{}).Where("id = ?", id).Update("name", name)
	if result.Error != nil {
		return fmt.Errorf("更新对话名称失败: %v", result.Error)
	}
	return nil
}

// SaveMessage 保存消息到数据库，返回消息ID
func SaveMessage(role, content string) (uint, error) {
	if currentConversationID == 0 {
		return 0, fmt.Errorf("当前没有选中的对话")
	}

	// 确保消息表存在
	if err := ensureMessageTable(currentConversationID); err != nil {
		return 0, fmt.Errorf("确保消息表存在失败: %v", err)
	}

	tableName := fmt.Sprintf("messages_%d", currentConversationID)

	msg := ChatMessage{
		Role:      role,
		Content:   content,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
		IsDeleted: false,
	}

	// 使用原生SQL插入
	sql := fmt.Sprintf(`
		INSERT INTO %s (role, content, created_at, is_deleted) 
		VALUES (?, ?, ?, ?)
	`, tableName)

	result := db.Exec(sql, msg.Role, msg.Content, msg.CreatedAt, msg.IsDeleted)
	if result.Error != nil {
		return 0, fmt.Errorf("保存消息失败: %v", result.Error)
	}

	// 获取插入的ID
	var id uint
	db.Raw("SELECT last_insert_rowid()").Scan(&id)

	return id, nil
}

// DeleteMessage 软删除单条消息
func DeleteMessage(messageID uint) error {
	if currentConversationID == 0 {
		return fmt.Errorf("当前没有选中的对话")
	}

	tableName := fmt.Sprintf("messages_%d", currentConversationID)
	sql := fmt.Sprintf("UPDATE %s SET is_deleted = ? WHERE id = ?", tableName)

	result := db.Exec(sql, true, messageID)
	if result.Error != nil {
		return fmt.Errorf("删除消息失败: %v", result.Error)
	}

	return nil
}

// HistoryMessage 历史消息结构
type HistoryMessage struct {
	ID      uint
	Role    string
	Content string
}

// LoadHistoryMessages 从数据库加载历史消息（只加载未删除的记录）
func LoadHistoryMessages() ([]HistoryMessage, error) {
	if currentConversationID == 0 {
		return []HistoryMessage{}, nil
	}

	// 确保表存在
	if err := ensureMessageTable(currentConversationID); err != nil {
		return []HistoryMessage{}, nil // 表不存在时返回空列表
	}

	tableName := fmt.Sprintf("messages_%d", currentConversationID)

	// 使用原生SQL查询
	type MessageRow struct {
		ID        uint
		Role      string
		Content   string
		CreatedAt string
		IsDeleted bool
	}

	var rows []MessageRow
	sql := fmt.Sprintf("SELECT id, role, content, created_at, is_deleted FROM %s WHERE is_deleted = ? ORDER BY created_at ASC", tableName)
	result := db.Raw(sql, false).Scan(&rows)
	if result.Error != nil {
		// 如果表不存在，返回空列表而不是错误
		if strings.Contains(result.Error.Error(), "no such table") {
			return []HistoryMessage{}, nil
		}
		return nil, fmt.Errorf("查询消息失败: %v", result.Error)
	}

	// 转换为返回格式
	history := make([]HistoryMessage, len(rows))
	for i, row := range rows {
		history[i] = HistoryMessage{
			ID:      row.ID,
			Role:    row.Role,
			Content: row.Content,
		}
	}

	return history, nil
}

// ClearAllMessages 软删除当前对话的所有聊天记录
func ClearAllMessages() error {
	if currentConversationID == 0 {
		return fmt.Errorf("当前没有选中的对话")
	}

	tableName := fmt.Sprintf("messages_%d", currentConversationID)
	sql := fmt.Sprintf("UPDATE %s SET is_deleted = ? WHERE is_deleted = ?", tableName)

	result := db.Exec(sql, true, false)
	if result.Error != nil {
		return fmt.Errorf("清除消息失败: %v", result.Error)
	}

	return nil
}
