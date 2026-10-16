package graph

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"adsctl/internal/output"
	"adsctl/internal/state"
)

func videoFixture(t *testing.T, handler http.HandlerFunc) (*Client, string, VideoUploadOptions) {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	c := New("credential-fixture", "v25.0")
	c.BaseURL = s.URL
	c.Backoff = time.Millisecond
	file := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(file, []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	return c, file, VideoUploadOptions{Profile: "write", StateDir: t.TempDir(), ID: "upload_fixture"}
}

func readTransfer(t *testing.T, r *http.Request) (string, string) {
	t.Helper()
	if err := r.ParseMultipartForm(1024); err != nil {
		t.Error(err)
		return "", ""
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	f, _, err := r.FormFile("video_file_chunk")
	if err != nil {
		t.Error(err)
		return "", ""
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Error(err)
	}
	if r.FormValue("upload_phase") != "transfer" || r.FormValue("upload_session_id") != "123" {
		t.Error("wrong session", r.Form)
	}
	return r.FormValue("start_offset"), string(b)
}

func TestVideoUploadOffsetsCorrectionAndPrivateCheckpoint(t *testing.T) {
	transfers, starts, finishes := 0, 0, 0
	c, file, opt := videoFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer credential-fixture" || r.URL.Path != "/v25.0/act_456/advideos" || r.URL.RawQuery != "" {
			t.Error("scope/auth")
		}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			offset, b := readTransfer(t, r)
			transfers++
			switch transfers {
			case 1:
				if offset != "0" || b != "0123" {
					t.Error(offset, b)
				}
				_, _ = fmt.Fprint(w, `{"start_offset":"4","end_offset":"7"}`)
			case 2:
				if offset != "4" || b != "456" {
					t.Error(offset, b)
				}
				w.WriteHeader(400)
				_, _ = fmt.Fprint(w, `{"error":{"code":100,"error_subcode":1363037,"error_data":{"start_offset":"6","end_offset":"10"},"message":"offset correction"}}`)
			case 3:
				if offset != "6" || b != "6789" {
					t.Error(offset, b)
				}
				_, _ = fmt.Fprint(w, `{"start_offset":"10","end_offset":"10"}`)
			default:
				t.Error("too many transfers")
			}
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		switch r.Form.Get("upload_phase") {
		case "start":
			starts++
			if r.Form.Get("file_size") != "10" {
				t.Error(r.Form)
			}
			_, _ = fmt.Fprint(w, `{"upload_session_id":"123","video_id":"789","start_offset":"0","end_offset":"4"}`)
		case "finish":
			finishes++
			if r.Form.Get("upload_session_id") != "123" || r.Form.Get("published") != "" {
				t.Error(r.Form)
			}
			_, _ = fmt.Fprint(w, `{"success":true}`)
		default:
			t.Error(r.Form)
		}
	})
	response, err := c.UploadVideo(context.Background(), "act_456", file, opt)
	if err != nil || response.Value["id"] != "789" {
		t.Fatal(err, response)
	}
	opt.Resume = true
	if _, err = c.UploadVideo(context.Background(), "act_456", file, opt); err != nil {
		t.Fatal(err)
	}
	if starts != 1 || finishes != 1 || transfers != 3 {
		t.Fatal(starts, finishes, transfers)
	}
	cpPath, _ := state.Path(opt.StateDir, opt.ID)
	b, _ := os.ReadFile(cpPath)
	info, _ := os.Stat(cpPath)
	if strings.Contains(string(b), c.Token) || strings.Contains(string(b), "access_token") || info.Mode().Perm() != 0600 {
		t.Fatal("checkpoint is not private")
	}
}

