package umanagedagents

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Option adjusts NewClient. Options are applied after the environment settings.
type Option func(*options)

type options struct {
	caFile  string
	timeout time.Duration
}

// WithCAFile trusts only the CA certificates (PEM) in path, for an API behind a private CA. Certificate and host name
// verification stay on; there is no option to turn them off. Without it NewClient reads MANAGED_AGENTS_CA_FILE (then
// MVPP_CA_FILE); when neither is set the system roots are used.
func WithCAFile(path string) Option { return func(o *options) { o.caFile = path } }

// WithTimeout sets the limit of one HTTP request (default 30 s). It does not limit WaitTurn or Stream, which follow
// the caller's context.
func WithTimeout(d time.Duration) Option { return func(o *options) { o.timeout = d } }

func transportFor(caFile string) (*http.Transport, error) {
	base := http.DefaultTransport.(*http.Transport).Clone()
	if caFile == "" {
		return base, nil
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("CA file contains no PEM certificate")
	}
	base.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return base, nil
}

func noRedirect(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

// Stream subscribes to the Session's live events (GET /v1/sessions/{id}/events/stream) and yields each one as it
// arrives. Open it before sending an input so no event is missed. Heartbeat comments are skipped. The stream ends when
// ctx ends (the yielded error is then ctx.Err()) or when the server closes it (no error): the end of a stream is not
// the end of a turn — use WaitTurn or the session.status_idle event of your input for that. Closing the stream never
// stops the Session's work. After a disconnect, read the missed events from Events (the persisted history).
func (c *Client) Stream(ctx context.Context, sessionID string) iter.Seq2[SessionEvent, error] {
	return func(yield func(SessionEvent, error) bool) {
		target := c.API.GetConfig().Servers[0].URL + "/v1/sessions/" + url.PathEscape(sessionID) + "/events/stream"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			yield(SessionEvent{}, err)
			return
		}
		req.Header.Set("x-api-key", c.key)
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("User-Agent", c.API.GetConfig().UserAgent)
		resp, err := c.stream.Do(req)
		if err != nil {
			yield(SessionEvent{}, err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
			yield(SessionEvent{}, &GenericOpenAPIError{body: body, error: resp.Status})
			return
		}
		reader := bufio.NewReaderSize(resp.Body, 64<<10)
		var data []string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if ctx.Err() != nil {
					yield(SessionEvent{}, ctx.Err())
				} else if !errors.Is(err, io.EOF) {
					yield(SessionEvent{}, err)
				}
				return
			}
			line = strings.TrimRight(line, "\r\n")
			switch {
			case line == "":
				if len(data) == 0 {
					continue
				}
				var event SessionEvent
				err := json.Unmarshal([]byte(strings.Join(data, "\n")), &event)
				data = data[:0]
				if !yield(event, err) {
					return
				}
			case strings.HasPrefix(line, ":"):
				// heartbeat or comment
			case strings.HasPrefix(line, "data:"):
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
		}
	}
}

// EventType returns the "type" of any event variant (for example "agent.message" or "session.status_idle").
func EventType(event SessionEvent) string {
	head := eventHead(event)
	return head.Type
}

// EventID returns the "id" of any event variant.
func EventID(event SessionEvent) string { return eventHead(event).ID }

// EventInputID returns the "input_event_id" of any event variant ("" when null).
func EventInputID(event SessionEvent) string { return eventHead(event).InputEventID }

type head struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	InputEventID string `json:"input_event_id"`
}

func eventHead(event SessionEvent) head {
	raw, err := json.Marshal(event)
	var h head
	if err == nil {
		_ = json.Unmarshal(raw, &h)
	}
	return h
}

// UploadFile uploads a local file of up to 512 MiB through the chunked upload API (create → parts → complete) and
// returns its File. Parts are read one at a time; a failed part is retried up to twice (a part is idempotent).
func (c *Client) UploadFile(ctx context.Context, path string) (*ResponseFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return nil, err
	}
	sum := hex.EncodeToString(digest.Sum(nil))
	ctx = c.Context(ctx)
	body := NewRequestUploadCreate(filepath.Base(path), info.Size())
	body.Sha256 = &sum
	up, _, err := c.API.UploadsAPI.CreateUpload(ctx).RequestUploadCreate(*body).Execute()
	if err != nil {
		return nil, err
	}
	base := c.API.GetConfig().Servers[0].URL + "/v1/uploads/" + url.PathEscape(up.Id) + "/parts/"
	for n := int64(1); n <= up.PartCount; n++ {
		offset := (n - 1) * up.PartSize
		size := min(up.PartSize, info.Size()-offset)
		var last error
		for attempt := 0; attempt < 3; attempt++ {
			if last = c.putPart(ctx, base+strconv.FormatInt(n, 10), io.NewSectionReader(file, offset, size), size); last == nil {
				break
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}
		if last != nil {
			return nil, fmt.Errorf("part %d: %w", n, last)
		}
	}
	done, _, err := c.API.UploadsAPI.CompleteUpload(ctx, up.Id).Execute()
	return done, err
}

func (c *Client) putPart(ctx context.Context, target string, part *io.SectionReader, size int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, part)
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("x-api-key", c.key)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("User-Agent", c.API.GetConfig().UserAgent)
	resp, err := c.stream.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return &GenericOpenAPIError{body: body, error: resp.Status}
	}
	return nil
}
