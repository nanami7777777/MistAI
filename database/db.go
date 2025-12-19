package database

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Message 嵌入式消息结构
type Message struct {
	MID       int       `bson:"mid"`       // 消息序号
	SenderID  string    `bson:"sender_id"` // 发送者ID: "user" 或 "assistant"
	Content   string    `bson:"content"`   // 消息内容
	Timestamp time.Time `bson:"timestamp"` // 消息时间戳
	Status    string    `bson:"status"`    // 消息状态: "sent", "read", "deleted"
}

// Session 会话结构（嵌入式消息）
type Session struct {
	ID            primitive.ObjectID `bson:"_id,omitempty"`   // 会话ID
	Type          string             `bson:"type"`            // 会话类型: "bot" (固定为机器人对话)
	Name          string             `bson:"name"`            // 会话名称
	Participants  []string           `bson:"participants"`    // 参与者: ["user", "assistant"]
	CreatedAt     time.Time          `bson:"created_at"`      // 会话创建时间
	LastMessageAt time.Time          `bson:"last_message_at"` // 最新消息时间
	MessageCount  int                `bson:"message_count"`   // 消息总数
	Messages      []Message          `bson:"messages"`        // 消息数组
	IsDeleted     bool               `bson:"is_deleted"`      // 软删除标记
}

// HistoryMessage 历史消息结构（用于返回给调用方，保持兼容性）
type HistoryMessage struct {
	ID      uint   // 兼容性字段，实际使用MID
	Role    string // 对应SenderID
	Content string
}

// Conversation 对话结构（兼容性，映射到Session）
type Conversation struct {
	ID        primitive.ObjectID
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
	IsDeleted bool
}

var client *mongo.Client
var database *mongo.Database
var sessionsCollection *mongo.Collection
var wordsCollection *mongo.Collection
var sentencesCollection *mongo.Collection
var currentSessionID primitive.ObjectID

const (
	DatabaseName             = "chat_assistant"
	SessionsCollection       = "sessions"
	WordsCollection          = "words"
	SentencesCollection      = "sentences"
	DefaultConnectionTimeout = 10 * time.Second
)

// InitDB 初始化MongoDB连接
func InitDB(connectionString string) error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultConnectionTimeout)
	defer cancel()

	if connectionString == "" {
		connectionString = "mongodb://localhost:27017"
	}

	var err error
	client, err = mongo.Connect(ctx, options.Client().ApplyURI(connectionString))
	if err != nil {
		return fmt.Errorf("连接MongoDB失败: %v", err)
	}

	if err = client.Ping(ctx, nil); err != nil {
		return fmt.Errorf("MongoDB连接测试失败: %v", err)
	}

	database = client.Database(DatabaseName)
	sessionsCollection = database.Collection(SessionsCollection)
	wordsCollection = database.Collection(WordsCollection)
	sentencesCollection = database.Collection(SentencesCollection)

	if err := createIndexes(); err != nil {
		return fmt.Errorf("创建索引失败: %v", err)
	}

	if err := initializeDefaultSession(); err != nil {
		return fmt.Errorf("初始化默认会话失败: %v", err)
	}

	return nil
}

// createIndexes 创建必要的索引
func createIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "is_deleted", Value: 1}, {Key: "last_message_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "participants", Value: 1}, {Key: "type", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "name", Value: "text"}},
		},
	}

	_, err := sessionsCollection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("创建索引失败: %v", err)
	}

	return nil
}

// initializeDefaultSession 初始化默认会话
func initializeDefaultSession() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	count, err := sessionsCollection.CountDocuments(ctx, bson.M{"is_deleted": false})
	if err != nil {
		return fmt.Errorf("查询会话数量失败: %v", err)
	}

	if count == 0 {
		conv, err := CreateConversation("新对话")
		if err != nil {
			return fmt.Errorf("创建默认对话失败: %v", err)
		}
		currentSessionID = conv.ID
	} else {
		var session Session
		opts := options.FindOne().SetSort(bson.M{"last_message_at": -1})
		err := sessionsCollection.FindOne(ctx, bson.M{"is_deleted": false}, opts).Decode(&session)
		if err != nil {
			return fmt.Errorf("加载会话失败: %v", err)
		}
		currentSessionID = session.ID
	}

	return nil
}

// CloseDB 关闭数据库连接
func CloseDB() error {
	if client == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return client.Disconnect(ctx)
}

// CreateConversation 创建新对话
func CreateConversation(name string) (*Conversation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now()
	session := &Session{
		ID:            primitive.NewObjectID(),
		Type:          "bot",
		Name:          name,
		Participants:  []string{"user", "assistant"},
		CreatedAt:     now,
		LastMessageAt: now,
		MessageCount:  0,
		Messages:      []Message{},
		IsDeleted:     false,
	}

	_, err := sessionsCollection.InsertOne(ctx, session)

	if err != nil {
		return nil, fmt.Errorf("创建对话失败: %v", err)
	}

	return &Conversation{
		ID:        session.ID,
		Name:      session.Name,
		CreatedAt: session.CreatedAt,
		UpdatedAt: session.LastMessageAt,
		IsDeleted: session.IsDeleted,
	}, nil
}

