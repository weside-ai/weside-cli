package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/spf13/viper"
)

// timelineBody is one page across two rooms: a memory without a room, an
// unknown kind, a tool call and messages, oldest first.
const timelineBody = `{
  "companion_id": 7,
  "entries": [
    {"kind": "message", "at": "2026-10-03T08:00:00+00:00", "room_ref": "room:11", "room_kind": "dm", "room_title": "Foxy",
     "id": "m1", "row_id": 1, "sender": {"kind": "human", "display_name": "Foxy", "user_id": 1}, "text": "Guten Morgen\nNox"},
    {"kind": "memory", "at": "2026-10-03T09:00:00+00:00", "id": 5, "type": "journal", "title": "Morgen", "text": "Ein ruhiger Morgen", "memory_date": "2026-10-03"},
    {"kind": "message", "at": "2026-10-03T10:00:00+00:00", "room_ref": "room:22", "room_kind": "group", "room_title": "Familie",
     "id": "m2", "row_id": 2, "sender": {"kind": "self", "display_name": null, "companion_id": 7}, "text": "Hallo zusammen"},
    {"kind": "future_kind", "at": "2026-10-03T10:30:00+00:00", "text": "never shown"},
    {"kind": "tool_call", "at": "2026-10-04T11:00:00+00:00", "room_ref": "room:11", "room_kind": "dm", "room_title": "Foxy",
     "id": 9, "tool_call_id": "tc1", "tool_name": "notes_write", "outcome": "error", "output_excerpt": "disk full"},
    {"kind": "tool_call", "at": "2026-10-04T11:05:00+00:00", "room_ref": "room:11", "room_kind": "dm", "room_title": "Foxy",
     "id": 10, "tool_call_id": "tc2", "tool_name": "web_search", "outcome": "success", "output_excerpt": "found"},
    {"kind": "tool_call", "at": "2026-10-04T11:06:00+00:00", "room_ref": "room:11", "room_kind": "dm", "room_title": "Foxy",
     "id": 11, "tool_call_id": null, "tool_name": "pending_tool", "outcome": null, "output_excerpt": null}
  ],
  "next_cursor": "abc123",
  "scanned_to": "2026-10-01T00:00:00+00:00"
}`

// resetTimelineFlags clears the package flag state one test may leave behind.
func resetTimelineFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		for _, name := range []string{"include", "room", "person", "since", "until", "contains", "cursor", "limit", "group"} {
			flag := companionsTimelineCmd.Flags().Lookup(name)
			_ = flag.Value.Set(flag.DefValue)
			flag.Changed = false
		}
	}
	reset()
	t.Cleanup(reset)
}

func setTimelineFlag(t *testing.T, name, value string) {
	t.Helper()
	if err := companionsTimelineCmd.Flags().Set(name, value); err != nil {
		t.Fatalf("set --%s: %v", name, err)
	}
}

// timelineServer answers the name lookup and the timeline; it records the
// timeline request's query and counts every request.
func timelineServer(t *testing.T, body string, status int) (*httptest.Server, *url.Values, *int32) {
	t.Helper()
	var query url.Values
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/companions":
			_, _ = w.Write([]byte(companionListResponse("7", "Nox")))
		case "/api/v2/companions/7/timeline":
			query = r.URL.Query()
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &query, &calls
}

func runTimeline(t *testing.T, args ...string) string {
	t.Helper()
	return captureStdout(t, func() error {
		return companionsTimelineCmd.RunE(companionsTimelineCmd, args)
	})
}

// TestCompanionsTimelineSendsEveryFlag resolves the name to 7 and sends each
// flag to the v2 path: include repeated, dates widened to the UTC day.
func TestCompanionsTimelineSendsEveryFlag(t *testing.T) {
	resetTimelineFlags(t)
	srv, query, _ := timelineServer(t, timelineBody, http.StatusOK)
	humanOutput(t, srv.URL)
	setTimelineFlag(t, "include", "messages, memories")
	setTimelineFlag(t, "room", "room:11")
	setTimelineFlag(t, "person", "Foxy")
	setTimelineFlag(t, "contains", "morgen")
	setTimelineFlag(t, "cursor", "c0")
	setTimelineFlag(t, "limit", "20")
	setTimelineFlag(t, "since", "2026-10-03")
	setTimelineFlag(t, "until", "2026-10-04")

	runTimeline(t, "Nox")

	want := url.Values{
		"include":  {"messages", "memories"},
		"room":     {"room:11"},
		"person":   {"Foxy"},
		"contains": {"morgen"},
		"cursor":   {"c0"},
		"limit":    {"20"},
		"since":    {"2026-10-03T00:00:00Z"},
		"until":    {"2026-10-04T23:59:59.999999Z"},
	}
	if !reflect.DeepEqual(*query, want) {
		t.Fatalf("query = %v, want %v", *query, want)
	}
}

