package db

import (
	"database/sql"
	"errors"
	"time"

	_ "github.com/lib/pq"
	"github.com/maydietwice/task-manager/internal/task"
)

type Repository struct {
	db         *sql.DB
	createStmt *sql.Stmt
	deleteStmt *sql.Stmt
	getStmt    *sql.Stmt
	listStmt   *sql.Stmt
	updateStmt *sql.Stmt
}

type DBConfig struct {
	ConnectionString string
	MaxOpenConns     int
	MaxIdleConns     int
	MaxIdleTime      time.Duration
	MaxLifetime      time.Duration
}

func NewConnection(config DBConfig) (*sql.DB, error) {
	db, err := sql.Open("postgres", config.ConnectionString)

	if err != nil {
		return nil, err
	}

	err = db.Ping()

	if err != nil {
		return nil, errors.New("Database is not answering, connection failed")
	}

	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxIdleTime(config.MaxIdleTime)
	db.SetConnMaxLifetime(config.MaxLifetime)

	return db, nil
}

func NewRepository(db *sql.DB) (*Repository, error) {
	createStmt, err := db.Prepare(
		`INSERT
		INTO
			tasks(
				id,
				owner_id,
				title,
				description,
				status,
				created_at,
				updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
	)

	if err != nil {
		return nil, err
	}

	deleteStmt, err := db.Prepare(
		`DELETE
		FROM
			tasks
		WHERE
			id = $1
			AND owner_id = $2`,
	)

	if err != nil {
		return nil, err
	}

	getStmt, err := db.Prepare(
		`SELECT
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
			AND tasks.owner_id = $2`,
	)

	if err != nil {
		return nil, err
	}

	listStmt, err := db.Prepare(
		`SELECT
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
		ORDER BY
			tasks.created_at
		LIMIT
			$2
		OFFSET
			$3`,
	)

	if err != nil {
		return nil, err
	}

	updateStmt, err := db.Prepare(
		`UPDATE
			tasks
		SET
			status = $1,
			title = $2,
			description = $3,
			updated_at = $4
		WHERE
			id = $5
			AND owner_id = $6`,
	)

	if err != nil {
		return nil, err
	}

	newRepository := Repository{
		db:         db,
		createStmt: createStmt,
		deleteStmt: deleteStmt,
		getStmt:    getStmt,
		listStmt:   listStmt,
		updateStmt: updateStmt,
	}

	return &newRepository, nil
}

func (r *Repository) Create(t task.Task) error {
	_, err := r.createStmt.Exec(t.Id, t.OwnerId, t.Title, t.Description, t.Status, t.CreatedAt, t.UpdatedAt)

	return err
}

func (r *Repository) Delete(id, ownerId string) error {
	_, err := r.deleteStmt.Exec(id, ownerId)

	return err
}

func (r *Repository) Get(id, ownerId string) (*task.Task, error) {
	row := r.getStmt.QueryRow(id, ownerId)

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

	if err == sql.ErrNoRows {
		return nil, nil
	}

	return &t, err
}

func (r *Repository) List(ownerId string, page, limit int) ([]task.Task, error) {
	rows, err := r.listStmt.Query(ownerId, limit, (page-1)*limit)

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

	return tList, nil
}

func (r *Repository) Update(id, ownerId, title, description string, status task.Status, updatedAt time.Time) error {
	_, err := r.updateStmt.Exec(status, title, description, updatedAt, id, ownerId)

	return err
}
