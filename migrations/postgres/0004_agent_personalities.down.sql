-- Rollback agent personality system

DROP FUNCTION IF EXISTS apply_rank_demotion(UUID);
DROP FUNCTION IF EXISTS apply_rank_promotion(UUID);
DROP FUNCTION IF EXISTS calculate_agent_specialization(UUID);

DROP TABLE IF EXISTS agent_rank_events;
DROP TABLE IF EXISTS agent_missions;
DROP TABLE IF EXISTS agent_personalities;
