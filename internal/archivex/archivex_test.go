package archivex

import (
	"archive/tar"
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
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

// writeZIPMembers writes count members each carrying a payload of payloadBytes.
func writeZIPMembers(t *testing.T, count, payloadBytes int) string {
	t.Helper()
	archivePath := filepath.Join(t.TempDir(), "archive.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create ZIP archive: %v", err)
	}
	writer := zip.NewWriter(file)
	payload := make([]byte, payloadBytes)
	for i := range count {
		header := &zip.FileHeader{Name: "public_html/file" + strconv.Itoa(i) + ".txt", Method: zip.Store}
		header.SetMode(0644)
		member, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("create ZIP member: %v", err)
		}
		if _, err := member.Write(payload); err != nil {
			t.Fatalf("write ZIP member: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close ZIP writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close ZIP archive: %v", err)
	}
	return archivePath
}

func TestScanEnforcesZIPLimits(t *testing.T) {
	t.Run("total size exceeded", func(t *testing.T) {
		path := writeZIPMembers(t, 3, 100)
		if err := Scan(context.Background(), path, TypeZIP, Limits{MaxTotalBytes: 200}); !errors.Is(err, ErrArchiveTooLarge) {
			t.Fatalf("Scan() error = %v, want ErrArchiveTooLarge", err)
		}
	})
	t.Run("member count exceeded", func(t *testing.T) {
		path := writeZIPMembers(t, 5, 1)
		if err := Scan(context.Background(), path, TypeZIP, Limits{MaxMembers: 3}); !errors.Is(err, ErrTooManyMembers) {
			t.Fatalf("Scan() error = %v, want ErrTooManyMembers", err)
		}
	})
	t.Run("within limits", func(t *testing.T) {
		path := writeZIPMembers(t, 3, 100)
		if err := Scan(context.Background(), path, TypeZIP, Limits{MaxTotalBytes: 10000, MaxMembers: 100}); err != nil {
			t.Fatalf("Scan() unexpected error: %v", err)
		}
	})
	t.Run("unlimited accepts large archive", func(t *testing.T) {
		path := writeZIPMembers(t, 10, 1000)
		if err := Scan(context.Background(), path, TypeZIP, Limits{}); err != nil {
			t.Fatalf("Scan() unexpected error: %v", err)
		}
	})
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
			if err := Scan(context.Background(), archivePath, TypeZIP, Limits{}); !errors.Is(err, test.want) {
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
			if err := Scan(context.Background(), archivePath, TypeTAR, Limits{}); !errors.Is(err, test.want) {
				t.Fatalf("Scan() error = %v, want %v", err, test.want)
			}
		})
	}
}

// writeTARMembers writes count regular members each declaring a size of sizeEach.
func writeTARMembers(t *testing.T, count int, sizeEach int64) string {
	t.Helper()
	archivePath := filepath.Join(t.TempDir(), "archive.tar")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create TAR archive: %v", err)
	}
	writer := tar.NewWriter(file)
	payload := make([]byte, sizeEach)
	for i := range count {
		header := tar.Header{Name: "public_html/file" + strconv.Itoa(i) + ".txt", Mode: 0644, Size: sizeEach, Typeflag: tar.TypeReg}
		if err := writer.WriteHeader(&header); err != nil {
			t.Fatalf("write TAR header: %v", err)
		}
		if sizeEach > 0 {
			if _, err := writer.Write(payload); err != nil {
				t.Fatalf("write TAR member: %v", err)
			}
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

func TestScanEnforcesTARLimits(t *testing.T) {
	t.Run("total size exceeded", func(t *testing.T) {
		path := writeTARMembers(t, 3, 100)
		if err := Scan(context.Background(), path, TypeTAR, Limits{MaxTotalBytes: 200}); !errors.Is(err, ErrArchiveTooLarge) {
			t.Fatalf("Scan() error = %v, want ErrArchiveTooLarge", err)
		}
	})
	t.Run("member count exceeded", func(t *testing.T) {
		path := writeTARMembers(t, 5, 1)
		if err := Scan(context.Background(), path, TypeTAR, Limits{MaxMembers: 3}); !errors.Is(err, ErrTooManyMembers) {
			t.Fatalf("Scan() error = %v, want ErrTooManyMembers", err)
		}
	})
	t.Run("within limits", func(t *testing.T) {
		path := writeTARMembers(t, 3, 100)
		if err := Scan(context.Background(), path, TypeTAR, Limits{MaxTotalBytes: 10000, MaxMembers: 100}); err != nil {
			t.Fatalf("Scan() unexpected error: %v", err)
		}
	})
}

func TestScanAcceptsRegularArchiveMembers(t *testing.T) {
	archivePath := writeTAR(t, tar.Header{Name: "public_html/index.html", Mode: 0644, Size: 7, Typeflag: tar.TypeReg})
	if err := Scan(context.Background(), archivePath, TypeTAR, Limits{}); err != nil {
		t.Fatalf("Scan() returned an unexpected error: %v", err)
	}
}

func TestValidateBSDTarListingsRejectsUnsafeRARMembers(t *testing.T) {
	tests := []struct {
		name    string
		names   string
		verbose string
		want    error
	}{
		{name: "parent traversal", names: "../escape\n", verbose: "-rw-r--r-- file\n", want: ErrUnsafePath},
		{name: "symbolic link", names: "public/link\n", verbose: "lrwxrwxrwx link -> /etc\n", want: ErrUnsafeMember},
		{name: "regular files", names: "public/index.html\npublic/assets/\n", verbose: "-rw-r--r-- index.html\ndrwxr-xr-x assets\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateBSDTarListings([]byte(test.names), []byte(test.verbose), Limits{}, nil)
			if !errors.Is(err, test.want) {
				t.Fatalf("validateBSDTarListings() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestValidateLSARListingRejectsUnsafeRARMembers(t *testing.T) {
	tests := []struct {
		name   string
		json   string
		want   error
		limits Limits
	}{
		{name: "parent traversal", json: `{"lsarContents":[{"XADFileName":"../escape","XADFileType":"Regular"}]}`, want: ErrUnsafePath},
		{name: "symbolic link", json: `{"lsarContents":[{"XADFileName":"link","XADFileType":"Symbolic Link","XADLinkDestination":"/etc"}]}`, want: ErrUnsafeMember},
		{name: "regular file", json: `{"lsarContents":[{"XADFileName":"public/index.html","XADFileType":"Regular"}]}`},
		{name: "invalid listing", json: `{}`, want: ErrInvalidArchive},
		{name: "oversized member", json: `{"lsarContents":[{"XADFileName":"big.bin","XADFileType":"Regular","XADFileSize":5000}]}`, want: ErrArchiveTooLarge, limits: Limits{MaxTotalBytes: 1000}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateLSARListing([]byte(test.json), test.limits, nil)
			if !errors.Is(err, test.want) {
				t.Fatalf("validateLSARListing() error = %v, want %v", err, test.want)
			}
		})
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
		"archive.RAR":     TypeRAR,
		"archive.gz":      TypeUnknown,
	}
	for name, want := range tests {
		if got := DetectType(name); got != want {
			t.Fatalf("DetectType(%q) = %d, want %d", name, got, want)
		}
	}
}