// TestCompanionsTimelineSendsAnOffsetUnchanged keeps the "+" of an RFC 3339
// offset: a hand-built query would turn it into a space.
func TestCompanionsTimelineSendsAnOffsetUnchanged(t *testing.T) {
	resetTimelineFlags(t)
	srv, query, _ := timelineServer(t, timelineBody, http.StatusOK)
	humanOutput(t, srv.URL)
	setTimelineFlag(t, "since", "2026-10-03T08:00:00+02:00")

	runTimeline(t, "7")

	if got := query.Get("since"); got != "2026-10-03T08:00:00+02:00" {
		t.Fatalf("since = %q, want the offset unchanged", got)
	}
	if query.Has("include") || query.Has("limit") {
		t.Errorf("unset flags reached the wire: %v", *query)
	}
}

// TestCompanionsTimelineJSONIsTheResponse prints the response's keys and values unchanged.
func TestCompanionsTimelineJSONIsTheResponse(t *testing.T) {
	resetTimelineFlags(t)
	srv, _, _ := timelineServer(t, timelineBody, http.StatusOK)
	humanOutput(t, srv.URL)
	viper.Set("json", true)

	out := runTimeline(t, "Nox")

	var got, want any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	_ = json.Unmarshal([]byte(timelineBody), &want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON output differs from the response:\n%s", out)
	}
}

func assertInOrder(t *testing.T, out string, parts ...string) {
	t.Helper()
	last := -1
	for _, part := range parts {
		at := strings.Index(out, part)
		if at < 0 {
			t.Fatalf("output lacks %q:\n%s", part, out)
		}
		if at < last {
			t.Fatalf("%q is out of order:\n%s", part, out)
		}
		last = at
	}
}

// TestCompanionsTimelineTimeView prints one table in page order, maps sender
// and outcome, skips an unknown kind and folds a line break.
func TestCompanionsTimelineTimeView(t *testing.T) {
	resetTimelineFlags(t)
	srv, _, _ := timelineServer(t, timelineBody, http.StatusOK)
	humanOutput(t, srv.URL)

	out := runTimeline(t, "Nox")

	assertInOrder(t, out, "Guten Morgen Nox", `[memory journal "Morgen"] 2026-10-03`, "You", "notes_write failed", "web_search ok", "pending_tool …")
	if strings.Contains(out, "never shown") || strings.Contains(out, "future_kind") {
		t.Errorf("an unknown kind was printed:\n%s", out)
	}
	if strings.Contains(out, "==") {
		t.Errorf("time view printed sections:\n%s", out)
	}
	if !strings.Contains(out, "(older: --cursor abc123)") {
		t.Errorf("cursor hint missing:\n%s", out)
	}
	if strings.Contains(out, "searched back") {
		t.Errorf("scanned_to hint without a search:\n%s", out)
	}
}

// TestCompanionsTimelineGroupRoom prints one section per room in order of first
// appearance, the memory without a room under "no room", lines oldest first.
func TestCompanionsTimelineGroupRoom(t *testing.T) {
	resetTimelineFlags(t)
	srv, _, _ := timelineServer(t, timelineBody, http.StatusOK)
	humanOutput(t, srv.URL)
	setTimelineFlag(t, "group", "room")

	out := runTimeline(t, "Nox")

	assertInOrder(t, out,
		"== Foxy (room:11, dm) ==", "Guten Morgen", "notes_write", "web_search", "pending_tool",
		"== no room ==", "Ein ruhiger Morgen",
		"== Familie (room:22, group) ==", "Hallo zusammen",
	)
	if strings.Count(out, "== Foxy") != 1 {
		t.Errorf("room:11 printed more than one section:\n%s", out)
	}
}

// TestCompanionsTimelineGroupDay prints one section per UTC date.
func TestCompanionsTimelineGroupDay(t *testing.T) {
	resetTimelineFlags(t)
	srv, _, _ := timelineServer(t, timelineBody, http.StatusOK)
	humanOutput(t, srv.URL)
	setTimelineFlag(t, "group", "day")

	out := runTimeline(t, "Nox")

	assertInOrder(t, out, "== 2026-10-03 ==", "Guten Morgen", "Ein ruhiger Morgen", "Hallo zusammen", "== 2026-10-04 ==", "notes_write", "pending_tool")
}

