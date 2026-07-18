// Package archivex securely extracts tenant-owned archives.
package archivex

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

var (
	// ErrUnsupported indicates that an archive format is not supported.
	ErrUnsupported = errors.New("unsupported archive format")
	// ErrUnsafePath indicates that an archive member would escape the destination.
	ErrUnsafePath = errors.New("archive member escapes the destination")
	// ErrUnsafeMember indicates that an archive contains a link or special file.
	ErrUnsafeMember = errors.New("archive contains a link or special file")
	// ErrInvalidTenant indicates that the extraction user is not managed by Servika.
	ErrInvalidTenant = errors.New("invalid tenant user")
)

// Type identifies a supported archive format.
type Type uint8

const (
	// TypeUnknown identifies an unsupported archive format.
	TypeUnknown Type = iota
	// TypeZIP identifies a ZIP archive.
	TypeZIP
	// TypeTAR identifies an uncompressed TAR archive.
	TypeTAR
	// TypeTARGzip identifies a gzip-compressed TAR archive.
	TypeTARGzip
	// TypeTARBzip2 identifies a bzip2-compressed TAR archive.
	TypeTARBzip2
	// TypeTARXz identifies an xz-compressed TAR archive.
	TypeTARXz
)

const safePath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// DetectType determines an archive type from its case-insensitive filename suffix.
func DetectType(name string) Type {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return TypeZIP
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return TypeTARGzip
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"):
		return TypeTARBzip2
	case strings.HasSuffix(lower, ".tar.xz"), strings.HasSuffix(lower, ".txz"):
		return TypeTARXz
	case strings.HasSuffix(lower, ".tar"):
		return TypeTAR
	default:
		return TypeUnknown
	}
}

func unsafeMemberName(name string) bool {
	name = strings.ReplaceAll(name, `\`, "/")
	if strings.HasPrefix(name, "/") {
		return true
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

// Scan validates every archive member without writing files.
func Scan(ctx context.Context, archivePath string, archiveType Type) error {
	switch archiveType {
	case TypeZIP:
		return scanZIP(ctx, archivePath)
	case TypeTAR, TypeTARGzip, TypeTARBzip2, TypeTARXz:
		return scanTAR(ctx, archivePath, archiveType)
	default:
		return ErrUnsupported
	}
}

func scanZIP(ctx context.Context, archivePath string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open ZIP archive: %w", err)
	}
	defer reader.Close()

	for _, member := range reader.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		mode := member.Mode()
		if mode&os.ModeSymlink != 0 || mode&(os.ModeDevice|os.ModeNamedPipe|os.ModeSocket) != 0 {
			return ErrUnsafeMember
		}
		if unsafeMemberName(member.Name) {
			return ErrUnsafePath
		}
	}
	return nil
}

func scanTAR(ctx context.Context, archivePath string, archiveType Type) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer file.Close()

	var reader io.Reader = file
	var gzipReader *gzip.Reader
	var xzCommand *exec.Cmd
	var xzStderr bytes.Buffer

	switch archiveType {
	case TypeTARGzip:
		gzipReader, err = gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("open gzip stream: %w", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	case TypeTARBzip2:
		reader = bzip2.NewReader(file)
	case TypeTARXz:
		xzCommand = exec.CommandContext(ctx, "xz", "-dc")
		xzCommand.Env = []string{"PATH=" + safePath}
		xzCommand.Stdin = file
		xzCommand.Stderr = &xzStderr
		pipe, pipeErr := xzCommand.StdoutPipe()
		if pipeErr != nil {
			return fmt.Errorf("open xz output: %w", pipeErr)
		}
		if err := xzCommand.Start(); err != nil {
			return fmt.Errorf("start xz: %w", err)
		}
		defer func() {
			if xzCommand != nil && xzCommand.Process != nil {
				_ = xzCommand.Process.Kill()
				_ = xzCommand.Wait()
			}
		}()
		reader = pipe
	}

	tarReader := tar.NewReader(reader)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, nextErr := tarReader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return fmt.Errorf("read TAR archive: %w", nextErr)
		}
		switch header.Typeflag {
		case tar.TypeSymlink, tar.TypeLink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			return ErrUnsafeMember
		}
		if unsafeMemberName(header.Name) {
			return ErrUnsafePath
		}
	}

	if xzCommand != nil {
		waitErr := xzCommand.Wait()
		xzCommand = nil
		if waitErr != nil {
			return fmt.Errorf("decompress xz archive: %s: %w", strings.TrimSpace(xzStderr.String()), waitErr)
		}
	}
	return nil
}

func tenantCommand(ctx context.Context, systemUser string, arguments ...string) *exec.Cmd {
	fullArguments := append([]string{"-u", systemUser, "--"}, arguments...)
	command := exec.CommandContext(ctx, "runuser", fullArguments...)
	command.Env = []string{
		"PATH=" + safePath,
		"HOME=/home/" + systemUser,
		"USER=" + systemUser,
		"LOGNAME=" + systemUser,
	}
	return command
}

// Extract validates an archive and extracts it as the owning tenant user.
func Extract(ctx context.Context, archivePath, destination, systemUser string) (string, error) {
	archiveType := DetectType(archivePath)
	if archiveType == TypeUnknown {
		return "", ErrUnsupported
	}
	if !strings.HasPrefix(systemUser, "c_") || len(systemUser) == 2 {
		return "", ErrInvalidTenant
	}
	for _, character := range systemUser[2:] {
		if character != '_' && (character < '0' || character > '9') && (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') {
			return "", ErrInvalidTenant
		}
	}
	if err := Scan(ctx, archivePath, archiveType); err != nil {
		return "", err
	}

	var command *exec.Cmd
	if archiveType == TypeZIP {
		command = tenantCommand(ctx, systemUser, "unzip", "-o", "-q", archivePath, "-d", destination)
	} else {
		file, err := os.Open(archivePath)
		if err != nil {
			return "", fmt.Errorf("open archive: %w", err)
		}
		defer file.Close()

		flag := "-x"
		switch archiveType {
		case TypeTARGzip:
			flag = "-xz"
		case TypeTARBzip2:
			flag = "-xj"
		case TypeTARXz:
			flag = "-xJ"
		}
		command = tenantCommand(ctx, systemUser, "tar", flag, "-f", "-", "-C", destination)
		command.Stdin = file
	}

	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("extract archive as tenant: %w", err)
	}
	return string(output), nil
}
