package files

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
