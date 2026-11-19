package main

import (
	"encoding/json"
	"strings"
)

func uploadSaveMetadata() photoUploadMetadata {
	latitude, longitude := 13.75, 100.5
	return photoUploadMetadata{
		ID: testUploadImageID, Filename: "cat.jpg", SHA256: strings.Repeat("a", 64),
		Latitude: &latitude, Longitude: &longitude, City: "Bangkok", Country: "Thailand",
		CaptureTime: "2026-06-15 14:30:45", TimeSource: "DateTimeOriginal", UTCOffset: "+07:00", Timestamp: "2026-06-15T07:30:45.000Z",
		Raw: json.RawMessage(`{"DateTimeOriginal":"2026:06:15 14:30:45","OffsetTimeOriginal":"+07:00","GPSLatitude":[13,45,0],"GPSLatitudeRef":"N","GPSLongitude":[100,30,0],"GPSLongitudeRef":"E"}`),
	}
}
