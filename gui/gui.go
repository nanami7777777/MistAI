package gui

import (
	mywidget "Mist/MyWidget"
	"Mist/database"
	"Mist/service"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/atotto/clipboard"
	hook "github.com/robotn/gohook"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var (
	messages                  []service.Message
	mu                        sync.Mutex
	entry                     *mywidget.MyMultiLine
	chatTranscript            *mywidget.ReadOnlyEntry
	chatList                  *fyne.Container
	scroll                    *container.Scroll
	conversationList          *fyne.Container
	historyList               *widget.List               // 新增：历史记录列表
	dictionaryEntries         []database.DictionaryEntry // 新增：用于存储单词条目
	historyTooltipContent     *widget.RichText
	mainWindow                fyne.Window
	settingsWindow            fyne.Window
	currentHotkeyGeneration   int64
	messageService            *service.MessageService
	conversationService       *service.ConversationService
	configService             *service.ConfigService
	currentStreamingLabel     *widget.Label
	currentStreamingContainer *fyne.Container
)

type providerOption struct {
	Key          string
	Label        string
	DefaultURL   string
	DefaultModel string
}

var providerOptions = []providerOption{
	{
		Key:          "openrouter",
		Label:        "OpenRouter",
		DefaultURL:   "https://openrouter.ai/api/v1/chat/completions",
		DefaultModel: "",
	},
	{
		Key:          "openai",
		Label:        "OpenAI",
		DefaultURL:   "https://api.openai.com/v1/chat/completions",
		DefaultModel: "gpt-4.1-mini",
	},
	{
		Key:          "custom",
		Label:        "自定义/其他",
		DefaultURL:   "",
		DefaultModel: "",
	},
}

func createMainLayout(convListContainer, chatArea, historyView fyne.CanvasObject) fyne.CanvasObject {
	// 将聊天区域和历史记录视图放在一个水平分割器中
	rightSplit := container.NewHSplit(chatArea, historyView)
	rightSplit.SetOffset(0.7) // 聊天区域占70%

	// 将左侧的对话列表和右侧的分割器放在另一个水平分割器中
	mainSplit := container.NewHSplit(convListContainer, rightSplit)
	mainSplit.SetOffset(0.14) // 对话列表占比进一步调小

	return mainSplit
}

func createChatArea() fyne.CanvasObject {
	chatTranscript = mywidget.NewReadOnlyEntry("")
	chatTranscript.OnRightTapped = func() {
		if chatTranscript == nil {
			return
		}
		selected := strings.TrimSpace(chatTranscript.SelectedText())
		if selected == "" {
			dialog.ShowInformation("提示", "请先在对话中选中要查询的单词", mainWindow)
			return
		}
		sendWordToLLM(selected)
	}

	scroll = container.NewVScroll(chatTranscript)
	scroll.SetMinSize(fyne.NewSize(200, 300))

	entry = mywidget.NewMyMultiLine(sendMessage)
	entry.SetPlaceHolder("输入问题…")

	stopBtn := widget.NewButton("停止输出", func() {
		messageService.StopStreaming()
	})

	settingsBtn := widget.NewButton("⚙", showSettingsDialog)
	settingsBtn.Importance = widget.MediumImportance

	topBar := container.NewBorder(nil, nil, nil, settingsBtn, widget.NewLabel(""))
	bottomButtons := container.NewHBox(stopBtn)
	bottom := container.NewBorder(nil, nil, nil, bottomButtons, entry)

	chatList = container.NewVBox()
	loadCurrentConversationMessagesAsync()

	return container.NewBorder(topBar, bottom, nil, nil, scroll)
}

func createConversationList() fyne.CanvasObject {
	conversationList = container.NewVBox()
	refreshConversationListAsync()

	convScroll := container.NewVScroll(conversationList)
	convScroll.SetMinSize(fyne.NewSize(140, 300))

	newConvBtn := widget.NewButton("+", func() {
		conv, err := conversationService.CreateConversation("新对话")
		if err != nil {
			showError("创建对话失败", err)
			return
		}
		switchConversation(conv.ID)
	})

	return container.NewBorder(newConvBtn, nil, nil, nil, convScroll)
}

func parseHotkeyString(hotkeyStr string) []string {
	parts := strings.Split(hotkeyStr, "+")
	var keys []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			keys = append(keys, p)
		}
	}
	if len(keys) == 0 {
		return []string{"ctrl", "c"}
	}
	return keys
}

