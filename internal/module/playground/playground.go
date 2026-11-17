// Package playground keeps the full songs an admin generates in the prompt playground. Each one is
// stored at playground/<id>.<format>; the admin picks which ones the landing page shows as samples.
package playground

import "time"

// Song is one playground generation.
type Song struct {
	ID               string
	Title            string
	Caption          string
	Lyrics           string
	StylePrompt      string
	AudioKey         string
	Format           string
	DurationSec      int
	SizeBytes        int
	Provider         string
	Model            string
	ShowOnLanding    bool
	LandingOrder     int
	CreatedByAdminID string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// SaveInput is what the prompt playground hands over after a successful music run.
type SaveInput struct {
	Title       string
	Lyrics      string
	StylePrompt string
	Audio       []byte
	Format      string
	DurationSec int
	Provider    string
	Model       string
}
