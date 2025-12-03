package main

import (
	mywidget "Mist/MyWidget"
	"Mist/config"
	"Mist/database"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	Stream      bool      `json:"stream"`
}

type ChatResponseChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

var (
	messages         []Message
	mu               sync.Mutex
	entry            *mywidget.MyMultiLine
	chatList         *fyne.Container
	scroll           *container.Scroll
	conversationList *fyne.Container
	currentConvID    uint
	mainWindow       fyne.Window
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

	myApp := app.New()
	w := myApp.NewWindow("AI 助手")
	mainWindow = w

	// 获取当前对话ID
	currentConvID = database.GetCurrentConversationID()

	// 创建对话列表容器
	conversationList = container.NewVBox()

	// 加载并显示对话列表
	refreshConversationList()

	// 创建对话列表的滚动容器
	convScroll := container.NewVScroll(conversationList)
	convScroll.SetMinSize(fyne.NewSize(200, 300))

	// 新建对话按钮
	newConvBtn := widget.NewButton("新建对话", func() {
		conv, err := database.CreateConversation("新对话")
		if err != nil {
			fmt.Printf("创建对话失败: %v\n", err)
			return
		}
		// 切换到新对话
		switchConversation(conv.ID)
		refreshConversationList()
	})

	// 对话列表区域（包含按钮和列表）
	convListContainer := container.NewBorder(newConvBtn, nil, nil, nil, convScroll)

	// 用 container.NewVBox 创建垂直容器
	chatList = container.NewVBox()

	// 加载当前对话的历史消息
	loadCurrentConversationMessages()

	scroll = container.NewVScroll(chatList)
	scroll.SetMinSize(fyne.NewSize(200, 300))

	entry = mywidget.NewMyMultiLine(sendMessage)
	entry.SetPlaceHolder("输入问题…")

	// 清除聊天记录按钮
	clearBtn := widget.NewButton("清除记录", func() {
		// 清除数据库中的记录
		if err := database.ClearAllMessages(); err != nil {
			fmt.Printf("清除记录失败: %v\n", err)
			return
		}

		// 清除内存中的消息
		mu.Lock()
		messages = []Message{}
		mu.Unlock()

		// 清除 UI 中的显示
		chatList.RemoveAll()
		chatList.Refresh()
	})

	// 设置按钮
	settingsBtn := widget.NewButton("⚙", showSettingsDialog)
	settingsBtn.Importance = widget.MediumImportance

	// 顶部工具栏：设置按钮在右侧
	topBar := container.NewBorder(nil, nil, nil, settingsBtn, widget.NewLabel(""))

	// 底部容器：输入框和清除按钮
	bottomButtons := container.NewHBox(clearBtn)
	bottom := container.NewBorder(nil, nil, nil, bottomButtons, entry)

	// 右侧聊天区域
	chatArea := container.NewBorder(topBar, bottom, nil, nil, scroll)

	// 整体布局：左侧对话列表，右侧聊天区域
	content := container.NewHSplit(convListContainer, chatArea)
	content.SetOffset(0.25) // 左侧占25%宽度

	w.SetContent(content)
	w.Resize(fyne.NewSize(800, 600))
	w.ShowAndRun()
}

