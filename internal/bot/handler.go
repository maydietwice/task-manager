package bothandler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/maydietwice/task-manager/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Handler struct {
	client proto.TaskServiceClient
	bot    *tgbotapi.BotAPI
	secret []byte
}

func NewHandler(client proto.TaskServiceClient, bot *tgbotapi.BotAPI, secret string) *Handler {
	return &Handler{client: client, bot: bot, secret: []byte(secret)}
}

func (h *Handler) HandleUpdate(update tgbotapi.Update) {
	if update.Message == nil || !update.Message.IsCommand() {
		return
	}

	switch update.Message.Command() {
	case "create":
		h.create(update)
	case "delete":
		h.delete(update)
	case "get":
		h.get(update)
	case "update":
		h.update(update)
	case "list":
		h.list(update)
	case "start":
		h.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Input command from the list below."))
	default:
		h.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid command."))
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
