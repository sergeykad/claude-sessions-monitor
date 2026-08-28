package session

import (
	"bytes"
	"errors"
)

// lsofNameColumn is the 1-based index of lsof's NAME column, which holds the
// path. COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE NAME.
const lsofNameColumn = 9

// parseLsofCwd pulls the working directory out of `lsof -p <pid>` output.
//
// NAME is the last column, so the path is the whole rest of the line. Reading
// it as the last whitespace-separated field instead turns a directory called
// "/Users/x/My Project" into "Project", and csm then keys that session under a
// project that does not exist.
//
// This lives outside the darwin file so it can be tested anywhere, since lsof
// itself cannot be run in a test.
func parseLsofCwd(out []byte) (string, error) {
	for _, line := range bytes.Split(out, []byte("\n")) {
		if !bytes.Contains(line, []byte(" cwd ")) {
			continue
		}
		rest := bytes.TrimLeft(line, " \t")
		for i := 1; i < lsofNameColumn && rest != nil; i++ {
			gap := bytes.IndexAny(rest, " \t")
			if gap < 0 {
				rest = nil
				break
			}
			rest = bytes.TrimLeft(rest[gap:], " \t")
		}
		if path := bytes.TrimRight(rest, " \t\r"); len(path) > 0 {
			return string(path), nil
		}
	}
	return "", errors.New("no cwd line in lsof output")
}
