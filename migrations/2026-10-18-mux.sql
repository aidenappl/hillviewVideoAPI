-- Mux migration, phase 1 (hillviewtv). Run by hand against the hillviewtv
-- database before deploying phase 2; phase 1 code does not read these columns.
--
--   mux_asset_id     Mux asset id, for API calls (tracks, renditions, deletes)
--   mux_playback_id  public playback id; url becomes stream.mux.com/{id}.m3u8
--   legacy_url       the Cloudflare Stream url a migrated row had, for rollback
ALTER TABLE videos
  ADD COLUMN mux_asset_id    VARCHAR(64)  NULL AFTER url,
  ADD COLUMN mux_playback_id VARCHAR(64)  NULL AFTER mux_asset_id,
  ADD COLUMN legacy_url      VARCHAR(512) NULL AFTER mux_playback_id,
  ADD UNIQUE KEY videos_mux_asset_id (mux_asset_id);
