package llm

import (
	"Mist/config"
	"Mist/global"

	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func CallChatAPI(ctx context.Context, messages []global.Message, responseChan chan string) (string, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return "", fmt.Errorf("配置未加载")
	}

	if len(messages) > 10 {
		messages = messages[len(messages)-10:]
	}

	// -------------------- 构建系统提示 --------------------

	// 获取用户最新输入（假设最后一条是用户消息）
	targetContent := ""
	if len(messages) > 0 {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "user" {
				targetContent = messages[i].Content
				break
			}
		}
	}

	systemMsg := global.Message{
		Role: "system",
		Content: fmt.Sprintf(`**角色与指令:**
你是一个专业的翻译与词典数据解析引擎。你的每一次回复必须同时包含两个严格分离的部分：用户友好回复 (USER_RESPONSE) 和结构化数据 (DATA_JSON)。

**核心规则：**
1.  **用户友好回复 (USER_RESPONSE):** 这是面向用户的自然语言回复。
2.  **结构化数据 (DATA_JSON):** 必须将 JSON 放置在 <DATA_JSON> 和 </DATA_JSON> 标签内。

---

**【任务判断与执行】**

分析用户输入的历史记录和最新请求，判断当前任务类型：

### 任务类型 A: 单词/词组查询 (Dictionary Task)
**条件：** 用户输入是单个单词或短小词组（例如：run, serendipity, database system）。
**DATA_JSON 结构要求：** 必须严格遵循以下**完整的、嵌套的** Word 模型。

// JSON Schema for Dictionary Task
{
  "task_type": "dictionary", // 标记为字典任务
  "english_word": "...", 
  "pos_entries": [
    {
      "pos": "...", 
      "senses": [
        {
          "meaning": "...", 
          "examples": [
            {
              "sentence": "...",
              "translation": "..."
            }
          ]
        }
      ]
    }
  ]
}
**USER_RESPONSE 要求：** 友好地介绍该词的含义、词性和用法。

### 任务类型 B: 句子翻译 (Translation Task)
**条件：** 用户输入是一个完整的句子（例如：I ran out of time.）。
**DATA_JSON 结构要求：** 必须严格遵循以下**精简的** Translation 模型。

// JSON Schema for Translation Task
{
  "task_type": "translation", // 标记为翻译任务
  "source_text": "...",     // 原始输入文本
  "target_translation": "...", // 目标语言的完整翻译
  "explanation": "..."      // 句子意思或背景的解释
}
**USER_RESPONSE 要求：** 直接提供翻译结果，并顺带解释句子的意思。

---

**目标处理内容：** %s

--- **以下是聊天历史记录** ---
`, targetContent),
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

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.APIURL, bytes.NewBuffer(jsonBytes))
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
		if errors.Is(err, context.Canceled) {
			return "", err
		}
		return "", fmt.Errorf("读取流式响应失败: %w", err)
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

// DictionaryTask represents the structure for a dictionary query.
type DictionaryTask struct {
	TaskType    string     `json:"task_type"`
	EnglishWord string     `json:"english_word"`
	PosEntries  []PosEntry `json:"pos_entries"`
}

// PosEntry represents a part-of-speech entry.
type PosEntry struct {
	Pos    string  `json:"pos"`
	Senses []Sense `json:"senses"`
}

// Sense represents a specific meaning of a word.
type Sense struct {
	Meaning  string    `json:"meaning"`
	Examples []Example `json:"examples"`
}

// Example represents an example sentence.
type Example struct {
	Sentence    string `json:"sentence"`
	Translation string `json:"translation"`
}

// TranslationTask represents the structure for a sentence translation.
type TranslationTask struct {
	TaskType          string `json:"task_type"`
	SourceText        string `json:"source_text"`
	TargetTranslation string `json:"target_translation"`
	Explanation       string `json:"explanation"`
}

// BaseTask is used to determine the task type from the JSON data.
type BaseTask struct {
	TaskType string `json:"task_type"`
}

// ParseLLMResponse separates the user-friendly response from the structured JSON data.
// It returns the user response, the JSON string, and an error if parsing fails.
func ParseLLMResponse(response string) (string, string, error) {
	const dataTagStart = "<DATA_JSON>"
	const dataTagEnd = "</DATA_JSON>"

	start := strings.Index(response, dataTagStart)
	if start == -1 {
		// If the tag is not found, we assume the entire response is the user-facing part
		// and there is no JSON data. This is a valid case.
		return response, "", nil
	}

	end := strings.Index(response, dataTagEnd)
	if end == -1 {
		return "", "", fmt.Errorf("found <DATA_JSON> but no closing </DATA_JSON>")
	}

	// The user response is everything before the <DATA_JSON> tag.
	userResponse := strings.TrimSpace(response[:start])

	// The JSON data is everything between the tags.
	jsonData := strings.TrimSpace(response[start+len(dataTagStart) : end])

	return userResponse, jsonData, nil
}
