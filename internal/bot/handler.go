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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var defaultReplyKeyboard = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("create"),
		tgbotapi.NewKeyboardButton("list"),
	),
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

func (h *Handler) logAndNotify(update tgbotapi.Update, err error) {
	if err != nil {
		log.Printf("error handling request | chatID: %v, err: %v", update.CallbackQuery.From.ID, err)
		var chatId int64
		if update.CallbackQuery != nil {
			chatId = update.CallbackQuery.From.ID
		}
		if update.Message != nil {
			chatId = update.Message.From.ID
		}
		msg := tgbotapi.NewMessage(chatId, "Can not handle your request. Contact support.")
		_, err := h.bot.Send(msg)
		if err != nil {
			log.Printf("error sending message | chatID: %v, err: %v", chatId, err)
		}
	}
}

func (h *Handler) returnToMainMenu(ctx context.Context, update tgbotapi.Update, err error) {
	if update.Message == nil {
		h.repo.ClearState(ctx, update.CallbackQuery.From.ID)

		if err != nil {
			h.logAndNotify(update, err)
		}

		msg := tgbotapi.NewMessage(update.CallbackQuery.From.ID, "This bot helps you managing your tasks. Choose one of the buttons below.")

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

	if err != nil {
		h.logAndNotify(update, err)
	}

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "This bot helps you managing your tasks. Choose one of the buttons below.")

	msg.ReplyMarkup = defaultReplyKeyboard
	_, err = h.bot.Send(msg)
	if err != nil {
		for err != nil {
			_, err = h.bot.Send(msg)
		}
	}
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
	case "list":
		h.handleList(ctx, update)
	default:
		_, err := h.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid command."))
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
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
	msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Choose one task from the list below, page: 1")
	kb := createListInlineKeyboard(tasks, page, cursorNext.Format(time.RFC3339), cursorPrevious.Format(time.RFC3339))
	msg.ReplyMarkup = kb
	_, err = h.bot.Send(msg)
	if err != nil {
		h.returnToMainMenu(ctx, update, err)
	}
}

func (h *Handler) handleCallback(update tgbotapi.Update) {
	ctx := context.WithValue(context.Background(), clientinterceptor.ChatIDKey, update.CallbackQuery.From.ID)
	callbackData := strings.Split(update.CallbackData(), ";")
	if len(callbackData) < 1 {
		return
	}
	buttonPressed := callbackData[0]
	switch buttonPressed {
	case "list":
		h.handleListButtonPressed(ctx, update, callbackData)
	case "get":
		h.handleGetButtonPressed(ctx, update, callbackData)
	case "delete":
		h.handleDeleteButtonPressed(ctx, update, callbackData)
	case "update":
		h.handleUpdateButtonPressed(ctx, update, callbackData)
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

		tasks, err := h.getCachedTasks(ctx, update.CallbackQuery.From.ID, page)
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
				msg := tgbotapi.NewMessage(update.CallbackQuery.From.ID, "No more tasks")
				_, err := h.bot.Request(msg)
				if err != nil {
					h.returnToMainMenu(ctx, update, err)
				}
				return
			}
			h.cacheTasks(ctx, update.CallbackQuery.From.ID, page, tasks)
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
		kb := createListInlineKeyboard(tasks, page, cursorNext.Format(time.RFC3339), cursorPrevious.Format(time.RFC3339))
		msg := tgbotapi.NewEditMessageText(update.CallbackQuery.From.ID, update.CallbackQuery.Message.MessageID, fmt.Sprintf("Choose one task from the list below, page: %d", page))
		msgMrkp := tgbotapi.NewEditMessageReplyMarkup(update.CallbackQuery.From.ID, update.CallbackQuery.Message.MessageID, kb)
		msg.ReplyMarkup = msgMrkp.ReplyMarkup
		_, err = h.bot.Request(msg)
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
		}
		return

	case "previous":
		page, err := strconv.Atoi(callbackData[2])
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
			return
		}
		page -= 1
		if page < 1 {
			msg := tgbotapi.NewMessage(update.CallbackQuery.From.ID, "You're already on 1st page!")
			_, err := h.bot.Send(msg)
			if err != nil {
				h.returnToMainMenu(ctx, update, err)
			}
			return
		}
		cursor := callbackData[3]

		tasks, err := h.getCachedTasks(ctx, update.CallbackQuery.From.ID, page)
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
				msg := tgbotapi.NewMessage(update.CallbackQuery.From.ID, "No more tasks")
				_, err := h.bot.Request(msg)
				if err != nil {
					h.returnToMainMenu(ctx, update, err)
				}
				return
			}
			h.cacheTasks(ctx, update.CallbackQuery.From.ID, page, tasks)
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
		kb := createListInlineKeyboard(tasks, page, cursorNext.Format(time.RFC3339), cursorPrevious.Format(time.RFC3339))
		msg := tgbotapi.NewEditMessageText(update.CallbackQuery.From.ID, update.CallbackQuery.Message.MessageID, fmt.Sprintf("Choose one task from the list below, page: %d", page))
		msgMrkp := tgbotapi.NewEditMessageReplyMarkup(update.CallbackQuery.From.ID, update.CallbackQuery.Message.MessageID, kb)
		msg.ReplyMarkup = msgMrkp.ReplyMarkup
		_, err = h.bot.Request(msg)
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
		}
		return
	}
}

