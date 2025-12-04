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
			Content   string `json:"content"`
			Reasoning string `json:"reasoning"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
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
			"enabled": false,
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

	var fullContent strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	lineCount := 0

	log.Debug("开始处理流式响应...")

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lineCount++

		log.Debugf("处理第 %d 行: %s", lineCount, line)

		// 跳过空行和非数据行
		if line == "" || (!strings.HasPrefix(line, "data: ") && line != "data: [DONE]") {
			log.Debugf("跳过非数据行: %s", line)
			continue
		}

		// 检查是否为结束标记
		if line == "data: [DONE]" {
			log.Debug("检测到结束标记 [DONE]")
			break
		}

		// 提取JSON数据部分
		jsonData := strings.TrimPrefix(line, "data: ")
		log.Debugf("提取的JSON数据: %s", jsonData)

		// 跳过非JSON行（如处理状态信息）
		if !strings.HasPrefix(jsonData, "{") {
			log.Debugf("跳过非JSON数据: %s", jsonData)
			continue
		}

		// 解析JSON chunk
		var chunk ChatResponseChunk
		if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
			log.Warnf("解析JSON chunk失败: %v, 数据: %s", err, jsonData)
			continue
		}

		// 提取内容
		if len(chunk.Choices) > 0 {
			content := chunk.Choices[0].Delta.Content
			reasoning := chunk.Choices[0].Delta.Reasoning

			if content != "" {
				log.Debugf("提取到内容: '%s'", content)
				fullContent.WriteString(content)
				// 发送实际内容到通道，而不是原始行数据
				if responseChan != nil {
					responseChan <- content
				}
			}

			if reasoning != "" {
				log.Debugf("提取到推理内容: '%s'", reasoning)
			}

			// 检查是否完成
			if chunk.Choices[0].FinishReason == "stop" {
				log.Debug("检测到完成标记")
				break
			}
		} else {
			log.Debug("chunk.Choices 为空")
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("读取流式响应失败: %v", err)
	}

	// 关闭响应通道
	if responseChan != nil {
		close(responseChan)
	}

	result := fullContent.String()
	log.Infof("流式响应处理完成，总共处理 %d 行，最终内容长度: %d", lineCount, len(result))

	if result == "" {
		return "", fmt.Errorf("API返回为空")
	}

	return result, nil
}
