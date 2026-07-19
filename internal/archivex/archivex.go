// Package archivex securely extracts tenant-owned archives.
package archivex

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"encoding/json"
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
	// ErrRARUnavailable indicates that no safely supported RAR extractor is installed.
	ErrRARUnavailable = errors.New("RAR extraction is unavailable")
	// ErrInvalidArchive indicates that an archive listing could not be validated.
	ErrInvalidArchive = errors.New("archive could not be validated")
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
	// TypeRAR identifies a RAR archive.
	TypeRAR
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
	case strings.HasSuffix(lower, ".rar"):
		return TypeRAR
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
	case TypeRAR:
		return scanRAR(ctx, archivePath)
	default:
		return ErrUnsupported
	}
}

type lsarListing struct {
	Contents []struct {
		Name            string `json:"XADFileName"`
		Type            string `json:"XADFileType"`
		LinkDestination string `json:"XADLinkDestination"`
	} `json:"lsarContents"`
}

func safeCommand(ctx context.Context, name string, arguments ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Env = []string{"PATH=" + safePath, "LANG=C", "LC_ALL=C"}
	return command
}

func rarTool() (string, bool) {
	if _, err := exec.LookPath("bsdtar"); err == nil {
		return "bsdtar", true
	}
	if _, err := exec.LookPath("unar"); err == nil {
		if _, listErr := exec.LookPath("lsar"); listErr == nil {
			return "unar", true
		}
	}
	return "", false
}

func scanRAR(ctx context.Context, archivePath string) error {
	tool, ok := rarTool()
	if !ok {
		return ErrRARUnavailable
	}
	if tool == "bsdtar" {
		return scanRARWithBSDTar(ctx, archivePath)
	}
	return scanRARWithLSAR(ctx, archivePath)
}

func scanRARWithBSDTar(ctx context.Context, archivePath string) error {
	namesOutput, err := safeCommand(ctx, "bsdtar", "-tf", archivePath).Output()
	if err != nil {
		return ErrInvalidArchive
	}
	verboseOutput, err := safeCommand(ctx, "bsdtar", "-tvf", archivePath).Output()
	if err != nil {
		return ErrInvalidArchive
	}
	return validateBSDTarListings(namesOutput, verboseOutput)
}

func validateBSDTarListings(namesOutput, verboseOutput []byte) error {
	for _, line := range strings.Split(string(namesOutput), "\n") {
		name := strings.TrimSuffix(line, "\r")
		if name != "" && unsafeMemberName(name) {
			return ErrUnsafePath
		}
	}
	for _, line := range strings.Split(string(verboseOutput), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line[0] != '-' && line[0] != 'd' {
			return ErrUnsafeMember
		}
	}
	return nil
}

func scanRARWithLSAR(ctx context.Context, archivePath string) error {
	output, err := safeCommand(ctx, "lsar", "-json", archivePath).Output()
	if err != nil {
		return ErrInvalidArchive
	}
	return validateLSARListing(output)
}

func validateLSARListing(output []byte) error {
	var listing lsarListing
	if err := json.Unmarshal(output, &listing); err != nil || len(listing.Contents) == 0 {
		return ErrInvalidArchive
	}
	for _, member := range listing.Contents {
		if unsafeMemberName(member.Name) {
			return ErrUnsafePath
		}
		if member.LinkDestination != "" || (member.Type != "Regular" && member.Type != "Directory") {
			return ErrUnsafeMember
		}
	}
	return nil
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
	switch archiveType {
	case TypeZIP:
		command = tenantCommand(ctx, systemUser, "unzip", "-o", "-q", archivePath, "-d", destination)
	case TypeRAR:
		tool, ok := rarTool()
		if !ok {
			return "", ErrRARUnavailable
		}
		if tool == "bsdtar" {
			command = tenantCommand(ctx, systemUser, "bsdtar", "-x", "-f", archivePath, "-C", destination)
		} else {
			command = tenantCommand(ctx, systemUser, "unar", "-f", "-D", "-o", destination, archivePath)
		}
	default:
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
