package connectors

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/http2"

	"github.com/mydisha/keirouter/backend/internal/core"
)

// Cursor's agent.v1 protocol is a Connect-streaming RPC over HTTP/2: the client
// opens a single bidirectional stream to /agent.v1.AgentService/Run and
// exchanges length-prefixed protobuf frames in both directions for the lifetime
// of one turn. Each frame is a 5-byte header (1 flag byte + 4-byte big-endian
// payload length) followed by the payload. The server may set the end-stream
// flag on a trailing JSON frame to signal an error.

// connectEndStreamFlag marks a Connect end-of-stream frame whose payload is a
// JSON object (optionally carrying {"error":{code,message}}).
const connectEndStreamFlag = 0b00000010

// cursorFrameWriter serializes Connect frames onto the request body. Writes are
// mutex-guarded because heartbeats and handshake replies are emitted from
// separate goroutines than the initial request frame.
type cursorFrameWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (f *cursorFrameWriter) writeFrame(payload []byte, flags byte) error {
	header := make([]byte, 5)
	header[0] = flags
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.w.Write(header); err != nil {
		return err
	}
	if _, err := f.w.Write(payload); err != nil {
		return err
	}
	if fl, ok := f.w.(http.Flusher); ok {
		fl.Flush()
	}
	return nil
}

// writeMessage frames and writes a normal (flags=0) protobuf message.
func (f *cursorFrameWriter) writeMessage(payload []byte) error {
	return f.writeFrame(payload, 0)
}

// cursorFrame is one decoded inbound Connect frame.
type cursorFrame struct {
	flags   byte
	payload []byte
}

// endStream reports whether this frame carries the Connect end-stream flag.
func (fr cursorFrame) endStream() bool { return fr.flags&connectEndStreamFlag != 0 }

// cursorFrameReader reads length-prefixed Connect frames from the response body.
type cursorFrameReader struct {
	r *bufio.Reader
}

func newCursorFrameReader(r io.Reader) *cursorFrameReader {
	return &cursorFrameReader{r: bufio.NewReaderSize(r, 64*1024)}
}

// next reads the next frame, blocking until a full frame is available. It
// returns io.EOF when the stream is exhausted.
func (cr *cursorFrameReader) next() (cursorFrame, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(cr.r, header); err != nil {
		return cursorFrame{}, err
	}
	length := binary.BigEndian.Uint32(header[1:])
	if length > maxResponseBodyBytes {
		return cursorFrame{}, errors.New("cursor: frame exceeds max size")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(cr.r, payload); err != nil {
		return cursorFrame{}, err
	}
	return cursorFrame{flags: header[0], payload: payload}, nil
}

// parseConnectEndStreamError decodes an end-stream frame payload. It returns a
// non-nil error only when the payload carries an {"error":...} object.
func parseConnectEndStreamError(payload []byte) *cursorErr {
	if len(payload) == 0 {
		return nil
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(payload, &env) != nil {
		return nil
	}
	if env.Error.Code == "" && env.Error.Message == "" {
		return nil
	}
	kind := core.ErrUpstream
	switch env.Error.Code {
	case "resource_exhausted":
		kind = core.ErrRateLimit
	case "unauthenticated", "permission_denied":
		kind = core.ErrAuth
	case "invalid_argument":
		kind = core.ErrBadRequest
	}
	msg := env.Error.Message
	if msg == "" {
		msg = env.Error.Code
	}
	return &cursorErr{kind: kind, message: msg}
}

// cursorStream is an open bidirectional Connect stream: a writer for client
// frames, a reader for server frames, and the resources to tear it down.
type cursorStream struct {
	writer   *cursorFrameWriter
	reader   *cursorFrameReader
	resp     *http.Response
	bodyPipe *io.PipeWriter
	cancel   context.CancelFunc
}

// close tears down the stream, ending the request body and releasing the
// response body and underlying connection.
func (s *cursorStream) close() {
	if s.bodyPipe != nil {
		_ = s.bodyPipe.Close()
	}
	if s.resp != nil && s.resp.Body != nil {
		_ = s.resp.Body.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

// cursorHTTP2Transport is a lazily-initialized HTTP/2 transport shared across
// Cursor requests. Cursor's endpoint speaks h2 over TLS; a dedicated transport
// keeps the bidi semantics independent of the shared HTTP/1-oriented client.
var (
	cursorH2Once      sync.Once
	cursorH2Transport *http2.Transport
)

func cursorTransport() *http2.Transport {
	cursorH2Once.Do(func() {
		cursorH2Transport = &http2.Transport{
			TLSClientConfig: &tls.Config{NextProtos: []string{"h2"}},
			ReadIdleTimeout: 30 * time.Second,
			PingTimeout:     15 * time.Second,
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				d := &tls.Dialer{Config: cfg}
				conn, err := d.DialContext(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				return conn, nil
			},
		}
	})
	return cursorH2Transport
}

// openCursorStream dials the Run RPC and returns an open bidi stream. The
// initial request frame must be written by the caller via stream.writer.
func openCursorStream(ctx context.Context, endpoint string, headers map[string]string) (*cursorStream, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	pr, pw := io.Pipe()

	req, err := http.NewRequestWithContext(streamCtx, http.MethodPost, endpoint, pr)
	if err != nil {
		cancel()
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	transport := cursorTransport()
	resp, err := transport.RoundTrip(req)
	if err != nil {
		_ = pw.Close()
		cancel()
		return nil, err
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		_ = resp.Body.Close()
		_ = pw.Close()
		cancel()
		return nil, httpStatusError("cursor", "", resp, body)
	}

	return &cursorStream{
		writer:   &cursorFrameWriter{w: pw},
		reader:   newCursorFrameReader(resp.Body),
		resp:     resp,
		bodyPipe: pw,
		cancel:   cancel,
	}, nil
}

// cursorEndpoint builds the Run RPC URL from a base URL.
func cursorEndpoint(base string) string {
	u, err := url.Parse(base)
	if err != nil {
		return joinURL(base, "agent.v1.AgentService/Run")
	}
	u.Path = "/agent.v1.AgentService/Run"
	u.RawQuery = ""
	return u.String()
}
