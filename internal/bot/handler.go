package bothandler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	clientinterceptor "github.com/maydietwice/task-manager/internal/bot/interceptor"
	rdb "github.com/maydietwice/task-manager/internal/redis"
	"github.com/maydietwice/task-manager/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var defaultReplyKeyboard = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("create"),
		tgbotapi.NewKeyboardButton("list"),
	),

	// tgbotapi.NewKeyboardButtonRow(
	// 	tgbotapi.NewKeyboardButton("get"),
	// 	tgbotapi.NewKeyboardButton("update"),
	// 	tgbotapi.NewKeyboardButton("list"),
	// ),
)

type Handler struct {
	client proto.TaskServiceClient
	bot    *tgbotapi.BotAPI
	repo   *rdb.Repository
	secret []byte
}

func NewHandler(client proto.TaskServiceClient, bot *tgbotapi.BotAPI, secret string, repo *rdb.Repository) *Handler {
	return &Handler{client: client, bot: bot, secret: []byte(secret), repo: repo}
}

func (h *Handler) HandleUpdate(update tgbotapi.Update) {
	if update.Message != nil {
		h.handleMessage(update)
	}
	if update.CallbackQuery != nil {
		h.handleCallback(update)
	}
}

func (h *Handler) handleMessage(update tgbotapi.Update) {
	ctx := context.WithValue(context.Background(), clientinterceptor.ChatIDKey, update.Message.Chat.ID)
	if update.Message.IsCommand() {
		if update.Message.Command() != "start" {
			_, err := h.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid command."))
			if err != nil {
				h.returnToMainMenu(ctx, update, err)
			}

			return
		}
		h.returnToMainMenu(ctx, update, nil)

		return
	}

	chatInfo, err := h.repo.GetChatInfo(context.Background(), update.Message.Chat.ID)
	if err != nil {
		h.returnToMainMenu(ctx, update, err)

		return
	}
	action, ok := chatInfo["state"]
	if !ok {
		action = update.Message.Text
	}
	switch action {
	case "create":
		h.handleCreate(ctx, update, chatInfo)
	// case "delete":
	// 	h.handleDelete(ctx, update, chatInfo)
	// case "get":
	// case "update":
	case "list":
		h.handleList(ctx, update)
	default:
		_, err := h.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid command."))
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
		}
	}
}

func (h *Handler) handleCallback(update tgbotapi.Update) {
	ctx := context.WithValue(context.Background(), clientinterceptor.ChatIDKey, update.CallbackQuery.Message.Chat.ID)
	callbackData := strings.Split(update.CallbackData(), ";")
	if len(callbackData) < 1 {
		return
	}
	buttonPressed := callbackData[0]
	switch buttonPressed {
	case "list":
		h.handleListButtonPressed(ctx, update, callbackData)
		return
	default:
		return
	}
}