func (h *Handler) handleGetButtonPressed(ctx context.Context, update tgbotapi.Update, callbackData []string) {
	taskId := callbackData[1]
	resp, err := h.client.GetTask(ctx, &proto.GetTaskRequest{Id: taskId})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			msg := tgbotapi.NewMessage(update.CallbackQuery.From.ID, "Task is not found")
			_, err = h.bot.Request(msg)
			if err != nil {
				h.returnToMainMenu(ctx, update, err)
				return
			}
			return
		}
		h.logAndNotify(update, err)
		return
	}
	task := resp.Task
	var taskStatus = "Pending"
	if task.GetStatus() == proto.Status_STATUS_INPROGRESS {
		taskStatus = "In progress"
	} else if task.GetStatus() == proto.Status_STATUS_DONE {
		taskStatus = "Done"
	}
	taskCreatedAt := task.GetCreatedAt().AsTime().Format("02 Jan 2006 15:04")
	taskUpdatedAt := task.GetUpdatedAt().AsTime().Format("02 Jan 2006 15:04")
	requestTxt := fmt.Sprintf("%v\n\n%v\n\nStatus: %v\n\nCreated: %v\nLast updated: %v\n", task.GetTitle(), task.GetDescription(), taskStatus, taskCreatedAt, taskUpdatedAt)
	request := tgbotapi.NewEditMessageText(update.CallbackQuery.From.ID, update.CallbackQuery.Message.MessageID, requestTxt)
	kb := createGetInlineKeyboard(task)
	requestReplMrkp := tgbotapi.NewEditMessageReplyMarkup(update.CallbackQuery.From.ID, update.CallbackQuery.Message.MessageID, kb)
	request.ReplyMarkup = requestReplMrkp.ReplyMarkup
	_, err = h.bot.Request(request)
	if err != nil {
		h.returnToMainMenu(ctx, update, err)
	}
}

func (h *Handler) handleDeleteButtonPressed(ctx context.Context, update tgbotapi.Update, callbackData []string) {
	taskId := callbackData[1]
	_, err := h.client.DeleteTask(ctx, &proto.DeleteTaskRequest{Id: taskId})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			_, err := h.bot.Send(tgbotapi.NewMessage(update.CallbackQuery.From.ID, "Task is not found"))
			if err != nil {
				h.logAndNotify(update, err)
			}
		}
		return
	}
	_, err = h.bot.Request(tgbotapi.NewMessage(update.CallbackQuery.From.ID, "Task succescfully deleted"))
	if err != nil {
		h.logAndNotify(update, nil)
		return
	}
	_, err = h.bot.Request(tgbotapi.NewDeleteMessage(update.CallbackQuery.From.ID, update.CallbackQuery.Message.MessageID))
	if err != nil {
		h.logAndNotify(update, err)
	}
}

