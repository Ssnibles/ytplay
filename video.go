package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type video struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	URL        string   `json:"url"`
	Channel    string   `json:"channel"`
	Uploader   string   `json:"uploader"`
	Duration   *float64 `json:"duration"`
	ChannelID  string   `json:"channel_id"`
	ChannelURL string   `json:"channel_url"`
	IEKey      string   `json:"ie_key"`
}

func (v video) isChannel() bool {
	return v.IEKey == "YoutubeTab" || (strings.HasPrefix(v.ID, "UC") && v.Duration == nil && strings.Contains(v.URL, "/channel/"))
}

func (v video) channelTargetURL() string {
	if v.ChannelURL != "" {
		return v.ChannelURL
	}
	if v.ChannelID != "" {
		return "https://www.youtube.com/channel/" + v.ChannelID
	}
	if strings.Contains(v.URL, "/channel/") || strings.Contains(v.URL, "/@") {
		return v.URL
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
	subs       *int64 // channel subscriber count
	chViews    *int64 // total views across all the channel's videos
	views      *int64 // views on this video
	likes      *int64 // likes on this video
	uploaded   string // upload date, YYYYMMDD
	channelURL string // canonical channel URL
	channelID  string // channel id (fallback when URL is missing)
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

// lines renders the stats as up to detailLines rows, combining views and post
// date on one line. Unknown fields are skipped.
func (d videoDetail) lines(width int) []string {
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
	return out[:min(len(out), detailLines)]
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
