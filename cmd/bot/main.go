package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	bothandler "github.com/maydietwice/task-manager/internal/bot"
	"github.com/maydietwice/task-manager/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func init() {
	godotenv.Load()
}

func main() {
	tgBot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_BOT_TOKEN"))

	if err != nil {
		log.Fatalf("unable to connect with bot: %v\n", err)
	}

	tgBot.Debug = true

	log.Printf("authorized on account %s\n", tgBot.Self.UserName)

	commands := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "start", Description: "starts bot"},
		tgbotapi.BotCommand{Command: "create", Description: "create new task, format: title | description"},
		tgbotapi.BotCommand{Command: "delete", Description: "delete task, format: id"},
		tgbotapi.BotCommand{Command: "get", Description: "get task, format: id"},
		tgbotapi.BotCommand{Command: "update", Description: "update task, format: id | status(0 - pending; 1 - running; 2 - done) | title | description (leave empty to not update)"},
		tgbotapi.BotCommand{Command: "list", Description: "list of your tasks, format: page | limit"},
	)

	_, err = tgBot.Request(commands)

	if err != nil {
		log.Fatalf("unable to request bot's commands: %v\n", err)
	}

	conn, err := grpc.NewClient(os.Getenv("GRPC_SERVER_ADDRESS"), grpc.WithTransportCredentials(insecure.NewCredentials()))

	if err != nil {
		log.Fatalf("client connection failed: %v\n", err)
	}

	defer conn.Close()

	client := proto.NewTaskServiceClient(conn)

	handler := bothandler.NewHandler(client, tgBot, os.Getenv("JWT_SECRET_KEY"))

	u := tgbotapi.NewUpdate(0)

	u.Timeout = 60

	updates := tgBot.GetUpdatesChan(u)

	go func() {
		for update := range updates {
			go handler.HandleUpdate(update)
		}
	}()

	quit := make(chan os.Signal, 1)

	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit

	tgBot.StopReceivingUpdates()

	log.Print("Bot stopped gracefully")
}
