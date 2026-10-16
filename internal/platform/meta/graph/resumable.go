package graph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"time"

	"adsctl/internal/config"
	"adsctl/internal/output"
	"adsctl/internal/state"
)

const MaxUploadSize int64 = 1 << 30

// VideoCheckpoint contains identity and protocol state, never credentials.
type VideoCheckpoint struct {
	Version    int    `json:"version"`
	Profile    string `json:"profile"`
	Account    string `json:"account_id"`
	APIVersion string `json:"api_version"`
	Endpoint   string `json:"endpoint"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	Phase      string `json:"phase"`
	SessionID  string `json:"upload_session_id,omitempty"`
	VideoID    string `json:"video_id,omitempty"`
	Start      int64  `json:"start_offset"`
	End        int64  `json:"end_offset"`
}

type VideoUploadOptions struct {
	Profile, StateDir, ID string
	Resume                bool
}

func fileDigest(ctx context.Context, f *os.File) (string, error) {
	h := sha256.New()
	buf := make([]byte, 64*1024)
	for {
		if ctx.Err() != nil {
			return "", output.Fail("TIMEOUT", "Upload cancelled or deadline exceeded.", 7)
		}
		n, err := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
		}
		if err == io.EOF {
			return hex.EncodeToString(h.Sum(nil)), nil
		}
		if err != nil {
			return "", output.Fail("INPUT", "Cannot read upload file.", 2)
		}
	}
}

// UploadVideo resumes only the checkpointed session. Start and finish are never
// repeated after an unreadable acknowledgment; finish is reconciled by video ID.
func (c *Client) UploadVideo(ctx context.Context, account, path string, opt VideoUploadOptions) (result Response, err error) {
	unlock, err := state.Lock(opt.StateDir, opt.ID)
	if err != nil {
		return result, err
	}
	defer unlock()
	cpPath, err := state.Path(opt.StateDir, opt.ID)
	if err != nil {
		return result, err
	}
	f, err := os.Open(path)
	if err != nil {
		return result, output.Fail("INPUT", "Cannot open upload file.", 2)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > MaxUploadSize {
		return result, output.Fail("INPUT", "Upload requires a nonempty regular file at most 1 GiB.", 2)
	}
	hash, err := fileDigest(ctx, f)
	if err != nil {
		return result, err
	}
	currentInfo, err := f.Stat()
	if err != nil || currentInfo.Size() != info.Size() {
		return result, output.Fail("ASSET_CHANGED", "Upload file size changed during hashing.", 5)
	}
	endpoint := c.BaseURL
	if endpoint == "https://graph.facebook.com" {
		endpoint = "https://graph-video.facebook.com"
	}
	cp := VideoCheckpoint{Version: 1, Profile: opt.Profile, Account: account, APIVersion: c.Version, Endpoint: endpoint, Size: info.Size(), SHA256: hash, Phase: "new"}
	if opt.Resume {
		var saved VideoCheckpoint
		if err = state.Read(cpPath, &saved); err != nil {
			return result, err
		}
		if saved.Version != 1 || saved.Profile != cp.Profile || saved.Account != cp.Account || saved.APIVersion != cp.APIVersion || saved.Endpoint != cp.Endpoint {
			return result, output.Fail("UPLOAD_SCOPE", "Upload checkpoint scope differs from the selected profile/account/API/endpoint.", 4)
		}
		if saved.Size != cp.Size || saved.SHA256 != cp.SHA256 {
			return result, output.Fail("ASSET_CHANGED", "Upload file differs from the checkpoint size or SHA-256.", 5)
		}
		cp = saved
	} else {
		if _, e := os.Stat(cpPath); !os.IsNotExist(e) {
			return result, output.Fail("OUTPUT_EXISTS", "Upload checkpoint already exists; resume its ID.", 2)
		}
	}
	defer func() {
		result.Value = map[string]any{"id": cp.VideoID, "upload_id": opt.ID, "upload_session_id": cp.SessionID, "upload_phase": cp.Phase, "start_offset": cp.Start, "end_offset": cp.End}
	}()
	save := func() error { return state.Save(opt.StateDir, opt.ID, cp) }
	unknown := func(message string) error {
		e := output.Fail("MUTATION_UNKNOWN", message, 10)
		e.SuggestedAction = "Preserve this checkpoint; inspect the account/video before starting another upload. Resume with --resume " + opt.ID + "."
		return e
	}
	if cp.Phase == "done" {
		return result, nil
	}
	if cp.Phase == "starting" {
		return result, unknown("Start acknowledgment is unknown; a new upload session will not be created.")
	}
	if cp.Phase == "finishing" {
		if config.ID(cp.VideoID) != nil {
			return result, unknown("Finish cannot be reconciled without a video ID.")
		}
		response, e := c.Request(ctx, "GET", cp.VideoID, url.Values{"fields": {"id,status"}})
		if e != nil {
			oe := output.AsError(e)
			oe.Code, oe.Exit, oe.Retryable = "MUTATION_UNKNOWN", 10, false
			oe.SuggestedAction = "Finish remains unknown. Reconcile again with --resume " + opt.ID + "."
			return result, oe
		}
		status, _ := response.Value["status"].(map[string]any)
		upload, _ := status["uploading_phase"].(map[string]any)
		confirmed := status["video_status"] == "ready" || status["video_status"] == "processing" || upload["status"] == "complete"
		if fmt.Sprint(response.Value["id"]) != cp.VideoID || !confirmed {
			return result, unknown("Finish is not yet confirmed by video status; retry reconciliation with the same checkpoint.")
		}
		cp.Phase = "done"
		return result, save()
	}
	if cp.Phase != "new" && cp.Phase != "transfer" && cp.Phase != "finish" {
		return result, output.Fail("INPUT", "Invalid upload checkpoint phase.", 2)
	}
	// A separate client keeps video POSTs on Meta's video host and status GETs on Graph.
	video := New(c.Token, c.Version)
	video.BaseURL, video.HTTP, video.AppSecret = endpoint, c.HTTP, c.AppSecret
	video.RequestTimeout, video.Backoff, video.Log = c.RequestTimeout, c.Backoff, c.Log
	edge := account + "/advideos"
	if cp.Phase == "new" {
		cp.Phase = "starting"
		if err = save(); err != nil {
			return result, err
		}
		response, e := video.Request(ctx, "POST", edge, url.Values{"upload_phase": {"start"}, "file_size": {strconv.FormatInt(cp.Size, 10)}})
		if e != nil {
			if output.AsError(e).Exit != 10 {
				cp.Phase = "new"
				if err = save(); err != nil {
					return result, err
				}
			}
			return result, e
		}
		cp.SessionID, cp.VideoID = fmt.Sprint(response.Value["upload_session_id"]), fmt.Sprint(response.Value["video_id"])
		if config.ID(cp.SessionID) != nil || config.ID(cp.VideoID) != nil {
			return result, unknown("Start returned no usable session/video ID.")
		}
		if err = cp.offsets(response.Value); err != nil {
			_ = save()
			return result, err
		}
		cp.Phase = "transfer"
		if err = save(); err != nil {
			return result, unknown("Cannot checkpoint the created session; reconcile before retrying.")
		}
	}
	if config.ID(cp.SessionID) != nil || config.ID(cp.VideoID) != nil || !cp.validOffsets() {
		return result, output.Fail("INPUT", "Invalid upload checkpoint IDs or offsets.", 2)
	}
	recoveries := 0
	for cp.Start != cp.End {
		if ctx.Err() != nil {
			return result, output.Fail("TIMEOUT", "Upload interrupted; resume the same upload ID.", 7)
		}
		before := cp.Start
		response, e := video.uploadMultipart(ctx, account, "videos", "video_file_chunk", path, io.NewSectionReader(f, cp.Start, cp.End-cp.Start), cp.End-cp.Start, url.Values{"upload_phase": {"transfer"}, "upload_session_id": {cp.SessionID}, "start_offset": {strconv.FormatInt(cp.Start, 10)}})
		if e != nil {
			oe := output.AsError(e)
			provider, _ := oe.Provider.(map[string]any)
			correction, _ := provider["error_data"].(map[string]any)
			if fmt.Sprint(provider["error_subcode"]) == "1363037" && correction != nil && recoveries < 3 {
				if err = cp.offsets(correction); err != nil {
					return result, err
				}
			} else if (oe.Exit == 10 || provider["is_transient"] == true) && recoveries < 3 && ctx.Err() == nil {
				// Re-send only the same session/offset. Meta rejects stale offsets with
				// 1363037 and authoritative offsets, so this cannot create another asset.
			} else {
				return result, e
			}
			recoveries++
		} else {
			if err = cp.offsets(response.Value); err != nil {
				return result, err
			}
			if cp.Start <= before {
				recoveries++
			}
			if recoveries > 3 {
				return result, output.Fail("UPLOAD_STALLED", "Upload made no progress within the recovery bound; resume the same ID.", 8)
			}
		}
		if err = save(); err != nil {
			return result, err
		}
		if e != nil || cp.Start <= before {
			timer := time.NewTimer(c.Backoff * time.Duration(1<<min(recoveries, 3)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return result, output.Fail("TIMEOUT", "Upload interrupted; resume the same upload ID.", 7)
			case <-timer.C:
			}
		}
	}
	// Detect edits during the transfer before committing the uploaded asset.
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return result, err
	}
	currentHash, err := fileDigest(ctx, f)
	if err != nil {
		return result, err
	}
	if currentHash != cp.SHA256 {
		return result, output.Fail("ASSET_CHANGED", "Upload file changed during transfer; finish was not sent.", 5)
	}
	cp.Phase = "finishing"
	if err = save(); err != nil {
		return result, err
	}
	response, err := video.Request(ctx, "POST", edge, url.Values{"upload_phase": {"finish"}, "upload_session_id": {cp.SessionID}})
	if err != nil {
		if output.AsError(err).Exit != 10 {
			cp.Phase = "finish"
			if e := save(); e != nil {
				return result, e
			}
		}
		return result, err
	}
	if response.Value["success"] != true {
		return result, unknown("Finish returned no success acknowledgment; reconcile the existing video.")
	}
	cp.Phase = "done"
	if err = save(); err != nil {
		return result, unknown("Cannot checkpoint finish acknowledgment; reconcile the existing video.")
	}
	return result, nil
}

func (cp VideoCheckpoint) validOffsets() bool {
	return cp.Start >= 0 && cp.Start <= cp.End && cp.End <= cp.Size && (cp.Start != cp.End || cp.End == cp.Size)
}
func (cp *VideoCheckpoint) offsets(value map[string]any) error {
	start, e1 := strconv.ParseInt(fmt.Sprint(value["start_offset"]), 10, 64)
	end, e2 := strconv.ParseInt(fmt.Sprint(value["end_offset"]), 10, 64)
	if e1 != nil || e2 != nil {
		return output.Fail("MUTATION_UNKNOWN", "Upload returned unreadable offsets; resume the existing session.", 10)
	}
	candidate := *cp
	candidate.Start, candidate.End = start, end
	if !candidate.validOffsets() {
		return output.Fail("MUTATION_UNKNOWN", "Upload returned offsets outside the file; resume the existing session.", 10)
	}
	cp.Start, cp.End = start, end
	return nil
}
