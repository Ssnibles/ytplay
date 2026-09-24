package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type thumbInfo struct {
	URL string `json:"url"`
}

type video struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	URL         string      `json:"url"`
	Channel     string      `json:"channel"`
	Uploader    string      `json:"uploader"`
	Duration    *float64    `json:"duration"`
	ChannelID   string      `json:"channel_id"`
	ChannelURL  string      `json:"channel_url"`
	IEKey       string      `json:"ie_key"`
	Thumbnails  []thumbInfo `json:"thumbnails"`
	Followers   *int64      `json:"channel_follower_count"`
	Description string      `json:"description"`
}

func (v video) thumbURL() string {
	if v.isChannel() {
		if len(v.Thumbnails) > 0 {
			best := v.Thumbnails[len(v.Thumbnails)-1].URL
			if strings.HasPrefix(best, "//") {
				return "https:" + best
			}
			return best
		}
		return ""
	}
	return fmt.Sprintf("https://i.ytimg.com/vi/%s/mqdefault.jpg", v.ID)
}

func (v video) isChannel() bool {
	if strings.EqualFold(v.IEKey, "YoutubeTab") || strings.EqualFold(v.IEKey, "YoutubeChannel") {
		return true
	}
	if v.Duration == nil && (strings.Contains(v.URL, "/channel/") || strings.Contains(v.URL, "/@") || strings.Contains(v.URL, "/c/") || strings.Contains(v.URL, "/user/")) {
		return true
	}
	if strings.HasPrefix(v.ID, "UC") && len(v.ID) == 24 && v.Duration == nil {
		return true
	}
	return false
}

func (v video) channelTargetURL() string {
	if v.ChannelURL != "" {
		return v.ChannelURL
	}
	if v.ChannelID != "" {
		return "https://www.youtube.com/channel/" + v.ChannelID
	}
	if strings.Contains(v.URL, "/channel/") || strings.Contains(v.URL, "/@") || strings.Contains(v.URL, "/c/") || strings.Contains(v.URL, "/user/") {
		return v.URL
	}
	if v.URL != "" && strings.HasPrefix(v.URL, "http") {
		return v.URL
	}
	if strings.HasPrefix(v.ID, "UC") {
		return "https://www.youtube.com/channel/" + v.ID
	}
	return ""
}

func (v video) watchURL() string {
	if v.isChannel() {
		return v.channelTargetURL()
	}
	if strings.HasPrefix(v.URL, "http") {
		return v.URL
	}
	return "https://www.youtube.com/watch?v=" + v.ID
}

func (v video) channel() string {
	if v.Channel != "" {
		return v.Channel
	}
	if v.Uploader != "" {
		return v.Uploader
	}
	if v.isChannel() {
		return v.Title
	}
	return ""
}

func (v video) duration() string {
	if v.isChannel() {
		return "Channel"
	}
	if v.Duration == nil || *v.Duration <= 0 {
		return "?:??"
	}
	d := int(math.Round(*v.Duration))
	h := d / 3600
	m := (d % 3600) / 60
	s := d % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// videoDetail carries channel/video stats fetched on demand for the selected
// video — flat search results don't include any of these. Fields are pointers
// because YouTube omits them for some videos/channels (hidden subscriber
// counts, unlisted views, …).
type videoDetail struct {
	subs        *int64 // channel subscriber count
	chViews     *int64 // total views across all the channel's videos
	views       *int64 // views on this video
	likes       *int64 // likes on this video
	uploaded    string // upload date, YYYYMMDD
	channelURL  string // canonical channel URL
	channelID   string // channel id (fallback when URL is missing)
	description string // channel description
}

func (d videoDetail) date() string {
	t, err := time.Parse("20060102", d.uploaded)
	if err != nil {
		return ""
	}
	return t.Format("Jan 2, 2006")
}

// channelLink resolves the canonical channel URL for a video, preferring the
// full URL the extractor reported and falling back to the channel id.
func (d videoDetail) channelLink() string {
	if d.channelURL != "" {
		return d.channelURL
	}
	if d.channelID != "" {
		return "https://www.youtube.com/channel/" + d.channelID
	}
	return ""
}

// wrapText wraps s into lines of at most width runes, breaking on spaces.
// Consecutive blank lines are collapsed into a single blank line.
func wrapText(s string, width int) []string {
	if width <= 0 {
		return nil
	}
	var out []string
	rawLines := strings.Split(s, "\n")
	lastWasBlank := false
	for _, raw := range rawLines {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			if len(out) > 0 && !lastWasBlank {
				out = append(out, "")
				lastWasBlank = true
			}
			continue
		}
		lastWasBlank = false
		words := strings.Fields(raw)
		if len(words) == 0 {
			continue
		}
		var cur strings.Builder
		curLen := 0
		for _, w := range words {
			wLen := len([]rune(w))
			if curLen == 0 {
				for wLen > width {
					out = append(out, string([]rune(w)[:width]))
					w = string([]rune(w)[width:])
					wLen = len([]rune(w))
				}
				cur.WriteString(w)
				curLen = wLen
			} else if curLen+1+wLen <= width {
				cur.WriteString(" ")
				cur.WriteString(w)
				curLen += 1 + wLen
			} else {
				out = append(out, cur.String())
				cur.Reset()
				for wLen > width {
					out = append(out, string([]rune(w)[:width]))
					w = string([]rune(w)[width:])
					wLen = len([]rune(w))
				}
				cur.WriteString(w)
				curLen = wLen
			}
		}
		if curLen > 0 {
			out = append(out, cur.String())
		}
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// lines renders the stats as up to maxLines rows, combining views and post
// date on one line and wrapping channel descriptions over lines. Unknown fields are skipped.
func (d videoDetail) lines(width int, maxLines ...int) []string {
	limit := detailLines
	if len(maxLines) > 0 {
		limit = maxLines[0]
	}
	if limit <= 0 {
		return nil
	}
	var out []string
	if d.subs != nil {
		out = append(out, hint(formatCount(*d.subs)+" subscribers", width))
	}
	if d.chViews != nil {
		out = append(out, hint(formatCount(*d.chViews)+" total channel views", width))
	}
	var detail []string
	if d.views != nil {
		detail = append(detail, formatCount(*d.views)+" views")
	}
	if d.likes != nil {
		detail = append(detail, formatCount(*d.likes)+" likes")
	}
	if d.date() != "" {
		detail = append(detail, "posted "+d.date())
	}
	if len(detail) > 0 {
		out = append(out, hint(strings.Join(detail, " · "), width))
	}
	if d.description != "" {
		for _, ln := range wrapText(d.description, width) {
			if len(out) >= limit {
				break
			}
			if ln == "" {
				out = append(out, "")
			} else {
				out = append(out, hint(ln, width))
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// formatCount renders a view/subscriber count compactly: 999, 1.2k, 45k, 1.5M.
func formatCount(n int64) string {
	var div int64
	var suffix string
	switch {
	case n >= 1_000_000_000:
		div, suffix = 1_000_000_000, "B"
	case n >= 1_000_000:
		div, suffix = 1_000_000, "M"
	case n >= 1_000:
		div, suffix = 1_000, "k"
	default:
		return strconv.FormatInt(n, 10)
	}
	s := fmt.Sprintf("%.1f", float64(n)/float64(div))
	return strings.TrimSuffix(s, ".0") + suffix
}
