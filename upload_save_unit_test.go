//go:build !integration

package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestValidatePhotoUpload(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*photoUploadMetadata)
	}{
		{"image ID", func(m *photoUploadMetadata) { m.ID = "../other-image" }},
		{"filename path", func(m *photoUploadMetadata) { m.Filename = "../cat.jpg" }},
		{"filename type", func(m *photoUploadMetadata) { m.Filename = "cat.heic" }},
		{"filename length", func(m *photoUploadMetadata) { m.Filename = strings.Repeat("a", 252) + ".jpg" }},
		{"filename control", func(m *photoUploadMetadata) { m.Filename = "cat\x00.jpg" }},
		{"hash", func(m *photoUploadMetadata) { m.SHA256 = strings.Repeat("x", 64) }},
		{"missing latitude", func(m *photoUploadMetadata) { m.Latitude = nil }},
		{"missing longitude", func(m *photoUploadMetadata) { m.Longitude = nil }},
		{"latitude range", func(m *photoUploadMetadata) { v := 90.01; m.Latitude = &v }},
		{"longitude range", func(m *photoUploadMetadata) { v := -180.01; m.Longitude = &v }},
		{"coordinate NaN", func(m *photoUploadMetadata) { v := math.NaN(); m.Latitude = &v }},
		{"altitude infinity", func(m *photoUploadMetadata) { v := math.Inf(1); m.Altitude = &v }},
		{"missing city", func(m *photoUploadMetadata) { m.City = " \t " }},
		{"country control", func(m *photoUploadMetadata) { m.Country = "Thai\nland" }},
		{"digitization time", func(m *photoUploadMetadata) { m.TimeSource = "CreateDate" }},
		{"modification time", func(m *photoUploadMetadata) { m.TimeSource = "ModifyDate" }},
		{"missing time", func(m *photoUploadMetadata) { m.CaptureTime = "" }},
		{"calendar rollover", func(m *photoUploadMetadata) { m.CaptureTime = "2026-02-30 14:30:45" }},
		{"missing offset", func(m *photoUploadMetadata) { m.UTCOffset = "" }},
		{"offset format", func(m *photoUploadMetadata) { m.UTCOffset = "Europe/Berlin" }},
		{"offset range", func(m *photoUploadMetadata) { m.UTCOffset = "+14:01" }},
		{"offset minutes", func(m *photoUploadMetadata) { m.UTCOffset = "+07:60" }},
		{"missing timestamp", func(m *photoUploadMetadata) { m.Timestamp = "" }},
		{"timestamp without zone", func(m *photoUploadMetadata) { m.Timestamp = "2026-06-15T14:30:45" }},
		{"double offset conversion", func(m *photoUploadMetadata) { m.Timestamp = "2026-06-15T00:30:45Z" }},
		{"raw null", func(m *photoUploadMetadata) { m.Raw = json.RawMessage(`null`) }},
		{"raw array", func(m *photoUploadMetadata) { m.Raw = json.RawMessage(`[]`) }},
		{"raw malformed", func(m *photoUploadMetadata) { m.Raw = json.RawMessage(`{`) }},
		{"raw missing capture", func(m *photoUploadMetadata) { m.Raw = json.RawMessage(`{"OffsetTimeOriginal":"+07:00"}`) }},
		{"raw wrong capture", func(m *photoUploadMetadata) {
			m.Raw = json.RawMessage(`{"DateTimeOriginal":"2026:06:15 14:31:45","OffsetTimeOriginal":"+07:00"}`)
		}},
		{"raw wrong offset", func(m *photoUploadMetadata) {
			m.Raw = json.RawMessage(`{"DateTimeOriginal":"2026:06:15 14:30:45","OffsetTimeOriginal":"+08:00"}`)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := uploadSaveMetadata()
			tt.change(&m)
			if _, err := validatePhotoUpload(m); err == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
	for _, tt := range []struct {
		name, capture, offset, timestamp string
		numeric                          bool
	}{
		{"east", "2026-06-15 14:30:45", "+07:00", "2026-06-15T07:30:45Z", false},
		{"west half-hour", "2026-01-01 00:15:00", "-03:30", "2026-01-01T03:45:00Z", false},
		{"quarter-hour leap day", "2024-03-01 00:15:00", "+05:45", "2024-02-29T18:30:00Z", false},
		{"numeric EXIF keys", "2026-06-15 14:30:45", "+07:00", "2026-06-15T07:30:45Z", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := uploadSaveMetadata()
			m.CaptureTime, m.UTCOffset, m.Timestamp = tt.capture, tt.offset, tt.timestamp
			wall, err := time.Parse("2006-01-02 15:04:05", tt.capture)
			if err != nil {
				t.Fatal(err)
			}
			timeKey, offsetKey := "DateTimeOriginal", "OffsetTimeOriginal"
			if tt.numeric {
				timeKey, offsetKey = "36867", "36881"
			}
			m.Raw, _ = json.Marshal(map[string]string{timeKey: wall.Format("2006:01:02 15:04:05"), offsetKey: tt.offset})
			m.SHA256 = strings.ToUpper(m.SHA256)
			m.City = " Bangkok "
			m.Country = " Thailand "
			valid, err := validatePhotoUpload(m)
			if err != nil || valid.takenAt.Format(time.RFC3339) != tt.timestamp || valid.SHA256 != strings.Repeat("a", 64) || valid.City != "Bangkok" || valid.Country != "Thailand" {
				t.Fatalf("valid metadata rejected or altered: %v", err)
			}
		})
	}
}