func TestVideoUploadInterruptedResumeAndIdentity(t *testing.T) {
	starts, transfers, finishes := 0, 0, 0
	blocked := true
	c, file, opt := videoFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			offset, b := readTransfer(t, r)
			transfers++
			if offset == "0" {
				_, _ = fmt.Fprint(w, `{"start_offset":"4","end_offset":"10"}`)
				return
			}
			if offset != "4" || b != "456789" {
				t.Error(offset, b)
			}
			if blocked {
				w.WriteHeader(400)
				_, _ = fmt.Fprint(w, `{"error":{"code":100,"message":"fixture interruption"}}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"start_offset":"10","end_offset":"10"}`)
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("upload_phase") == "start" {
			starts++
			_, _ = fmt.Fprint(w, `{"upload_session_id":"123","video_id":"789","start_offset":"0","end_offset":"4"}`)
		} else {
			finishes++
			_, _ = fmt.Fprint(w, `{"success":true}`)
		}
	})
	if _, err := c.UploadVideo(context.Background(), "act_456", file, opt); err == nil {
		t.Fatal("wanted interruption")
	}
	opt.Resume = true
	for _, scope := range []string{"profile", "account", "version", "endpoint", "file"} {
		t.Run(scope, func(t *testing.T) {
			changed := opt
			account, oldVersion, oldURL := "act_456", c.Version, c.BaseURL
			switch scope {
			case "profile":
				changed.Profile = "other"
			case "account":
				account = "act_999"
			case "version":
				c.Version = "v24.0"
			case "endpoint":
				c.BaseURL += "/other"
			case "file":
				if err := os.WriteFile(file, []byte("changed!!!"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := c.UploadVideo(context.Background(), account, file, changed)
			c.Version, c.BaseURL = oldVersion, oldURL
			if scope == "file" {
				if e := os.WriteFile(file, []byte("0123456789"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if err == nil || (output.AsError(err).Code != "UPLOAD_SCOPE" && output.AsError(err).Code != "ASSET_CHANGED") {
				t.Fatal(err)
			}
		})
	}
	if starts != 1 || transfers != 2 {
		t.Fatal("identity rejection made remote calls")
	}
	blocked = false
	response, err := c.UploadVideo(context.Background(), "act_456", file, opt)
	if err != nil || response.Value["id"] != "789" || starts != 1 || transfers != 3 || finishes != 1 {
		t.Fatal(err, response, starts, transfers, finishes)
	}
}

func TestVideoUploadAmbiguousStartAndFinish(t *testing.T) {
	for _, phase := range []string{"start", "finish"} {
		t.Run(phase, func(t *testing.T) {
			starts, finishes, gets := 0, 0, 0
			ready := false
			c, file, opt := videoFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					gets++
					if ready {
						_, _ = fmt.Fprint(w, `{"id":"789","status":{"video_status":"ready"}}`)
					} else {
						_, _ = fmt.Fprint(w, `{"id":"789","status":{"video_status":"uploading"}}`)
					}
					return
				}
				if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
					_, _ = readTransfer(t, r)
					_, _ = fmt.Fprint(w, `{"start_offset":"10","end_offset":"10"}`)
					return
				}
				_ = r.ParseForm()
				if r.Form.Get("upload_phase") == "start" {
					starts++
					if phase != "start" {
						_, _ = fmt.Fprint(w, `{"upload_session_id":"123","video_id":"789","start_offset":"0","end_offset":"10"}`)
						return
					}
				} else {
					finishes++
				}
				w.WriteHeader(502)
				_, _ = fmt.Fprint(w, "unreadable credential-fixture body")
			})
			response, err := c.UploadVideo(context.Background(), "act_456", file, opt)
			if err == nil || output.AsError(err).Exit != 10 || output.AsError(err).HTTPStatus != 502 {
				t.Fatal(err, response)
			}
			opt.Resume = true
			if _, err = c.UploadVideo(context.Background(), "act_456", file, opt); err == nil || output.AsError(err).Exit != 10 {
				t.Fatal(err)
			}
			ready = true
			response, err = c.UploadVideo(context.Background(), "act_456", file, opt)
			if phase == "finish" {
				if err != nil || response.Value["id"] != "789" || finishes != 1 || gets != 2 {
					t.Fatal(err, response, finishes, gets)
				}
			} else if err == nil || finishes != 0 || gets != 0 {
				t.Fatal(err, finishes, gets)
			}
			if starts != 1 {
				t.Fatal("duplicated start")
			}
		})
	}
}

func TestVideoUploadAmbiguousTransferRecoveryIsBounded(t *testing.T) {
	transfers, starts := 0, 0
	recover := false
	c, file, opt := videoFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			offset, b := readTransfer(t, r)
			if offset != "0" || b != "0123456789" {
				t.Error(offset, b)
			}
			transfers++
			if recover {
				_, _ = fmt.Fprint(w, `{"start_offset":"10","end_offset":"10"}`)
			} else {
				w.WriteHeader(503)
			}
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("upload_phase") == "start" {
			starts++
			_, _ = fmt.Fprint(w, `{"upload_session_id":"123","video_id":"789","start_offset":"0","end_offset":"10"}`)
		} else {
			_, _ = fmt.Fprint(w, `{"success":true}`)
		}
	})
	if _, err := c.UploadVideo(context.Background(), "act_456", file, opt); err == nil || output.AsError(err).Exit != 10 || transfers != 4 {
		t.Fatal(err, transfers)
	}
	recover, opt.Resume = true, true
	if _, err := c.UploadVideo(context.Background(), "act_456", file, opt); err != nil || starts != 1 {
		t.Fatal(err, starts)
	}
}

