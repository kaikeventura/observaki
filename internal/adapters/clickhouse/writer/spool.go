package writer

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	spoolMagic      = "OKB1"
	batchPrefix     = "batch-"
	batchSuffix     = ".bin"
	tempSuffix      = ".tmp"
	checkpointName  = "checkpoint.json"
	dlqDirName      = "dlq"
	maxHeaderBytes  = 1 << 20
	spoolFilePerm   = 0o600
	spoolDirPerm    = 0o700
	checkpointPerm  = 0o600
	checksumBytes   = sha256.Size
	batchHeaderSize = 8
)

// batchMeta is the durable description of one spooled batch. Seq is the
// monotonic checkpoint unit; DedupeToken is stable across replays.
type batchMeta struct {
	Seq                  uint64 `json:"seq"`
	Table                string `json:"table"`
	Signal               string `json:"signal"`
	Rows                 int    `json:"rows"`
	DedupeToken          string `json:"dedupe_token"`
	MinTimestampUnixNano int64  `json:"min_timestamp_unix_nano"`
	MaxTimestampUnixNano int64  `json:"max_timestamp_unix_nano"`
	CreatedAtUnixNano    int64  `json:"created_at_unix_nano"`
	DataSHA256           string `json:"data_sha256"`
}

type spoolFile struct {
	seq  uint64
	path string
}

// spool is the local durable buffer for the single-node profile. A batch is
// written to a temporary file, fsynced, and atomically renamed before it can
// be acknowledged to the producer. The checkpoint only advances after
// ClickHouse confirms the insert.
type spool struct {
	dir string
}

func openSpool(dir string) (*spool, error) {
	if dir == "" {
		return nil, errors.New("spool directory is empty")
	}
	if err := os.MkdirAll(dir, spoolDirPerm); err != nil {
		return nil, fmt.Errorf("create spool directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, dlqDirName), spoolDirPerm); err != nil {
		return nil, fmt.Errorf("create spool dlq directory: %w", err)
	}
	s := &spool{dir: dir}
	if err := s.removeTemporary(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *spool) batchPath(seq uint64) string {
	return filepath.Join(s.dir, fmt.Sprintf("%s%020d%s", batchPrefix, seq, batchSuffix))
}

func (s *spool) dlqPath(seq uint64) string {
	return filepath.Join(s.dir, dlqDirName, fmt.Sprintf("%s%020d%s", batchPrefix, seq, batchSuffix))
}

func (s *spool) removeTemporary() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("read spool directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), batchPrefix) || !strings.HasSuffix(entry.Name(), tempSuffix) {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, entry.Name())); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove incomplete spool batch %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// write persists meta and data. The caller must fill DataSHA256; write does
// not hash the payload a second time because the writer already hashed it to
// derive the deduplication token.
func (s *spool) write(meta *batchMeta, data []byte) (string, error) {
	sum, err := hex.DecodeString(meta.DataSHA256)
	if err != nil || len(sum) != checksumBytes {
		return "", fmt.Errorf("batch has an invalid data checksum %q", meta.DataSHA256)
	}
	if meta.CreatedAtUnixNano == 0 {
		meta.CreatedAtUnixNano = time.Now().UnixNano()
	}
	header, err := json.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("encode spool header: %w", err)
	}
	if len(header) > maxHeaderBytes {
		return "", fmt.Errorf("spool header is %d bytes, want <= %d", len(header), maxHeaderBytes)
	}
	path := s.batchPath(meta.Seq)
	temp := path + tempSuffix

	file, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, spoolFilePerm)
	if err != nil {
		return "", fmt.Errorf("create spool batch: %w", err)
	}
	cleanup := func(cause error) (string, error) {
		_ = file.Close()
		_ = os.Remove(temp)
		return "", cause
	}

	var prefix [batchHeaderSize]byte
	copy(prefix[:4], spoolMagic)
	binary.BigEndian.PutUint32(prefix[4:], uint32(len(header)))
	if _, err := file.Write(prefix[:]); err != nil {
		return cleanup(fmt.Errorf("write spool header prefix: %w", err))
	}
	if _, err := file.Write(header); err != nil {
		return cleanup(fmt.Errorf("write spool header: %w", err))
	}
	if _, err := file.Write(data); err != nil {
		return cleanup(fmt.Errorf("write spool data: %w", err))
	}
	if _, err := file.Write(sum); err != nil {
		return cleanup(fmt.Errorf("write spool checksum: %w", err))
	}
	if err := file.Sync(); err != nil {
		return cleanup(fmt.Errorf("sync spool batch: %w", err))
	}
	if err := file.Close(); err != nil {
		return cleanup(fmt.Errorf("close spool batch: %w", err))
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return "", fmt.Errorf("publish spool batch: %w", err)
	}
	if err := syncDir(s.dir); err != nil {
		return "", err
	}
	return path, nil
}

func (s *spool) read(path string) (*batchMeta, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read spool batch: %w", err)
	}
	if len(raw) < batchHeaderSize+checksumBytes {
		return nil, nil, fmt.Errorf("spool batch %s is too short", filepath.Base(path))
	}
	if string(raw[:4]) != spoolMagic {
		return nil, nil, fmt.Errorf("spool batch %s has bad magic", filepath.Base(path))
	}
	headerLen := int(binary.BigEndian.Uint32(raw[4:batchHeaderSize]))
	if headerLen <= 0 || headerLen > maxHeaderBytes {
		return nil, nil, fmt.Errorf("spool batch %s has invalid header length %d", filepath.Base(path), headerLen)
	}
	dataStart := batchHeaderSize + headerLen
	dataEnd := len(raw) - checksumBytes
	if dataStart > dataEnd {
		return nil, nil, fmt.Errorf("spool batch %s has truncated payload", filepath.Base(path))
	}

	var meta batchMeta
	if err := json.Unmarshal(raw[batchHeaderSize:dataStart], &meta); err != nil {
		return nil, nil, fmt.Errorf("decode spool header: %w", err)
	}
	data := raw[dataStart:dataEnd]
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != meta.DataSHA256 {
		return nil, nil, fmt.Errorf("spool batch %s checksum mismatch", filepath.Base(path))
	}
	return &meta, data, nil
}

