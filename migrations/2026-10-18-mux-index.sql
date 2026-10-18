-- Mux migration, phase 4 (hillviewtv). Rows that shared one Cloudflare Stream
-- video (e.g. videos 62/63) share one Mux asset, so mux_asset_id is indexed
-- for webhook lookups but not unique.
ALTER TABLE videos
  DROP INDEX videos_mux_asset_id,
  ADD INDEX videos_mux_asset_id (mux_asset_id);
