-- Habilita extensão para geração de UUIDs
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 1. Tabela de Carteiras
CREATE TABLE IF NOT EXISTS wallets (
    id UUID PRIMARY KEY,
    player_id VARCHAR(100) NOT NULL,
    currency VARCHAR(10) NOT NULL,
    balance BIGINT NOT NULL DEFAULT 0,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    CONSTRAINT uq_player_currency UNIQUE (player_id, currency),
    CONSTRAINT chk_balance_positive CHECK (balance >= 0)
);

-- 2. Tabela de Transações (Idempotência e Histórico de Apostas/Ganhos)
CREATE TABLE IF NOT EXISTS wager_transactions (
    id UUID PRIMARY KEY,
    provider_id VARCHAR(50) NOT NULL,
    external_id VARCHAR(100) NOT NULL,
    idempotency_key VARCHAR(150) NOT NULL UNIQUE,
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    player_id VARCHAR(100) NOT NULL,
    kind VARCHAR(20) NOT NULL,
    amount BIGINT NOT NULL,
    currency VARCHAR(10) NOT NULL,
    status VARCHAR(20) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    CONSTRAINT uq_provider_external_tx UNIQUE (provider_id, external_id)
);

-- 3. Tabela do Ledger Financeiro (Extrato Imutável / Auditoria)
CREATE TABLE IF NOT EXISTS wallet_ledger_entries (
    id UUID PRIMARY KEY,
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    transaction_id UUID NOT NULL REFERENCES wager_transactions(id),
    entry_type VARCHAR(10) NOT NULL,
    amount BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 4. Tabela Outbox (Transactional Outbox Pattern para Mensajaria Distribuída)
CREATE TABLE IF NOT EXISTS outbox (
    id UUID PRIMARY KEY,
    aggregate_type VARCHAR(50) NOT NULL,
    aggregate_id VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    processed_at TIMESTAMP WITH TIME ZONE
);