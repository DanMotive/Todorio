-- 0015: narrow the reaction set to four (✅ ❌ ⚠️ ❓).
--
-- The old set had ten emoji. Rows with the removed ones would stay in the table forever and
-- simply never render, so they are cleaned up here. The exclamation mark is the only one with an
-- obvious replacement, so it becomes the warning sign; everything else is dropped.
--
-- The UPDATE skips rows where the same user already reacted with the warning sign on the same
-- target, because (target_type, target_id, user_id, emoji) is UNIQUE - those rows are removed by
-- the DELETE below instead.

UPDATE reactions r
   SET emoji = '⚠️'
 WHERE r.emoji = '❗'
   AND NOT EXISTS (
       SELECT 1 FROM reactions x
        WHERE x.target_type = r.target_type
          AND x.target_id   = r.target_id
          AND x.user_id     = r.user_id
          AND x.emoji       = '⚠️'
   );

DELETE FROM reactions
 WHERE emoji NOT IN ('✅', '❌', '⚠️', '❓');