func (h *Handler) handleListButtonPressed(ctx context.Context, update tgbotapi.Update, callbackData []string) {
	listBtn := callbackData[1]
	switch listBtn {
	case "next":
		page, err := strconv.Atoi(callbackData[2])
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
			return
		}
		page += 1
		cursor := callbackData[3]

		tasks, err := h.getCachedTasks(ctx, update.CallbackQuery.Message.Chat.ID, page)
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
			return
		}
		if len(tasks) < 1 {
			timeBefore, err := time.Parse(time.RFC3339, cursor)
			if err != nil {
				h.returnToMainMenu(ctx, update, err)
				return
			}
			resp, err := h.client.ListTask(ctx, &proto.ListTaskRequest{After: timestamppb.New(timeBefore)})
			if err != nil {
				h.returnToMainMenu(ctx, update, err)
				return
			}

			tasks = resp.GetTasks()
			if len(tasks) < 1 {
				msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, "No more tasks")
				_, err := h.bot.Request(msg)
				if err != nil {
					h.returnToMainMenu(ctx, update, err)
				}
				return
			}
			h.cacheTasks(ctx, update.CallbackQuery.Message.Chat.ID, page, tasks)
		}
		cursorNext := time.Now()
		var cursorPrevious time.Time
		for _, task := range tasks {
			if !cursorNext.Before(task.CreatedAt.AsTime()) {
				cursorNext = task.CreatedAt.AsTime()
			}
			if !cursorPrevious.After(task.CreatedAt.AsTime()) {
				cursorPrevious = task.CreatedAt.AsTime()
			}
		}
		kb := createTasksInlineKeyboard(tasks, page, cursorNext.Format(time.RFC3339), cursorPrevious.Format(time.RFC3339))
		msg := tgbotapi.NewEditMessageText(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, fmt.Sprintf("Choose one task from the list below, page: %d", page))
		msgMrkp := tgbotapi.NewEditMessageReplyMarkup(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, kb)
		msg.ReplyMarkup = msgMrkp.ReplyMarkup
		_, err = h.bot.Request(msg)
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
		}
		HFields := []string{
			"state", "list",
			"step", "choice",
		}
		err = h.repo.SetChatInfo(ctx, update.CallbackQuery.Message.Chat.ID, HFields...)
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
		}

	case "previous":
		page, err := strconv.Atoi(callbackData[2])
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
			return
		}
		page -= 1
		if page < 1 {
			msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, "You're already on 1st page!")
			_, err := h.bot.Send(msg)
			if err != nil {
				h.returnToMainMenu(ctx, update, err)
			}
			return
		}
		cursor := callbackData[3]

		tasks, err := h.getCachedTasks(ctx, update.CallbackQuery.Message.Chat.ID, page)
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
			return
		}
		if len(tasks) < 1 {
			timeBefore, err := time.Parse(time.RFC3339, cursor)
			if err != nil {
				h.returnToMainMenu(ctx, update, err)
				return
			}
			resp, err := h.client.ListTask(ctx, &proto.ListTaskRequest{After: timestamppb.New(timeBefore)})
			if err != nil {
				h.returnToMainMenu(ctx, update, err)
				return
			}

			tasks = resp.GetTasks()
			if len(tasks) < 1 {
				msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, "No more tasks")
				_, err := h.bot.Request(msg)
				if err != nil {
					h.returnToMainMenu(ctx, update, err)
				}
				return
			}
			h.cacheTasks(ctx, update.CallbackQuery.Message.Chat.ID, page, tasks)
		}
		cursorNext := time.Now()
		var cursorPrevious time.Time
		for _, task := range tasks {
			if !cursorNext.Before(task.CreatedAt.AsTime()) {
				cursorNext = task.CreatedAt.AsTime()
			}
			if !cursorPrevious.After(task.CreatedAt.AsTime()) {
				cursorPrevious = task.CreatedAt.AsTime()
			}
		}
		kb := createTasksInlineKeyboard(tasks, page, cursorNext.Format(time.RFC3339), cursorPrevious.Format(time.RFC3339))
		msg := tgbotapi.NewEditMessageText(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, fmt.Sprintf("Choose one task from the list below, page: %d", page))
		msgMrkp := tgbotapi.NewEditMessageReplyMarkup(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, kb)
		msg.ReplyMarkup = msgMrkp.ReplyMarkup
		_, err = h.bot.Request(msg)
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
		}
		HFields := []string{
			"state", "list",
			"step", "choice",
		}
		err = h.repo.SetChatInfo(ctx, update.CallbackQuery.Message.Chat.ID, HFields...)
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
		}
	}
}

// func (h *Handler) newCtx(update tgbotapi.Update) (context.Context, error) {
// 	jwtToken := jwt.New(jwt.SigningMethodHS256)

// 	jwtToken.Claims = jwt.MapClaims{
// 		"owner_id": strconv.Itoa(int(update.Message.Chat.ID)),
// 	}

// 	token, err := jwtToken.SignedString(h.secret)
// 	if err != nil {
// 		return nil, err
// 	}

// 	md := metadata.Pairs("authorization", "Bearer "+token)

// 	ctx := metadata.NewOutgoingContext(context.Background(), md)

// 	return ctx, nil
// }

func (h *Handler) returnToMainMenu(ctx context.Context, update tgbotapi.Update, err error) {
	if update.Message == nil {
		h.repo.ClearState(ctx, update.CallbackQuery.Message.Chat.ID)

		msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, "This bot helps you managing your tasks. Choose one of the buttons below.")

		if err != nil {
			log.Printf("error handling request | chatID: %v, err: %v", update.CallbackQuery.Message.Chat.ID, err)
			msg = tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, "Can not handle your request. Contact support.")
		}

		msg.ReplyMarkup = defaultReplyKeyboard
		_, err = h.bot.Send(msg)
		if err != nil {
			for err != nil {
				_, err = h.bot.Send(msg)
			}
		}

		return
	}
	h.repo.ClearState(ctx, update.Message.Chat.ID)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "This bot helps you managing your tasks. Choose one of the buttons below.")

	if err != nil {
		log.Printf("error handling request | chatID: %v, err: %v", update.Message.Chat.ID, err)
		msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Can not handle your request. Contact support.")
	}

	msg.ReplyMarkup = defaultReplyKeyboard
	_, err = h.bot.Send(msg)
	if err != nil {
		for err != nil {
			_, err = h.bot.Send(msg)
		}
	}
}