func (h *Handler) handleUpdateButtonPressed(ctx context.Context, update tgbotapi.Update, callbackData []string) {
	updateBtn := callbackData[1]
	var task *proto.Task
	switch updateBtn {
	case "status":
		taskID := callbackData[3]
		if callbackData[2] == "InProgress" {
			resp, err := h.client.UpdateTask(ctx, &proto.UpdateTaskRequest{Id: taskID, Status: proto.Status_STATUS_INPROGRESS})
			if err != nil {
				h.logAndNotify(update, err)
				return
			}
			task = resp.GetTask()
		}
		if callbackData[2] == "Done" {
			resp, err := h.client.UpdateTask(ctx, &proto.UpdateTaskRequest{Id: taskID, Status: proto.Status_STATUS_DONE})
			if err != nil {
				h.logAndNotify(update, err)
				return
			}
			task = resp.GetTask()
		}
	}
	if task != nil {
		var taskStatus = "Pending"
		if task.GetStatus() == proto.Status_STATUS_INPROGRESS {
			taskStatus = "In progress"
		} else if task.GetStatus() == proto.Status_STATUS_DONE {
			taskStatus = "Done"
		}
		taskCreatedAt := task.GetCreatedAt().AsTime().Format("02 Jan 2006 15:04")
		taskUpdatedAt := task.GetUpdatedAt().AsTime().Format("02 Jan 2006 15:04")
		requestTxt := fmt.Sprintf("%v\n\n%v\n\nStatus: %v\n\nCreated: %v\nLast updated: %v\n", task.GetTitle(), task.GetDescription(), taskStatus, taskCreatedAt, taskUpdatedAt)
		request := tgbotapi.NewEditMessageText(update.CallbackQuery.From.ID, update.CallbackQuery.Message.MessageID, requestTxt)
		kb := createGetInlineKeyboard(task)
		requestReplMrkp := tgbotapi.NewEditMessageReplyMarkup(update.CallbackQuery.From.ID, update.CallbackQuery.Message.MessageID, kb)
		request.ReplyMarkup = requestReplMrkp.ReplyMarkup
		_, err := h.bot.Request(request)
		if err != nil {
			h.returnToMainMenu(ctx, update, err)
		}
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

func createListInlineKeyboard(tasks []*proto.Task, page int, cursorNext, cursorPrevious string) tgbotapi.InlineKeyboardMarkup {
	kbRows := make([][]tgbotapi.InlineKeyboardButton, 0, 7)
	for _, task := range tasks {
		kbBtn := tgbotapi.NewInlineKeyboardButtonData(truncateTitle(task.Title), fmt.Sprintf("get;%v", task.Id))
		kbRows = append(kbRows, []tgbotapi.InlineKeyboardButton{kbBtn})
	}

	btnPrevious := tgbotapi.NewInlineKeyboardButtonData("<-", fmt.Sprintf("list;previous;%d;%v", page, cursorPrevious))
	btnNext := tgbotapi.NewInlineKeyboardButtonData("->", fmt.Sprintf("list;next;%d;%v", page, cursorNext))
	kbRows = append(kbRows, []tgbotapi.InlineKeyboardButton{btnPrevious, btnNext})

	return tgbotapi.NewInlineKeyboardMarkup(kbRows...)
}

func createGetInlineKeyboard(task *proto.Task) tgbotapi.InlineKeyboardMarkup {
	kbRows := make([][]tgbotapi.InlineKeyboardButton, 0, 3)
	btnInProgress := tgbotapi.NewInlineKeyboardButtonData("In progress", fmt.Sprintf("update;status;InProgress;%v", task.GetId()))
	btnDone := tgbotapi.NewInlineKeyboardButtonData("Done", fmt.Sprintf("update;status;Done;%v", task.GetId()))
	btnChangeTitle := tgbotapi.NewInlineKeyboardButtonData("Change title", fmt.Sprintf("update;title;%v", task.GetId()))
	btnChangeDescription := tgbotapi.NewInlineKeyboardButtonData("Change description", fmt.Sprintf("update;desc;%v", task.GetId()))
	btnDelete := tgbotapi.NewInlineKeyboardButtonData("Delete task", fmt.Sprintf("delete;%v", task.GetId()))

	kbRows = append(kbRows,
		[]tgbotapi.InlineKeyboardButton{btnInProgress},
		[]tgbotapi.InlineKeyboardButton{btnDone},
		[]tgbotapi.InlineKeyboardButton{btnChangeTitle, btnChangeDescription},
		[]tgbotapi.InlineKeyboardButton{btnDelete},
	)
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
