CREATE TABLE IF NOT EXISTS wallets (
    id UUID PRIMARY KEY,
    player_id VARCHAR(100) NOT NULL,
    currency VARCHAR(10) NOT NULL,
    balance BIGINT NOT NULL DEFAULT 0,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT unique_player_currency UNIQUE (player_id, currency)
);

CREATE TABLE IF NOT EXISTS wager_transactions (
    id UUID PRIMARY KEY,
    provider_id VARCHAR(50) NOT NULL,
    external_id VARCHAR(100) NOT NULL,
    idempotency_key VARCHAR(100) NOT NULL UNIQUE,
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    player_id VARCHAR(100) NOT NULL,
    kind VARCHAR(20) NOT NULL,
    amount BIGINT NOT NULL,
    currency VARCHAR(10) NOT NULL,
    status VARCHAR(20) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS wallet_ledger_entries (
    id UUID PRIMARY KEY,
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    transaction_id UUID NOT NULL REFERENCES wager_transactions(id),
    entry_type VARCHAR(10) NOT NULL,
    amount BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS outbox (
    id UUID PRIMARY KEY,
    aggregate_type VARCHAR(50),
    aggregate_id VARCHAR(100),
    payload JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    processed_at TIMESTAMP
);