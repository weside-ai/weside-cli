package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/ui"
)

// timelineTextWidth is how many runes of an entry's text one table row shows.
// Fixed: nothing in the CLI measures the terminal, and --json has the full text.
const timelineTextWidth = 100

// timelineServerLimit is the server's page size when no --limit is sent.
const timelineServerLimit = 50

var (
	tlInclude  string
	tlRoom     string
	tlPerson   string
	tlSince    string
	tlUntil    string
	tlContains string
	tlGroup    string
	tlCursor   string
	tlLimit    int
)

// timelineFlags are the parsed, validated flags of one `companions timeline` call.
type timelineFlags struct {
	include  []string
	room     string
	person   string
	since    string
	until    string
	contains string
	cursor   string
	limit    int // 0 = server default
	group    string
}

// companionsTimelineCmd reads the owner's companion timeline (WA-2410): one
// page of the messages of all the companion's rooms, its memories,
// conversation memories and tool calls on one time axis, oldest first.
var companionsTimelineCmd = &cobra.Command{
	Use:   "timeline [id|name]",
	Short: "Show what a companion lived across its rooms, memories and tool calls",
	Long: `Read one page of the companion's timeline (GET /api/v2/companions/{id}/timeline).

The first page is the newest one; --cursor reads further back. Entries come
oldest first. --include picks the sources (messages, memories,
conversation_memories, tool_calls; default: messages). --since/--until take a
date (YYYY-MM-DD, the whole UTC day) or an RFC 3339 time.

--contains and --person read a bounded number of rows per page: a page can come
back short or empty while the cursor still continues the search.

--group (time, room, day) only arranges the page in the terminal; the server
does not group.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		flags, err := readTimelineFlags(cmd)
		if err != nil {
			return err
		}
		companionID, err := resolveCompanion(firstArg(args))
		if err != nil {
			return err
		}
		client, err := newAuthenticatedClientV2()
		if err != nil {
			return err
		}

		var result map[string]any
		path := "/companions/" + companionID + "/timeline?" + timelineQuery(&flags).Encode()
		if err := client.Get(context.Background(), path, &result); err != nil {
			return fmt.Errorf("reading companion timeline: %w", err)
		}

		if IsJSON() {
			ui.PrintJSON(result)
			return nil
		}
		printCompanionTimeline(result, &flags)
		return nil
	},
}

// readTimelineFlags validates every flag before any request: a wrong flag
// fails here, not as a 422 after the name lookup.
func readTimelineFlags(cmd *cobra.Command) (timelineFlags, error) {
	f := timelineFlags{
		include:  csvSlice(tlInclude),
		room:     strings.TrimSpace(tlRoom),
		person:   strings.TrimSpace(tlPerson),
		contains: strings.TrimSpace(tlContains),
		cursor:   strings.TrimSpace(tlCursor),
		group:    tlGroup,
	}
	var err error
	if f.since, err = parseTimelineTime(tlSince, false); err != nil {
		return f, fmt.Errorf("--since: %w", err)
	}
	if f.until, err = parseTimelineTime(tlUntil, true); err != nil {
		return f, fmt.Errorf("--until: %w", err)
	}
	if f.since != "" && f.until != "" {
		since, _ := time.Parse(time.RFC3339Nano, f.since)
		until, _ := time.Parse(time.RFC3339Nano, f.until)
		if since.After(until) {
			return f, fmt.Errorf("--since %s is after --until %s", f.since, f.until)
		}
	}
	if cmd.Flags().Changed("limit") {
		if tlLimit < 1 || tlLimit > 200 {
			return f, fmt.Errorf("--limit must be between 1 and 200, got %d", tlLimit)
		}
		f.limit = tlLimit
	}
	switch f.group {
	case "time", "room", "day":
	default:
		return f, fmt.Errorf("--group must be time, room or day, got %q", f.group)
	}
	return f, nil
}

// parseTimelineTime turns a --since/--until value into the wire's RFC 3339.
// A date is that whole UTC day: its first instant for since, its last for
// until. An RFC 3339 time passes through unchanged; anything else is an error.
func parseTimelineTime(value string, endOfDay bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if day, err := time.ParseInLocation("2006-01-02", value, time.UTC); err == nil {
		if endOfDay {
			day = day.Add(24*time.Hour - time.Microsecond)
		}
		return day.Format(time.RFC3339Nano), nil
	}
	if _, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return value, nil
	}
	return "", fmt.Errorf("%q is neither a date (YYYY-MM-DD) nor an RFC 3339 time", value)
}

// timelineQuery builds the request's query string. Extracted so a flag
// reaching the wire is a test rather than a claim.
func timelineQuery(f *timelineFlags) url.Values {
	q := url.Values{}
	for _, source := range f.include {
		q.Add("include", source)
	}
	for key, value := range map[string]string{
		"room":     f.room,
		"person":   f.person,
		"since":    f.since,
		"until":    f.until,
		"contains": f.contains,
		"cursor":   f.cursor,
	} {
		if value != "" {
			q.Set(key, value)
		}
	}
	if f.limit > 0 {
		q.Set("limit", strconv.Itoa(f.limit))
	}
	return q
}

// timelineRow is one entry as a table row plus the keys it groups by.
type timelineRow struct {
	roomRef   string
	roomTitle string
	roomKind  string
	day       string
	cells     []string
}

// printCompanionTimeline renders one page. Entries arrive oldest first and
// keep that order inside every section; an unknown kind is skipped (the union
// is open).
func printCompanionTimeline(result map[string]any, f *timelineFlags) {
	var rows []timelineRow
	for _, item := range asSlice(result["entries"]) {
		entry, _ := item.(map[string]any)
		if row, ok := timelineEntryRow(entry); ok {
			rows = append(rows, row)
		}
	}

	if len(rows) == 0 {
		ui.Println("(no entries)")
	} else {
		headers := []string{"AT", "KIND", "ROOM", "FROM", "TEXT"}
		switch f.group {
		case "room":
			printTimelineSections(headers, rows, func(r timelineRow) string { return r.roomRef }, roomSectionTitle(rows))
		case "day":
			printTimelineSections(headers, rows, func(r timelineRow) string { return r.day }, func(key string) string { return key })
		default:
			ui.PrintTable(headers, timelineCells(rows))
		}
	}

	if next, _ := result["next_cursor"].(string); next != "" {
		ui.Printf("(older: --cursor %s)\n", next)
	}
	limit := f.limit
	if limit == 0 {
		limit = timelineServerLimit
	}
	searching := f.contains != "" || f.person != ""
	if scanned, _ := result["scanned_to"].(string); searching && scanned != "" && len(asSlice(result["entries"])) < limit {
		ui.Printf("(searched back to %s)\n", clockTime(scanned))
	}
}

// printTimelineSections prints one table per key, sections in the order their
// first entry appears.
func printTimelineSections(headers []string, rows []timelineRow, key func(timelineRow) string, title func(string) string) {
	var order []string
	sections := map[string][]timelineRow{}
	for _, row := range rows {
		k := key(row)
		if _, seen := sections[k]; !seen {
			order = append(order, k)
		}
		sections[k] = append(sections[k], row)
	}
	for i, k := range order {
		if i > 0 {
			ui.Println()
		}
		ui.Printf("== %s ==\n", title(k))
		ui.PrintTable(headers, timelineCells(sections[k]))
	}
}

// roomSectionTitle names a room section by the first title and kind any of
// its entries carries; a memory names only its room ref.
func roomSectionTitle(rows []timelineRow) func(string) string {
	titles, kinds := map[string]string{}, map[string]string{}
	for _, row := range rows {
		if titles[row.roomRef] == "" {
			titles[row.roomRef] = row.roomTitle
		}
		if kinds[row.roomRef] == "" {
			kinds[row.roomRef] = row.roomKind
		}
	}
	return func(ref string) string {
		if ref == "" {
			return "no room"
		}
		detail := ref
		if kind := kinds[ref]; kind != "" {
			detail += ", " + kind
		}
		if title := titles[ref]; title != "" {
			return title + " (" + detail + ")"
		}
		return detail
	}
}

func timelineCells(rows []timelineRow) [][]string {
	cells := make([][]string, len(rows))
	for i, row := range rows {
		cells[i] = row.cells
	}
	return cells
}

// timelineEntryRow maps one wire entry to a row; false for a kind this CLI
// does not know.
func timelineEntryRow(entry map[string]any) (timelineRow, bool) {
	str := func(key string) string { s, _ := entry[key].(string); return s }
	row := timelineRow{
		roomRef:   str("room_ref"),
		roomTitle: str("room_title"),
		roomKind:  str("room_kind"),
	}
	row.day = "unknown day"
	if at, err := time.Parse(time.RFC3339Nano, str("at")); err == nil {
		row.day = at.UTC().Format("2006-01-02")
	}
	room := row.roomTitle
	if room == "" {
		room = row.roomRef
	}

	var label, from, text string
	switch kind := str("kind"); kind {
	case "message":
		label = "message"
		sender, _ := entry["sender"].(map[string]any)
		if sender["kind"] == "self" {
			// The companion's own line. Not "You": the reader is its owner.
			from = "companion (self)"
		} else {
			from, _ = sender["display_name"].(string)
		}
		text = str("text")
	case "memory", "conversation_memory":
		label = kind
		from = fmt.Sprintf("[memory %s %q]", str("type"), str("title"))
		if day := str("memory_date"); day != "" {
			from += " " + day
		}
		text = str("text")
	case "tool_call":
		label = "tool_call"
		outcome := "…"
		switch entry["outcome"] {
		case "success":
			outcome = "ok"
		case "error":
			outcome = "failed"
		}
		from = str("tool_name") + " " + outcome
		text = str("output_excerpt")
	default:
		return row, false
	}
	row.cells = []string{clockTime(entry["at"]), label, room, from, cutRunes(oneLine(text), timelineTextWidth)}
	return row, true
}

// oneLine folds line breaks and tabs into single spaces: a table row is one line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// cutRunes shortens s to at most maxRunes runes, ending in "…" when cut; it
// never splits a UTF-8 sequence (unlike truncate, which cuts bytes).
func cutRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes-1]) + "…"
}

func init() {
	f := companionsTimelineCmd.Flags()
	f.StringVar(&tlInclude, "include", "", "comma-separated sources: messages, memories, conversation_memories, tool_calls (default: messages)")
	f.StringVar(&tlRoom, "room", "", "only this room, as room:<id>")
	f.StringVar(&tlPerson, "person", "", "only messages whose sender name contains this ('you' = the companion)")
	f.StringVar(&tlSince, "since", "", "oldest moment: YYYY-MM-DD (UTC day start) or RFC 3339")
	f.StringVar(&tlUntil, "until", "", "newest moment: YYYY-MM-DD (UTC day end) or RFC 3339")
	f.StringVar(&tlContains, "contains", "", "only entries whose text contains this (case-insensitive)")
	f.StringVar(&tlGroup, "group", "time", "arrange the page by time, room or day (client-side)")
	f.StringVar(&tlCursor, "cursor", "", "next_cursor of a previous page: read further back")
	f.IntVar(&tlLimit, "limit", 0, "entries per page, 1-200 (default: server's 50)")
}
