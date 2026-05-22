CREATE TYPE workflow_status AS ENUM (
    'pending',
    'running',
    'completed',
    'failed',
    'compensating',
    'compensated'
);

CREATE TYPE step_status AS ENUM (
    'pending',
    'running',
    'completed',
    'failed',
    'compensating',
    'compensated',
    'skipped'
);

CREATE TABLE IF NOT EXISTS workflows (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(255) NOT NULL,
    status          workflow_status NOT NULL DEFAULT 'pending',
    input           JSONB NOT NULL DEFAULT '{}',
    current_step    INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT,
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS workflow_steps (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id     UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    step_index      INTEGER NOT NULL,
    name            VARCHAR(255) NOT NULL,
    task_type       VARCHAR(255) NOT NULL,
    status          step_status NOT NULL DEFAULT 'pending',
    input           JSONB NOT NULL DEFAULT '{}',
    output          JSONB,
    last_error      TEXT,
    compensation_task_type VARCHAR(255),
    started_at      TIMESTAMP WITH TIME ZONE,
    completed_at    TIMESTAMP WITH TIME ZONE,
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workflow_steps_workflow_id ON workflow_steps(workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflows_status ON workflows(status);