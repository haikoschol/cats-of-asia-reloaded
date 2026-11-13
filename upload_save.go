package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	errDuplicateUpload     = errors.New("This image ID or file hash is already saved with different metadata")
	errUploadPlaceConflict = errors.New("These coordinates and altitude already belong to a different city or country")
)

type photoUploadMetadata struct {
	ID          string          `json:"id"`
	Filename    string          `json:"filename"`
	SHA256      string          `json:"sha256"`
	Latitude    *float64        `json:"latitude"`
	Longitude   *float64        `json:"longitude"`
	Altitude    *float64        `json:"altitude"`
	City        string          `json:"city"`
	Country     string          `json:"country"`
	CaptureTime string          `json:"captureTime"`
	TimeSource  string          `json:"timeSource"`
	UTCOffset   string          `json:"utcOffset"`
	Timestamp   string          `json:"timestamp"`
	Raw         json.RawMessage `json:"raw"`
}

type validatedPhotoUpload struct {
	photoUploadMetadata
	takenAt       time.Time
	offsetMinutes int
}

type savedPhotoUpload struct {
	ID           string `json:"id"`
	Success      bool   `json:"success"`
	AlreadySaved bool   `json:"alreadySaved"`
}

func validUploadText(value string, maxLength int) bool {
	return value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= maxLength &&
		!strings.ContainsFunc(value, unicode.IsControl)
}

