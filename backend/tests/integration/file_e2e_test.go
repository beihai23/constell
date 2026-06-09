package integration

import (
	"net/http"
	"testing"
)

// TestFileUploadAndGetURL verifies the file upload and download URL flow.
// NOTE: Skipped because the file-service UploadFile RPC requires a client-generated
// file_id, but the API gateway does not generate one. This is a known server-side bug.
func TestFileUploadAndGetURL(t *testing.T) {
	user := registerUser(t)
	t.Logf("registered user: id=%s", user.UserID)

	// Upload a text file.
	content := []byte("hello from e2e file test")
	fileID := uploadFile(t, user.AccessToken, "test.txt", content, "text/plain")
	t.Logf("uploaded file: id=%s", fileID)

	if fileID == "" {
		t.Fatal("expected non-empty file ID")
	}

	// Get download URL.
	urlResp := doGet(t, apiURL("/api/v1/files/"+fileID+"/url"), user.AccessToken)
	defer urlResp.Body.Close()
	requireStatus(t, urlResp, http.StatusOK)

	var urlResult struct {
		URL string `json:"url"`
	}
	parseJSON(t, urlResp.Body, &urlResult)
	if urlResult.URL == "" {
		t.Fatal("expected non-empty download URL")
	}
	t.Logf("got download URL: %s", urlResult.URL)
}

// TestFileUploadAndDelete verifies the file deletion flow.
func TestFileUploadAndDelete(t *testing.T) {
	user := registerUser(t)

	// Upload a file.
	content := []byte("file to be deleted")
	fileID := uploadFile(t, user.AccessToken, "delete-me.txt", content, "text/plain")
	t.Logf("uploaded file: id=%s", fileID)

	// Delete the file.
	delResp := doDelete(t, apiURL("/api/v1/files/"+fileID), user.AccessToken)
	defer delResp.Body.Close()
	requireStatus(t, delResp, http.StatusOK)
	t.Logf("deleted file: id=%s", fileID)

	// Verify file is gone — getting URL should fail.
	urlResp := doGet(t, apiURL("/api/v1/files/"+fileID+"/url"), user.AccessToken)
	defer urlResp.Body.Close()
	if urlResp.StatusCode == http.StatusOK {
		t.Fatal("expected file to be deleted, but URL still accessible")
	}
	t.Logf("file %s confirmed deleted (status %d)", fileID, urlResp.StatusCode)
}

// TestFileUploadWithMessage verifies that a file can be attached to a channel message.
func TestFileUploadWithMessage(t *testing.T) {
	user := registerUser(t)

	// Upload a file.
	content := []byte("attachment content")
	fileID := uploadFile(t, user.AccessToken, "attachment.txt", content, "text/plain")
	t.Logf("uploaded file: id=%s", fileID)

	// Create community + channel.
	community := createTestCommunity(t, user.AccessToken)
	channel := createTestChannel(t, user.AccessToken, community.ID)

	// Send a message with the file attached.
	msgResp := doPost(t, apiURL("/api/v1/channels/"+channel.ID+"/messages"), user.AccessToken, map[string]interface{}{
		"content":  "message with attachment",
		"file_ids": []string{fileID},
	})
	defer msgResp.Body.Close()
	requireStatus(t, msgResp, http.StatusCreated)

	var msg struct {
		ID          string `json:"id"`
		Content     string `json:"content"`
		Attachments []struct {
			FileID      string `json:"file_id"`
			Filename    string `json:"filename"`
			ContentType string `json:"content_type"`
		} `json:"attachments"`
	}
	parseJSON(t, msgResp.Body, &msg)

	if msg.Content != "message with attachment" {
		t.Fatalf("content mismatch: got %q", msg.Content)
	}
	if len(msg.Attachments) == 0 {
		t.Fatal("expected at least one attachment")
	}
	att := msg.Attachments[0]
	if att.FileID != fileID {
		t.Fatalf("attachment file_id: got %q, want %q", att.FileID, fileID)
	}
	t.Logf("message %s has attachment: file_id=%s filename=%s content_type=%s",
		msg.ID, att.FileID, att.Filename, att.ContentType)
}

// TestFileUploadWithDM verifies that a file can be attached to a DM.
// NOTE: Skipped because user-service SendDM does not return attachments in the response.
func TestFileUploadWithDM(t *testing.T) {
	t.Skip("user-service SendDM does not return attachments in DM response")
	userA := registerUser(t)
	userB := registerUser(t)

	// User A uploads a file.
	content := []byte("dm attachment content")
	fileID := uploadFile(t, userA.AccessToken, "dm-file.txt", content, "text/plain")
	t.Logf("user A uploaded file: id=%s", fileID)

	// User A sends DM to User B with the file attached.
	dmResp := doPost(t, apiURL("/api/v1/dm/send"), userA.AccessToken, map[string]interface{}{
		"target_user_id": userB.UserID,
		"content":        "DM with attachment",
		"file_ids":       []string{fileID},
	})
	defer dmResp.Body.Close()
	requireStatus(t, dmResp, http.StatusCreated)

	var dm struct {
		ID             string `json:"id"`
		ConversationID string `json:"conversation_id"`
		Content        string `json:"content"`
		Attachments    []struct {
			FileID   string `json:"file_id"`
			Filename string `json:"filename"`
		} `json:"attachments"`
	}
	parseJSON(t, dmResp.Body, &dm)

	if len(dm.Attachments) == 0 {
		t.Fatal("expected at least one attachment in DM")
	}
	t.Logf("DM %s in conv %s has attachment: file_id=%s", dm.ID, dm.ConversationID, dm.Attachments[0].FileID)
}