// TestCompanionsTimelineEscapesAndCutsRemoteText escapes a terminal control
// and cuts a long multibyte text without breaking UTF-8.
func TestCompanionsTimelineEscapesAndCutsRemoteText(t *testing.T) {
	resetTimelineFlags(t)
	long := strings.Repeat("ü", 150)
	body := `{"companion_id": 7, "entries": [
	  {"kind": "message", "at": "2026-10-03T08:00:00+00:00", "room_ref": "room:11", "id": "m1", "row_id": 1,
	   "sender": {"kind": "human", "display_name": "Ev\u001b[31mil"}, "text": "` + long + `"}
	], "next_cursor": null, "scanned_to": null}`
	srv, _, _ := timelineServer(t, body, http.StatusOK)
	humanOutput(t, srv.URL)

	out := runTimeline(t, "Nox")

	if strings.Contains(out, "\x1b") || !strings.Contains(out, `Ev\x1b[31mil`) {
		t.Errorf("the control character was not escaped:\n%q", out)
	}
	if !utf8.ValidString(out) {
		t.Fatalf("output is not valid UTF-8")
	}
	if !strings.Contains(out, strings.Repeat("ü", 99)+"…") || strings.Contains(out, strings.Repeat("ü", 100)) {
		t.Errorf("text was not cut to 100 runes:\n%s", out)
	}
	if strings.Contains(out, "--cursor") {
		t.Errorf("cursor hint without a cursor:\n%s", out)
	}
}

func TestCutRunes(t *testing.T) {
	tests := []struct {
		name, in, want string
		max            int
	}{
		{"short stays", "abc", "abc", 5},
		{"exact stays", "äöü", "äöü", 3},
		{"multibyte cut", "äöüß", "äö…", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cutRunes(tt.in, tt.max); got != tt.want {
				t.Errorf("cutRunes(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
		})
	}
}

// TestCompanionsTimelineSearchFooter names how far a short search page read;
// an empty page without a cursor says so.
func TestCompanionsTimelineSearchFooter(t *testing.T) {
	resetTimelineFlags(t)
	body := `{"companion_id": 7, "entries": [], "next_cursor": null, "scanned_to": "2026-10-01T00:00:00+00:00"}`
	srv, _, _ := timelineServer(t, body, http.StatusOK)
	humanOutput(t, srv.URL)
	setTimelineFlag(t, "contains", "regen")

	out := runTimeline(t, "Nox")

	for _, want := range []string{"(no entries)", "(searched back to 2026-10-01 00:00 +00:00)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "--cursor") {
		t.Errorf("cursor hint without a cursor:\n%s", out)
	}
}

// TestCompanionsTimelineRejectsFlagsLocally fails before any request,
// including the name lookup.
func TestCompanionsTimelineRejectsFlagsLocally(t *testing.T) {
	tests := []struct{ flag, value, wantErr string }{
		{"since", "yesterday", "--since"},
		{"until", "2026-13-01", "--until"},
		{"limit", "0", "between 1 and 200"},
		{"limit", "201", "between 1 and 200"},
		{"group", "foo", "--group must be"},
	}
	for _, tt := range tests {
		t.Run(tt.flag+"="+tt.value, func(t *testing.T) {
			resetTimelineFlags(t)
			srv, _, calls := timelineServer(t, timelineBody, http.StatusOK)
			humanOutput(t, srv.URL)
			setTimelineFlag(t, tt.flag, tt.value)

			err := companionsTimelineCmd.RunE(companionsTimelineCmd, []string{"Nox"})

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want one naming %q", err, tt.wantErr)
			}
			if n := atomic.LoadInt32(calls); n != 0 {
				t.Errorf("%d request(s) sent for an invalid flag", n)
			}
		})
	}
}

// TestCompanionsTimelineNotFound surfaces the server's 404 as an error.
func TestCompanionsTimelineNotFound(t *testing.T) {
	resetTimelineFlags(t)
	body := `{"type": "about:blank", "title": "Not Found", "status": 404, "detail": "Companion not found"}`
	srv, _, _ := timelineServer(t, body, http.StatusNotFound)
	humanOutput(t, srv.URL)

	err := companionsTimelineCmd.RunE(companionsTimelineCmd, []string{"7"})

	if err == nil || !strings.Contains(err.Error(), "reading companion timeline") || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v, want the wrapped 404", err)
	}
}

func TestParseTimelineTime(t *testing.T) {
	tests := []struct {
		name, in string
		end      bool
		want     string
		wantErr  bool
	}{
		{"empty", "", false, "", false},
		{"date as since", "2026-10-03", false, "2026-10-03T00:00:00Z", false},
		{"date as until", "2026-10-03", true, "2026-10-03T23:59:59.999999Z", false},
		{"rfc3339 passes", "2026-10-03T08:00:00+02:00", false, "2026-10-03T08:00:00+02:00", false},
		{"garbage", "last week", false, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTimelineTime(tt.in, tt.end)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("parseTimelineTime(%q, %v) = %q, %v; want %q, err %v", tt.in, tt.end, got, err, tt.want, tt.wantErr)
			}
		})
	}
}