func callChatAPI(messages []Message, onChunk func(string)) (string, error) {
	// 获取当前配置
	cfg := config.GetConfig()
	if cfg == nil {
		return "", fmt.Errorf("配置未加载")
	}

	// 添加系统消息
	systemMsg := Message{
		Role:    "system",
		Content: "你是一个有用的AI助手。",
	}
	allMessages := append([]Message{systemMsg}, messages...)

	// 构建请求
	reqBody := ChatRequest{
		Model:       cfg.Model,
		Messages:    allMessages,
		Temperature: 1.0,
		Stream:      true,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %v", err)
	}

	// 创建HTTP请求
	req, err := http.NewRequest("POST", cfg.APIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API返回错误 (状态码: %d): %s", resp.StatusCode, string(bodyBytes))
	}

	// 处理流式响应 (SSE格式) - 真正的流式处理
	var fullResponse strings.Builder

	// 使用 bufio.Scanner 逐行读取，实现真正的流式处理
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行
		if line == "" {
			continue
		}

		// 处理SSE格式: data: {...}
		if strings.HasPrefix(line, "data: ") {
			jsonStr := strings.TrimPrefix(line, "data: ")
			jsonStr = strings.TrimSpace(jsonStr)

			if jsonStr == "[DONE]" {
				break
			}

			if jsonStr != "" {
				var chunk ChatResponseChunk
				if err := json.Unmarshal([]byte(jsonStr), &chunk); err != nil {
					// 解析失败，跳过这一行
					continue
				}

				// 检查是否有choices数组且不为空
				if len(chunk.Choices) > 0 {
					content := chunk.Choices[0].Delta.Content
					if content != "" {
						fullResponse.WriteString(content)
						// 实时回调新内容
						if onChunk != nil {
							onChunk(content)
						}
					}
				}
			}
		} else if strings.HasPrefix(line, "{") {
			// 如果直接是JSON（没有data:前缀），也尝试解析
			var chunk ChatResponseChunk
			if err := json.Unmarshal([]byte(line), &chunk); err == nil {
				if len(chunk.Choices) > 0 {
					content := chunk.Choices[0].Delta.Content
					if content != "" {
						fullResponse.WriteString(content)
						// 实时回调新内容
						if onChunk != nil {
							onChunk(content)
						}
					}
				}
			}
		}
	}

	// 检查扫描错误
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("读取流式响应失败: %v", err)
	}

	result := fullResponse.String()

	// 如果结果为空，返回错误信息
	if result == "" {
		return "", fmt.Errorf("API返回为空")
	}

	return result, nil
}

// refreshConversationList 刷新对话列表
func refreshConversationList() {
	conversationList.RemoveAll()

	conversations, err := database.GetAllConversations()
	if err != nil {
		fmt.Printf("加载对话列表失败: %v\n", err)
		return
	}

	currentID := database.GetCurrentConversationID()

	for _, conv := range conversations {
		convID := conv.ID
		convName := conv.Name

		// 创建对话项按钮
		convBtn := widget.NewButton(convName, func(id uint) func() {
			return func() {
				switchConversation(id)
			}
		}(convID))

		// 如果是当前对话，高亮显示
		if convID == currentID {
			convBtn.Importance = widget.HighImportance
		} else {
			convBtn.Importance = widget.MediumImportance
		}

		// 创建删除按钮
		deleteBtn := widget.NewButton("删除", func(id uint) func() {
			return func() {
				// 如果删除的是当前对话，需要切换到其他对话
				if id == currentID {
					conversations, _ := database.GetAllConversations()
					found := false
					for _, c := range conversations {
						if c.ID != id {
							switchConversation(c.ID)
							found = true
							break
						}
					}
					// 如果没有其他对话，创建新对话
					if !found {
						newConv, err := database.CreateConversation("新对话")
						if err != nil {
							fmt.Printf("创建新对话失败: %v\n", err)
							return
						}
						switchConversation(newConv.ID)
					}
				}
				// 删除对话
				if err := database.DeleteConversation(id); err != nil {
					fmt.Printf("删除对话失败: %v\n", err)
					return
				}
				refreshConversationList()
			}
		}(convID))

		// 创建对话项容器
		convItem := container.NewBorder(nil, nil, nil, deleteBtn, convBtn)
		conversationList.Add(convItem)
	}

	conversationList.Refresh()
}

// switchConversation 切换对话
func switchConversation(convID uint) {
	// 设置当前对话
	if err := database.SetCurrentConversationID(convID); err != nil {
		fmt.Printf("切换对话失败: %v\n", err)
		return
	}

	currentConvID = convID

	// 加载新对话的消息
	loadCurrentConversationMessages()

	// 刷新对话列表（更新高亮）
	refreshConversationList()
}

// loadCurrentConversationMessages 加载当前对话的消息
func loadCurrentConversationMessages() {
	// 清除当前显示
	chatList.RemoveAll()

	// 加载历史消息
	history, err := database.LoadHistoryMessages()
	if err != nil {
		fmt.Printf("加载历史消息失败: %v\n", err)
		return
	}

	// 转换为 Message 类型
	historyMessages := make([]Message, len(history))
	for i, h := range history {
		historyMessages[i] = Message{
			Role:    h.Role,
			Content: h.Content,
		}
	}

	mu.Lock()
	messages = historyMessages
	mu.Unlock()

	// 在 UI 中显示历史消息
	for i, msg := range historyMessages {
		sender := "你"
		if msg.Role == "assistant" {
			sender = "AI"
		} else if msg.Role == "system" {
			continue // 跳过系统消息
		}
		appendMessage(chatList, sender, msg.Content, history[i].ID)
	}

	chatList.Refresh()
	if scroll != nil {
		scroll.ScrollToBottom()
	}
}

