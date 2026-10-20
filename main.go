package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

const baseURL = "https://api.assemblyai.com/v2"

type utterance struct {
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
}

type transcript struct {
	ID         string      `json:"id"`
	Status     string      `json:"status"`
	Error      string      `json:"error"`
	Utterances []utterance `json:"utterances"`
}

type client struct {
	apiKey string
	http   *http.Client
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if err := run(); err != nil {
		slog.Error("failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	lang := flag.String("lang", "", "audio language: it or en (required when submitting a file)")
	id := flag.String("id", "", "transcript id to check")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage:\n  diarizatr -lang it|en <audio file>\n  diarizatr -id <transcript id>\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	apiKey := os.Getenv("Z_ASSEMBLYAI_KEY")
	if apiKey == "" {
		return errors.New("Z_ASSEMBLYAI_KEY is not set")
	}
	c := &client{apiKey: apiKey, http: &http.Client{}}

	switch {
	case *id != "" && flag.NArg() == 0:
		return c.check(*id)
	case *id == "" && flag.NArg() == 1:
		if *lang != "it" && *lang != "en" {
			return fmt.Errorf("invalid -lang %q, must be it or en", *lang)
		}
		return c.submit(flag.Arg(0), *lang)
	default:
		flag.Usage()
		return errors.New("invalid arguments")
	}
}

// submit uploads the audio file and starts an async transcription job.
func (c *client) submit(path, lang string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}

	slog.Info("uploading", "file", path, "bytes", info.Size())
	var up struct {
		UploadURL string `json:"upload_url"`
	}
	pr := &progressReader{r: f, total: info.Size(), last: -1}
	err = c.do(http.MethodPost, baseURL+"/upload", "application/octet-stream", pr, &up)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}

	req, err := json.Marshal(map[string]any{
		"audio_url":      up.UploadURL,
		"language_code":  lang,
		"speaker_labels": true,
	})
	if err != nil {
		return err
	}
	var t transcript
	if err := c.do(http.MethodPost, baseURL+"/transcript", "application/json", bytes.NewReader(req), &t); err != nil {
		return fmt.Errorf("submit transcript: %w", err)
	}

	slog.Info("job submitted", "id", t.ID, "status", t.Status)
	fmt.Println("check the transcription with")
	fmt.Println()
	fmt.Printf("diarizatr -id %s\n", t.ID)
	return nil
}

// check fetches the job status and, when completed, writes <date>.txt in the working dir.
func (c *client) check(id string) error {
	var t transcript
	if err := c.do(http.MethodGet, baseURL+"/transcript/"+id, "", nil, &t); err != nil {
		return fmt.Errorf("get transcript: %w", err)
	}

	switch t.Status {
	case "completed":
	case "error":
		return fmt.Errorf("transcription failed: %s", t.Error)
	default:
		slog.Info("job not completed", "id", id, "status", t.Status)
		return nil
	}

	var sb strings.Builder
	for _, u := range t.Utterances {
		fmt.Fprintf(&sb, "Speaker %s: %s\n", u.Speaker, u.Text)
	}
	now := time.Now()
	out := now.Format("2006-01-02") + ".txt"
	if _, err := os.Stat(out); err == nil {
		out = fmt.Sprintf("%s-%d.txt", now.Format("2006-01-02"), now.Unix())
	}
	if err := os.WriteFile(out, []byte(sb.String()), 0o644); err != nil {
		return err
	}

	slog.Info("transcript saved", "id", id, "file", out, "utterances", len(t.Utterances))
	return nil
}

func (c *client) do(method, url, contentType string, body io.Reader, out any) error {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.apiKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if l, ok := body.(interface{ Len() int64 }); ok {
		req.ContentLength = l.Len()
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.Unmarshal(data, out)
}

// progressReader wraps the upload body and draws a progress bar on stderr.
type progressReader struct {
	r     io.Reader
	total int64
	read  int64
	last  int
}

func (p *progressReader) Len() int64 { return p.total }

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)

	pct := 100
	if p.total > 0 {
		pct = int(p.read * 100 / p.total)
	}
	if pct != p.last {
		p.last = pct
		const width = 30
		bar := strings.Repeat("#", pct*width/100) + strings.Repeat(" ", width-pct*width/100)
		fmt.Fprintf(os.Stderr, "\r[%s] %3d%% %.1f/%.1f MB", bar, pct, float64(p.read)/1e6, float64(p.total)/1e6)
	}
	return n, err
}
