package db

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maydietwice/task-manager/internal/task"
)

const (
	maxRetries  = 5
	connTimeout = time.Second
	retryDelay  = 3 * time.Second
)

type Repository struct {
	db *pgxpool.Pool
}

type DBConfig struct {
	ConnectionString string
	MaxOpenConns     int
	MaxIdleConns     int
	MaxIdleTime      time.Duration
	MaxLifetime      time.Duration
}

func NewConnection(config DBConfig) (*pgxpool.Pool, error) {
	var conn *pgxpool.Pool
	var err error

	for i := 1; i <= maxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), connTimeout)
		conn, err = pgxpool.New(ctx, config.ConnectionString)
		cancel()
		if err != nil {
			log.Printf("Can not create pgxpool, retrying... %v/%v", i, maxRetries)
			time.Sleep(retryDelay)
			continue
		}

		ctx, cancel = context.WithTimeout(context.Background(), connTimeout)
		err = conn.Ping(ctx)
		cancel()
		if err != nil {
			log.Printf("Can not create pgxpool, retrying... %v/%v", i, maxRetries)
			time.Sleep(retryDelay)
			continue
		}
		break
	}

	return conn, err
}

func NewRepository(db *pgxpool.Pool) (*Repository, error) {
	newRepository := Repository{
		db: db,
	}

	return &newRepository, nil
}

func (r *Repository) Create(ctx context.Context, t task.Task) error {
	query := `INSERT
		INTO
			tasks(
				id,
				owner_id,
				title,
				description,
				status,
				created_at,
				updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.db.Exec(ctx, query, t.Id, t.OwnerId, t.Title, t.Description, t.Status, t.CreatedAt, t.UpdatedAt)

	return err
}

func (r *Repository) Delete(ctx context.Context, id, ownerId string) (int64, error) {
	query := `DELETE
		FROM
			tasks
		WHERE
			id = $1
			AND owner_id = $2`
	tag, err := r.db.Exec(ctx, query, id, ownerId)

	return tag.RowsAffected(), err
}

func (r *Repository) Get(ctx context.Context, id, ownerId string) (*task.Task, error) {
	query := `SELECT
			tasks.id,
			tasks.owner_id,
			tasks.title,
			tasks.description,
			tasks.status,
			tasks.created_at,
			tasks.updated_at
		FROM
			tasks
		WHERE
			tasks.id = $1
			AND tasks.owner_id = $2`
	row := r.db.QueryRow(ctx, query, id, ownerId)
	t := task.Task{}
	err := row.Scan(
		&t.Id,
		&t.OwnerId,
		&t.Title,
		&t.Description,
		&t.Status,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}

	return &t, err
}

func (r *Repository) List(ctx context.Context, ownerId string, after time.Time) ([]task.Task, error) {
	query := `SELECT
			tasks.id,
			tasks.owner_id,
			tasks.title,
			tasks.description,
			tasks.status,
			tasks.created_at,
			tasks.updated_at
		FROM
			tasks
		WHERE
			tasks.owner_id = $1
			AND tasks.created_at < $2
		ORDER BY
			tasks.created_at DESC
		LIMIT
			5`
	rows, err := r.db.Query(ctx, query, ownerId, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	t := task.Task{}
	tList := make([]task.Task, 0)
	for rows.Next() {
		err := rows.Scan(
			&t.Id,
			&t.OwnerId,
			&t.Title,
			&t.Description,
			&t.Status,
			&t.CreatedAt,
			&t.UpdatedAt,
		)
		if err != nil {
			return tList, err
		}

		tList = append(tList, t)
	}
	if rows.Err() != nil {
		return tList, err
	}

	return tList, nil
}

func (r *Repository) Update(ctx context.Context, id, ownerId, title, description string, status task.Status, updatedAt time.Time) error {
	query := `UPDATE
			tasks
		SET
			status = $1,
			title = $2,
			description = $3,
			updated_at = $4
		WHERE
			id = $5
			AND owner_id = $6`
	_, err := r.db.Exec(ctx, query, status, title, description, updatedAt, id, ownerId)

	return err
}