func registerClipboardHotkey(hotkey []string, w fyne.Window, generation int64) {
	hook.Register(hook.KeyDown, hotkey, func(e hook.Event) {
		if atomic.LoadInt64(&currentHotkeyGeneration) != generation {
			return
		}
		if settingsWindow != nil {
			return
		}
		txt, err := clipboard.ReadAll()
		if err != nil {
			log.Println("读取剪贴板失败:", err)
			return
		}
		go func() {
			startTime := time.Now()
			timeout := 3 * time.Second
			fyne.Do(func() {
				entry.SetText(txt)
				entry.Refresh()
				w.Show()
				w.RequestFocus()
				w.Canvas().Focus(entry)
			})
			for {
				if time.Since(startTime) > timeout {
					break
				}
				fyne.Do(func() {
					w.Show()
					w.RequestFocus()
					if w.Canvas().Focused() != entry {
						w.Canvas().Focus(entry)
					}
				})
				time.Sleep(50 * time.Millisecond)
				if w.Canvas().Focused() == entry {
					break
				}
			}
		}()
	})
}

func setupHotKeys(w fyne.Window) {
	hotkeyStr := "ctrl+c"
	if configService != nil {
		cfg, err := configService.GetConfig()
		if err == nil && cfg.Hotkey != "" {
			hotkeyStr = cfg.Hotkey
		}
	}

	gen := atomic.AddInt64(&currentHotkeyGeneration, 1)
	registerClipboardHotkey(parseHotkeyString(hotkeyStr), w, gen)

	go func() {
		s := hook.Start()
		<-hook.Process(s)
	}()
}

// Run 启动并运行 Fyne GUI
func Run() {
	// Initialize services
	messageService = service.NewMessageService()
	conversationService = service.NewConversationService()
	configService = service.NewConfigService()

	// Set up event handlers
	messageService.SetMessageEventHandler(handleMessageEvent)

	myApp := app.New()
	w := myApp.NewWindow("AI 助手")
	mainWindow = w

	convListContainer := createConversationList()
	chatArea := createChatArea()
	historyView := createHistoryView() // 创建历史记录视图
	content := createMainLayout(convListContainer, chatArea, historyView)

	w.SetContent(content)
	w.Resize(fyne.NewSize(1200, 800)) // 调整窗口大小以容纳新视图

	setupHotKeys(w)

	w.ShowAndRun()
}

func createHistoryView() fyne.CanvasObject {
	historyList = widget.NewList(
		func() int {
			return len(dictionaryEntries)
		},
		func() fyne.CanvasObject {
			// 使用 mywidget.NewHoverableLabel 创建列表项
			return mywidget.NewHoverableLabel("template")
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			// 类型断言为 *mywidget.HoverableLabel
			label := o.(*mywidget.HoverableLabel)
			index := int(i)
			if index < 0 || index >= len(dictionaryEntries) {
				label.SetText("")
				label.OnHoverIn = nil
				label.OnHoverOut = nil
				label.OnDoubleTapped = nil
				return
			}
			entry := dictionaryEntries[index]
			label.SetText(entry.EnglishWord)

			// 设置悬停事件
			label.OnHoverIn = func() {
				showHistoryDetails(index)
			}

			label.OnHoverOut = func() {
				hideHistoryDetails(index)
			}

			// 设置双击事件
			label.OnDoubleTapped = func() {
				if index < 0 || index >= len(dictionaryEntries) {
					return
				}
				currentEntry := dictionaryEntries[index]
				if err := database.DeleteDictionaryEntry(currentEntry.EnglishWord); err != nil {
					showError("删除单词失败", err)
					return
				}
				refreshHistoryView()
			}
		},
	)

	if historyTooltipContent == nil {
		historyTooltipContent = widget.NewRichText()
		historyTooltipContent.Wrapping = fyne.TextWrapWord
	}
	detailScroll := container.NewVScroll(historyTooltipContent)
	detailScroll.SetMinSize(fyne.NewSize(200, 150))
	split := container.NewVSplit(historyList, detailScroll)
	split.SetOffset(0.6)

	refreshHistoryView()

	return split
}

func refreshHistoryView() {
	entries, err := database.GetAllDictionaryEntries()
	if err != nil {
		showError("获取历史记录失败", err)
		return
	}
	dictionaryEntries = entries
	historyList.Refresh()
}

