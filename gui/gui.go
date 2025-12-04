package gui

import (
	mywidget "Mist/MyWidget"
	"Mist/service"
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/atotto/clipboard"
	hook "github.com/robotn/gohook"
	log "github.com/sirupsen/logrus"
)

var (
	messages                  []service.Message
	mu                        sync.Mutex
	entry                     *mywidget.MyMultiLine
	chatList                  *fyne.Container
	scroll                    *container.Scroll
	conversationList          *fyne.Container
	currentConvID             uint
	mainWindow                fyne.Window
	messageService            *service.MessageService
	conversationService       *service.ConversationService
	configService             *service.ConfigService
	currentStreamingLabel     *widget.Label
	currentStreamingContainer *fyne.Container
)

// Global map to store message metadata for efficient lookup
var messageMetadata = make(map[*fyne.Container]MessageMetadata)
var metadataMu sync.RWMutex

// MessageMetadata stores metadata for a message widget
type MessageMetadata struct {
	ID      uint
	Sender  string
	Content string
}

// Run 启动并运行 Fyne GUI
func Run() {
	// Initialize services
	messageService = service.NewMessageService()
	conversationService = service.NewConversationService()
	configService = service.NewConfigService()

	// Set up event handlers
	messageService.SetMessageEventHandler(handleMessageEvent)

	// 获取当前对话ID（由 conversation service 管理）
	currentConvID = conversationService.GetCurrentConversationID()

	myApp := app.New()
	w := myApp.NewWindow("AI 助手")
	mainWindow = w

	// 创建对话列表容器
	conversationList = container.NewVBox()
	// 异步加载并显示对话列表
	refreshConversationListAsync()

	// 对话列表的滚动容器
	convScroll := container.NewVScroll(conversationList)
	convScroll.SetMinSize(fyne.NewSize(200, 300))

	// 新建对话按钮
	newConvBtn := widget.NewButton("新建对话", func() {
		conv, err := conversationService.CreateConversation("新对话")
		if err != nil {
			showError("创建对话失败", err)
			return
		}
		switchConversation(conv.ID)
	})

	convListContainer := container.NewBorder(newConvBtn, nil, nil, nil, convScroll)

	// 聊天区
	chatList = container.NewVBox()
	// 异步加载消息
	loadCurrentConversationMessagesAsync()

	scroll = container.NewVScroll(chatList)
	scroll.SetMinSize(fyne.NewSize(200, 300))

	entry = mywidget.NewMyMultiLine(sendMessage)
	entry.SetPlaceHolder("输入问题…")

	// 清除聊天记录按钮
	clearBtn := widget.NewButton("清除记录", func() {
		if err := messageService.ClearAllMessages(); err != nil {
			showError("清除记录失败", err)
			return
		}
		mu.Lock()
		messages = []service.Message{}
		mu.Unlock()
		chatList.RemoveAll()
		chatList.Refresh()
	})

	settingsBtn := widget.NewButton("⚙", showSettingsDialog)
	settingsBtn.Importance = widget.MediumImportance

	topBar := container.NewBorder(nil, nil, nil, settingsBtn, widget.NewLabel(""))
	bottomButtons := container.NewHBox(clearBtn)
	bottom := container.NewBorder(nil, nil, nil, bottomButtons, entry)

	chatArea := container.NewBorder(topBar, bottom, nil, nil, scroll)
	content := container.NewHSplit(convListContainer, chatArea)
	content.SetOffset(0.25)

	w.SetContent(content)
	w.Resize(fyne.NewSize(800, 600))

	//热键检测
	hook.Register(hook.KeyDown, []string{"ctrl", "c"}, func(e hook.Event) {
		txt, err := clipboard.ReadAll()
		if err != nil {
			log.Println("读取剪贴板失败:", err)
			return
		}
		fyne.Do(func() {
			entry.SetText(txt)
			w.Canvas().Focus(entry)
			entry.Refresh()
			w.Hide()
			w.Show()
			w.RequestFocus()
		})
	})
	// 启动 hook 消息循环
	go func() {
		s := hook.Start()
		<-hook.Process(s)
	}()
	defer hook.End()

	w.ShowAndRun()
}

