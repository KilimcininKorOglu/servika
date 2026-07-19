package files

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileMetadataReturnsContractFieldsForPermissionsDisplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.html")
	if err := os.WriteFile(path, []byte("content"), 0640); err != nil {
		t.Fatalf("write metadata fixture: %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("stat metadata fixture: %v", err)
	}

	mode, permissions, owner, group := fileMetadata(info)
	if mode != "0640" || permissions != "-rw-r-----" {
		t.Fatalf("fileMetadata() mode = %q, permissions = %q", mode, permissions)
	}
	if owner == "" || group == "" {
		t.Fatal("fileMetadata() omitted owner or group")
	}
}

func TestParseMultipartUploadRejectsBodyAboveRequestLimit(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "large.txt")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write([]byte(strings.Repeat("x", 128))); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body.Bytes()))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()

	err = parseMultipartUpload(response, request, 64, 16)
	if !errors.Is(err, errUploadTooLarge) {
		t.Fatalf("parseMultipartUpload() error = %v, want upload size error", err)
	}
}

func TestParseMultipartUploadKeepsMalformedBodyDistinctFromSizeLimit(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("not multipart"))
	request.Header.Set("Content-Type", "multipart/form-data; boundary=missing")
	response := httptest.NewRecorder()

	err := parseMultipartUpload(response, request, 1024, 16)
	if err == nil {
		t.Fatal("parseMultipartUpload() accepted malformed multipart data")
	}
	if errors.Is(err, errUploadTooLarge) {
		t.Fatal("parseMultipartUpload() classified malformed data as oversized")
	}
}
