package cmd

import (
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weside-ai/weside-cli/internal/api"
)

// These tests drive the real `runSticker*` functions against an httptest
// server, not a re-implementation of their rendering. A test that rebuilt the
// table itself would pass over a command that formats nothing.
//
// The load-bearing one is TestStickerPublishSurfacesThe409: `publish` exists
// to make the provenance refusal visible, so a verb that could not go red
// would be as absent as one that does not exist.

func captureStdoutStickers(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), runErr
}

func TestStickerPacksListRendersTheVisibleSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/stickers/packs" || r.Method != http.MethodGet {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if got := r.URL.Query().Get("limit"); got != "2" {
			t.Errorf("limit not forwarded: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[`+
			`{"id":1,"slug":"nox-alltag","title":"Nox Alltag","source_kind":"uploaded",`+
			`"visibility":"private","rights_confirmed_at":null},`+
			`{"id":2,"slug":"weside-starter","title":"Starter","source_kind":"weside_org",`+
			`"visibility":"public","rights_confirmed_at":"2026-09-10T00:00:00Z"}`+
			`],"next_cursor":"djE6MTox"}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out, err := captureStdoutStickers(t, func() error {
		return runStickerPacksList(context.Background(), client, 2, "")
	})
	if err != nil {
		t.Fatalf("list returned an error: %v", err)
	}
	for _, want := range []string{
		"nox-alltag", "weside-starter", "unconfirmed", "confirmed", "djE6MTox",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
}

func TestStickerPacksListForwardsTheCursor(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Query().Get("cursor")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[],"next_cursor":null}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	if _, err := captureStdoutStickers(t, func() error {
		return runStickerPacksList(context.Background(), client, 0, "djE6MTox")
	}); err != nil {
		t.Fatalf("list returned an error: %v", err)
	}
	if seen != "djE6MTox" {
		t.Errorf("cursor not forwarded, server saw %q", seen)
	}
}

func TestStickerPacksShowRendersStickers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/stickers/packs/12" {
			t.Errorf("wrong path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":12,"slug":"nox-alltag","title":"Nox Alltag",`+
			`"source_kind":"uploaded","visibility":"private","rights_confirmed_at":null,`+
			`"telegram_set_name":"nox_alltag_by_noxbot","stickers":[`+
			`{"id":5,"shortcode":"stampf","format":"webp","emoji":["🐴","🖤"],"position":0}]}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out, err := captureStdoutStickers(t, func() error {
		return runStickerPacksShow(context.Background(), client, "12")
	})
	if err != nil {
		t.Fatalf("show returned an error: %v", err)
	}
	for _, want := range []string{
		"Nox Alltag", "stampf", "webp", "🐴", "t.me/addstickers/nox_alltag_by_noxbot",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
}

func TestStickerUploadSendsMultipartWithTheRightsField(t *testing.T) {
	var (
		gotFileName string
		gotBytes    []byte
		gotRights   string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
			t.Errorf("not a multipart request: %q (%v)", r.Header.Get("Content-Type"), err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		reader := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(part)
			switch part.FormName() {
			case "file":
				gotFileName = part.FileName()
				gotBytes = data
			case "rights_confirmed":
				gotRights = string(data)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":7,"slug":"nox-alltag","visibility":"private",`+
			`"rights_confirmed_at":"2026-09-10T00:00:00Z","source_kind":"uploaded"}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	packPath := filepath.Join(dir, "nox-alltag.wsp")
	if err := os.WriteFile(packPath, []byte("PK\x03\x04not-a-real-zip"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	client := api.NewClient(srv.URL, "token")
	out, err := captureStdoutStickers(t, func() error {
		return runStickerPacksUpload(context.Background(), client, packPath, true)
	})
	if err != nil {
		t.Fatalf("upload returned an error: %v", err)
	}
	if gotFileName != "nox-alltag.wsp" {
		t.Errorf("file part named %q, want nox-alltag.wsp", gotFileName)
	}
	if string(gotBytes) != "PK\x03\x04not-a-real-zip" {
		t.Errorf("file bytes arrived altered: %q", gotBytes)
	}
	if gotRights != "true" {
		t.Errorf("rights_confirmed arrived as %q, want \"true\"", gotRights)
	}
	if !strings.Contains(out, "nox-alltag") {
		t.Errorf("output is missing the slug:\n%s", out)
	}
}

func TestStickerUploadWithoutConfirmationSendsFalse(t *testing.T) {
	var gotRights string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		reader := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(part)
			if part.FormName() == "rights_confirmed" {
				gotRights = string(data)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":8,"slug":"s","visibility":"private","rights_confirmed_at":null}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	packPath := filepath.Join(dir, "s.wsp")
	if err := os.WriteFile(packPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	client := api.NewClient(srv.URL, "token")
	if _, err := captureStdoutStickers(t, func() error {
		return runStickerPacksUpload(context.Background(), client, packPath, false)
	}); err != nil {
		t.Fatalf("upload returned an error: %v", err)
	}
	if gotRights != "false" {
		t.Errorf("rights_confirmed arrived as %q, want \"false\" — a default that "+
			"silently claimed the rights would be the provenance gate walked past",
			gotRights)
	}
}

func TestStickerPublishSurfacesThe409(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = fmt.Fprint(w, `{"type":"https://weside.ai/errors/sticker-rights-unconfirmed",`+
			`"detail":"Sticker pack 'nox-alltag' was uploaded and its rights are not confirmed."}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out, err := captureStdoutStickers(t, func() error {
		return runStickerPacksPublish(context.Background(), client, "12", false)
	})
	if err == nil {
		t.Fatal("publish returned nil on a 409 — the verb cannot go red, which " +
			"makes it as absent as one that does not exist")
	}
	if !strings.Contains(err.Error(), "409") {
		t.Errorf("the error does not carry the status: %v", err)
	}
	if !strings.Contains(err.Error(), "rights are not confirmed") {
		t.Errorf("the error does not carry the API's detail: %v", err)
	}
	if strings.Contains(out, "now public") || strings.Contains(out, "is now") {
		t.Errorf("publish printed a success line over a refusal:\n%s", out)
	}
}

