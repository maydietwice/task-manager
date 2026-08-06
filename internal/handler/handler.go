package handler

import (
	"context"
	"fmt"

	"github.com/maydietwice/task-manager/internal/service"
	"github.com/maydietwice/task-manager/internal/task"
	"github.com/maydietwice/task-manager/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Handler struct {
	service *service.Service
	proto.UnimplementedTaskServiceServer
}

func NewHandler(s *service.Service) *Handler {
	return &Handler{service: s}
}

func (h *Handler) Register(ctx context.Context, r *proto.RegisterRequest) (*proto.RegisterResponse, error) {
	jwtToken, err := h.service.Register()
	if err != nil {
		return nil, err
	}

	return &proto.RegisterResponse{Token: jwtToken}, nil
}

func getOwnerID(ctx context.Context) (string, error) {
	ownerID, ok := ctx.Value("owner_id").(string)
	if !ok {
		err := fmt.Errorf("Type expected: string, got: %T", ctx.Value("owner_id"))
		return "", err
	}

	return ownerID, nil
}

func taskToProto(t task.Task) *proto.Task {
	protoTask := proto.Task{
		Id:          t.Id,
		OwnerId:     t.OwnerId,
		Title:       t.Title,
		Description: t.Description,
		Status:      proto.Status(t.Status),
		CreatedAt:   timestamppb.New(t.CreatedAt),
		UpdatedAt:   timestamppb.New(t.UpdatedAt),
	}

	return &protoTask
}

func (h *Handler) CreateTask(ctx context.Context, r *proto.CreateTaskRequest) (*proto.CreateTaskResponse, error) {
	ownerID, err := getOwnerID(ctx)
	if err != nil {
		return nil, err
	}

	t, err := h.service.CreateTask(ownerID, r.Title, r.Description)
	if err != nil {
		return nil, err
	}

	return &proto.CreateTaskResponse{Task: taskToProto(*t)}, nil
}

func (h *Handler) DeleteTask(ctx context.Context, r *proto.DeleteTaskRequest) (*proto.DeleteTaskResponse, error) {
	ownerID, err := getOwnerID(ctx)
	if err != nil {
		return nil, err
	}

	err = h.service.DeleteTask(r.Id, ownerID)
	if err != nil {
		return nil, err
	}

	return &proto.DeleteTaskResponse{Success: true}, nil
}

func (h *Handler) GetTask(ctx context.Context, r *proto.GetTaskRequest) (*proto.GetTaskResponse, error) {
	ownerID, err := getOwnerID(ctx)
	if err != nil {
		return nil, err
	}

	t, err := h.service.GetTask(r.Id, ownerID)
	if err != nil {
		return nil, err
	}

	if t == nil {
		return nil, status.Error(codes.NotFound, "task not found")
	}

	return &proto.GetTaskResponse{Task: taskToProto(*t)}, nil
}

func (h *Handler) UpdateTask(ctx context.Context, r *proto.UpdateTaskRequest) (*proto.UpdateTaskResponse, error) {
	ownerID, err := getOwnerID(ctx)
	if err != nil {
		return nil, err
	}

	task, err := h.service.UpdateTask(r.Id, ownerID, r.Title, r.Description, task.Status(r.Status))
	if err != nil {
		return nil, err
	}

	return &proto.UpdateTaskResponse{Task: taskToProto(task)}, nil
}

func (h *Handler) ListTask(ctx context.Context, r *proto.ListTaskRequest) (*proto.ListTaskResponse, error) {
	ownerID, err := getOwnerID(ctx)
	if err != nil {
		return nil, err
	}

	list, err := h.service.ListTask(ownerID, r.After.AsTime())
	if err != nil {
		return nil, err
	}

	conv := make([]*proto.Task, 0, len(list))

	for _, t := range list {
		conv = append(conv, taskToProto(t))
	}

	return &proto.ListTaskResponse{Tasks: conv}, nil
}
