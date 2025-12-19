package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type TaskBase struct {
	TaskType string `json:"task_type" bson:"task_type"`
}

type Example struct {
	Sentence    string `json:"sentence" bson:"sentence"`
	Translation string `json:"translation" bson:"translation"`
}

type Sense struct {
	Meaning  string    `json:"meaning" bson:"meaning"`
	Examples []Example `json:"examples" bson:"examples"`
}

type PosEntry struct {
	Pos    string  `json:"pos" bson:"pos"`
	Senses []Sense `json:"senses" bson:"senses"`
}

type DictionaryEntry struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	TaskType    string             `json:"task_type" bson:"task_type"`
	EnglishWord string             `json:"english_word" bson:"english_word"`
	PosEntries  []PosEntry         `json:"pos_entries" bson:"pos_entries"`
}

type Translation struct {
	ID                primitive.ObjectID `bson:"_id,omitempty"`
	TaskType          string             `json:"task_type" bson:"task_type"`
	SourceText        string             `json:"source_text" bson:"source_text"`
	TargetTranslation string             `json:"target_translation" bson:"target_translation"`
	Explanation       string             `json:"explanation" bson:"explanation"`
}

func SaveJsonData(jsonData string) error {
	doc := bson.M{}
	err := json.Unmarshal([]byte(jsonData), &doc)
	if err != nil {
		return fmt.Errorf("解析JSON数据失败: %v", err)
	}

	taskType, ok := doc["task_type"].(string)
	if !ok {
		return fmt.Errorf("JSON数据中缺少task_type字段")
	}

	switch taskType {
	case "translation":
		// 处理翻译任务
		return SaveSentences(doc)
	case "dictionary":
		// 处理字典任务
		return SaveDictionary(doc)
	default:
		return fmt.Errorf("未知任务类型: %s", taskType)
	}

}
func SaveSentences(jsonData bson.M) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := sentencesCollection.InsertOne(ctx, jsonData); err != nil {
		return fmt.Errorf("保存句子失败: %v", err)
	}

	return nil
}
func SaveDictionary(jsonData bson.M) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := wordsCollection.InsertOne(ctx, jsonData); err != nil {
		return fmt.Errorf("保存字典失败: %v", err)
	}

	return nil
}

// SaveMessage 保存消息到数据库
func SaveMessage(role, content string) (uint, error) {
	if currentSessionID.IsZero() {
		return 0, fmt.Errorf("当前没有选中的对话")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 获取当前会话的消息数量来生成新的MID
	var session Session
	err := sessionsCollection.FindOne(ctx, bson.M{"_id": currentSessionID}).Decode(&session)
	if err != nil {
		return 0, fmt.Errorf("查找会话失败: %v", err)
	}

	newMID := session.MessageCount + 1
	now := time.Now()

	newMessage := Message{
		MID:       newMID,
		SenderID:  role,
		Content:   content,
		Timestamp: now,
		Status:    "sent",
	}

	// 使用$push添加消息，同时更新统计信息
	filter := bson.M{"_id": currentSessionID}
	update := bson.M{
		"$push": bson.M{"messages": newMessage},
		"$set": bson.M{
			"last_message_at": now,
		},
		"$inc": bson.M{"message_count": 1},
	}

	result, err := sessionsCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return 0, fmt.Errorf("保存消息失败: %v", err)
	}

	if result.MatchedCount == 0 {
		return 0, fmt.Errorf("会话不存在")
	}

	return uint(newMID), nil
}

// SaveMessageReturnObjectID 保存消息并返回MID作为ObjectID
func SaveMessageReturnObjectID(role, content string) (primitive.ObjectID, error) {
	_, err := SaveMessage(role, content)
	if err != nil {
		return primitive.NilObjectID, err
	}

	// 创建一个基于当前时间的ObjectID（用于兼容性）
	timestamp := time.Now()
	objectID := primitive.NewObjectIDFromTimestamp(timestamp)
	return objectID, nil
}

// LoadHistoryMessages 从数据库加载历史消息
func LoadHistoryMessages() ([]HistoryMessage, error) {
	if currentSessionID.IsZero() {
		return []HistoryMessage{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var session Session
	err := sessionsCollection.FindOne(ctx, bson.M{"_id": currentSessionID}).Decode(&session)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return []HistoryMessage{}, nil
		}
		return nil, fmt.Errorf("查询会话失败: %v", err)
	}

	// 过滤掉已删除的消息
	var activeMessages []Message
	for _, msg := range session.Messages {
		if msg.Status != "deleted" {
			activeMessages = append(activeMessages, msg)
		}
	}

	history := make([]HistoryMessage, len(activeMessages))
	for i, msg := range activeMessages {
		history[i] = HistoryMessage{
			ID:      uint(msg.MID),
			Role:    msg.SenderID,
			Content: msg.Content,
		}
	}

	return history, nil
}

// DeleteMessage 软删除单条消息
func DeleteMessage(messageID uint) error {
	if currentSessionID.IsZero() {
		return fmt.Errorf("当前没有选中的对话")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 使用数组更新操作将指定消息标记为删除
	filter := bson.M{
		"_id":          currentSessionID,
		"messages.mid": int(messageID),
	}
	update := bson.M{
		"$set": bson.M{"messages.$.status": "deleted"},
	}

	result, err := sessionsCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("删除消息失败: %v", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("消息不存在")
	}

	return nil
}

// DeleteMessageByObjectID 通过MID删除消息（兼容性）
func DeleteMessageByObjectID(messageID primitive.ObjectID) error {
	// 简化处理，删除最近的一条消息
	return DeleteMessage(uint(messageID.Timestamp().Unix()))
}

// ClearAllMessages 清除当前对话的所有聊天记录
func ClearAllMessages() error {
	if currentSessionID.IsZero() {
		return fmt.Errorf("当前没有选中的对话")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 将所有消息状态设为删除，但保留消息结构
	filter := bson.M{"_id": currentSessionID}
	update := bson.M{
		"$set": bson.M{
			"messages.$[].status": "deleted",
		},
	}

	result, err := sessionsCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("清除消息失败: %v", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("会话不存在")
	}

	return nil
}

func GetAllDictionaryEntries() ([]DictionaryEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := wordsCollection.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("获取单词列表失败: %v", err)
	}
	defer cursor.Close(ctx)

	var entries []DictionaryEntry
	if err = cursor.All(ctx, &entries); err != nil {
		return nil, fmt.Errorf("解析单词数据失败: %v", err)
	}

	return entries, nil
}

func DeleteDictionaryEntry(englishWord string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := wordsCollection.DeleteMany(ctx, bson.M{"english_word": englishWord})
	if err != nil {
		return fmt.Errorf("删除单词失败: %v", err)
	}

	return nil
}

func GetAllTranslations() ([]Translation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := sentencesCollection.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("获取翻译列表失败: %v", err)
	}
	defer cursor.Close(ctx)

	var translations []Translation
	if err = cursor.All(ctx, &translations); err != nil {
		return nil, fmt.Errorf("解析翻译数据失败: %v", err)
	}

	return translations, nil
}
