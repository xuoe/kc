package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh/terminal"
)

type ioError struct{ error }

func isTerminal(stream interface{}) bool {
	f, ok := stream.(*os.File)
	if !ok {
		return false
	}
	return terminal.IsTerminal(int(f.Fd()))
}

func isInteractive(stream interface{}) bool {
	f, ok := stream.(*os.File)
	if !ok {
		// Likely a buffer used during tests.
		return true
	}
	return terminal.IsTerminal(int(f.Fd()))
}

type screen interface {
	io.Writer
	clear()
}

func newSelectionScreen(r io.Reader, w io.Writer) screen {
	for _, fd := range []interface{}{r, w} {
		if f, ok := fd.(*os.File); !ok || !terminal.IsTerminal(int(f.Fd())) {
			return nopScreen{w}
		}
	}
	s := &selectionScreen{
		cursor:      newCursor(r, w),
		lineCounter: &lineCounter{w: w},
	}
	return s
}

type nopScreen struct {
	io.Writer
}

func (s nopScreen) Write(p []byte) (int, error) {
	return s.Writer.Write(p)
}

func (s nopScreen) clear() {}

var (
	escSetCursorPosition = "\x1b[%d;%dH"
	escCursorPosition    = []byte("\x1b[6n")
	escEraseTillBottom   = []byte("\x1b[J")
)

type selectionScreen struct {
	*lineCounter
	cursor *cursor
}

func (s *selectionScreen) clear() {
	l, c := s.cursor.line, s.cursor.col // previous line/col
	L, _ := s.cursor.position()
	if L == l && s.lines == 0 { // no lines were written in the meantime
		return
	}
	if s.lines == 0 {
		s.lines = -1
	}
	l = L - s.lines - 1
	s.lines = 0
	s.cursor.move(l, c)
	if _, err := s.Write(escEraseTillBottom); err != nil {
		panic(ioError{err})
	}
}

type cursor struct {
	io.Reader
	io.Writer
	line, col int
}

func newCursor(r io.Reader, w io.Writer) *cursor {
	c := &cursor{
		Reader: r,
		Writer: w,
	}
	c.line, c.col = c.position()
	return c
}

func (c *cursor) move(line, col int) {
	if _, err := fmt.Fprintf(c.Writer, escSetCursorPosition, line, col); err != nil {
		panic(ioError{err})
	}
	c.line, c.col = line, col
}

func (c *cursor) position() (line, col int) {
	if _, err := c.Write(escCursorPosition); err != nil {
		panic(ioError{err})
	}
	state, err := terminal.MakeRaw(0)
	if err != nil {
		panic(ioError{err})
	}
	defer terminal.Restore(0, state)

	var (
		x, y  []byte
		split bool
		buf   = make([]byte, 1)
	)
LOOP:
	for {
		switch n, err := c.Read(buf); err {
		case nil:
		case io.EOF:
			if n == 0 {
				break LOOP
			}
		default:
			panic(ioError{err})
		}
		b := buf[0]
		switch {
		case b == 0:
			panic(ioError{errors.New("NUL cursor position")})
		case b == 0x1b, b == '[':
			// skip
		case b == ';':
			split = true
		case b == 'R':
			break LOOP
		case !split:
			x = append(x, b)
		case split:
			y = append(y, b)
		}
		buf[0] = 0
	}
	line, _ = strconv.Atoi(string(x))
	col, _ = strconv.Atoi(string(y))
	return
}

func readRawByte(r io.Reader) byte {
	state, err := terminal.MakeRaw(0)
	if err != nil {
		panic(ioError{err})
	}
	defer terminal.Restore(0, state)
	return readByte(r)
}

func readByte(r io.Reader) byte {
	p := make([]byte, 1)
	switch _, err := r.Read(p); err {
	case nil, io.EOF:
	default:
		panic(ioError{err})
	}
	return p[0]
}

func readLine(r io.Reader) string {
	var buf strings.Builder
	p := make([]byte, 1)
LOOP:
	for {
		_, err := r.Read(p)
		switch err {
		case nil:
		case io.EOF:
			break LOOP
		default:
			panic(ioError{err})
		}
		switch p[0] {
		case '\n', '\r':
			break LOOP
		default:
			buf.Write(p)
		}
	}
	return buf.String()
}

type lineCounter struct {
	w            io.Writer
	lines        int
	bytes        int
	hasEmptyLine bool
}

var newline = []byte{'\n'}

func (w *lineCounter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.bytes += n
	w.lines += bytes.Count(p[:n], newline)
	w.hasEmptyLine = len(p) == 1 && p[0] == '\n'
	return n, err
}
