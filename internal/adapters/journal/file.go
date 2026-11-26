// Package journal keeps the events of an ingestion run in a file: the ingestion.Journal
// port, on disk.
package journal

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"iter"
	"os"
	"path/filepath"
	"strconv"

	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

// ErrCorrupt is wrapped when a journal has a bad line that is not the last one: the file
// was damaged after it was written, and the events after the line cannot be trusted.
var ErrCorrupt = errors.New("corrupt journal")

var table = crc32.MakeTable(crc32.Castagnoli)

// File is a Journal in a file, one line per event: the CRC-32C of the event's JSON in 8
// hexadecimal digits, a space and the JSON.
//
//	3f1c09a2 {"type":"ChunkEmbedded","run":"ingest","chunk":"9b0e..."}
//
// Append writes all its events with one write and then calls Sync, which waits for the
// disk: when it returns, the events survive a crash of the process and of the machine.
// A crash in the middle of an append can leave the last line cut, or with bytes the disk
// never got; Open drops that line, whose events Append never confirmed.
type File struct {
	path string
	f    *os.File
}

// Open opens the journal at path, creating it if it does not exist. It reads the file
// once: a bad last line is cut off, and a bad line before the last fails with ErrCorrupt.
func Open(path string) (*File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	// A new file exists for good only when its directory does too: sync the entry.
	if serr := syncDir(filepath.Dir(path)); serr != nil {
		return nil, errors.Join(serr, f.Close())
	}
	good, err := goodLength(f)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("journal %s: %w", path, err), f.Close())
	}
	if err := cutAt(f, good); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return &File{path: path, f: f}, nil
}

// Append writes events at the end of the file and waits for the disk.
func (j *File) Append(events []domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	var buf []byte
	for _, e := range events {
		data, err := encode(e)
		if err != nil {
			return err
		}
		buf = fmt.Appendf(buf, "%08x %s\n", crc32.Checksum(data, table), data)
	}
	if _, err := j.f.Write(buf); err != nil {
		return err
	}
	return j.f.Sync()
}

// Replay reads the file from the start and yields its events, oldest first.
func (j *File) Replay() iter.Seq2[domain.Event, error] {
	return func(yield func(domain.Event, error) bool) {
		f, err := os.Open(j.path)
		if err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = f.Close() }() // read only: a failed Close loses nothing
		for n, line := range lines(f) {
			e, err := parse(line)
			if err != nil {
				yield(nil, fmt.Errorf("%w: line %d: %w", ErrCorrupt, n, err))
				return
			}
			if !yield(e, nil) {
				return
			}
		}
	}
}

// Close closes the file. The events are already on disk: Append synced them.
func (j *File) Close() error { return j.f.Close() }

// parse checks the CRC of a line and decodes its event. The line has no newline.
func parse(line []byte) (domain.Event, error) {
	sum, data, ok := bytes.Cut(line, []byte(" "))
	if !ok || len(sum) != 8 {
		return nil, errors.New("no checksum")
	}
	want, err := strconv.ParseUint(string(sum), 16, 32)
	if err != nil {
		return nil, fmt.Errorf("bad checksum %q", sum)
	}
	if got := crc32.Checksum(data, table); got != uint32(want) {
		return nil, fmt.Errorf("checksum %08x, the line has %08x", want, got)
	}
	return decode(data)
}

// lines yields the whole lines of r, numbered from 1, without the newline. A last line
// without a newline is not yielded: it was cut.
func lines(r io.Reader) iter.Seq2[int, []byte] {
	return func(yield func(int, []byte) bool) {
		br := bufio.NewReader(r)
		for n := 1; ; n++ {
			line, err := br.ReadBytes('\n')
			if err != nil { // io.EOF, with a cut line or nothing in line
				return
			}
			if !yield(n, line[:len(line)-1]) {
				return
			}
		}
	}
}

// goodLength returns how many bytes of f, from the start, are whole good lines. A bad
// line is allowed only as the last one.
func goodLength(f *os.File) (int64, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	var good int64
	bad := 0 // the number of the first bad line, 0 if none
	for n, line := range lines(f) {
		if bad != 0 {
			return 0, fmt.Errorf("%w: line %d is bad and line %d follows it", ErrCorrupt, bad, n)
		}
		if _, err := parse(line); err != nil {
			bad = n
			continue
		}
		good += int64(len(line)) + 1
	}
	return good, nil
}

// cutAt drops what f holds past size, if anything, and waits for the disk.
func cutAt(f *os.File, size int64) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() == size {
		return nil
	}
	if err := f.Truncate(size); err != nil {
		return err
	}
	return f.Sync()
}

// syncDir waits until the entries of the directory dir are on disk.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(d.Sync(), d.Close())
}
