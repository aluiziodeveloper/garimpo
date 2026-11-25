// Command table-tests cuts a text into chunks of at most -size words. The function is
// small; the lab is about its table test.
//
//	docker compose exec dev go run ./labs/table-tests -size 4 "a goroutine is a function running concurrently with other goroutines"
//	docker compose exec dev go test -v ./labs/table-tests
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
)

func main() {
	size := flag.Int("size", 4, "words per chunk")
	flag.Parse()

	chunks, err := chunkWords(strings.Join(flag.Args(), " "), *size)
	if err != nil {
		fmt.Fprintln(os.Stderr, "table-tests:", err)
		os.Exit(2)
	}
	for i, c := range chunks {
		fmt.Printf("%d: %s\n", i+1, c)
	}
}

// chunkWords splits text into words and joins them back in chunks of at most size
// words, with single spaces. The last chunk may be shorter.
func chunkWords(text string, size int) ([]string, error) {
	if size < 1 {
		return nil, errors.New("size must be at least 1")
	}
	var chunks []string
	for words := range slices.Chunk(strings.Fields(text), size) {
		chunks = append(chunks, strings.Join(words, " "))
	}
	return chunks, nil
}