func (h *Handler) handleCreate(ctx context.Context, update tgbotapi.Update, chatInfo map[string]string) {
	switch chatInfo["step"] {
	case "title":
		if update.Message.Text == "" {
			_, err := h.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Title can not be empty, please enter you title."))
			if err != nil {
				h.returnToMainMenu(ctx, update, err)
			}
			return
		}
		hFields := []string{
			"step", "description",
			"title", update.Message.Text,
			"description", "",
		}
		err := h.repo.SetChatInfo(context.Background(), update.Message.Chat.ID, hFields...)
		if err != nil {
			h.returnToMainMenu(ctx, update, err)

			return
		}

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Enter your tasks's description(what's you gonna do?)")
		msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
		_, err = h.bot.Send(msg)
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
		}

		return
	case "description":
		resp, err := h.client.CreateTask(ctx, &proto.CreateTaskRequest{Title: chatInfo["title"], Description: update.Message.Text})
		if err != nil {
			h.returnToMainMenu(ctx, update, err)

			return
		}
		msgText := fmt.Sprintf("ID: %v\nTitle: %v\nDescription: %v\nStatus: %v\nCreated at: %v\nUpdated at: %v\n", resp.Task.Id, resp.Task.Title, resp.Task.Description, resp.Task.Status, resp.Task.CreatedAt, resp.Task.UpdatedAt)
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)
		_, err = h.bot.Send(msg)
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
		}

		h.returnToMainMenu(ctx, update, nil)

		return
	default:
		hFields := []string{
			"state", "create",
			"step", "title",
			"title", "",
			"description", "",
		}

		err := h.repo.SetChatInfo(context.Background(), update.Message.Chat.ID, hFields...)
		if err != nil {
			h.returnToMainMenu(ctx, update, err)

			return
		}

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Enter your tasks's name")
		msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
		_, err = h.bot.Send(msg)
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
		}

		return
	}
}

func (h *Handler) handleList(ctx context.Context, update tgbotapi.Update) {
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Collecting your tasks...")
	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
	_, err := h.bot.Send(msg)
	if err != nil {
		h.returnToMainMenu(ctx, update, err)
	}
	timeBefore := time.Now()
	resp, err := h.client.ListTask(ctx, &proto.ListTaskRequest{After: timestamppb.New(timeBefore)})
	if err != nil {
		h.returnToMainMenu(ctx, update, err)
	}
	page := 1
	tasks := resp.GetTasks()
	if len(tasks) < 1 {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "You dont have any tasks yet")
		msg.ReplyMarkup = defaultReplyKeyboard
		_, err := h.bot.Send(msg)
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
		}
		return
	}

	h.cacheTasks(ctx, update.Message.Chat.ID, page, tasks)

	cursorNext := timeBefore
	var cursorPrevious time.Time
	for _, task := range tasks {
		if !cursorNext.Before(task.CreatedAt.AsTime()) {
			cursorNext = task.CreatedAt.AsTime()
		}
		if !cursorPrevious.After(task.CreatedAt.AsTime()) {
			cursorPrevious = task.CreatedAt.AsTime()
		}
	}
	kb := createTasksInlineKeyboard(tasks, page, cursorNext.Format(time.RFC3339), cursorPrevious.Format(time.RFC3339))
	msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Choose one task from the list below, page: 1")
	msg.ReplyMarkup = kb
	_, err = h.bot.Send(msg)
	if err != nil {
		h.returnToMainMenu(ctx, update, err)
	}
}

func (h *Handler) cacheTasks(ctx context.Context, chatId int64, page int, tasks []*proto.Task) {
	cacheTasks, err := json.Marshal(tasks)
	if err != nil {
		log.Printf("Unable to marshal tasks: %v", err)
	}

	err = h.repo.CacheUserTasks(ctx, chatId, page, string(cacheTasks))
	if err != nil {
		log.Printf("Caching error: %v", err)
	}
}

