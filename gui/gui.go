package gui

import (
	mywidget "Mist/MyWidget"
	"Mist/config"
	"Mist/database"
	"Mist/global"
	"Mist/llm"
	"fmt"
	"strings"
	"sync"

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
	messages         []global.Message
	mu               sync.Mutex
	entry            *mywidget.MyMultiLine
	chatList         *fyne.Container
	scroll           *container.Scroll
	conversationList *fyne.Container
	currentConvID    uint
	mainWindow       fyne.Window
)

// Run 启动并运行 Fyne GUI
func Run() {
	// 获取当前对话ID（由 database 管理）
	currentConvID = database.GetCurrentConversationID()

	myApp := app.New()
	w := myApp.NewWindow("AI 助手")
	mainWindow = w

	// 创建对话列表容器
	conversationList = container.NewVBox()
	// 加载并显示对话列表
	refreshConversationList()

	// 对话列表的滚动容器
	convScroll := container.NewVScroll(conversationList)
	convScroll.SetMinSize(fyne.NewSize(200, 300))

	// 新建对话按钮
	newConvBtn := widget.NewButton("新建对话", func() {
		conv, err := database.CreateConversation("新对话")
		if err != nil {
			fmt.Printf("创建对话失败: %v\n", err)
			return
		}
		switchConversation(conv.ID)
		refreshConversationList()
	})

	convListContainer := container.NewBorder(newConvBtn, nil, nil, nil, convScroll)

	// 聊天区
	chatList = container.NewVBox()
	loadCurrentConversationMessages()

	scroll = container.NewVScroll(chatList)
	scroll.SetMinSize(fyne.NewSize(200, 300))

	entry = mywidget.NewMyMultiLine(sendMessage)
	entry.SetPlaceHolder("输入问题…")

	// 清除聊天记录按钮
	clearBtn := widget.NewButton("清除记录", func() {
		if err := database.ClearAllMessages(); err != nil {
			fmt.Printf("清除记录失败: %v\n", err)
			return
		}
		mu.Lock()
		messages = []global.Message{}
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
					conversations, _ := database.GetAllConversations()
					found := false
					for _, c := range conversations {
						if c.ID != id {
							switchConversation(c.ID)
							found = true
							break
						}
					}
					if !found {
						newConv, err := database.CreateConversation("新对话")
						if err != nil {
							fmt.Printf("创建新对话失败: %v\n", err)
							return
						}
						switchConversation(newConv.ID)
					}
				}
				if err := database.DeleteConversation(id); err != nil {
					fmt.Printf("删除对话失败: %v\n", err)
					return
				}
				refreshConversationList()
			}
		}(convID))

		convItem := container.NewBorder(nil, nil, nil, deleteBtn, convBtn)
		conversationList.Add(convItem)
	}

	conversationList.Refresh()
}

// switchConversation 切换对话
func switchConversation(convID uint) {
	if err := database.SetCurrentConversationID(convID); err != nil {
		fmt.Printf("切换对话失败: %v\n", err)
		return
	}
	currentConvID = convID
	loadCurrentConversationMessages()
	refreshConversationList()
}

// loadCurrentConversationMessages 加载当前对话的消息
func loadCurrentConversationMessages() {
	chatList.RemoveAll()

	history, err := database.LoadHistoryMessages()
	if err != nil {
		fmt.Printf("加载历史消息失败: %v\n", err)
		return
	}

	historyMessages := make([]global.Message, len(history))
	for i, h := range history {
		historyMessages[i] = global.Message{Role: h.Role, Content: h.Content}
	}

	mu.Lock()
	messages = historyMessages
	mu.Unlock()

	for i, msg := range historyMessages {
		sender := "你"
		if msg.Role == "assistant" {
			sender = "AI"
		} else if msg.Role == "system" {
			continue
		}
		appendMessage(chatList, sender, msg.Content, history[i].ID)
	}

	chatList.Refresh()
	if scroll != nil {
		scroll.ScrollToBottom()
	}
}

// createMessageWithDeleteButton 创建带删除按钮的消息容器
func createMessageWithDeleteButton(label *widget.Label, messageID uint, onDelete func()) *fyne.Container {
	deleteBtn := widget.NewButton("删除", onDelete)
	
	return container.NewBorder(nil, nil, nil, deleteBtn, label)
}

// removeContainerFromList 从容器中移除指定的元素
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

// getIndex 获取元素在切片中的索引

