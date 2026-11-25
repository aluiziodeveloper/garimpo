// Command maxprocs shows how many CPUs a Go program sees inside a container and what
// happens when a worker pool is sized by the machine instead of by the container.
//
//	docker compose run --rm lab go run ./labs/maxprocs
//	docker compose run --rm lab go run ./labs/maxprocs -pool numcpu
//	docker compose run --rm -e GODEBUG=containermaxprocs=0 lab go run ./labs/maxprocs -jobs 4000
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The cgroup v2 CPU files of the current container: accounting and quota.
const (
	cpuStatPath = "/sys/fs/cgroup/cpu.stat"
	cpuMaxPath  = "/sys/fs/cgroup/cpu.max"
)

func main() {
	pool := flag.String("pool", "gomaxprocs", "how to size the pool: gomaxprocs or numcpu")
	jobs := flag.Int("jobs", 400, "CPU-bound jobs to run")
	flag.Parse()

	if err := run(os.Stdout, *pool, *jobs); err != nil {
		fmt.Fprintln(os.Stderr, "maxprocs:", err)
		os.Exit(1)
	}
}

func run(w io.Writer, pool string, jobs int) error {
	var n int
	switch pool {
	case "gomaxprocs":
		n = workers()
	case "numcpu":
		n = runtime.NumCPU() // INC-01: the machine's CPUs, not the container's.
	default:
		return fmt.Errorf("unknown pool %q (want gomaxprocs or numcpu)", pool)
	}

	if _, err := fmt.Fprintf(w, "NumCPU: %d  GOMAXPROCS: %d  pool: %s = %d workers\n",
		runtime.NumCPU(), runtime.GOMAXPROCS(0), pool, n); err != nil {
		return err
	}
	if err := printQuota(w, cpuMaxPath); err != nil {
		return err
	}

	before, statErr := readThrottled(cpuStatPath)
	start := time.Now()
	burn(n, jobs)
	elapsed := time.Since(start)
	after, _ := readThrottled(cpuStatPath)

	if _, err := fmt.Fprintf(w, "%d jobs in %v\n", jobs, elapsed.Round(time.Millisecond)); err != nil {
		return err
	}
	if statErr != nil {
		_, err := fmt.Fprintf(w, "throttling: not available (%v)\n", statErr)
		return err
	}
	_, err := fmt.Fprintf(w, "throttled: %d periods, %v summed over CPUs\n",
		after.periods-before.periods, (after.time - before.time).Round(time.Millisecond))
	return err
}

// workers is how many CPU-bound goroutines to start: as many as the scheduler can run
// at once. Since Go 1.25, GOMAXPROCS already follows the container's CPU limit, unless
// GODEBUG=containermaxprocs=0 or the main module's go.mod says go 1.24 or older.
func workers() int {
	return runtime.GOMAXPROCS(0)
}

// burn runs jobs CPU-bound jobs on n goroutines.
func burn(n, jobs int) {
	queue := make(chan int)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			for seed := range queue {
				hashChain(seed)
			}
		})
	}
	for i := range jobs {
		queue <- i
	}
	close(queue)
	wg.Wait()
}

// hashChain is a fixed amount of pure CPU work.
func hashChain(seed int) [32]byte {
	sum := sha256.Sum256([]byte(strconv.Itoa(seed)))
	for range 20_000 {
		sum = sha256.Sum256(sum[:])
	}
	return sum
}

// printQuota prints the container's CPU quota, the number Go 1.25+ uses for GOMAXPROCS.
func printQuota(w io.Writer, path string) error {
	cpus, limited, err := readQuota(path)
	switch {
	case err != nil:
		_, err = fmt.Fprintf(w, "quota: not available (%v)\n", err)
	case limited:
		_, err = fmt.Fprintf(w, "quota: %g CPUs\n", cpus)
	default:
		_, err = fmt.Fprintln(w, "quota: none")
	}
	return err
}

// readQuota reads a cgroup v2 cpu.max file, "<quota> <period>" in microseconds, and returns
// the quota in CPUs. A quota of "max" means no limit: limited is false.
func readQuota(path string) (cpus float64, limited bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false, err
	}
	quota, period, ok := strings.Cut(strings.TrimSpace(string(data)), " ")
	if !ok {
		return 0, false, fmt.Errorf("%s: want \"<quota> <period>\", got %q", path, data)
	}
	if quota == "max" {
		return 0, false, nil
	}
	q, err := strconv.ParseInt(quota, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("%s: quota: %w", path, err)
	}
	p, err := strconv.ParseInt(period, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("%s: period: %w", path, err)
	}
	if p <= 0 {
		return 0, false, fmt.Errorf("%s: period %d is not positive", path, p)
	}
	return float64(q) / float64(p), true, nil
}

type throttled struct {
	periods int64
	time    time.Duration
}

// readThrottled reads nr_throttled and throttled_usec from a cgroup v2 cpu.stat file.
func readThrottled(path string) (throttled, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return throttled{}, err
	}

	var t throttled
	for line := range strings.Lines(string(data)) {
		key, value, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "nr_throttled":
			t.periods = v
		case "throttled_usec":
			t.time = time.Duration(v) * time.Microsecond
		}
	}
	return t, nil
}