// GetAllConversations 获取所有对话
func GetAllConversations() ([]Conversation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{"is_deleted": false}
	opts := options.Find().SetSort(bson.M{"last_message_at": -1})

	cursor, err := sessionsCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("查询对话失败: %v", err)
	}
	defer cursor.Close(ctx)

	var sessions []Session
	if err = cursor.All(ctx, &sessions); err != nil {
		return nil, fmt.Errorf("解析对话数据失败: %v", err)
	}

	conversations := make([]Conversation, len(sessions))
	for i, session := range sessions {
		conversations[i] = Conversation{
			ID:        session.ID,
			Name:      session.Name,
			CreatedAt: session.CreatedAt,
			UpdatedAt: session.LastMessageAt,
			IsDeleted: session.IsDeleted,
		}
	}

	return conversations, nil
}

// DeleteConversation 通过ObjectID软删除对话
func DeleteConversation(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{"_id": id}
	update := bson.M{"$set": bson.M{"is_deleted": true}}

	result, err := sessionsCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("删除对话失败: %v", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("对话不存在")
	}

	return nil
}

// UpdateConversationName 通过ObjectID更新对话名称
func UpdateConversationName(id primitive.ObjectID, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{"_id": id}
	update := bson.M{"$set": bson.M{"name": name}}

	result, err := sessionsCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("更新对话名称失败: %v", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("对话不存在")
	}

	return nil
}

// GetCurrentConversationID 获取当前对话的ObjectID
func GetCurrentConversationID() primitive.ObjectID {
	return currentSessionID
}

// SetCurrentConversationID 设置当前对话ObjectID
func SetCurrentConversationID(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	count, err := sessionsCollection.CountDocuments(ctx, bson.M{
		"_id":        id,
		"is_deleted": false,
	})
	if err != nil {
		return fmt.Errorf("验证对话存在性失败: %v", err)
	}

	if count == 0 {
		return fmt.Errorf("对话不存在或已被删除")
	}

	currentSessionID = id
	return nil
}

// GetConversationByObjectID 通过ObjectID获取对话
func GetConversationByObjectID(id primitive.ObjectID) (*Conversation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var session Session
	filter := bson.M{"_id": id, "is_deleted": false}

	err := sessionsCollection.FindOne(ctx, filter).Decode(&session)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("对话不存在")
		}
		return nil, fmt.Errorf("查询对话失败: %v", err)
	}

	return &Conversation{
		ID:        session.ID,
		Name:      session.Name,
		CreatedAt: session.CreatedAt,
		UpdatedAt: session.LastMessageAt,
		IsDeleted: session.IsDeleted,
	}, nil
}

// SearchConversations 搜索对话
func SearchConversations(keyword string, limit int) ([]Conversation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{
		"is_deleted": false,
		"$or": []bson.M{
			{"name": bson.M{"$regex": keyword, "$options": "i"}},
			{"messages.content": bson.M{"$regex": keyword, "$options": "i"}},
		},
	}

	opts := options.Find().
		SetSort(bson.M{"last_message_at": -1}).
		SetLimit(int64(limit))

	cursor, err := sessionsCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("搜索对话失败: %v", err)
	}
	defer cursor.Close(ctx)

	var sessions []Session
	if err = cursor.All(ctx, &sessions); err != nil {
		return nil, fmt.Errorf("解析搜索结果失败: %v", err)
	}

	conversations := make([]Conversation, len(sessions))
	for i, session := range sessions {
		conversations[i] = Conversation{
			ID:        session.ID,
			Name:      session.Name,
			CreatedAt: session.CreatedAt,
			UpdatedAt: session.LastMessageAt,
			IsDeleted: session.IsDeleted,
		}
	}

	return conversations, nil
}

// GetConversationStats 获取对话统计信息
func GetConversationStats() (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sessionCount, err := sessionsCollection.CountDocuments(ctx, bson.M{"is_deleted": false})
	if err != nil {
		return nil, fmt.Errorf("统计会话数量失败: %v", err)
	}

	// 统计总消息数
	pipeline := []bson.M{
		{"$match": bson.M{"is_deleted": false}},
		{"$project": bson.M{"message_count": 1}},
		{"$group": bson.M{
			"_id":            nil,
			"total_messages": bson.M{"$sum": "$message_count"},
		}},
	}

	cursor, err := sessionsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("统计消息数量失败: %v", err)
	}
	defer cursor.Close(ctx)

	var result struct {
		TotalMessages int `bson:"total_messages"`
	}
	if cursor.Next(ctx) {
		cursor.Decode(&result)
	}

	return map[string]interface{}{
		"total_conversations":     sessionCount,
		"total_messages":          result.TotalMessages,
		"current_conversation_id": currentSessionID.Hex(),
	}, nil
}

// GetSessionByObjectID 获取完整的会话信息（包含所有消息）
func GetSessionByObjectID(id primitive.ObjectID) (*Session, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var session Session
	err := sessionsCollection.FindOne(ctx, bson.M{"_id": id}).Decode(&session)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("会话不存在")
		}
		return nil, fmt.Errorf("查询会话失败: %v", err)
	}

	return &session, nil
}