// handleMessageEvent handles events from the message service
func handleMessageEvent(event service.MessageEvent) {
	fyne.Do(func() {
		switch event.Type {
		case "user_message_sent":
			if event.Message != nil {
				appendMessage(chatList, "你", event.Message.Content, event.Message.ID)
				entry.SetText("")
			}
		case "ai_response_start":
			// Create streaming message container
			currentStreamingLabel = widget.NewLabel("AI: ")
			currentStreamingLabel.Wrapping = fyne.TextWrapWord
			currentStreamingContainer = container.NewVBox(currentStreamingLabel)
			chatList.Add(currentStreamingContainer)
			scroll.ScrollToBottom()
		case "ai_response_chunk":
			// Update streaming content in real-time
			if currentStreamingLabel != nil && event.Content != "" {
				currentText := currentStreamingLabel.Text
				currentStreamingLabel.SetText(currentText + event.Content)
				currentStreamingLabel.Refresh()
				scroll.ScrollToBottom()
			}
		case "ai_response_complete":
			if event.Message != nil {
				mu.Lock()
				messages = append(messages, *event.Message)
				mu.Unlock()

				// Replace streaming container with final message
				if currentStreamingContainer != nil {
					// Remove streaming container
					removeContainerFromList(chatList, currentStreamingContainer)
					currentStreamingContainer = nil
					currentStreamingLabel = nil
				}

				// Add final message with delete button
				appendMessage(chatList, "AI", event.Message.Content, event.Message.ID)
				scroll.ScrollToBottom()
			}
		case "error":
			if event.Error != nil {
				dialog.ShowError(fmt.Errorf("发生错误: %v", event.Error), mainWindow)
			}
		}
	})
}

// showError shows an error dialog
func showError(operation string, err error) {
	fmt.Printf("%s: %v\n", operation, err)
	dialog.ShowError(fmt.Errorf("%s: %v", operation, err), mainWindow)
}

// refreshConversationListAsync 异步刷新对话列表
func refreshConversationListAsync() {
	go func() {
		fyne.Do(refreshConversationList)
	}()
}

// refreshConversationList 刷新对话列表
func refreshConversationList() {

	conversationList.RemoveAll()

	conversations, err := conversationService.GetAllConversations()
	if err != nil {
		showError("加载对话列表失败", err)
		return
	}

	currentID := conversationService.GetCurrentConversationID()

	for _, conv := range conversations {
		convID := conv.ID
		convName := conv.Name
		convBtn := widget.NewButton(convName, func(id uint) func() {
			return func() { switchConversation(id) }
		}(convID))

		if convID == currentID {
			convBtn.Importance = widget.HighImportance
		} else {
			convBtn.Importance = widget.MediumImportance
		}

		deleteBtn := widget.NewButton("删除", func(id uint) func() {
			return func() {
				if id == currentID {
					conversations, _ := conversationService.GetAllConversations()
					found := false
					for _, c := range conversations {
						if c.ID != id {
							switchConversation(c.ID)
							found = true
							break
						}
					}
					if !found {
						newConv, err := conversationService.CreateConversation("新对话")
						if err != nil {
							showError("创建新对话失败", err)
							return
						}
						switchConversation(newConv.ID)
					}
				}
				if err := conversationService.DeleteConversation(id); err != nil {
					showError("删除对话失败", err)
					return
				}
				refreshConversationListAsync() // Also make this async
			}
		}(convID))

		convItem := container.NewBorder(nil, nil, nil, deleteBtn, convBtn)
		conversationList.Add(convItem)
	}

	conversationList.Refresh()
}

// switchConversation 切换对话
func switchConversation(convID uint) {
	if err := conversationService.SwitchConversation(convID); err != nil {
		showError("切换对话失败", err)
		return
	}
	currentConvID = convID
	loadCurrentConversationMessagesAsync() // Make this async
	refreshConversationListAsync()         // Make this async
}

// loadCurrentConversationMessagesAsync 异步加载当前对话的消息
func loadCurrentConversationMessagesAsync() {
	go func() {
		fyne.Do(loadCurrentConversationMessages)
	}()
}

// loadCurrentConversationMessages 加载当前对话的消息
func loadCurrentConversationMessages() {
	chatList.RemoveAll()

	starttime := time.Now()
	history, err := messageService.LoadMessages()
	if err != nil {
		showError("加载历史消息失败", err)
		return
	}

	mu.Lock()
	messages = history
	mu.Unlock()

	// Limit the number of messages we display initially to improve performance
	// Based on our memory, we should cap initial rendering
	maxMessagesToShow := 100
	startIndex := 0
	if len(history) > maxMessagesToShow {
		startIndex = len(history) - maxMessagesToShow
	}

	fmt.Println("读取历史消息耗时：", time.Since(starttime))
	for i := startIndex; i < len(history); i++ {
		sender := "你"
		if history[i].Role == "assistant" {
			sender = "AI"
		} else if history[i].Role == "system" {
			continue
		}
		appendMessage(chatList, sender, history[i].Content, history[i].ID)
	}
	fmt.Println("界面渲染耗时：", time.Since(starttime))

	if scroll != nil {
		scroll.ScrollToBottom()
	}
}

// createMessageWithDeleteButton creates a message container with a delete button
// Uses a shared delete handler with message ID to avoid creating closures in loops
func createMessageWithDeleteButton(label *widget.Label, messageID uint) *fyne.Container {
	deleteBtn := widget.NewButton("删除", func() {
		handleDeleteMessage(messageID)
	})
	return container.NewBorder(nil, nil, nil, deleteBtn, label)
}

