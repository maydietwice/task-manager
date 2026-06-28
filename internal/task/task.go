package task

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Status int

const (
	StatusPending Status = iota
	StatusInProgress
	StatusDone
)

type Task struct {
	Id          string
	OwnerId     string
	Title       string
	Description string
	Status      Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func NewTask(ownerId, title, description string) (*Task, error) {
	if title == "" {
		return nil, errors.New("Title can not be empty")
	}
	task := Task{
		OwnerId:     ownerId,
		Id:          uuid.New().String(),
		Title:       title,
		Description: description,
		Status:      StatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	return &task, nil
}
