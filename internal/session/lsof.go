package session

import (
	"bytes"
	"errors"
)

// lsofFieldsBeforeName is how many whitespace-free columns come before NAME:
// COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE.
const lsofFieldsBeforeName = 8

// parseLsofCwd pulls the working directory out of `lsof -p <pid>` output.
//
// NAME is the last column, so the path is the whole rest of the line. Reading
// it as the last whitespace-separated field instead turns a directory called
// "/Users/x/My Project" into "Project", and csm then keys that session under a
// project that does not exist. Splitting on whitespace and rejoining is the
// same bug in another form: it collapses runs of spaces inside the path.
//
// This lives outside the darwin file so it can be tested anywhere, since lsof
// itself cannot be run in a test.
func parseLsofCwd(out []byte) (string, error) {
	for _, line := range bytes.Split(out, []byte("\n")) {
		if !bytes.Contains(line, []byte(" cwd ")) {
			continue
		}
		if path := fieldTail(line, lsofFieldsBeforeName); len(path) > 0 {
			return string(path), nil
		}
	}
	return "", errors.New("no cwd line in lsof output")
}

// fieldTail returns what is left of a line after skipping that many
// whitespace-separated fields, with the surrounding whitespace trimmed. It
// returns nil when the line runs out first.
func fieldTail(line []byte, skip int) []byte {
	rest := bytes.TrimLeft(line, " \t")
	for range skip {
		gap := bytes.IndexAny(rest, " \t")
		if gap < 0 {
			return nil
		}
		rest = bytes.TrimLeft(rest[gap:], " \t")
	}
	return bytes.TrimRight(rest, " \t\r")
}
