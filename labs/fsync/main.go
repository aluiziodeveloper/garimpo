// Program fsync measures what it costs to make a write durable. It appends records to a
// file, one write each, and calls Sync (fsync) after every n of them, for several n: a
// write only reaches the kernel's page cache, and only Sync waits for the disk.
//
//	go run ./labs/fsync -records 2000 -size 128 -every 1,8,64,512,0
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// syncer is what a measure writes to: an *os.File, or a fake in the tests.
type syncer interface {
	io.Writer
	Sync() error
}

// result is what a measure saw: how long it took and how long each Sync took.
type result struct {
	records int
	elapsed time.Duration
	took    []time.Duration // one per Sync, sorted
}

func (r result) syncs() int { return len(r.took) }

// perSecond is the records written per second, the syncs included.
func (r result) perSecond() float64 { return float64(r.records) / r.elapsed.Seconds() }

// percentile returns the Sync time that a fraction q of the syncs did not pass.
func (r result) percentile(q float64) time.Duration {
	if len(r.took) == 0 {
		return 0
	}
	return r.took[int(float64(len(r.took)-1)*q)]
}

// appendRecords writes records records of size bytes to f, one Write each, and calls Sync
// after every every records and once more for the last ones, so that every record is
// synced when it returns. every 0 never calls Sync: the records may still be in the page
// cache when it returns.
func appendRecords(f syncer, records, size, every int) (result, error) {
	record := append(bytes.Repeat([]byte("x"), size-1), '\n')
	r := result{records: records}
	sync := func() error {
		t := time.Now()
		if err := f.Sync(); err != nil {
			return err
		}
		r.took = append(r.took, time.Since(t))
		return nil
	}
	start := time.Now()
	for i := 1; i <= records; i++ {
		if _, err := f.Write(record); err != nil {
			return r, err
		}
		if every > 0 && (i%every == 0 || i == records) {
			if err := sync(); err != nil {
				return r, err
			}
		}
	}
	r.elapsed = time.Since(start)
	slices.Sort(r.took)
	return r, nil
}

// measure appends to a new file in dir and removes it afterwards.
func measure(dir string, records, size, every int) (result, error) {
	f, err := os.CreateTemp(dir, "fsync-lab-*")
	if err != nil {
		return result{}, err
	}
	r, err := appendRecords(f, records, size, every)
	return r, errors.Join(err, f.Close(), os.Remove(f.Name()))
}

func main() {
	dir := flag.String("dir", os.TempDir(), "the directory of the file")
	records := flag.Int("records", 2000, "the records to append")
	size := flag.Int("size", 128, "the bytes of each record, the newline included")
	list := flag.String("every", "1,8,64,512,0", "sync after every n records, comma separated; 0 never syncs")
	flag.Parse()
	if *records < 1 || *size < 1 {
		fmt.Fprintln(os.Stderr, "fsync: -records and -size must be at least 1")
		os.Exit(2)
	}

	fmt.Printf("%d records of %d bytes in %s\n", *records, *size, *dir)
	fmt.Printf("%-7s %-12s %-7s %-10s %-10s %s\n", "every", "records/s", "syncs", "sync p50", "sync p99", "total")
	for f := range strings.SplitSeq(*list, ",") {
		every, err := strconv.Atoi(f)
		if err != nil || every < 0 {
			fmt.Fprintf(os.Stderr, "fsync: bad -every %q\n", f)
			os.Exit(2)
		}
		r, err := measure(*dir, *records, *size, every)
		if err != nil {
			fmt.Fprintln(os.Stderr, "fsync:", err)
			os.Exit(1)
		}
		name := strconv.Itoa(every)
		if every == 0 {
			name = "never"
		}
		fmt.Printf("%-7s %-12.0f %-7d %-10v %-10v %v\n", name, r.perSecond(), r.syncs(),
			r.percentile(0.50).Round(time.Microsecond), r.percentile(0.99).Round(time.Microsecond),
			r.elapsed.Round(time.Millisecond))
	}
}