func (s *spool) all() ([]spoolFile, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("read spool directory: %w", err)
	}
	files := make([]spoolFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), batchPrefix) || !strings.HasSuffix(entry.Name(), batchSuffix) {
			continue
		}
		seq, err := parseBatchSeq(entry.Name())
		if err != nil {
			return nil, err
		}
		files = append(files, spoolFile{seq: seq, path: filepath.Join(s.dir, entry.Name())})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].seq < files[j].seq })
	return files, nil
}

func parseBatchSeq(name string) (uint64, error) {
	base := strings.TrimSuffix(strings.TrimPrefix(name, batchPrefix), batchSuffix)
	if base == name {
		return 0, fmt.Errorf("invalid spool batch name %q", name)
	}
	seq, err := strconv.ParseUint(base, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid spool batch name %q: %w", name, err)
	}
	return seq, nil
}

func (s *spool) remove(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove spool batch: %w", err)
	}
	return syncDir(s.dir)
}

func (s *spool) moveToDLQ(path string, cause error) (string, error) {
	seq, err := parseBatchSeq(filepath.Base(path))
	if err != nil {
		return "", err
	}
	destination := s.dlqPath(seq)
	if err := os.Rename(path, destination); err != nil {
		return "", fmt.Errorf("move spool batch to dlq: %w", err)
	}
	record := struct {
		Seq   uint64 `json:"seq"`
		Path  string `json:"path"`
		Cause string `json:"cause"`
		Moved string `json:"moved_at"`
	}{
		Seq:   seq,
		Path:  destination,
		Cause: cause.Error(),
		Moved: time.Now().UTC().Format(time.RFC3339Nano),
	}
	encoded, _ := json.MarshalIndent(record, "", "  ")
	if err := os.WriteFile(destination+".error.json", encoded, spoolFilePerm); err != nil {
		return "", fmt.Errorf("write dlq record: %w", err)
	}
	if err := syncDir(filepath.Join(s.dir, dlqDirName)); err != nil {
		return "", err
	}
	if err := syncDir(s.dir); err != nil {
		return "", err
	}
	return destination, nil
}

type checkpoint struct {
	LastAckedSeq uint64 `json:"last_acked_seq"`
	UpdatedAt    string `json:"updated_at"`
}

func (s *spool) readCheckpoint() (uint64, error) {
	raw, err := os.ReadFile(filepath.Join(s.dir, checkpointName))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read spool checkpoint: %w", err)
	}
	var value checkpoint
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("decode spool checkpoint: %w", err)
	}
	return value.LastAckedSeq, nil
}

func (s *spool) writeCheckpoint(seq uint64) error {
	value := checkpoint{
		LastAckedSeq: seq,
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode spool checkpoint: %w", err)
	}
	path := filepath.Join(s.dir, checkpointName)
	temp := path + tempSuffix
	file, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, checkpointPerm)
	if err != nil {
		return fmt.Errorf("create spool checkpoint: %w", err)
	}
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		_ = os.Remove(temp)
		return fmt.Errorf("write spool checkpoint: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(temp)
		return fmt.Errorf("sync spool checkpoint: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("close spool checkpoint: %w", err)
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("publish spool checkpoint: %w", err)
	}
	return syncDir(s.dir)
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open spool directory for sync: %w", err)
	}
	defer func() { _ = dir.Close() }()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync spool directory: %w", err)
	}
	return nil
}
