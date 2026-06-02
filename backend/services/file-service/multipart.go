package main

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/minio/minio-go/v7"

	"github.com/constell/constell/backend/pkg/middleware"
	filev1 "github.com/constell/constell/backend/pkg/proto/file/v1"
)

func (s *FileService) InitMultipartUpload(ctx context.Context, req *connect.Request[filev1.InitMultipartUploadRequest]) (*connect.Response[filev1.InitMultipartUploadResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	msg := req.Msg
	if msg.FileId == "" || msg.Filename == "" || msg.ContentType == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("file_id, filename, and content_type are required"))
	}

	key := "originals/" + msg.FileId
	core := &minio.Core{Client: s.minio.Client}

	uploadID, err := core.NewMultipartUpload(ctx, s.minio.Bucket, key, minio.PutObjectOptions{
		ContentType: msg.ContentType,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("init multipart upload: %w", err))
	}

	now := time.Now()
	meta := &FileMeta{
		ID:          msg.FileId,
		UploaderID:  callerID,
		Filename:    msg.Filename,
		ContentType: msg.ContentType,
		Size:        0,
		Status:      "uploading",
		Bucket:      s.minio.Bucket,
		CreatedAt:   now,
	}
	if err := s.repo.InsertFileMeta(ctx, meta); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save file metadata: %w", err))
	}

	return connect.NewResponse(&filev1.InitMultipartUploadResponse{
		UploadId: uploadID,
	}), nil
}

func (s *FileService) UploadPart(ctx context.Context, req *connect.Request[filev1.UploadPartRequest]) (*connect.Response[filev1.UploadPartResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	msg := req.Msg
	if msg.FileId == "" || msg.UploadId == "" || len(msg.Data) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("file_id, upload_id, and data are required"))
	}

	key := "originals/" + msg.FileId
	core := &minio.Core{Client: s.minio.Client}

	part, err := core.PutObjectPart(ctx, s.minio.Bucket, key, msg.UploadId, int(msg.PartNumber), bytes.NewReader(msg.Data), int64(len(msg.Data)), minio.PutObjectPartOptions{})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("upload part: %w", err))
	}

	return connect.NewResponse(&filev1.UploadPartResponse{
		Etag: part.ETag,
	}), nil
}

func (s *FileService) CompleteMultipartUpload(ctx context.Context, req *connect.Request[filev1.CompleteMultipartUploadRequest]) (*connect.Response[filev1.CompleteMultipartUploadResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	msg := req.Msg
	if msg.FileId == "" || msg.UploadId == "" || len(msg.Parts) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("file_id, upload_id, and parts are required"))
	}

	key := "originals/" + msg.FileId
	core := &minio.Core{Client: s.minio.Client}

	// Convert proto CompletedPart to minio CompletePart.
	parts := make([]minio.CompletePart, len(msg.Parts))
	for i, p := range msg.Parts {
		parts[i] = minio.CompletePart{
			PartNumber: int(p.PartNumber),
			ETag:       p.Etag,
		}
	}

	uploadInfo, err := core.CompleteMultipartUpload(ctx, s.minio.Bucket, key, msg.UploadId, parts, minio.PutObjectOptions{})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("complete multipart upload: %w", err))
	}

	// Update file status to ready.
	if err := s.repo.UpdateFileStatus(ctx, msg.FileId, "ready"); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update file status: %w", err))
	}

	// Fetch metadata for response.
	meta, err := s.repo.GetFileMeta(ctx, msg.FileId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get file metadata: %w", err))
	}

	fileInfo := &filev1.FileInfo{
		Id:          meta.ID,
		Filename:    meta.Filename,
		ContentType: meta.ContentType,
		Size:        uploadInfo.Size,
		Url:         s.baseURL + "/" + s.minio.Bucket + "/" + key,
		CreatedAt:   meta.CreatedAt.Unix(),
	}

	return connect.NewResponse(&filev1.CompleteMultipartUploadResponse{
		File: fileInfo,
	}), nil
}
