package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FileMeta struct {
	ID          string
	UploaderID  string
	Filename    string
	ContentType string
	Size        int64
	Status      string
	Bucket      string
	CreatedAt   time.Time
}

type FileRepository interface {
	InsertFileMeta(ctx context.Context, f *FileMeta) error
	GetFileMeta(ctx context.Context, id string) (*FileMeta, error)
	UpdateFileStatus(ctx context.Context, id, status string) error
	DeleteFileMeta(ctx context.Context, id string) error
}

type repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) FileRepository {
	return &repository{pool: pool}
}

func (r *repository) InsertFileMeta(ctx context.Context, f *FileMeta) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO file_metadata (id, uploader_id, filename, content_type, size, status, bucket, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		f.ID, f.UploaderID, f.Filename, f.ContentType, f.Size, f.Status, f.Bucket, f.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert file_metadata: %w", err)
	}
	return nil
}

func (r *repository) GetFileMeta(ctx context.Context, id string) (*FileMeta, error) {
	var f FileMeta
	err := r.pool.QueryRow(ctx,
		`SELECT id, uploader_id, filename, content_type, size, status, bucket, created_at
		 FROM file_metadata WHERE id = $1`, id,
	).Scan(&f.ID, &f.UploaderID, &f.Filename, &f.ContentType, &f.Size, &f.Status, &f.Bucket, &f.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("file not found: %w", err)
		}
		return nil, fmt.Errorf("query file_metadata: %w", err)
	}
	return &f, nil
}

func (r *repository) UpdateFileStatus(ctx context.Context, id, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE file_metadata SET status = $1 WHERE id = $2`, status, id)
	if err != nil {
		return fmt.Errorf("update file status: %w", err)
	}
	return nil
}

func (r *repository) DeleteFileMeta(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM file_metadata WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete file_metadata: %w", err)
	}
	return nil
}