func appendMessage(chatList *fyne.Container, sender, content string, messageID uint) {
	label := widget.NewLabel(sender + ": " + content)
	label.Wrapping = fyne.TextWrapWord

	onDelete := func() {
		if err := database.DeleteMessage(messageID); err != nil {
			fmt.Printf("删除消息失败: %v\n", err)
			return
		}

		mu.Lock()
		history, err := database.LoadHistoryMessages()
		if err == nil {
			historyMessages := make([]global.Message, len(history))
			for i, h := range history {
				historyMessages[i] = global.Message{Role: h.Role, Content: h.Content}
			}
			messages = historyMessages
		} else {
			for i := range messages {
				if (sender == "你" && messages[i].Role == "user" && messages[i].Content == content) ||
					(sender == "AI" && messages[i].Role == "assistant" && messages[i].Content == content) {
					messages = append(messages[:i], messages[i+1:]...)
					break
				}
			}
		}
		mu.Unlock()

		// 查找并删除对应的消息行
		for _, obj := range chatList.Objects {
			if container, ok := obj.(*fyne.Container); ok {
				if container.Objects[1] == label { // 假设label在container中的位置是固定的
					index := -1
					for i, o := range chatList.Objects {
						if o == container {
							index = i
							break
						}
					}
					
					if index != -1 {
						chatList.Objects = append(chatList.Objects[:index], chatList.Objects[index+1:]...)
						chatList.Refresh()
					}
					break
				}
			}
		}
	}

	messageRow := createMessageWithDeleteButton(label, messageID, onDelete)
	chatList.Add(messageRow)
}

func appendStreamingMessage(chatList *fyne.Container, sender string) (*widget.Label, *fyne.Container) {
	label := widget.NewLabel(sender + ": ")
	label.Wrapping = fyne.TextWrapWord

	deleteBtn := widget.NewButton("删除", func() {})
	deleteBtn.Disable()

	messageRow := createMessageWithDeleteButton(label, 0, func() {}) // 流式消息暂时没有ID
	chatList.Add(messageRow)

	return label, messageRow
}

func sendMessage() {
	text := entry.Text
	if text == "" {
		return
	}

	mu.Lock()
	messages = append(messages, global.Message{Role: "user", Content: text})
	mu.Unlock()

	userMsgID, err := database.SaveMessage("user", text)
	if err != nil {
		fmt.Printf("保存用户消息失败: %v\n", err)
		return
	}

	appendMessage(chatList, "你", text, userMsgID)
	entry.SetText("")

	go func(userText string) {
		mu.Lock()
		messagesCopy := make([]global.Message, len(messages))
		copy(messagesCopy, messages)
		mu.Unlock()

		var aiLabel *widget.Label
		var aiMessageRow *fyne.Container
		var fullContent strings.Builder
		var aiMsgID uint

		fyne.Do(func() {
			aiLabel, aiMessageRow = appendStreamingMessage(chatList, "AI")
			scroll.ScrollToBottom()
		})

		responseChan := make(chan string)
		//持续接收通道中的数据
		go func() {
			for chunk := range responseChan {
				fyne.Do(func() {
					//持续追加内容
					fullContent.WriteString(chunk)
					if aiLabel != nil {
						aiLabel.SetText("AI: " + fullContent.String())
						aiLabel.Refresh()
						scroll.ScrollToBottom()
					}
				})
			}
		}()
		response, err := llm.CallChatAPI(messagesCopy,responseChan)
		if err != nil {
			response = "错误: " + err.Error()
			fyne.Do(func() {
				if aiLabel != nil {
					aiLabel.SetText("AI: " + response)
					aiLabel.Refresh()
				}
			})
		}

		if fullContent.Len() == 0 {
			fullContent.WriteString(response)
		}

		finalResponse := fullContent.String()

		mu.Lock()
		messages = append(messages, global.Message{Role: "assistant", Content: finalResponse})
		mu.Unlock()

		aiMsgID, err = database.SaveMessage("assistant", finalResponse)
		if err != nil {
			fmt.Printf("保存 AI 回复失败: %v\n", err)
		}

		fyne.Do(func() {
			if aiMessageRow != nil && aiLabel != nil {
				index := -1
				for i, obj := range chatList.Objects {
					if obj == aiMessageRow {
						index = i
						break
					}
				}
				
				if index != -1 {
					chatList.Objects = append(chatList.Objects[:index], chatList.Objects[index+1:]...)
					chatList.Refresh()
				}
				
				onDelete := func() {
					if err := database.DeleteMessage(aiMsgID); err != nil {
						fmt.Printf("删除消息失败: %v\n", err)
						return
					}
					mu.Lock()
					for i := range messages {
						if messages[i].Role == "assistant" && messages[i].Content == finalResponse {
							messages = append(messages[:i], messages[i+1:]...)
							break
						}
					}
					mu.Unlock()
					removeContainerFromList(chatList, aiMessageRow)
				}

				aiMessageRow = createMessageWithDeleteButton(aiLabel, aiMsgID, onDelete)
				chatList.Add(aiMessageRow)
				chatList.Refresh()
			}
			scroll.ScrollToBottom()
		})
	}(text)
}

func showSettingsDialog() {
	if mainWindow == nil {
		return
	}

	cfg := config.GetConfig()
	if cfg == nil {
		var err error
		cfg, err = config.LoadConfig()
		if err != nil {
			dialog.ShowError(fmt.Errorf("加载配置失败: %v", err), mainWindow)
			return
		}
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

			newConfig := &config.AppConfig{APIKey: apiKeyEntry.Text, APIURL: apiURLEntry.Text, Model: modelEntry.Text}
			if err := config.SaveConfig(newConfig); err != nil {
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