func buildDictionaryEntryDetails(entry database.DictionaryEntry) string {
	var b strings.Builder
	for _, posEntry := range entry.PosEntries {
		b.WriteString("词性: ")
		b.WriteString(posEntry.Pos)
		b.WriteString("\n")
		for _, sense := range posEntry.Senses {
			b.WriteString("  释义: ")
			b.WriteString(sense.Meaning)
			b.WriteString("\n")
			for _, example := range sense.Examples {
				b.WriteString("    例句: ")
				b.WriteString(example.Sentence)
				b.WriteString(" - ")
				b.WriteString(example.Translation)
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

func showHistoryDetails(index int) {
	if index < 0 || index >= len(dictionaryEntries) {
		return
	}
	entry := dictionaryEntries[index]
	if historyTooltipContent == nil {
		historyTooltipContent = widget.NewRichText()
		historyTooltipContent.Wrapping = fyne.TextWrapWord
	}
	var segments []widget.RichTextSegment

	if entry.EnglishWord != "" {
		segments = append(segments, &widget.TextSegment{
			Text: entry.EnglishWord,
			Style: widget.RichTextStyle{
				ColorName: theme.ColorNamePrimary,
				SizeName:  theme.SizeNameSubHeadingText,
				TextStyle: fyne.TextStyle{Bold: true},
			},
		})
	}

	for _, posEntry := range entry.PosEntries {
		segments = append(segments, &widget.TextSegment{
			Text: "词性: ",
			Style: widget.RichTextStyle{
				ColorName: theme.ColorNameForeground,
				TextStyle: fyne.TextStyle{Bold: true},
				Inline:    true,
			},
		})
		segments = append(segments, &widget.TextSegment{
			Text: posEntry.Pos,
			Style: widget.RichTextStyle{
				ColorName: theme.ColorNamePrimary,
				TextStyle: fyne.TextStyle{Bold: true},
			},
		})

		for i, sense := range posEntry.Senses {
			segments = append(segments, &widget.TextSegment{
				Text: fmt.Sprintf("  %d. 释义: ", i+1),
				Style: widget.RichTextStyle{
					ColorName: theme.ColorNameForeground,
					TextStyle: fyne.TextStyle{Bold: true},
				},
			})
			segments = append(segments, &widget.TextSegment{
				Text: sense.Meaning,
				Style: widget.RichTextStyle{
					ColorName: theme.ColorNameForeground,
				},
			})

			for _, example := range sense.Examples {
				segments = append(segments, &widget.TextSegment{
					Text: "    例句: ",
					Style: widget.RichTextStyle{
						ColorName: theme.ColorNamePrimary,
						TextStyle: fyne.TextStyle{Bold: true},
					},
				})
				segments = append(segments, &widget.TextSegment{
					Text: example.Sentence,
					Style: widget.RichTextStyle{
						ColorName: theme.ColorNameHyperlink,
						TextStyle: fyne.TextStyle{Italic: true},
					},
				})

				segments = append(segments, &widget.TextSegment{
					Text: "    译文: ",
					Style: widget.RichTextStyle{
						ColorName: theme.ColorNamePrimary,
						TextStyle: fyne.TextStyle{Bold: true},
					},
				})
				segments = append(segments, &widget.TextSegment{
					Text: example.Translation,
					Style: widget.RichTextStyle{
						ColorName: theme.ColorNameForeground,
					},
				})
			}
		}
	}

	if len(segments) == 0 {
		segments = append(segments, &widget.TextSegment{
			Text: "暂无详细释义",
			Style: widget.RichTextStyle{
				ColorName: theme.ColorNameForeground,
			},
		})
	}

	historyTooltipContent.Segments = segments
	historyTooltipContent.Refresh()
}

func hideHistoryDetails(index int) {
}

func handleUserMessageSent(event service.MessageEvent) {
	if event.Message != nil {
		appendToTranscript("你", event.Message.Content)
		entry.SetText("")
		updateCurrentConversationTitleIfNeeded(event.Message.Content)
	}
}

func handleAIResponseStart() {
	if chatTranscript == nil {
		return
	}
	text := chatTranscript.Text
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "AI: "
	chatTranscript.SetText(text)
	if scroll != nil {
		scroll.ScrollToBottom()
	}
}

func handleAIResponseChunk(event service.MessageEvent) {
	if chatTranscript == nil || event.Content == "" {
		return
	}
	currentText := chatTranscript.Text
	currentText += event.Content
	chatTranscript.SetText(currentText)
	if scroll != nil {
		scroll.ScrollToBottom()
	}
}

func handleAIResponseComplete(event service.MessageEvent) {
	if event.Message != nil {
		mu.Lock()
		messages = append(messages, *event.Message)
		mu.Unlock()

		if chatTranscript != nil && event.Message.Content != "" {
			text := chatTranscript.Text
			if !strings.Contains(text, event.Message.Content) {
				appendToTranscript("AI", event.Message.Content)
			} else if !strings.HasSuffix(text, "\n") {
				text += "\n"
				chatTranscript.SetText(text)
			}
			if scroll != nil {
				scroll.ScrollToBottom()
			}
		}
	}
	refreshHistoryView()
}

func handleErrorEvent(event service.MessageEvent) {
	if event.Error != nil {
		dialog.ShowError(fmt.Errorf("发生错误: %v", event.Error), mainWindow)
	}
}

// handleMessageEvent handles events from the message service
func handleMessageEvent(event service.MessageEvent) {
	fyne.Do(func() {
		switch event.Type {
		case "user_message_sent":
			handleUserMessageSent(event)
		case "ai_response_start":
			handleAIResponseStart()
		case "ai_response_chunk":
			handleAIResponseChunk(event)
		case "ai_response_complete":
			handleAIResponseComplete(event)
		case "error":
			handleErrorEvent(event)
		}
	})
}

func generateConversationTitleFromMessage(content string) string {
	text := strings.TrimSpace(content)
	if text == "" {
		return ""
	}
	if idx := strings.Index(text, "\n"); idx >= 0 {
		text = text[:idx]
	}
	runes := []rune(text)
	maxLen := 20
	if len(runes) > maxLen {
		text = string(runes[:maxLen]) + "..."
	}
	return text
}

func updateCurrentConversationTitleIfNeeded(content string) {
	go func() {
		convID := conversationService.GetCurrentConversationID()
		if convID.IsZero() {
			return
		}
		conv, err := database.GetConversationByObjectID(convID)
		if err != nil {
			log.Errorf("获取对话失败: %v", err)
			return
		}
		if conv.Name != "新对话" {
			return
		}
		title := generateConversationTitleFromMessage(content)
		if title == "" {
			return
		}
		if err := database.UpdateConversationName(convID, title); err != nil {
			log.Errorf("更新对话名称失败: %v", err)
			return
		}
		refreshConversationListAsync()
	}()
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
		item := mywidget.NewConversationItem(convName, convID == currentID)

		item.OnTapped = func(id primitive.ObjectID) func() {
			return func() { switchConversation(id) }
		}(convID)

		item.OnRightTapped = func(id primitive.ObjectID) func(*fyne.PointEvent) {
			return func(_ *fyne.PointEvent) {
				dialog.ShowConfirm("删除对话", "确定要删除该对话吗？", func(ok bool) {
					if !ok {
						return
					}
					deleteConversation(id)
				}, mainWindow)
			}
		}(convID)

		conversationList.Add(item)
	}

	conversationList.Refresh()
}

func deleteConversation(id primitive.ObjectID) {
	if id == conversationService.GetCurrentConversationID() {
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
	refreshConversationListAsync()
}

// switchConversation 切换对话
func switchConversation(convID primitive.ObjectID) {
	if err := conversationService.SwitchConversation(convID); err != nil {
		showError("切换对话失败", err)
		return
	}

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
	if chatTranscript != nil {
		chatTranscript.SetText("")
	}
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
		appendToTranscript(sender, history[i].Content)
	}
	fmt.Println("界面渲染耗时：", time.Since(starttime))

	if scroll != nil {
		scroll.ScrollToBottom()
	}
}

func normalizeMarkdownContent(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\\r\\n", "\n")
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "  \n", "\n\n")
	return s
}

func sendWordToLLM(word string) {
	w := strings.TrimSpace(word)
	if w == "" {
		return
	}
	messageService.StreamMessage(service.SendMessageRequest{Content: w})
}

func appendToTranscript(sender, content string) {
	if chatTranscript == nil {
		return
	}
	text := chatTranscript.Text
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += sender + ": " + normalizeMarkdownContent(content) + "\n"
	chatTranscript.SetText(text)
	if scroll != nil {
		scroll.ScrollToBottom()
	}
}

// handleDeleteMessage handles deletion of a message by its ID
func handleDeleteMessage(messageID uint) {
	// Find the container and metadata associated with this message ID
	var targetContainer *fyne.Container
	var targetMetadata service.Metadata

	service.MetadataMu.RLock()
	for container, metadata := range service.MessageMetadata {
		if metadata.ID == messageID {
			targetContainer = container
			targetMetadata = metadata
			break
		}
	}
	service.MetadataMu.RUnlock()

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
	service.MetadataMu.Lock()
	delete(service.MessageMetadata, targetContainer)
	service.MetadataMu.Unlock()
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

	// The service now handles all the complexity of streaming, parsing, and storing.
	// The GUI's role is just to initiate the request and then react to events.
	messageService.StreamMessage(service.SendMessageRequest{Content: text})
}

func showSettingsDialog() {
	if mainWindow == nil {
		return
	}

	if settingsWindow != nil {
		settingsWindow.Show()
		settingsWindow.RequestFocus()
		settingsWindow.CenterOnScreen()
		return
	}

	cfg, err := configService.GetConfig()
	if err != nil {
		showError("加载配置失败", err)
		return
	}

	settingsWindow = fyne.CurrentApp().NewWindow("设置")
	settingsWindow.SetOnClosed(func() {
		settingsWindow = nil
	})

	providerLabels := make([]string, len(providerOptions))
	for i, opt := range providerOptions {
		providerLabels[i] = opt.Label
	}

	providerSelect := widget.NewSelect(providerLabels, nil)

	currentProviderKey := cfg.Provider
	if currentProviderKey == "" {
		currentProviderKey = "openrouter"
	}

	initialLabel := providerLabels[0]
	for _, opt := range providerOptions {
		if opt.Key == currentProviderKey {
			initialLabel = opt.Label
			break
		}
	}

	providerSelect.SetSelected(initialLabel)

	apiKeyEntry := widget.NewEntry()
	apiKeyEntry.SetText(cfg.APIKey)
	apiKeyEntry.SetPlaceHolder("请输入 API Key")

	apiURLEntry := widget.NewEntry()
	apiURLEntry.SetText(cfg.APIURL)
	apiURLEntry.SetPlaceHolder("请输入 API URL")

	modelEntry := widget.NewEntry()
	modelEntry.SetText(cfg.Model)
	modelEntry.SetPlaceHolder("请输入模型名称")

	hotkeyEntry := widget.NewEntry()
	hotkeyEntry.SetPlaceHolder("例如: ctrl+c")
	if cfg.Hotkey != "" {
		hotkeyEntry.SetText(cfg.Hotkey)
	} else {
		hotkeyEntry.SetText("ctrl+c")
	}

	providerSelect.OnChanged = func(label string) {
		for _, opt := range providerOptions {
			if opt.Label == label {
				if opt.DefaultURL != "" {
					apiURLEntry.SetText(opt.DefaultURL)
				}
				if opt.DefaultModel != "" {
					modelEntry.SetText(opt.DefaultModel)
				}
				break
			}
		}
	}

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "API 类型", Widget: providerSelect},
			{Text: "API Key", Widget: apiKeyEntry},
			{Text: "API URL", Widget: apiURLEntry},
			{Text: "Model", Widget: modelEntry},
			{Text: "全局快捷键", Widget: hotkeyEntry},
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

			selectedProviderKey := ""
			for _, opt := range providerOptions {
				if opt.Label == providerSelect.Selected {
					selectedProviderKey = opt.Key
					break
				}
			}
			if selectedProviderKey == "" {
				selectedProviderKey = "custom"
			}

			hotkeyValue := strings.TrimSpace(hotkeyEntry.Text)
			if hotkeyValue == "" {
				hotkeyValue = "ctrl+c"
			}

			hotkeyParts := parseHotkeyString(hotkeyValue)
			normalizedHotkey := strings.Join(hotkeyParts, "+")

			newConfig := &service.AppConfig{
				Provider: selectedProviderKey,
				APIKey:   apiKeyEntry.Text,
				APIURL:   apiURLEntry.Text,
				Model:    modelEntry.Text,
				Hotkey:   normalizedHotkey,
			}
			if err := configService.SaveConfig(newConfig); err != nil {
				dialog.ShowError(fmt.Errorf("保存配置失败: %v", err), settingsWindow)
				return
			}
			cfgAfter, err := configService.GetConfig()
			if err != nil {
				dialog.ShowError(fmt.Errorf("保存后读取配置失败: %v", err), settingsWindow)
				return
			}
			if cfgAfter.Hotkey != normalizedHotkey {
				dialog.ShowError(fmt.Errorf("保存后配置与期望不一致: 当前为 %s，期望为 %s", cfgAfter.Hotkey, normalizedHotkey), settingsWindow)
				return
			}

			gen := atomic.AddInt64(&currentHotkeyGeneration, 1)
			registerClipboardHotkey(hotkeyParts, mainWindow, gen)

			dialog.ShowInformation("成功", fmt.Sprintf("配置已保存，当前全局快捷键：%s", cfgAfter.Hotkey), settingsWindow)
			settingsWindow.Close()
		},
		OnCancel: func() { settingsWindow.Close() },
	}

	settingsWindow.SetContent(form)
	settingsWindow.Resize(fyne.NewSize(500, 250))
	settingsWindow.CenterOnScreen()
	settingsWindow.Show()
}
