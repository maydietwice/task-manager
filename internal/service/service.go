package service

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/maydietwice/task-manager/internal/db"
	"github.com/maydietwice/task-manager/internal/task"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Service struct {
	repo   *db.Repository
	secret []byte
}

func NewService(repo *db.Repository, secret string) *Service {
	return &Service{repo: repo, secret: []byte(secret)}
}

func (s *Service) Register() (string, error) {
	id, err := uuid.NewUUID()

	if err != nil {
		return "", err
	}

	ownerId := id.String()

	newToken := jwt.New(jwt.SigningMethodHS256)

	jwtMap := jwt.MapClaims{}
	jwtMap["owner_id"] = ownerId

	newToken.Claims = jwtMap

	jwtString, err := newToken.SignedString(s.secret)

	if err != nil {
		return "", err
	}

	return jwtString, nil
}

func (s *Service) CreateTask(ownerId, title, description string) (*task.Task, error) {
	if ownerId == "" {
		return nil, errors.New("owner_id can't be an empty string")
	}

	if title == "" {
		return nil, errors.New("title can't be an empty string")
	}

	t, err := task.NewTask(ownerId, title, description)

	if err != nil {
		return nil, err
	}

	err = s.repo.Create(*t)

	if err != nil {
		return nil, err
	}

	return t, nil
}

func (s *Service) DeleteTask(id, ownerId string) error {
	if id == "" {
		return errors.New("id can't be an empty string")
	}

	return s.repo.Delete(id, ownerId)
}

func (s *Service) GetTask(id, ownerId string) (*task.Task, error) {
	if id == "" {
		return nil, errors.New("id can't be an empty string")
	}

	return s.repo.Get(id, ownerId)
}

func (s *Service) ListTask(ownerId string, page, limit int) ([]task.Task, error) {
	if ownerId == "" {
		return nil, errors.New("owner_id can't be an empty string")
	}

	if page < 1 {
		return nil, errors.New("page value can't be less than 1")
	}

	if limit < 1 {
		return nil, errors.New("limit value can't be less than 1")
	}

	if limit > 100 {
		limit = 100
	}

	return s.repo.List(ownerId, page, limit)
}

func (s *Service) UpdateTask(id, ownerId, title, description string, statusT task.Status) (task.Task, error) {
	if id == "" {
		return task.Task{}, errors.New("id can't be an empty string")
	}

	t, err := s.repo.Get(id, ownerId)

	if err != nil {
		return task.Task{}, err
	}

	if t == nil {
		return task.Task{}, status.Error(codes.NotFound, "task not found")
	}

	if title == "" {
		title = t.Title
	}

	if description == "" {
		description = t.Description
	}

	err = s.repo.Update(id, ownerId, title, description, statusT, time.Now())

	if err != nil {
		return task.Task{}, err
	}

	t.Title = title
	t.Description = description
	t.Status = statusT
	t.UpdatedAt = time.Now()

	return *t, nil
}
