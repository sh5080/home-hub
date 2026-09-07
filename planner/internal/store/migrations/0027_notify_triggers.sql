-- 알림 규칙을 "대상 + 조건"으로 바꾼다. kind 는 대상, params.cond 가 조건.
UPDATE notify_rules SET kind = 'care',
  params = json_set(json_remove(params, '$.hours'), '$.cond', 'since', '$.amount', json_extract(params, '$.hours'), '$.unit', 'hours', '$.watch', json('true'))
  WHERE kind = 'care_gap';
UPDATE notify_rules SET params = json_set(params, '$.cond', 'pending') WHERE kind IN ('tasks', 'routines');
UPDATE notify_rules SET params = json_set(params, '$.cond', 'none_today') WHERE kind = 'diary';
UPDATE notify_rules SET params = json_set(params, '$.cond', 'always') WHERE kind = 'custom';
UPDATE notify_rules SET kind = 'finance',
  params = json_set(params, '$.cond', 'since', '$.amount', coalesce((SELECT CAST(value AS INTEGER) FROM kv WHERE key = 'fin.upload_days'), 30), '$.unit', 'days')
  WHERE kind = 'finance_stale';
