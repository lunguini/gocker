package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lunguini/gocker/internal/termx"
)

// Logger provides rolling terminal display and file-based request logging.
type Logger struct {
	mu      sync.Mutex
	lines   []string
	maxShow int
	file    *os.File
	isTTY   bool
}

// NewLogger creates a logger that shows the last maxShow lines on the terminal
// and writes all entries to logFile. Pass "" for logFile to skip file logging.
func NewLogger(maxShow int, logFile string) (*Logger, error) {
	l := &Logger{maxShow: maxShow, isTTY: termx.StderrIsTTY()}
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		l.file = f
	}
	return l, nil
}

func (l *Logger) Close() {
	if l.file != nil {
		_ = l.file.Close()
	}
}

func (l *Logger) Log(line string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.lines = append(l.lines, line)
	if len(l.lines) > l.maxShow {
		l.lines = l.lines[len(l.lines)-l.maxShow:]
	}

	if l.file != nil {
		_, _ = fmt.Fprintln(l.file, line)
	}

	l.render()
}

func (l *Logger) render() {
	// When stderr isn't a terminal (daemon under launchd, or output
	// redirected to a file), ANSI cursor-movement codes would just fill the
	// log with escape sequences. Append the newest line plainly instead.
	if !l.isTTY {
		if n := len(l.lines); n > 0 {
			fmt.Fprintln(os.Stderr, l.lines[n-1])
		}
		return
	}
	// Move cursor up to clear previous display, then rewrite.
	// On first call there's nothing to clear, but ANSI codes handle that gracefully.
	n := min(len(l.lines), l.maxShow)

	var b strings.Builder
	// Move up and clear previous lines
	for range n {
		b.WriteString("\033[A\033[2K")
	}
	// Write current lines
	for _, line := range l.lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	fmt.Fprint(os.Stderr, b.String())
}

// loggingMiddleware wraps an http.Handler with request/response logging.
func loggingMiddleware(next http.Handler, logger *Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lw, r)
		dur := time.Since(start)
		path := r.URL.Path
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		line := fmt.Sprintf("%s %s %s %d %s",
			time.Now().Format("15:04:05"),
			r.Method, path, lw.status, formatDuration(dur))
		if lw.status >= 400 && len(lw.body) > 0 {
			errMsg := strings.TrimSpace(string(lw.body))
			if len(errMsg) > 200 {
				errMsg = errMsg[:200]
			}
			line += " " + errMsg
		}
		logger.Log(line)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	body   []byte
}

func (w *loggingResponseWriter) Write(b []byte) (int, error) {
	if w.status >= 400 {
		w.body = append(w.body, b...)
	}
	return w.ResponseWriter.Write(b)
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Unwrap allows http.ResponseController to access the underlying writer
// (needed for flushing, hijacking, etc.).
func (w *loggingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
}

// SetLogger attaches a logger to the server for request logging.
func (s *Server) SetLogger(logger *Logger) {
	s.logger = logger
}

// LogWriter returns an io.Writer that sends each write to the logger.
// Useful for redirecting runtime output (e.g., image pull progress).
func (l *Logger) LogWriter(prefix string) io.Writer {
	return &logWriter{logger: l, prefix: prefix}
}

type logWriter struct {
	logger *Logger
	prefix string
}

func (w *logWriter) Write(p []byte) (int, error) {
	for line := range strings.SplitSeq(strings.TrimRight(string(p), "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			w.logger.Log(fmt.Sprintf("%s %s%s",
				time.Now().Format("15:04:05"), w.prefix, line))
		}
	}
	return len(p), nil
}