func TestStickerPublishSendsTheVisibilityAndOptionalRights(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("publish used %s, want PATCH", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":12,"visibility":"public"}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out, err := captureStdoutStickers(t, func() error {
		return runStickerPacksPublish(context.Background(), client, "12", true)
	})
	if err != nil {
		t.Fatalf("publish returned an error: %v", err)
	}
	if !strings.Contains(body, `"visibility":"public"`) {
		t.Errorf("request body is missing the visibility: %s", body)
	}
	if !strings.Contains(body, `"rights_confirmed":true`) {
		t.Errorf("--confirm-rights did not reach the body: %s", body)
	}
	if !strings.Contains(out, "public") {
		t.Errorf("output is missing the new visibility:\n%s", out)
	}
}

func TestStickerExportTelegramSurfacesTheBotRequirement(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/stickers/packs/12/export/telegram" {
			t.Errorf("wrong path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = fmt.Fprint(w, `{"type":"https://weside.ai/errors/telegram-bot-required",`+
			`"detail":"Register a Telegram bot and link your Telegram account first."}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	_, err := captureStdoutStickers(t, func() error {
		return runStickerPacksExportTelegram(context.Background(), client, "12")
	})
	if err == nil {
		t.Fatal("export returned nil on a 409 telegram-bot-required")
	}
	if !strings.Contains(err.Error(), "Register a Telegram bot") {
		t.Errorf("the error does not carry the API's detail: %v", err)
	}
}

func TestStickerExportTelegramPrintsTheInstallLink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"set_name":"nox_alltag_by_noxbot",`+
			`"link":"https://t.me/addstickers/nox_alltag_by_noxbot"}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out, err := captureStdoutStickers(t, func() error {
		return runStickerPacksExportTelegram(context.Background(), client, "12")
	})
	if err != nil {
		t.Fatalf("export returned an error: %v", err)
	}
	if !strings.Contains(out, "https://t.me/addstickers/nox_alltag_by_noxbot") {
		t.Errorf("output is missing the install link:\n%s", out)
	}
}

func TestStickerSendPostsToTheShortcodePath(t *testing.T) {
	var (
		path string
		body string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"platform_message_id":"4711"}`)
	}))
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	out, err := captureStdoutStickers(t, func() error {
		return runStickerSend(context.Background(), client, "12", "stampf", "4")
	})
	if err != nil {
		t.Fatalf("send returned an error: %v", err)
	}
	if path != "/stickers/packs/12/stickers/stampf/send" {
		t.Errorf("wrong path %q", path)
	}
	if !strings.Contains(body, `"binding_id":"4"`) {
		t.Errorf("request body is missing the binding: %s", body)
	}
	if !strings.Contains(out, "4711") {
		t.Errorf("output is missing the platform message id:\n%s", out)
	}
}

func TestStickerExportRefusesWithoutATarget(t *testing.T) {
	stickerExportTelegram = false
	defer func() { stickerExportTelegram = false }()
	if err := stickerPacksExportCmd.RunE(stickerPacksExportCmd, []string{"12"}); err == nil {
		t.Fatal("export with no target returned nil — it must name --telegram")
	}
}

func TestStickerSendRefusesWithoutABinding(t *testing.T) {
	stickerSendBinding = ""
	if err := stickerSendCmd.RunE(stickerSendCmd, []string{"12", "stampf"}); err == nil {
		t.Fatal("send with no --binding returned nil")
	}
}
