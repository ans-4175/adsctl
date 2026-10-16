package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"time"

	"adsctl/internal/config"
	"adsctl/internal/output"
	"adsctl/internal/platform/meta/graph"
	"adsctl/internal/state"
	"github.com/spf13/cobra"
)

func (a *App) addMedia(g *cobra.Command, resource string) {
	var file, mode, resume, dir string
	var dry bool
	cmd := action("upload", "Upload media to the selected account", "write", func(c *cobra.Command, _ []string) error {
		r, err := a.runtime(c, true, true)
		if err != nil {
			return err
		}
		defer r.Cancel()
		info, err := os.Stat(file)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > graph.MaxUploadSize {
			return output.Fail("INPUT", "Specify a nonempty regular media --file at most 1 GiB.", 2)
		}
		receipt := map[string]any{"resource": resource, "file": file, "size": info.Size(), "dry_run": dry, "state": "planned"}
		selected := "single"
		if resource == "videos" {
			if mode != "auto" && mode != "single" && mode != "resumable" {
				return output.Fail("USAGE", "Video mode must be auto, single or resumable.", 2)
			}
			selected = mode
			if selected == "auto" {
				selected = "resumable"
			}
			if resume != "" && selected != "resumable" {
				return output.Fail("USAGE", "--resume requires resumable mode.", 2)
			}
		}
		receipt["mode"], receipt["max_file_size"] = selected, graph.MaxUploadSize
		receipt["limit_note"] = "1 GiB is a local file bound; provider limits still apply. Resumable chunks follow server offsets."
		if dry {
			return a.emit(c, receipt)
		}
		var response graph.Response
		if selected == "resumable" {
			root, e := state.Dir(dir)
			if e != nil {
				return e
			}
			id := resume
			if id == "" {
				b := make([]byte, 12)
				if _, e = rand.Read(b); e != nil {
					return e
				}
				id = "upload_" + hex.EncodeToString(b)
			}
			receipt["upload_id"] = id
			response, err = r.Client.UploadVideo(r.Ctx, r.Account, file, graph.VideoUploadOptions{Profile: r.Profile, StateDir: root, ID: id, Resume: resume != ""})
		} else {
			response, err = r.Client.Upload(r.Ctx, r.Account, resource, file)
		}
		if response.Value != nil {
			receipt["response"] = response.Value
		}

		if err != nil {
			receipt["state"] = "failed"
			if output.AsError(err).Exit == 10 {
				receipt["state"] = "unknown"
			}
			return a.partial(c, receipt, err)
		}
		receipt["response"] = response.Value
		receipt["state"] = "uploaded"
		if resource == "videos" && config.ID(fmt.Sprint(response.Value["id"])) != nil {
			receipt["state"] = "unknown"
			return a.partial(c, receipt, output.Fail("MUTATION_UNKNOWN", "Upload returned no video ID.", 10))
		}
		if resource == "images" {
			if _, ok := response.Value["images"].(map[string]any); !ok {
				receipt["state"] = "unknown"
				return a.partial(c, receipt, output.Fail("MUTATION_UNKNOWN", "Upload returned no image hashes.", 10))
			}
		}
		return a.emit(c, receipt)
	})
	cmd.Flags().StringVar(&file, "file", "", "Local media path")
	cmd.Flags().BoolVar(&dry, "dry-run", false, "Inspect file without uploading")
	g.AddCommand(cmd)
	if resource == "videos" {
		cmd.Flags().StringVar(&mode, "mode", "auto", "Upload mode: auto (resumable for every video), resumable, single")
		cmd.Flags().StringVar(&resume, "resume", "", "Resume an existing upload ID with the same file and scope")
		cmd.Flags().StringVar(&dir, "state-dir", "", "Private upload checkpoint directory")
		cmd := action("wait ID", "Wait for an account video to finish processing", "read", func(c *cobra.Command, args []string) error {
			r, err := a.runtime(c, true, false)
			if err != nil {
				return err
			}
			defer r.Cancel()
			if err = config.ID(args[0]); err != nil {
				return err
			}
			if err = verifyVideo(r, args[0]); err != nil {
				return err
			}
			a.meta["video_id"] = args[0]
			for {
				response, err := r.Client.Request(r.Ctx, "GET", args[0], url.Values{"fields": {"id,status"}})
				if err != nil {
					return err
				}
				status, _ := response.Value["status"].(map[string]any)
				switch status["video_status"] {
				case "ready":
					return a.emit(c, response.Value)
				case "error":
					return a.partial(c, response.Value, output.Fail("VIDEO_FAILED", "Video processing failed.", 8))
				}
				timer := time.NewTimer(time.Second)
				select {
				case <-r.Ctx.Done():
					timer.Stop()
					return output.Fail("TIMEOUT", "Video is still processing; retry wait with the same ID.", 7)
				case <-timer.C:
				}
			}
		})
		cmd.Args = cobra.ExactArgs(1)
		g.AddCommand(cmd)
	}
}
func verifyVideo(r *runtime, id string) error {
	result, err := r.Client.List(r.Ctx, r.Account+"/advideos", url.Values{"fields": {"id"}}, graph.PageOptions{Scope: r.Profile + "|" + r.Account, All: true, PageSize: 100, MaxItems: 10000})
	if err != nil {
		return err
	}
	for _, row := range result.Rows {
		if m, ok := row.(map[string]any); ok && fmt.Sprint(m["id"]) == id {
			return nil
		}
	}
	if !result.Complete {
		return output.Fail("INCOMPLETE", "Cannot verify video membership within row bound.", 9)
	}
	return output.Fail("ACCOUNT_MISMATCH", "Video is absent from the selected ad account.", 4)
}
