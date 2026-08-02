package osiris

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

// Task120ArchiveVersion is the archive contract. It is a regular-file and
// directory TAR subset; links, devices, sparse files, and special metadata
// are not accepted.
const Task120ArchiveVersion = "tar-ustar-safe-1"

// Task120ArchiveLimits bounds entry count, one entry, total extracted bytes,
// and path length. Zero fields select 10,000 entries, 64 MiB per entry, 256
// MiB total, and 4 KiB paths.
type Task120ArchiveLimits struct {
	MaxEntries    int
	MaxEntryBytes int64
	MaxTotalBytes int64
	MaxPathBytes  int
}

func (l Task120ArchiveLimits) normalized() Task120ArchiveLimits {
	if l.MaxEntries <= 0 {
		l.MaxEntries = 10_000
	}
	if l.MaxEntryBytes <= 0 {
		l.MaxEntryBytes = 64 << 20
	}
	if l.MaxTotalBytes <= 0 {
		l.MaxTotalBytes = 256 << 20
	}
	if l.MaxPathBytes <= 0 {
		l.MaxPathBytes = 4 << 10
	}
	return l
}

// Task120ArchiveEntry is a regular file or directory. Type must be "file"
// or "dir" when creating an archive; extraction returns only those types.
// Data is owned by the returned value and is never retained by the backend.
type Task120ArchiveEntry struct {
	Name string
	Type string
	Data []byte
}

func task120ArchivePath(name string, max int) (string, error) {
	if name == "" || !utf8.ValidString(name) || strings.IndexByte(name, 0) >= 0 {
		return "", fmt.Errorf("%w: invalid archive path", ErrTask120Path)
	}
	if len(name) > max {
		return "", task120LimitError("archive path", "path bytes", int64(len(name)), int64(max))
	}
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(name, "/") || (len(name) >= 2 && name[1] == ':') {
		return "", fmt.Errorf("%w: absolute path %q", ErrTask120Path, name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: traversal path %q", ErrTask120Path, name)
	}
	return clean, nil
}

// Task120CreateArchive writes a deterministic safe TAR subset. Entries are
// emitted in caller order, duplicate normalized names are rejected, and src
// and dst ownership remains with the caller.
func Task120CreateArchive(ctx context.Context, entries []Task120ArchiveEntry, dst io.Writer, limits Task120ArchiveLimits) error {
	if dst == nil {
		return fmt.Errorf("archive create: %w: nil destination", ErrTask120Invalid)
	}
	limits = limits.normalized()
	if len(entries) > limits.MaxEntries {
		return task120LimitError("archive create", "entries", int64(len(entries)), int64(limits.MaxEntries))
	}
	w := tar.NewWriter(dst)
	seen := make(map[string]struct{}, len(entries))
	var total int64
	for i, entry := range entries {
		if err := task120CheckContext(ctx, "archive create"); err != nil {
			return err
		}
		name, err := task120ArchivePath(entry.Name, limits.MaxPathBytes)
		if err != nil {
			return fmt.Errorf("archive entry %d: %w", i, err)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("archive entry %d: %w: duplicate %q", i, ErrTask120Invalid, name)
		}
		seen[name] = struct{}{}
		if entry.Type != "file" && entry.Type != "dir" {
			return fmt.Errorf("archive entry %d: %w: type must be file or dir", i, ErrTask120Invalid)
		}
		if entry.Type == "dir" && len(entry.Data) != 0 {
			return fmt.Errorf("archive entry %d: %w: directory data must be empty", i, ErrTask120Invalid)
		}
		if int64(len(entry.Data)) > limits.MaxEntryBytes {
			return task120LimitError("archive create", "entry bytes", int64(len(entry.Data)), limits.MaxEntryBytes)
		}
		if total > limits.MaxTotalBytes-int64(len(entry.Data)) {
			return task120LimitError("archive create", "total bytes", total+int64(len(entry.Data)), limits.MaxTotalBytes)
		}
		typeFlag := byte(tar.TypeReg)
		mode := int64(0o644)
		if entry.Type == "dir" {
			typeFlag = tar.TypeDir
			mode = 0o755
		}
		header := &tar.Header{Name: name, Mode: mode, Size: int64(len(entry.Data)), Typeflag: typeFlag, Format: tar.FormatUSTAR}
		if entry.Type == "dir" {
			header.Size = 0
		}
		if err := w.WriteHeader(header); err != nil {
			return fmt.Errorf("archive header: %w", err)
		}
		if entry.Type == "file" {
			if _, err := w.Write(entry.Data); err != nil {
				return fmt.Errorf("archive data: %w", err)
			}
			total += int64(len(entry.Data))
		}
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("archive close: %w", err)
	}
	return nil
}

// Task120ExtractArchive reads a safe TAR subset and invokes visit once per
// file or directory. It rejects links, special files, unsafe/duplicate paths,
// malformed metadata, and size/entry bombs before allocating entry data.
func Task120ExtractArchive(ctx context.Context, src io.Reader, limits Task120ArchiveLimits, visit func(Task120ArchiveEntry) error) error {
	if src == nil || visit == nil {
		return fmt.Errorf("archive extract: %w: nil source or visitor", ErrTask120Invalid)
	}
	limits = limits.normalized()
	r := tar.NewReader(&task120ContextReader{ctx: ctx, r: src})
	seen := make(map[string]struct{})
	var total int64
	count := 0
	for {
		if err := task120CheckContext(ctx, "archive extract"); err != nil {
			return err
		}
		header, err := r.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("archive header: %w", err)
		}
		count++
		if count > limits.MaxEntries {
			return task120LimitError("archive extract", "entries", int64(count), int64(limits.MaxEntries))
		}
		name, err := task120ArchivePath(header.Name, limits.MaxPathBytes)
		if err != nil {
			return err
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("archive extract: %w: duplicate %q", ErrTask120Invalid, name)
		}
		seen[name] = struct{}{}
		entry := Task120ArchiveEntry{Name: name}
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			entry.Type = "file"
			if header.Size < 0 || header.Size > limits.MaxEntryBytes {
				return task120LimitError("archive extract", "entry bytes", header.Size, limits.MaxEntryBytes)
			}
			if total > limits.MaxTotalBytes-header.Size {
				return task120LimitError("archive extract", "total bytes", total+header.Size, limits.MaxTotalBytes)
			}
			entry.Data, err = io.ReadAll(io.LimitReader(r, header.Size+1))
			if err != nil {
				return fmt.Errorf("archive data: %w", err)
			}
			if int64(len(entry.Data)) != header.Size {
				return fmt.Errorf("archive data: %w: short file %q", ErrTask120Invalid, name)
			}
			total += header.Size
		case tar.TypeDir:
			if header.Size != 0 {
				return fmt.Errorf("archive extract: %w: directory has data", ErrTask120Invalid)
			}
			entry.Type = "dir"
		case tar.TypeSymlink, tar.TypeLink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo, tar.TypeGNUSparse:
			return fmt.Errorf("archive extract: %w: unsupported link or special entry %q", ErrTask120Invalid, name)
		default:
			return fmt.Errorf("archive extract: %w: unsupported TAR type %d", ErrTask120Invalid, header.Typeflag)
		}
		if err := visit(entry); err != nil {
			return fmt.Errorf("archive visitor %q: %w", name, err)
		}
	}
}
