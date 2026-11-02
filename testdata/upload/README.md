# Synthetic EXIF fixtures

These small JPEGs were generated for tests; they are not user photos. The GPS
values are a chosen Bangkok test location (13.75, 100.5), and capture time is
2026-06-15 14:30:45 with offset +07:00 where present.

- `complete.jpg`: GPS, altitude 14 m, original capture time, and offset.
- `numeric-key.jpg`: DateTimeOriginal 36867 and offset 36881 stored directly in
  IFD0, reproducing the untranslated numeric-key case in the legacy uploader.
- `missing-gps.jpg`: capture time and offset, without GPS.
- `missing-time.jpg`: GPS and ModifyDate, without capture time.
- `missing-offset.jpg`: GPS and capture time, without a timezone offset.
- `below-sea-level.jpg`: altitude reference 1 (14 m below sea level).
- `no-exif.jpg`: a valid image without EXIF.
- `malformed-exif.jpg`: a valid image with a deliberately broken TIFF pointer
  in its EXIF APP1 segment. Image decoding and hashing can still succeed.

The Node tests parse the actual JPEG bytes with the bundled exifr library.
