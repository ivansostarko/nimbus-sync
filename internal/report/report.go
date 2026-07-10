package report

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/YOUR_USERNAME/nimbus-sync/internal/config"
)

// FileError records a per-file failure.
type FileError struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// RemoteResult is the outcome of syncing one remote.
type RemoteResult struct {
	Name       string      `json:"name"`
	Provider   string      `json:"provider"`
	RemotePath string      `json:"remote_path"`
	LocalPath  string      `json:"local_path"`
	Status     string      `json:"status"` // ok | failed | skipped
	Reason     string      `json:"reason,omitempty"`
	Started    time.Time   `json:"started"`
	Ended      time.Time   `json:"ended"`
	Downloaded []string    `json:"downloaded"`
	Deleted    []string    `json:"deleted"`
	Unchanged  int         `json:"unchanged"`
	Excluded   int         `json:"excluded"`
	Bytes      int64       `json:"bytes"`
	Errors     []FileError `json:"errors"`
}

func (r *RemoteResult) AddDownloaded(path string, size int64) {
	r.Downloaded = append(r.Downloaded, path)
	r.Bytes += size
}

func (r *RemoteResult) AddError(path string, err error) {
	r.Errors = append(r.Errors, FileError{Path: path, Error: err.Error()})
}

func (r *RemoteResult) Fail(err error) {
	r.Status = "failed"
	r.Reason = err.Error()
	r.Ended = time.Now()
}

func (r *RemoteResult) Finish() {
	r.Status = "ok"
	if len(r.Errors) > 0 {
		r.Status = "partial"
	}
	r.Ended = time.Now()
}

// Report aggregates one sync run.
type Report struct {
	Started time.Time       `json:"started"`
	Ended   time.Time       `json:"ended"`
	DryRun  bool            `json:"dry_run"`
	Remotes []*RemoteResult `json:"remotes"`
}

func New(dryRun bool) *Report {
	return &Report{Started: time.Now(), DryRun: dryRun}
}

func (rep *Report) StartRemote(name, provider, remotePath, localPath string) *RemoteResult {
	rr := &RemoteResult{
		Name: name, Provider: provider,
		RemotePath: remotePath, LocalPath: localPath,
		Started: time.Now(),
	}
	rep.Remotes = append(rep.Remotes, rr)
	return rr
}

func (rep *Report) SkipRemote(name, reason string) {
	rep.Remotes = append(rep.Remotes, &RemoteResult{
		Name: name, Status: "skipped", Reason: reason,
		Started: time.Now(), Ended: time.Now(),
	})
}

func (rep *Report) Finish() { rep.Ended = time.Now() }

// PrintSummary writes a compact table for CLI output.
func (rep *Report) PrintSummary(w io.Writer) {
	mode := ""
	if rep.DryRun {
		mode = " (dry run)"
	}
	fmt.Fprintf(w, "\nSync finished in %s%s\n\n", rep.Ended.Sub(rep.Started).Round(time.Millisecond), mode)
	fmt.Fprintf(w, "%-20s %-9s %10s %10s %10s %10s\n", "REMOTE", "STATUS", "NEW/CHG", "UNCHANGED", "DELETED", "ERRORS")
	for _, r := range rep.Remotes {
		fmt.Fprintf(w, "%-20s %-9s %10d %10d %10d %10d\n",
			r.Name, r.Status, len(r.Downloaded), r.Unchanged, len(r.Deleted), len(r.Errors))
	}
	fmt.Fprintln(w)
}

// PrintFull adds per-file detail.
func (rep *Report) PrintFull(w io.Writer) {
	rep.PrintSummary(w)
	for _, r := range rep.Remotes {
		if len(r.Downloaded)+len(r.Deleted)+len(r.Errors) == 0 {
			continue
		}
		fmt.Fprintf(w, "== %s (%s) ==\n", r.Name, r.Provider)
		for _, f := range r.Downloaded {
			fmt.Fprintf(w, "  + %s\n", f)
		}
		for _, f := range r.Deleted {
			fmt.Fprintf(w, "  - %s\n", f)
		}
		for _, e := range r.Errors {
			fmt.Fprintf(w, "  ! %s: %s\n", e.Path, e.Error)
		}
		fmt.Fprintln(w)
	}
}

// Save writes the report as timestamped JSON under the config's report dir.
func Save(cfg *config.Config, rep *Report) error {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	name := rep.Started.Format("20060102-150405") + ".json"
	return os.WriteFile(filepath.Join(cfg.ReportDir, name), data, 0o600)
}

// Latest loads the most recent saved report.
func Latest(cfg *config.Config) (*Report, error) {
	entries, err := os.ReadDir(cfg.ReportDir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no reports in %s", cfg.ReportDir)
	}
	sort.Strings(names)
	data, err := os.ReadFile(filepath.Join(cfg.ReportDir, names[len(names)-1]))
	if err != nil {
		return nil, err
	}
	var rep Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, err
	}
	return &rep, nil
}

var htmlTmpl = template.Must(template.New("report").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>Nimbus Sync report</title>
<style>
 body{font-family:system-ui,sans-serif;max-width:900px;margin:2rem auto;padding:0 1rem;color:#1a1a2e}
 h1{font-size:1.4rem} table{border-collapse:collapse;width:100%;margin:1rem 0}
 th,td{border:1px solid #ddd;padding:.4rem .6rem;text-align:left;font-size:.9rem}
 th{background:#f4f4f8} .ok{color:#0a7d33} .failed{color:#c0392b} .partial{color:#b9770e}
 code{background:#f4f4f8;padding:.1rem .3rem;border-radius:3px}
 ul{font-size:.85rem}
</style></head><body>
<h1>Nimbus Sync report</h1>
<p>Run started {{.Started.Format "2006-01-02 15:04:05"}}, finished {{.Ended.Format "15:04:05"}}{{if .DryRun}} — <strong>dry run</strong>{{end}}.</p>
<table>
<tr><th>Remote</th><th>Provider</th><th>Status</th><th>New/changed</th><th>Unchanged</th><th>Deleted</th><th>Errors</th></tr>
{{range .Remotes}}
<tr><td>{{.Name}}</td><td>{{.Provider}}</td><td class="{{.Status}}">{{.Status}}</td>
<td>{{len .Downloaded}}</td><td>{{.Unchanged}}</td><td>{{len .Deleted}}</td><td>{{len .Errors}}</td></tr>
{{end}}
</table>
{{range .Remotes}}
{{if or .Downloaded .Deleted .Errors}}
<h2>{{.Name}} <small>({{.RemotePath}} → <code>{{.LocalPath}}</code>)</small></h2>
<ul>
{{range .Downloaded}}<li>＋ {{.}}</li>{{end}}
{{range .Deleted}}<li>− {{.}}</li>{{end}}
{{range .Errors}}<li>⚠ {{.Path}}: {{.Error}}</li>{{end}}
</ul>
{{end}}
{{end}}
</body></html>`))

// WriteHTML renders the report as a standalone HTML page.
func WriteHTML(rep *Report, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return htmlTmpl.Execute(f, rep)
}
