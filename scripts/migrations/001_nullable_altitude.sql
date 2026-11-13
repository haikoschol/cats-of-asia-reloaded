-- Apply to the Reloaded photo database with psql --single-transaction and
-- -v ON_ERROR_STOP=1. Existing heights and photo references are preserved.
ALTER TABLE public.photos_coordinates ALTER COLUMN altitude DROP NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS photos_coordinates_unknown_altitude_idx
    ON public.photos_coordinates (point) WHERE altitude IS NULL;
