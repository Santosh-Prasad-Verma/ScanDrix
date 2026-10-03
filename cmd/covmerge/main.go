// Command covmerge merges Go coverage profiles into one.
//
// # Why this exists
//
// `go test -coverprofile` only instruments the package a test binary is built
// from. A test in an external `_test` package therefore drives code in the
// sibling package, and those statements land nowhere: the counters are compiled
// into the test binary but the profile only records blocks of the package under
// test.
//
// This repository uses external test packages almost everywhere, so the effect
// was not a rounding error. Every licensing controller reported 0.0% despite
// hundreds of passing tests exercising it, which failed the Codecov patch gate
// for a diff that was in fact well covered.
//
// The fix is the standard two-pass one: run the suite normally, then run it a
// second time with `-coverpkg` pointing at the package each external test
// drives, and merge. Passing `-coverpkg=./...` in a single pass does not work
// here: with several hundred packages every test binary instruments the entire
// module, and the resulting profile is truncated and non-reproducible.
//
// Merging is the sum of execution counts per (file, block-range). Two blocks
// with the same file and range are the same code measured by two different test
// binaries, so their counts add.
package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: covmerge <profile> [profile...]")
		os.Exit(2)
	}

	mode := "atomic"
	counts := make(map[string]uint64)

	for _, path := range os.Args[1:] {
		// #nosec G703 -- the arguments are the profile paths this tool was
		// invoked with by the CI job, not untrusted input; the tool is not a
		// long-running service and reads nothing but Go coverage profiles.
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "covmerge: %v\n", err)
			os.Exit(1)
		}

		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "mode:") {
				mode = strings.TrimSpace(strings.TrimPrefix(line, "mode:"))
				continue
			}
			// "<file>:<start>.<end> <stmts> <count>"
			i := strings.LastIndexByte(line, ' ')
			if i < 0 {
				continue
			}
			block := line[:i]
			n, err := strconv.ParseUint(strings.TrimSpace(line[i+1:]), 10, 64)
			if err != nil {
				continue
			}
			counts[block] += n
		}
		if err := sc.Err(); err != nil {
			f.Close()
			fmt.Fprintf(os.Stderr, "covmerge: reading %s: %v\n", path, err)
			os.Exit(1)
		}
		f.Close()
	}

	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	// Sorted output keeps the profile diffable between runs.
	sort.Strings(keys)

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	fmt.Fprintf(out, "mode: %s\n", mode)
	for _, k := range keys {
		fmt.Fprintf(out, "%s %d\n", k, counts[k])
	}
}
