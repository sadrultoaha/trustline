-- +goose Up
-- "na": the rule requires resources and there were none to evaluate (e.g. no
-- people synced yet). Not applicable tests do not decide a control's status.
ALTER TABLE tests DROP CONSTRAINT tests_last_status_check;
ALTER TABLE tests ADD CONSTRAINT tests_last_status_check CHECK (last_status IN ('pending','pass','fail','error','na'));
ALTER TABLE test_runs DROP CONSTRAINT test_runs_status_check;
ALTER TABLE test_runs ADD CONSTRAINT test_runs_status_check CHECK (status IN ('pass','fail','error','na'));

-- +goose Down
UPDATE test_runs SET status = 'pass' WHERE status = 'na';
UPDATE tests SET last_status = 'pending' WHERE last_status = 'na';
ALTER TABLE test_runs DROP CONSTRAINT test_runs_status_check;
ALTER TABLE test_runs ADD CONSTRAINT test_runs_status_check CHECK (status IN ('pass','fail','error'));
ALTER TABLE tests DROP CONSTRAINT tests_last_status_check;
ALTER TABLE tests ADD CONSTRAINT tests_last_status_check CHECK (last_status IN ('pending','pass','fail','error'));
