-- Keep a bounded comparison alongside the exact latest observation. Existing
-- readings have no comparison; never invent a baseline during migration.
BEGIN;
ALTER TABLE exchange_read_snapshots ADD COLUMN comparison jsonb;
ALTER TABLE exchange_read_snapshots ADD CONSTRAINT comparison_is_read_only CHECK (
  comparison IS NULL OR (
    jsonb_typeof(comparison) = 'object'
    AND octet_length(comparison::text) <= 2097152
    AND comparison @> '{"schemaVersion":1,"mode":"rest_observation_comparison","continuity":"not_established","executionReady":false,"detailLimit":200}'::jsonb
    AND comparison ?& ARRAY['status','previousCapturedAt','currentStartedAt','currentCapturedAt','intervalRelation','totalChanges','truncated','changes']
    AND jsonb_typeof(comparison->'currentStartedAt') = 'string'
    AND jsonb_typeof(comparison->'currentCapturedAt') = 'string'
    AND jsonb_typeof(comparison->'changes') = 'array'
    AND jsonb_array_length(comparison->'changes') <= 200
    AND (comparison->>'currentStartedAt')::timestamptz = started_at
    AND (comparison->>'currentCapturedAt')::timestamptz = captured_at
  )
);
COMMIT;