func appendMessage(chatList *fyne.Container, sender, content string, messageID uint) {
	// 创建消息标签
	label := widget.NewLabel(sender + ": " + content)
	label.Wrapping = fyne.TextWrapWord

	// 创建删除按钮
	var messageRow *fyne.Container
	deleteBtn := widget.NewButton("删除", func() {
		// 从数据库软删除
		if err := database.DeleteMessage(messageID); err != nil {
			fmt.Printf("删除消息失败: %v\n", err)
			return
		}

		// 从内存中移除对应的消息
		// 重新加载历史消息以同步 messages 数组
		mu.Lock()
		history, err := database.LoadHistoryMessages()
		if err == nil {
			// 转换为 Message 类型
			historyMessages := make([]Message, len(history))
			for i, h := range history {
				historyMessages[i] = Message{
					Role:    h.Role,
					Content: h.Content,
				}
			}
			messages = historyMessages
		} else {
			// 如果加载失败，尝试通过内容和角色匹配删除
			for i := range messages {
				if (sender == "你" && messages[i].Role == "user" && messages[i].Content == content) ||
					(sender == "AI" && messages[i].Role == "assistant" && messages[i].Content == content) {
					messages = append(messages[:i], messages[i+1:]...)
					break
				}
			}
		}
		mu.Unlock()

		// 从UI中移除整个消息行
		chatList.Remove(messageRow)
		chatList.Refresh()
	})

	// 使用 Border 布局：消息内容在中间，删除按钮在右侧
	messageRow = container.NewBorder(nil, nil, nil, deleteBtn, label)
	chatList.Add(messageRow)
}

// createChunkHandler 创建处理流式chunk的回调函数
// 这个函数返回的闭包会被多次调用（每次收到chunk时），但函数本身只创建一次
func createChunkHandler(fullContent *strings.Builder, aiLabel *widget.Label) func(string) {
	return func(chunk string) {
		fullContent.WriteString(chunk)
		// 在主线程更新标签内容
		fyne.Do(func() {
			if aiLabel != nil {
				aiLabel.SetText("AI: " + fullContent.String())
				aiLabel.Refresh()
				scroll.ScrollToBottom()
			}
		})
	}
}

// appendStreamingMessage 创建可流式更新的消息（用于AI回复）
func appendStreamingMessage(chatList *fyne.Container, sender string) (*widget.Label, *fyne.Container) {
	// 创建消息标签，初始内容为空
	label := widget.NewLabel(sender + ": ")
	label.Wrapping = fyne.TextWrapWord

	// 创建删除按钮（流式消息完成前暂时禁用）
	deleteBtn := widget.NewButton("删除", func() {})
	deleteBtn.Disable()

	var messageRow *fyne.Container
	messageRow = container.NewBorder(nil, nil, nil, deleteBtn, label)
	chatList.Add(messageRow)

	return label, messageRow
}