func (h *Handler) getCachedTasks(ctx context.Context, chatId int64, page int) ([]*proto.Task, error) {
	cached := h.repo.GetCachedUserTasks(ctx, chatId, page)
	if cached == nil {
		return nil, nil
	}
	tasks := make([]*proto.Task, 0, 5)
	err := json.Unmarshal(cached, &tasks)
	if err != nil {
		return nil, err
	}

	return tasks, nil
}

func createTasksInlineKeyboard(tasks []*proto.Task, page int, cursorNext, cursorPrevious string) tgbotapi.InlineKeyboardMarkup {
	kbRows := make([][]tgbotapi.InlineKeyboardButton, 0, 7)
	for _, task := range tasks {
		kbBtn := tgbotapi.NewInlineKeyboardButtonData(truncateTitle(task.Title), fmt.Sprintf("list;task;%v", task.Id))
		kbRows = append(kbRows, []tgbotapi.InlineKeyboardButton{kbBtn})
	}

	btnPrevious := tgbotapi.NewInlineKeyboardButtonData("<-", fmt.Sprintf("list;previous;%d;%v", page, cursorPrevious))
	btnNext := tgbotapi.NewInlineKeyboardButtonData("->", fmt.Sprintf("list;next;%d;%v", page, cursorNext))
	kbRows = append(kbRows, []tgbotapi.InlineKeyboardButton{btnPrevious, btnNext})

	return tgbotapi.NewInlineKeyboardMarkup(kbRows...)
}

func truncateTitle(title string) string {
	limit, byteIdx := 20, 0

	if utf8.RuneCountInString(title) > 20 {
		for i := 0; i < limit; i++ {
			_, size := utf8.DecodeRuneInString(title[byteIdx:])
			byteIdx += size
		}

		title = title[:byteIdx] + "..."
	}

	return title
}

// func (h *Handler) handleDelete(ctx context.Context, update tgbotapi.Update, chatInfo map[string]string) {
// 	switch chatInfo["step"] {
// 	case "id":
// 	default:
// 		h.repo.
// 			resp, err := h.client.ListTask(ctx, &proto.ListTaskRequest{After: timestamppb.New(time.Now())})
// 		if err != nil {
// 			h.returnToMainMenu(ctx, update, err)
// 			return
// 		}
// 		tasks := resp.GetTasks()

// 		replyKeyboard := tgbotapi.NewInlineKeyboardMarkup()
// 		taskNum := 1
// 		for _, task := range tasks {
// 			kbButtonText := fmt.Sprintf("%d) %v", taskNum, task.Title)
// 			kbButton := tgbotapi.InlineKeyboardButton{Text: kbButtonText}
// 			taskNum++
// 			kbRow := tgbotapi.NewInlineKeyboardRow(kbButton)
// 			replyKeyboard.InlineKeyboard = append(replyKeyboard.InlineKeyboard, kbRow)
// 		}
// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Choose task to delete")

// 	}
// }

// func (h *Handler) delete(update tgbotapi.Update) {
// 	ctx, err := h.newCtx(update)
// 	if err != nil {
// 		log.Printf("delete ctx err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

// 		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

// 		h.bot.Send(msg)

// 		return
// 	}

// 	userMsg := update.Message.CommandArguments()

// 	if userMsg == "" {
// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /delete id")

// 		h.bot.Send(msg)

// 		return
// 	}

// 	_, err = h.client.DeleteTask(ctx, &proto.DeleteTaskRequest{Id: userMsg})
// 	if err != nil {
// 		log.Printf("delete response err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

// 		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

// 		h.bot.Send(msg)

// 		return
// 	}

// 	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Task deleted.")

// 	h.bot.Send(msg)
// }

// func (h *Handler) get(update tgbotapi.Update) {
// 	ctx, err := h.newCtx(update)
// 	if err != nil {
// 		log.Printf("get ctx err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

// 		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

// 		h.bot.Send(msg)

// 		return
// 	}

// 	userMsg := update.Message.CommandArguments()

// 	if userMsg == "" {
// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /get id")

// 		h.bot.Send(msg)

// 		return
// 	}

// 	resp, err := h.client.GetTask(ctx, &proto.GetTaskRequest{Id: userMsg})

