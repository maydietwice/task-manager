package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/maydietwice/task-manager/internal/db"
	"github.com/maydietwice/task-manager/internal/handler"
	"github.com/maydietwice/task-manager/internal/middleware"
	"github.com/maydietwice/task-manager/internal/service"
	"github.com/maydietwice/task-manager/proto"
	"google.golang.org/grpc"
)

func init() {
	if err := godotenv.Load(); err != nil {
		log.Fatalf("Unable to load dotenv: %v\n", err)
	}
}

func main() {
	maxOpenConns, err := strconv.Atoi(os.Getenv("DB_MAX_OPEN_CONNS"))

	if err != nil {
		log.Fatalf("Convertion error: %v\n", err)
	}

	maxIdleConns, err := strconv.Atoi(os.Getenv("DB_MAX_IDLE_CONNS"))

	if err != nil {
		log.Fatalf("Convertion error: %v\n", err)
	}

	maxIdleTime, err := time.ParseDuration(os.Getenv("DB_CONN_MAX_IDLE_TIME"))

	if err != nil {
		log.Fatalf("Convertion error: %v\n", err)
	}

	maxLifetime, err := time.ParseDuration(os.Getenv("DB_CONN_MAX_LIFETIME"))

	if err != nil {
		log.Fatalf("Convertion error: %v\n", err)
	}

	config := db.DBConfig{
		ConnectionString: os.Getenv("DB_CONNECTION_STRING"),
		MaxOpenConns:     maxOpenConns,
		MaxIdleConns:     maxIdleConns,
		MaxIdleTime:      maxIdleTime,
		MaxLifetime:      maxLifetime,
	}
	database, err := db.NewConnection(config)

	if err != nil {
		log.Fatalf("Unable to initialize new connection to DB: %v\n", err)
	}

	log.Println("DB connection successful")

	repo, err := db.NewRepository(database)

	if err != nil {
		log.Fatalf("Unable to inititalize new repository: %v\n", err)
	}

	lis, err := net.Listen("tcp", os.Getenv("NET_LISTEN_ADDRESS"))

	if err != nil {
		log.Fatalf("Unable to initialize listener: %v\n", err)
	}

	defer lis.Close()

	serv := service.NewService(repo, os.Getenv("JWT_SECRET_KEY"))

	handler := handler.NewHandler(serv)

	server := grpc.NewServer(grpc.UnaryInterceptor(middleware.JWTInterceptor(os.Getenv("JWT_SECRET_KEY"))))

	proto.RegisterTaskServiceServer(server, handler)

	go func() {
		if err := server.Serve(lis); err != nil {
			log.Fatalf("Server is down: %v\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)

	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit

	server.GracefulStop()

	log.Println("Server stopped gracefully")
}
