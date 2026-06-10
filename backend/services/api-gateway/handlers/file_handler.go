package handlers

import (
	"io"
	"net/http"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	filev1 "github.com/constell/constell/backend/pkg/proto/file/v1"
	filev1connect "github.com/constell/constell/backend/pkg/proto/file/v1/filev1connect"
)

// FileHandler handles REST API requests for file operations.
type FileHandler struct {
	client filev1connect.FileServiceClient
}

// NewFileHandler creates a new FileHandler.
func NewFileHandler(client filev1connect.FileServiceClient) *FileHandler {
	return &FileHandler{client: client}
}

// fileInfoResponse is the JSON representation of file info.
type fileInfoResponse struct {
	ID           string `json:"id"`
	Filename     string `json:"filename"`
	ContentType  string `json:"content_type"`
	Size         int64  `json:"size"`
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnail_url"`
	CreatedAt    int64  `json:"created_at"`
}

// UploadFile handles POST /api/v1/files/upload.
func (h *FileHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form (max 50MB)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read file")
		return
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	cr := connect.NewRequest(&filev1.UploadFileRequest{
		FileId:      uuid.New().String(),
		Filename:    header.Filename,
		ContentType: contentType,
		Data:        data,
	})
	forwardAuth(r, cr)

	resp, err := h.client.UploadFile(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	f := resp.Msg.File
	writeJSON(w, http.StatusCreated, fileInfoResponse{
		ID:           f.GetId(),
		Filename:     f.GetFilename(),
		ContentType:  f.GetContentType(),
		Size:         f.GetSize(),
		URL:          f.GetUrl(),
		ThumbnailURL: f.GetThumbnailUrl(),
		CreatedAt:    f.GetCreatedAt(),
	})
}

// GetFileURL handles GET /api/v1/files/{id}/url.
func (h *FileHandler) GetFileURL(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "id")
	if fileID == "" {
		writeError(w, http.StatusBadRequest, "file id is required")
		return
	}

	cr := connect.NewRequest(&filev1.GetFilePresignedURLRequest{
		FileId: fileID,
	})
	forwardAuth(r, cr)

	resp, err := h.client.GetFilePresignedURL(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"url": resp.Msg.GetUrl(),
	})
}

// DeleteFile handles DELETE /api/v1/files/{id}.
func (h *FileHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "id")
	if fileID == "" {
		writeError(w, http.StatusBadRequest, "file id is required")
		return
	}

	cr := connect.NewRequest(&filev1.DeleteFileRequest{
		FileId: fileID,
	})
	forwardAuth(r, cr)

	_, err := h.client.DeleteFile(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