func TestVideoUploadLargeFileStreamsServerRanges(t *testing.T) {
	const size int64 = 106906451
	var received int64
	c, file, opt := videoFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			reader, err := r.MultipartReader()
			if err != nil {
				t.Error(err)
				return
			}
			var chunk int64
			for {
				part, e := reader.NextPart()
				if errors.Is(e, io.EOF) {
					break
				}
				if e != nil {
					t.Error(e)
					return
				}
				if part.FormName() == "video_file_chunk" {
					chunk, e = io.Copy(io.Discard, part)
					if e != nil {
						t.Error(e)
					}
				}
			}
			if chunk != min(4<<20, size-received) {
				t.Error("wrong range size", chunk, received)
			}
			received += chunk
			_, _ = fmt.Fprintf(w, `{"start_offset":"%d","end_offset":"%d"}`, received, min(received+4<<20, size))
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("upload_phase") == "start" {
			_, _ = fmt.Fprint(w, `{"upload_session_id":"123","video_id":"789","start_offset":"0","end_offset":"4194304"}`)
		} else {
			_, _ = fmt.Fprint(w, `{"success":true}`)
		}
	})
	if err := os.Truncate(file, size); err != nil {
		t.Fatal(err)
	}
	response, err := c.UploadVideo(context.Background(), "act_456", file, opt)
	if err != nil || received != size || response.Value["id"] != "789" {
		t.Fatal(err, received, response)
	}
}

func TestVideoUploadRequestDeadlineAndCancellation(t *testing.T) {
	var calls atomic.Int32
	c, file, opt := videoFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	})
	c.RequestTimeout = 20 * time.Millisecond
	if _, err := c.UploadVideo(context.Background(), "act_456", file, opt); err == nil || output.AsError(err).Exit != 10 {
		t.Fatal(err)
	}
	opt.Resume = true
	if _, err := c.UploadVideo(context.Background(), "act_456", file, opt); err == nil || calls.Load() != 1 {
		t.Fatal(err, calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.UploadVideo(ctx, "act_456", file, opt); err == nil || output.AsError(err).Exit != 7 {
		t.Fatal(err)
	}
}

func TestVideoUploadRejectsInvalidOffsetsAndStalls(t *testing.T) {
	for _, scenario := range []string{"invalid-start", "invalid-transfer", "stalled"} {
		t.Run(scenario, func(t *testing.T) {
			transfers, finishes := 0, 0
			c, file, opt := videoFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
					_, _ = readTransfer(t, r)
					transfers++
					if scenario == "invalid-transfer" {
						_, _ = fmt.Fprint(w, `{"start_offset":"-1","end_offset":"20"}`)
					} else {
						_, _ = fmt.Fprint(w, `{"start_offset":"0","end_offset":"4"}`)
					}
					return
				}
				_ = r.ParseForm()
				if r.Form.Get("upload_phase") == "start" {
					if scenario == "invalid-start" {
						_, _ = fmt.Fprint(w, `{"upload_session_id":"123","video_id":"789","start_offset":"0","end_offset":"20"}`)
					} else {
						_, _ = fmt.Fprint(w, `{"upload_session_id":"123","video_id":"789","start_offset":"0","end_offset":"4"}`)
					}
				} else {
					finishes++
					_, _ = fmt.Fprint(w, `{"success":true}`)
				}
			})
			if _, err := c.UploadVideo(context.Background(), "act_456", file, opt); err == nil {
				t.Fatal("expected bounded failure")
			}
			if finishes != 0 || transfers > 4 {
				t.Fatal(finishes, transfers)
			}
		})
	}
}

func TestVideoUploadTotalDeadlinePreservesTransfer(t *testing.T) {
	var starts, transfers, finishes atomic.Int32
	c, file, opt := videoFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			_, _ = readTransfer(t, r)
			transfers.Add(1)
			select {
			case <-r.Context().Done():
			case <-time.After(200 * time.Millisecond):
			}
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("upload_phase") == "start" {
			starts.Add(1)
			_, _ = fmt.Fprint(w, `{"upload_session_id":"123","video_id":"789","start_offset":"0","end_offset":"10"}`)
		} else {
			finishes.Add(1)
		}
	})
	c.RequestTimeout = time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.UploadVideo(ctx, "act_456", file, opt); err == nil || output.AsError(err).Exit != 10 {
		t.Fatal(err)
	}
	path, _ := state.Path(opt.StateDir, opt.ID)
	var cp VideoCheckpoint
	if err := state.Read(path, &cp); err != nil {
		t.Fatal(err)
	}
	if cp.Phase != "transfer" || cp.VideoID != "789" || starts.Load() != 1 || transfers.Load() != 1 || finishes.Load() != 0 {
		t.Fatal(cp)
	}
}
