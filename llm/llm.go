package llm

import (
	"Mist/config"
	"Mist/global"

	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
)

type ChatRequest struct {
	Model       string           `json:"model"`
	Messages    []global.Message `json:"messages"`
	Temperature float64          `json:"temperature"`
	Stream      bool             `json:"stream"`
	Reasoning   map[string]bool  `json:"reasoning"`
}

type ChatResponseChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func CallChatAPI(messages []global.Message, responseChan chan string) (string, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return "", fmt.Errorf("配置未加载")
	}

	// -------------------- 构建系统提示 --------------------

	systemMsg := global.Message{
		Role:    "system",
		Content: "你负责把用户的输入翻译成中文，如果用户输入的是单个单词则翻译成中文并附带用法和例句，如果是句子则翻译成中文并结合上下文对句意进行解释",
	}

	messagesToSend := append([]global.Message{systemMsg}, messages...)
	log.Info("Chat messages:", messagesToSend)

	// -------------------- 构建请求体 --------------------

	reqBody := ChatRequest{
		Model:       cfg.Model,
		Messages:    messagesToSend,
		Temperature: 1.0,
		Stream:      true,
		Reasoning: map[string]bool{
			"enabled": true,
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequest("POST", cfg.APIURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API返回错误 (状态码: %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var fullResponse strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		fullResponse.WriteString(line + "\n")
		if line=="[DONE]" {
			break
		}
		responseChan <- line // 发送每一行数据到通道
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("读取流式响应失败: %v", err)
	}

	result := fullResponse.String()
	if result == "" {
		return "", fmt.Errorf("API返回为空")
	}
	log.Info("Full response:", result)
	return result,nil
}