// 	if status.Code(err) == codes.NotFound {
// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Task not found.")

// 		h.bot.Send(msg)

// 		return
// 	}

// 	if err != nil {
// 		log.Printf("get response err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

// 		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

// 		h.bot.Send(msg)

// 		return
// 	}

// 	msgText := fmt.Sprintf("ID: %v\nTitle: %v\nDescription: %v\nStatus: %v\nCreated at: %v\nUpdated at: %v\n", resp.Task.Id, resp.Task.Title, resp.Task.Description, resp.Task.Status, resp.Task.CreatedAt, resp.Task.UpdatedAt)

// 	msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)

// 	h.bot.Send(msg)
// }

// func (h *Handler) update(update tgbotapi.Update) {
// 	ctx, err := h.newCtx(update)
// 	if err != nil {
// 		log.Printf("update ctx err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

// 		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

// 		h.bot.Send(msg)

// 		return
// 	}

// 	userMsg := strings.Split(update.Message.CommandArguments(), "|")

// 	if len(userMsg) < 2 || len(userMsg) > 4 {
// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /update id(!required) | status(!required) | title | description")

// 		h.bot.Send(msg)

// 		return
// 	}

// 	id := strings.TrimSpace(userMsg[0])

// 	statusT, err := strconv.Atoi(strings.TrimSpace(userMsg[1]))
// 	if err != nil {
// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /update id(!required) | status(!required) | title | description")

// 		h.bot.Send(msg)

// 		return
// 	}

// 	if statusT < 0 || statusT > 2 {
// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /update id(!required) | status(!required) | title | description")

// 		h.bot.Send(msg)

// 		return
// 	}

// 	var title, description string

// 	for i, v := range userMsg {
// 		switch i {
// 		case 2:
// 			title = strings.TrimSpace(v)
// 		case 3:
// 			description = strings.TrimSpace(v)
// 		default:
// 			continue
// 		}
// 	}

// 	resp, err := h.client.UpdateTask(ctx, &proto.UpdateTaskRequest{Id: id, Status: proto.Status(statusT), Title: title, Description: description})

// 	if status.Code(err) == codes.NotFound {
// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Task not found.")

// 		h.bot.Send(msg)

// 		return
// 	}

// 	if err != nil {
// 		log.Printf("update response err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

// 		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

// 		h.bot.Send(msg)

// 		return
// 	}

// 	msgText := fmt.Sprintf("ID: %v\nTitle: %v\nDescription: %v\nStatus: %v\nCreated at: %v\n pdated at: %v\n", resp.Task.Id, resp.Task.Title, resp.Task.Description, resp.Task.Status, resp.Task.CreatedAt, resp.Task.UpdatedAt)

// 	msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)

// 	h.bot.Send(msg)
// }

// func (h *Handler) list(update tgbotapi.Update) {
// 	ctx, err := h.newCtx(update)
// 	if err != nil {
// 		log.Printf("list ctx err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

// 		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

// 		h.bot.Send(msg)

// 		return
// 	}

// 	userMsg := strings.Split(update.Message.CommandArguments(), "|")

// 	if len(userMsg) != 2 {
// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /list page | limit")

// 		h.bot.Send(msg)

// 		return
// 	}

// 	page, err := strconv.Atoi(strings.TrimSpace(userMsg[0]))
// 	if err != nil {
// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /list page | limit")

// 		h.bot.Send(msg)

// 		return
// 	}

// 	limit, err := strconv.Atoi(strings.TrimSpace(userMsg[1]))
// 	if err != nil {
// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /list page | limit")

// 		h.bot.Send(msg)

// 		return
// 	}

// 	if limit > 10 {
// 		limit = 10
// 	}

// 	resp, err := h.client.ListTask(ctx, &proto.ListTaskRequest{Page: int32(page), Limit: int32(limit)})
// 	if err != nil {
// 		log.Printf("list response err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

// 		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

// 		h.bot.Send(msg)

// 		return
// 	}

// 	for _, msgTask := range resp.Tasks {
// 		msgText := fmt.Sprintf("ID: %v\nTitle: %v\nDescription: %v\nStatus: %v\nCreated at: %v\nUpdated at: %v\n", msgTask.Id, msgTask.Title, msgTask.Description, msgTask.Status, msgTask.CreatedAt, msgTask.UpdatedAt)

// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)

// 		h.bot.Send(msg)
// 	}
// }
