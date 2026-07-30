package bothandler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/golang-jwt/jwt/v5"
	rdb "github.com/maydietwice/task-manager/internal/redis"
	"github.com/maydietwice/task-manager/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var defaultReplyKeyboard = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("create"),
		tgbotapi.NewKeyboardButton("delete"),
	),

	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("get"),
		tgbotapi.NewKeyboardButton("update"),
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

func (h *Handler) HandleUpdate(update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	if update.Message.IsCommand() {
		if update.Message.Command() != "start" {
			h.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid command."))

			return
		}

		h.returnToMainMenu(update, nil)

		return
	}

	chatInfo, err := h.repo.GetChatInfo(context.Background(), update.Message.Chat.ID)
	if err != nil {
		h.returnToMainMenu(update, err)

		return
	}

	currentState, ok := chatInfo["state"]

	if !ok {
		switch update.Message.Text {
		case "create":
			h.handleCreate(update, chatInfo)
		case "delete":
		case "get":
		case "update":
		case "list":
		default:
			h.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid command."))

			return
		}

		return
	}

	switch currentState {
	case "create":
		h.handleCreate(update, chatInfo)
	case "delete":
	case "get":
	case "update":
	case "list":
	default:
	}
}

func (h *Handler) newCtx(update tgbotapi.Update) (context.Context, error) {
	jwtToken := jwt.New(jwt.SigningMethodHS256)

	jwtToken.Claims = jwt.MapClaims{
		"owner_id": strconv.Itoa(int(update.Message.Chat.ID)),
	}

	token, err := jwtToken.SignedString(h.secret)
	if err != nil {
		return nil, err
	}

	md := metadata.Pairs("authorization", "Bearer "+token)

	ctx := metadata.NewOutgoingContext(context.Background(), md)

	return ctx, nil
}

func (h *Handler) returnToMainMenu(update tgbotapi.Update, err error) {
	h.repo.ClearState(context.Background(), update.Message.Chat.ID)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "This bot helps you managing your tasks. Choose one of the buttons below.")

	if err != nil {
		log.Printf("error handling request | chatID: %v, err: %v", update.Message.Chat.ID, err)
		msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Can not handle your request. Contact support.")
	}

	msg.ReplyMarkup = defaultReplyKeyboard
	h.bot.Send(msg)
}

func (h *Handler) handleCreate(update tgbotapi.Update, chatInfo map[string]string) {
	step := chatInfo["step"]

	switch step {
	case "title":
		if update.Message.Text == "" {
			h.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Title can not be empty, please enter you title."))
			return
		}
		hFields := []string{
			"step", "description",
			"title", update.Message.Text,
			"description", "",
		}
		err := h.repo.SetChatInfo(context.Background(), update.Message.Chat.ID, hFields...)
		if err != nil {
			h.returnToMainMenu(update, err)

			return
		}

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Enter your tasks's description(what's you gonna do?)")
		msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
		h.bot.Send(msg)

		return
	case "description":
		ctx, err := h.newCtx(update)
		if err != nil {
			h.returnToMainMenu(update, err)

			return
		}
		resp, err := h.client.CreateTask(ctx, &proto.CreateTaskRequest{Title: chatInfo["title"], Description: update.Message.Text})
		if err != nil {
			h.returnToMainMenu(update, err)

			return
		}
		msgText := fmt.Sprintf("ID: %v\nTitle: %v\nDescription: %v\nStatus: %v\nCreated at: %v\nUpdated at: %v\n", resp.Task.Id, resp.Task.Title, resp.Task.Description, resp.Task.Status, resp.Task.CreatedAt, resp.Task.UpdatedAt)
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)
		h.bot.Send(msg)

		h.returnToMainMenu(update, nil)

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
			h.returnToMainMenu(update, err)

			return
		}

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Enter your tasks's name")
		msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
		h.bot.Send(msg)

		return
	}
}

