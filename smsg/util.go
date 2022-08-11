// SPDX-License-Identifier: Apache-2.0
package main

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"math/bits"
	"os"
	"strings"
)

type HashingCountingReader struct {
	io.Reader
	nread int
	hash  hash.Hash
}

func MakeSHA256HashingCountingReader(r io.Reader) HashingCountingReader {
	return HashingCountingReader{
		Reader: r,
		hash:   sha256.New(),
	}
}

func (r *HashingCountingReader) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)
	if err == nil {
		r.nread += n
		r.hash.Write(p[:n])
	}
	return
}

func ilog2(n uint64) int {
	if n <= 1 {
		return 1
	}
	return (64 - bits.LeadingZeros64(n)) - 1
}

// create error
func errorf(format string, arg ...interface{}) error {
	return fmt.Errorf(format, arg...)
}

// log error and exit
func fatalf(msg interface{}, arg ...interface{}) {
	var format string
	if s, ok := msg.(string); ok {
		format = s
	} else if s, ok := msg.(fmt.Stringer); ok {
		format = s.String()
	} else {
		format = fmt.Sprintf("%v", msg)
	}
	fmt.Fprintf(os.Stderr, format+"\n", arg...)
	os.Exit(1)
}

// isdir returns nil if path is a directory, or an error describing the issue
func isdir(path string) error {
	finfo, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !finfo.IsDir() {
		return errorf("%q is not a directory", path)
	}
	return nil
}

func plural(n int, one, other string) string {
	if n == 1 {
		return one
	}
	return other
}

func countByte(data []byte, subject byte) (count uint) {
	for _, b := range data {
		if b == subject {
			count++
		}
	}
	return
}

func imin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func imax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// relPath returns a relative name of path rooted in dir.
// If path is outside dir path is returned verbatim.
// path is assumed to be absolute.
func relPath(dir string, path string) string {
	if len(path) > len(dir) && path[len(dir)] == os.PathSeparator && strings.HasPrefix(path, dir) {
		return path[len(dir)+1:]
	} else if path == dir {
		return "."
	}
	return path
}

func isDotFilename(filename string) bool {
	i := strings.LastIndexByte(filename, '/') + 1
	if i >= len(filename) {
		return false
	}
	return filename[i] == '.'
}
