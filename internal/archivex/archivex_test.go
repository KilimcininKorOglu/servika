package archivex

import (
	"archive/tar"
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeZIP(t *testing.T, name string, mode os.FileMode) string {
	t.Helper()
	archivePath := filepath.Join(t.TempDir(), "archive.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create ZIP archive: %v", err)
	}
	writer := zip.NewWriter(file)
	header := &zip.FileHeader{Name: name, Method: zip.Store}
	header.SetMode(mode)
	member, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatalf("create ZIP member: %v", err)
	}
	if _, err := member.Write([]byte("content")); err != nil {
		t.Fatalf("write ZIP member: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close ZIP writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close ZIP archive: %v", err)
	}
	return archivePath
}

func writeTAR(t *testing.T, header tar.Header) string {
	t.Helper()
	archivePath := filepath.Join(t.TempDir(), "archive.tar")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create TAR archive: %v", err)
	}
	writer := tar.NewWriter(file)
	if err := writer.WriteHeader(&header); err != nil {
		t.Fatalf("write TAR header: %v", err)
	}
	if header.Size > 0 {
		if _, err := writer.Write([]byte("content")); err != nil {
			t.Fatalf("write TAR member: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close TAR writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close TAR archive: %v", err)
	}
	return archivePath
}

func TestScanRejectsUnsafeZIPMembers(t *testing.T) {
	tests := []struct {
		name       string
		memberName string
		mode       os.FileMode
		want       error
	}{
		{name: "parent traversal", memberName: "../../etc/passwd", mode: 0644, want: ErrUnsafePath},
		{name: "absolute path", memberName: "/etc/passwd", mode: 0644, want: ErrUnsafePath},
		{name: "Windows traversal", memberName: `..\..\etc\passwd`, mode: 0644, want: ErrUnsafePath},
		{name: "symbolic link", memberName: "public/link", mode: os.ModeSymlink | 0777, want: ErrUnsafeMember},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			archivePath := writeZIP(t, test.memberName, test.mode)
			if err := Scan(context.Background(), archivePath, TypeZIP); !errors.Is(err, test.want) {
				t.Fatalf("Scan() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestScanRejectsUnsafeTARMembers(t *testing.T) {
	tests := []struct {
		name   string
		header tar.Header
		want   error
	}{
		{name: "parent traversal", header: tar.Header{Name: "../escape", Mode: 0644, Size: 7, Typeflag: tar.TypeReg}, want: ErrUnsafePath},
		{name: "symbolic link", header: tar.Header{Name: "link", Linkname: "/etc", Mode: 0777, Typeflag: tar.TypeSymlink}, want: ErrUnsafeMember},
		{name: "hard link", header: tar.Header{Name: "link", Linkname: "../escape", Mode: 0777, Typeflag: tar.TypeLink}, want: ErrUnsafeMember},
		{name: "device", header: tar.Header{Name: "device", Mode: 0600, Typeflag: tar.TypeChar}, want: ErrUnsafeMember},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			archivePath := writeTAR(t, test.header)
			if err := Scan(context.Background(), archivePath, TypeTAR); !errors.Is(err, test.want) {
				t.Fatalf("Scan() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestScanAcceptsRegularArchiveMembers(t *testing.T) {
	archivePath := writeTAR(t, tar.Header{Name: "public_html/index.html", Mode: 0644, Size: 7, Typeflag: tar.TypeReg})
	if err := Scan(context.Background(), archivePath, TypeTAR); err != nil {
		t.Fatalf("Scan() returned an unexpected error: %v", err)
	}
}

func TestTenantCommandUsesExplicitEnvironment(t *testing.T) {
	t.Setenv("SERVIKA_JWT_SECRET", "must-not-be-inherited")
	command := tenantCommand(context.Background(), "c_example", "tar", "-x")
	joined := strings.Join(command.Env, "\n")
	if strings.Contains(joined, "SERVIKA_JWT_SECRET") {
		t.Fatal("tenantCommand() inherited a panel secret")
	}
	for _, expected := range []string{"PATH=" + safePath, "HOME=/home/c_example", "USER=c_example", "LOGNAME=c_example"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("tenantCommand() environment does not contain %q", expected)
		}
	}
}

func TestDetectTypeRecognizesSupportedFormats(t *testing.T) {
	tests := map[string]Type{
		"archive.ZIP":     TypeZIP,
		"archive.tar":     TypeTAR,
		"archive.tar.gz":  TypeTARGzip,
		"archive.tgz":     TypeTARGzip,
		"archive.tar.bz2": TypeTARBzip2,
		"archive.tbz2":    TypeTARBzip2,
		"archive.tar.xz":  TypeTARXz,
		"archive.txz":     TypeTARXz,
		"archive.gz":      TypeUnknown,
	}
	for name, want := range tests {
		if got := DetectType(name); got != want {
			t.Fatalf("DetectType(%q) = %d, want %d", name, got, want)
		}
	}
}