func (h *Handler) create(update tgbotapi.Update) {
	ctx, err := h.newCtx(update)
	if err != nil {
		log.Printf("create ctx err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

		h.bot.Send(msg)

		return
	}

	userMsg := strings.Split(update.Message.CommandArguments(), "|")

	var description string

	if strings.TrimSpace(userMsg[0]) == "" || len(userMsg) > 2 {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /create title | description")

		h.bot.Send(msg)

		return
	}

	if len(userMsg) == 2 {
		description = strings.TrimSpace(userMsg[1])
	}

	resp, err := h.client.CreateTask(ctx, &proto.CreateTaskRequest{Title: strings.TrimSpace(userMsg[0]), Description: description})
	if err != nil {
		log.Printf("create response err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

		h.bot.Send(msg)

		return
	}

	msgText := fmt.Sprintf("ID: %v\nTitle: %v\nDescription: %v\nStatus: %v\nCreated at: %v\nUpdated at: %v\n", resp.Task.Id, resp.Task.Title, resp.Task.Description, resp.Task.Status, resp.Task.CreatedAt, resp.Task.UpdatedAt)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)

	h.bot.Send(msg)
}

func (h *Handler) delete(update tgbotapi.Update) {
	ctx, err := h.newCtx(update)
	if err != nil {
		log.Printf("delete ctx err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

		h.bot.Send(msg)

		return
	}

	userMsg := update.Message.CommandArguments()

	if userMsg == "" {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /delete id")

		h.bot.Send(msg)

		return
	}

	_, err = h.client.DeleteTask(ctx, &proto.DeleteTaskRequest{Id: userMsg})
	if err != nil {
		log.Printf("delete response err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

		h.bot.Send(msg)

		return
	}

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Task deleted.")

	h.bot.Send(msg)
}

func (h *Handler) get(update tgbotapi.Update) {
	ctx, err := h.newCtx(update)
	if err != nil {
		log.Printf("get ctx err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

		h.bot.Send(msg)

		return
	}

	userMsg := update.Message.CommandArguments()

	if userMsg == "" {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /get id")

		h.bot.Send(msg)

		return
	}

	resp, err := h.client.GetTask(ctx, &proto.GetTaskRequest{Id: userMsg})

	if status.Code(err) == codes.NotFound {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Task not found.")

		h.bot.Send(msg)

		return
	}

	if err != nil {
		log.Printf("get response err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

		h.bot.Send(msg)

		return
	}

	msgText := fmt.Sprintf("ID: %v\nTitle: %v\nDescription: %v\nStatus: %v\nCreated at: %v\nUpdated at: %v\n", resp.Task.Id, resp.Task.Title, resp.Task.Description, resp.Task.Status, resp.Task.CreatedAt, resp.Task.UpdatedAt)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)

	h.bot.Send(msg)
}

func (h *Handler) update(update tgbotapi.Update) {
	ctx, err := h.newCtx(update)
	if err != nil {
		log.Printf("update ctx err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

		h.bot.Send(msg)

		return
	}

	userMsg := strings.Split(update.Message.CommandArguments(), "|")

	if len(userMsg) < 2 || len(userMsg) > 4 {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /update id(!required) | status(!required) | title | description")

		h.bot.Send(msg)

		return
	}

	id := strings.TrimSpace(userMsg[0])

	statusT, err := strconv.Atoi(strings.TrimSpace(userMsg[1]))
	if err != nil {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /update id(!required) | status(!required) | title | description")

		h.bot.Send(msg)

		return
	}

	if statusT < 0 || statusT > 2 {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /update id(!required) | status(!required) | title | description")

		h.bot.Send(msg)

		return
	}

	var title, description string

	for i, v := range userMsg {
		switch i {
		case 2:
			title = strings.TrimSpace(v)
		case 3:
			description = strings.TrimSpace(v)
		default:
			continue
		}
	}

	resp, err := h.client.UpdateTask(ctx, &proto.UpdateTaskRequest{Id: id, Status: proto.Status(statusT), Title: title, Description: description})

	if status.Code(err) == codes.NotFound {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Task not found.")

		h.bot.Send(msg)

		return
	}

	if err != nil {
		log.Printf("update response err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

		h.bot.Send(msg)

		return
	}

	msgText := fmt.Sprintf("ID: %v\nTitle: %v\nDescription: %v\nStatus: %v\nCreated at: %v\n pdated at: %v\n", resp.Task.Id, resp.Task.Title, resp.Task.Description, resp.Task.Status, resp.Task.CreatedAt, resp.Task.UpdatedAt)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)

	h.bot.Send(msg)
}

func (h *Handler) list(update tgbotapi.Update) {
	ctx, err := h.newCtx(update)
	if err != nil {
		log.Printf("list ctx err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

		h.bot.Send(msg)

		return
	}

	userMsg := strings.Split(update.Message.CommandArguments(), "|")

	if len(userMsg) != 2 {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /list page | limit")

		h.bot.Send(msg)

		return
	}

	page, err := strconv.Atoi(strings.TrimSpace(userMsg[0]))
	if err != nil {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /list page | limit")

		h.bot.Send(msg)

		return
	}

	limit, err := strconv.Atoi(strings.TrimSpace(userMsg[1]))
	if err != nil {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid format. Use /list page | limit")

		h.bot.Send(msg)

		return
	}

	if limit > 10 {
		limit = 10
	}

	resp, err := h.client.ListTask(ctx, &proto.ListTaskRequest{Page: int32(page), Limit: int32(limit)})
	if err != nil {
		log.Printf("list response err | chatId: %v, msgId: %v, err: %v\n", update.Message.Chat.ID, update.Message.MessageID, err)

		text := fmt.Sprintf("An error occurred. Contact support and provide userID: %v msgID: %v to them.", update.Message.Chat.ID, update.Message.MessageID)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)

		h.bot.Send(msg)

		return
	}

	for _, msgTask := range resp.Tasks {
		msgText := fmt.Sprintf("ID: %v\nTitle: %v\nDescription: %v\nStatus: %v\nCreated at: %v\nUpdated at: %v\n", msgTask.Id, msgTask.Title, msgTask.Description, msgTask.Status, msgTask.CreatedAt, msgTask.UpdatedAt)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)

		h.bot.Send(msg)
	}
}