// 发送消息
func sendMessage() {
	text := entry.Text
	if text == "" {
		return
	}

	mu.Lock()
	messages = append(messages, Message{Role: "user", Content: text})
	mu.Unlock()

	// 保存用户消息到数据库
	userMsgID, err := database.SaveMessage("user", text)
	if err != nil {
		fmt.Printf("保存用户消息失败: %v\n", err)
		return
	}

	appendMessage(chatList, "你", text, userMsgID)
	entry.SetText("")

	go func(userText string) {
		// 调用 API（读取 messages 时需要加锁）
		mu.Lock()
		messagesCopy := make([]Message, len(messages))
		copy(messagesCopy, messages)
		mu.Unlock()

		// 先创建流式消息标签
		var aiLabel *widget.Label
		var aiMessageRow *fyne.Container
		var fullContent strings.Builder
		var aiMsgID uint

		fyne.Do(func() {
			aiLabel, aiMessageRow = appendStreamingMessage(chatList, "AI")
			scroll.ScrollToBottom()
		})

		// 创建回调函数，实时更新UI
		// 注意：这个闭包只创建一次，但会被多次调用（每次收到chunk时）
		// goroutine结束后，闭包和捕获的变量会被自动垃圾回收
		onChunk := createChunkHandler(&fullContent, aiLabel)

		response, err := callChatAPI(messagesCopy, onChunk)
		if err != nil {
			response = "错误: " + err.Error()
			// 如果出错，也要更新UI显示错误信息
			fyne.Do(func() {
				if aiLabel != nil {
					aiLabel.SetText("AI: " + response)
					aiLabel.Refresh()
				}
			})
		}

		// 如果流式处理过程中没有收集到内容，使用最终响应
		if fullContent.Len() == 0 {
			fullContent.WriteString(response)
		}

		finalResponse := fullContent.String()

		mu.Lock()
		messages = append(messages, Message{Role: "assistant", Content: finalResponse})
		mu.Unlock()

		// 保存 AI 回复到数据库
		aiMsgID, err = database.SaveMessage("assistant", finalResponse)
		if err != nil {
			fmt.Printf("保存 AI 回复失败: %v\n", err)
		}

		// 流式完成后，启用删除按钮并设置正确的删除功能
		fyne.Do(func() {
			if aiMessageRow != nil && aiLabel != nil {
				// 从chatList中移除旧的消息行
				chatList.Remove(aiMessageRow)

				// 创建新的删除按钮
				deleteBtn := widget.NewButton("删除", func() {
					// 从数据库软删除
					if err := database.DeleteMessage(aiMsgID); err != nil {
						fmt.Printf("删除消息失败: %v\n", err)
						return
					}

					// 从内存中移除对应的消息
					mu.Lock()
					for i := range messages {
						if messages[i].Role == "assistant" && messages[i].Content == finalResponse {
							messages = append(messages[:i], messages[i+1:]...)
							break
						}
					}
					mu.Unlock()

					// 从UI中移除整个消息行
					chatList.Remove(aiMessageRow)
					chatList.Refresh()
				})

				// 创建新的消息行容器（带删除按钮）
				aiMessageRow = container.NewBorder(nil, nil, nil, deleteBtn, aiLabel)
				chatList.Add(aiMessageRow)
				chatList.Refresh()
			}
			scroll.ScrollToBottom()
		})
	}(text)
}

// showSettingsDialog 显示设置对话框
func showSettingsDialog() {
	if mainWindow == nil {
		return
	}

	cfg := config.GetConfig()
	if cfg == nil {
		// 如果配置未加载，尝试重新加载
		var err error
		cfg, err = config.LoadConfig()
		if err != nil {
			dialog.ShowError(fmt.Errorf("加载配置失败: %v", err), mainWindow)
			return
		}
	}

	// 创建对话框窗口
	settingsWindow := fyne.CurrentApp().NewWindow("设置")

	// 创建输入框
	apiKeyEntry := widget.NewEntry()
	apiKeyEntry.SetText(cfg.APIKey)
	apiKeyEntry.SetPlaceHolder("请输入 API Key")

	apiURLEntry := widget.NewEntry()
	apiURLEntry.SetText(cfg.APIURL)
	apiURLEntry.SetPlaceHolder("请输入 API URL")

	modelEntry := widget.NewEntry()
	modelEntry.SetText(cfg.Model)
	modelEntry.SetPlaceHolder("请输入模型名称")

	// 创建表单
	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "API Key", Widget: apiKeyEntry},
			{Text: "API URL", Widget: apiURLEntry},
			{Text: "Model", Widget: modelEntry},
		},
		OnSubmit: func() {
			// 验证输入
			if apiKeyEntry.Text == "" {
				dialog.ShowError(fmt.Errorf("API Key 不能为空"), settingsWindow)
				return
			}
			if apiURLEntry.Text == "" {
				dialog.ShowError(fmt.Errorf("API URL 不能为空"), settingsWindow)
				return
			}
			if modelEntry.Text == "" {
				dialog.ShowError(fmt.Errorf("Model 不能为空"), settingsWindow)
				return
			}

			// 保存配置
			newConfig := &config.AppConfig{
				APIKey: apiKeyEntry.Text,
				APIURL: apiURLEntry.Text,
				Model:  modelEntry.Text,
			}

			if err := config.SaveConfig(newConfig); err != nil {
				dialog.ShowError(fmt.Errorf("保存配置失败: %v", err), settingsWindow)
				return
			}

			dialog.ShowInformation("成功", "配置已保存", settingsWindow)
			settingsWindow.Close()
		},
		OnCancel: func() {
			settingsWindow.Close()
		},
	}

	settingsWindow.SetContent(form)
	settingsWindow.Resize(fyne.NewSize(500, 250))
	settingsWindow.CenterOnScreen()
	settingsWindow.Show()
}