// appendMessage adds a new message to the chat list
func appendMessage(chatList *fyne.Container, sender, content string, messageID uint) {
	label := widget.NewLabel(sender + ": " + content)
	label.Wrapping = fyne.TextWrapWord

	messageRow := createMessageWithDeleteButton(label, messageID)
	chatList.Add(messageRow)

	// Store metadata for this message
	metadataMu.Lock()
	messageMetadata[messageRow] = MessageMetadata{
		ID:      messageID,
		Sender:  sender,
		Content: content,
	}
	metadataMu.Unlock()
}

// handleDeleteMessage handles deletion of a message by its ID
func handleDeleteMessage(messageID uint) {
	// Find the container and metadata associated with this message ID
	var targetContainer *fyne.Container
	var targetMetadata MessageMetadata

	metadataMu.RLock()
	for container, metadata := range messageMetadata {
		if metadata.ID == messageID {
			targetContainer = container
			targetMetadata = metadata
			break
		}
	}
	metadataMu.RUnlock()

	if targetContainer == nil {
		showError("删除消息失败", fmt.Errorf("未找到要删除的消息"))
		return
	}

	if err := messageService.DeleteMessage(messageID); err != nil {
		showError("删除消息失败", err)
		return
	}

	mu.Lock()
	history, err := messageService.LoadMessages()
	if err == nil {
		messages = history
	} else {
		for i := range messages {
			if (targetMetadata.Sender == "你" && messages[i].Role == "user" && messages[i].Content == targetMetadata.Content) ||
				(targetMetadata.Sender == "AI" && messages[i].Role == "assistant" && messages[i].Content == targetMetadata.Content) {
				messages = append(messages[:i], messages[i+1:]...)
				break
			}
		}
	}
	mu.Unlock()

	// Remove the container from the chat list
	removeContainerFromList(chatList, targetContainer)

	// Clean up metadata
	metadataMu.Lock()
	delete(messageMetadata, targetContainer)
	metadataMu.Unlock()
}

// removeContainerFromList removes a container from a list
func removeContainerFromList(list *fyne.Container, item *fyne.Container) {
	index := -1
	for i, obj := range list.Objects {
		if obj == item {
			index = i
			break
		}
	}

	if index != -1 {
		list.Objects = append(list.Objects[:index], list.Objects[index+1:]...)
		list.Refresh()
	}
}

func sendMessage() {
	text := entry.Text
	if text == "" {
		return
	}

	// Add user message to local messages array immediately
	mu.Lock()
	messages = append(messages, service.Message{Role: "user", Content: text})
	mu.Unlock()

	// Send message through service (non-blocking)
	go func(userText string) {
		req := service.SendMessageRequest{Content: userText}
		// Create response channel for streaming
		responseChan := make(chan string, 100)

		// Start goroutine to handle streaming response
		go func() {
			for chunk := range responseChan {
				// The service will handle forwarding chunks to the event handler
				_ = chunk
			}
		}()

		_, err := messageService.StreamMessage(req, responseChan)
		if err != nil {
			fyne.Do(func() {
				showError("发送消息失败", err)
				// Clean up streaming container on error
				if currentStreamingContainer != nil {
					removeContainerFromList(chatList, currentStreamingContainer)
					currentStreamingContainer = nil
					currentStreamingLabel = nil
				}
			})
		}
	}(text)
}

func showSettingsDialog() {
	if mainWindow == nil {
		return
	}

	cfg, err := configService.GetConfig()
	if err != nil {
		showError("加载配置失败", err)
		return
	}

	settingsWindow := fyne.CurrentApp().NewWindow("设置")

	apiKeyEntry := widget.NewEntry()
	apiKeyEntry.SetText(cfg.APIKey)
	apiKeyEntry.SetPlaceHolder("请输入 API Key")

	apiURLEntry := widget.NewEntry()
	apiURLEntry.SetText(cfg.APIURL)
	apiURLEntry.SetPlaceHolder("请输入 API URL")

	modelEntry := widget.NewEntry()
	modelEntry.SetText(cfg.Model)
	modelEntry.SetPlaceHolder("请输入模型名称")

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "API Key", Widget: apiKeyEntry},
			{Text: "API URL", Widget: apiURLEntry},
			{Text: "Model", Widget: modelEntry},
		},
		OnSubmit: func() {
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

			newConfig := &service.AppConfig{APIKey: apiKeyEntry.Text, APIURL: apiURLEntry.Text, Model: modelEntry.Text}
			if err := configService.SaveConfig(newConfig); err != nil {
				dialog.ShowError(fmt.Errorf("保存配置失败: %v", err), settingsWindow)
				return
			}

			dialog.ShowInformation("成功", "配置已保存", settingsWindow)
			settingsWindow.Close()
		},
		OnCancel: func() { settingsWindow.Close() },
	}

	settingsWindow.SetContent(form)
	settingsWindow.Resize(fyne.NewSize(500, 250))
	settingsWindow.CenterOnScreen()
	settingsWindow.Show()
}
