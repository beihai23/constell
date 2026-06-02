package main

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/minio/minio-go/v7"

	pkgminio "github.com/constell/constell/backend/pkg/minio"
	"github.com/constell/constell/backend/pkg/middleware"
	filev1 "github.com/constell/constell/backend/pkg/proto/file/v1"
	"github.com/constell/constell/backend/pkg/proto/file/v1/filev1connect"
)

const maxFileSize = 5 * 1024 * 1024 // 5 MB

type FileService struct {
	repo    FileRepository
	minio   *pkgminio.Result
	baseURL string
}

var _ filev1connect.FileServiceHandler = (*FileService)(nil)

func NewFileService(repo FileRepository, minioResult *pkgminio.Result, baseURL string) *FileService {
	return &FileService{
		repo:    repo,
		minio:   minioResult,
		baseURL: baseURL,
	}
}

func (s *FileService) UploadFile(ctx context.Context, req *connect.Request[filev1.UploadFileRequest]) (*connect.Response[filev1.UploadFileResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	msg := req.Msg
	if msg.FileId == "" || msg.Filename == "" || msg.ContentType == "" || len(msg.Data) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("file_id, filename, content_type, and data are required"))
	}

	if len(msg.Data) > maxFileSize {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("file size exceeds 5 MB limit"))
	}

	now := time.Now()
	meta := &FileMeta{
		ID:          msg.FileId,
		UploaderID:  callerID,
		Filename:    msg.Filename,
		ContentType: msg.ContentType,
		Size:        int64(len(msg.Data)),
		Status:      "ready",
		Bucket:      s.minio.Bucket,
		CreatedAt:   now,
	}

	if err := s.repo.InsertFileMeta(ctx, meta); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save file metadata: %w", err))
	}

	// Upload original to MinIO.
	key := "originals/" + msg.FileId
	_, err := s.minio.Client.PutObject(ctx, s.minio.Bucket, key, bytes.NewReader(msg.Data), int64(len(msg.Data)), minio.PutObjectOptions{
		ContentType: msg.ContentType,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("upload to storage: %w", err))
	}

	// Generate and upload thumbnail if image.
	thumbnailURL := ""
	if isImage(msg.ContentType) {
		thumbData, thumbErr := generateThumbnail(msg.Data, msg.ContentType)
		if thumbErr == nil {
			thumbKey := "thumbnails/" + msg.FileId
			_, _ = s.minio.Client.PutObject(ctx, s.minio.Bucket, thumbKey, bytes.NewReader(thumbData), int64(len(thumbData)), minio.PutObjectOptions{
				ContentType: msg.ContentType,
			})
			thumbnailURL = s.baseURL + "/" + s.minio.Bucket + "/" + thumbKey
		}
	}

	fileInfo := &filev1.FileInfo{
		Id:           msg.FileId,
		Filename:     msg.Filename,
		ContentType:  msg.ContentType,
		Size:         int64(len(msg.Data)),
		Url:          s.baseURL + "/" + s.minio.Bucket + "/" + key,
		ThumbnailUrl: thumbnailURL,
		CreatedAt:    now.Unix(),
	}

	return connect.NewResponse(&filev1.UploadFileResponse{File: fileInfo}), nil
}

func (s *FileService) GetFilePresignedURL(ctx context.Context, req *connect.Request[filev1.GetFilePresignedURLRequest]) (*connect.Response[filev1.GetFilePresignedURLResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	msg := req.Msg
	if msg.FileId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("file_id is required"))
	}

	meta, err := s.repo.GetFileMeta(ctx, msg.FileId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("file not found"))
	}

	if meta.Status != "ready" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("file is not ready, status: %s", meta.Status))
	}

	key := "originals/" + msg.FileId
	presignedURL, err := s.minio.Client.PresignedGetObject(ctx, s.minio.Bucket, key, 15*time.Minute, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("generate presigned URL: %w", err))
	}

	expiresAt := time.Now().Add(15 * time.Minute).Unix()

	return connect.NewResponse(&filev1.GetFilePresignedURLResponse{
		Url:       presignedURL.String(),
		ExpiresAt: expiresAt,
	}), nil
}

func (s *FileService) DeleteFile(ctx context.Context, req *connect.Request[filev1.DeleteFileRequest]) (*connect.Response[filev1.DeleteFileResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	msg := req.Msg
	if msg.FileId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("file_id is required"))
	}

	meta, err := s.repo.GetFileMeta(ctx, msg.FileId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("file not found"))
	}

	if meta.UploaderID != callerID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("only the uploader can delete this file"))
	}

	// Delete original from MinIO.
	origKey := "originals/" + msg.FileId
	_ = s.minio.Client.RemoveObject(ctx, s.minio.Bucket, origKey, minio.RemoveObjectOptions{})

	// Delete thumbnail from MinIO (ignore error if it doesn't exist).
	thumbKey := "thumbnails/" + msg.FileId
	_ = s.minio.Client.RemoveObject(ctx, s.minio.Bucket, thumbKey, minio.RemoveObjectOptions{})

	if err := s.repo.DeleteFileMeta(ctx, msg.FileId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("delete file metadata: %w", err))
	}

	return connect.NewResponse(&filev1.DeleteFileResponse{}), nil
}