func validatePhotoUpload(input photoUploadMetadata) (validatedPhotoUpload, error) {
	input.ID = strings.ToLower(input.ID)
	if !cloudflareImagePattern.MatchString(input.ID) {
		return validatedPhotoUpload{}, errors.New("id must be a Cloudflare image UUID")
	}
	if !validUploadText(input.Filename, 255) || strings.ContainsAny(input.Filename, `/\`) ||
		!(strings.HasSuffix(strings.ToLower(input.Filename), ".jpg") || strings.HasSuffix(strings.ToLower(input.Filename), ".jpeg")) {
		return validatedPhotoUpload{}, errors.New("filename must be a JPEG filename of at most 255 characters without a path")
	}
	hash, err := normalizePhotoHash(input.SHA256)
	if err != nil {
		return validatedPhotoUpload{}, err
	}
	input.SHA256 = hash
	for _, coordinate := range []struct {
		value *float64
		limit float64
	}{{input.Latitude, 90}, {input.Longitude, 180}} {
		if coordinate.value == nil || math.IsNaN(*coordinate.value) || math.IsInf(*coordinate.value, 0) || math.Abs(*coordinate.value) > coordinate.limit {
			return validatedPhotoUpload{}, errors.New("latitude and longitude must be valid, explicit coordinates")
		}
	}
	if input.Altitude != nil && (math.IsNaN(*input.Altitude) || math.IsInf(*input.Altitude, 0)) {
		return validatedPhotoUpload{}, errors.New("altitude must be a finite number or null")
	}
	input.City, input.Country = strings.TrimSpace(input.City), strings.TrimSpace(input.Country)
	if !validUploadText(input.City, 255) || !validUploadText(input.Country, 255) {
		return validatedPhotoUpload{}, errors.New("city and country are required and must be at most 255 characters without control characters")
	}
	if input.TimeSource != "DateTimeOriginal" {
		return validatedPhotoUpload{}, errors.New("An original capture time is required; digitization and modification times are not accepted")
	}
	const layout = "2006-01-02 15:04:05"
	wallClock, err := time.Parse(layout, input.CaptureTime)
	if err != nil || wallClock.Year() < 1 || wallClock.Format(layout) != input.CaptureTime {
		return validatedPhotoUpload{}, errors.New("captureTime must be a valid original date and time")
	}
	// Use only the explicit photo offset, never the server/browser zone or the
	// legacy place offset. Keep the same accepted offset range as the EXIF UI.
	offset := input.UTCOffset
	if len(offset) != 6 || (offset[0] != '+' && offset[0] != '-') || offset[3] != ':' ||
		offset[1] < '0' || offset[1] > '9' || offset[2] < '0' || offset[2] > '9' ||
		offset[4] < '0' || offset[4] > '9' || offset[5] < '0' || offset[5] > '9' {
		return validatedPhotoUpload{}, errors.New("utcOffset must be an explicit EXIF offset such as +07:00")
	}
	hours, _ := strconv.Atoi(offset[1:3])
	minutes, _ := strconv.Atoi(offset[4:6])
	if hours > 14 || minutes > 59 || (hours == 14 && minutes != 0) {
		return validatedPhotoUpload{}, errors.New("utcOffset is out of range")
	}
	offsetMinutes := hours*60 + minutes
	if offset[0] == '-' {
		offsetMinutes = -offsetMinutes
	}
	takenAt := wallClock.Add(-time.Duration(offsetMinutes) * time.Minute)
	submittedTime, err := time.Parse(time.RFC3339Nano, input.Timestamp)
	if err != nil || !submittedTime.Equal(takenAt) || takenAt.Year() < 1 || takenAt.Year() > 9999 {
		return validatedPhotoUpload{}, errors.New("timestamp must agree with captureTime and utcOffset")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(input.Raw, &raw); err != nil || raw == nil {
		return validatedPhotoUpload{}, errors.New("raw must contain an EXIF JSON object")
	}
	// Preserve the original fields as per-photo evidence. A normalized UTC
	// timestamp alone no longer says which wall-clock time/offset the file had.
	rawString := func(keys ...string) string {
		for _, key := range keys {
			if value, ok := raw[key]; ok && string(value) != "null" {
				var text string
				if json.Unmarshal(value, &text) == nil {
					return strings.TrimSpace(text)
				}
				return ""
			}
		}
		return ""
	}
	if rawString("DateTimeOriginal", "36867") != wallClock.Format("2006:01:02 15:04:05") ||
		rawString("OffsetTimeOriginal", "36881") != input.UTCOffset {
		return validatedPhotoUpload{}, errors.New("Original EXIF capture time and UTC offset must match the submitted capture details")
	}
	return validatedPhotoUpload{photoUploadMetadata: input, takenAt: takenAt, offsetMinutes: offsetMinutes}, nil
}

func uploadJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Print("cannot write upload response")
	}
}

func (app *application) saveUploadedPhoto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		uploadJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method Not Allowed"})
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		uploadJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Content-Type must be application/json"})
		return
	}
	const maxMetadataBytes = 256 << 10
	r.Body = http.MaxBytesReader(w, r.Body, maxMetadataBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input photoUploadMetadata
	err = decoder.Decode(&input)
	if err == nil {
		err = decoder.Decode(new(any))
		if errors.Is(err, io.EOF) {
			err = nil
		} else if err == nil {
			err = errors.New("extra JSON value")
		}
	}
	if err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		uploadJSON(w, status, map[string]string{"error": "Invalid or oversized metadata JSON"})
		return
	}
	metadata, err := validatePhotoUpload(input)
	if err != nil {
		uploadJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err := app.verifyImageUpload(r.Context(), metadata.ID); err != nil {
		status := http.StatusBadGateway
		switch {
		case errors.Is(err, errImageUploadOwnership):
			status = http.StatusForbidden
		case errors.Is(err, errImageUploadIncomplete):
			status = http.StatusConflict
		case errors.Is(err, errImageUploadSession):
			status = http.StatusUnauthorized
		case errors.Is(err, errImageUploadConfiguration):
			status = http.StatusServiceUnavailable
		case errors.Is(err, errImageUploadTimeout):
			status = http.StatusGatewayTimeout
		}
		failure := map[string]string{"error": err.Error()}
		if errors.Is(err, errImageUploadIncomplete) {
			failure["code"] = "image_incomplete"
		}
		uploadJSON(w, status, failure)
		return
	}
	result, err := app.storeUploadedPhoto(r.Context(), metadata)
	if err != nil {
		if errors.Is(err, errDuplicateUpload) || errors.Is(err, errUploadPlaceConflict) {
			uploadJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		} else {
			// SQL errors may include submitted metadata or private connection
			// details. Keep both the response and log free of those values.
			log.Print("photo metadata transaction failed")
			uploadJSON(w, http.StatusInternalServerError, map[string]string{"error": "Photo metadata could not be saved; retry with the same image ID"})
		}
		return
	}
	status := http.StatusCreated
	if result.AlreadySaved {
		status = http.StatusOK
	}
	uploadJSON(w, status, result)
}

// existingUploadedPhoto distinguishes a safe replay from either kind of
// conflict. It is also used after an insert loses a concurrent uniqueness race.
func existingUploadedPhoto(ctx context.Context, tx *sql.Tx, metadata validatedPhotoUpload) (bool, error) {
	var id, hash string
	var sameMetadata bool
	err := tx.QueryRowContext(ctx, `SELECT p.id::text, p.sha256,
		COALESCE(p.filename=$3 AND p.timestamp=$4
		AND c.point=ST_SetSRID(ST_MakePoint($5,$6),4326)::geography
		AND c.altitude IS NOT DISTINCT FROM $7
		AND l.city=$8 AND l.country=$9
		AND EXISTS (SELECT 1 FROM photos_rawmetadata r WHERE r.photo_id=p.sha256 AND r.metadata=$10::jsonb),false)
		FROM photos_photo p
		LEFT JOIN photos_coordinates c ON c.id=p.coordinates_id
		LEFT JOIN photos_location l ON l.id=c.location_id
		WHERE p.id=$1 OR p.sha256=$2 LIMIT 1`, metadata.ID, metadata.SHA256, metadata.Filename, metadata.takenAt,
		*metadata.Longitude, *metadata.Latitude, metadata.Altitude, metadata.City, metadata.Country, string(metadata.Raw)).Scan(&id, &hash, &sameMetadata)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if id != metadata.ID || hash != metadata.SHA256 || !sameMetadata {
		return false, errDuplicateUpload
	}
	return true, nil
}

func (app *application) storeUploadedPhoto(ctx context.Context, metadata validatedPhotoUpload) (savedPhotoUpload, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	tx, err := app.contentDB.BeginTx(ctx, nil)
	if err != nil {
		return savedPhotoUpload{}, err
	}
	defer tx.Rollback()
	result := savedPhotoUpload{ID: metadata.ID, Success: true}
	if exists, err := existingUploadedPhoto(ctx, tx, metadata); err != nil {
		return savedPhotoUpload{}, err
	} else if exists {
		result.AlreadySaved = true
		return result, nil
	}

	var locationID int64
	// This legacy place column uses JavaScript getTimezoneOffset's sign.
	// Retain its initial value when reusing a place; individual photo times
	// always use their own explicit EXIF offset, preserved in rawmetadata.
	err = tx.QueryRowContext(ctx, `INSERT INTO photos_location (city,country,tzoffset)
		VALUES ($1,$2,$3) ON CONFLICT (city,country) DO NOTHING RETURNING id`, metadata.City, metadata.Country, -metadata.offsetMinutes).Scan(&locationID)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, "SELECT id FROM photos_location WHERE city=$1 AND country=$2", metadata.City, metadata.Country).Scan(&locationID)
	}
	if err != nil {
		return savedPhotoUpload{}, err
	}

	var coordinatesID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO photos_coordinates (point,altitude,location_id)
		VALUES (ST_SetSRID(ST_MakePoint($1,$2),4326)::geography,$3,$4)
		ON CONFLICT DO NOTHING RETURNING id`, *metadata.Longitude, *metadata.Latitude, metadata.Altitude, locationID).Scan(&coordinatesID)
	if errors.Is(err, sql.ErrNoRows) {
		var existingLocationID int64
		err = tx.QueryRowContext(ctx, `SELECT id,location_id FROM photos_coordinates
			WHERE point=ST_SetSRID(ST_MakePoint($1,$2),4326)::geography
			AND altitude IS NOT DISTINCT FROM $3`, *metadata.Longitude, *metadata.Latitude, metadata.Altitude).Scan(&coordinatesID, &existingLocationID)
		if err == nil && existingLocationID != locationID {
			return savedPhotoUpload{}, errUploadPlaceConflict
		}
	}
	if err != nil {
		return savedPhotoUpload{}, err
	}
	var id string
	err = tx.QueryRowContext(ctx, `INSERT INTO photos_photo (id,filename,sha256,timestamp,coordinates_id)
		VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING RETURNING id::text`, metadata.ID, metadata.Filename, metadata.SHA256, metadata.takenAt, coordinatesID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		exists, err := existingUploadedPhoto(ctx, tx, metadata)
		if err != nil {
			return savedPhotoUpload{}, err
		}
		if !exists {
			return savedPhotoUpload{}, errDuplicateUpload
		}
		// Roll back any place/coordinate created by this losing request.
		result.AlreadySaved = true
		return result, nil
	}
	if err != nil {
		return savedPhotoUpload{}, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO photos_rawmetadata (metadata,photo_id) VALUES ($1::jsonb,$2)", string(metadata.Raw), metadata.SHA256); err != nil {
		return savedPhotoUpload{}, err
	}
	if err := tx.Commit(); err != nil {
		return savedPhotoUpload{}, err
	}
	return result, nil
}
